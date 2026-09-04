// Package structwatcher 提供基于泛型的结构体变更监听功能。
// 通过嵌入 Watcher 指针到目标结构体中，
// 可以自动追踪结构体字段自创建或 Reset 以来的变更。
//
// 快照语义：New 与 Reset 会对引用类型字段（slice/map/pointer/interface）
// 做深拷贝，因此对这类字段的原地修改（如 s[0]=x、m[k]=v、*p=v）也能被检测到。
//
// 比较语义：
//   - 基础标量类型直接按值比较；两个 NaN 视为相等（避免永远误报变更）。
//   - time.Time 字段使用 Time.Equal 语义（同一时刻即视为相等，忽略单调时钟差异）。
//   - 指针字段按其指向的值比较（与 reflect.DeepEqual 一致）。
//   - func 字段仅追踪 nil 状态的变化（非 nil 的 func 无法比较是否相等）；
//     chan 字段同一 channel 视为相等。
//   - 未导出字段始终视为相等，不参与变更检测。
//
// 字段忽略：可通过 struct tag `watch:"-"` 将字段排除在变更检测之外，
// 适用于时间戳、版本号等无需追踪的噪音字段。被忽略的字段同样不参与
// 快照深拷贝，因此忽略大型引用字段还能降低 New/Reset 的开销。
// tag 取值仅支持 "-"，其他非空取值会在 New 时 panic（及早暴露拼写错误）。
//
// 并发安全：Watcher 本身不是并发安全的。
// 不要在多个 goroutine 中同时修改字段和调用 Changes/IsChanged/Reset。
// 如需并发访问，请使用 NewSync 创建 SyncWatcher（见 sync.go），
// 并保证所有字段写入都通过 SyncWatcher.Update 进行；或自行加锁保护。
//
// 已知限制：
//   - 不支持自引用的循环数据结构（深拷贝会导致栈溢出）。
//   - 未导出字段及其引用不会被追踪。
//   - 结构体按值拷贝后（p2 := *p），副本仍共享原 Watcher，其变更会计入原快照。
package structwatcher

import (
    "fmt"
    "math"
    "reflect"
    "strings"
    "sync"
    "time"
)

type (
    // Watchable 定义了可被监听的结构体必须实现的接口。
    // 目标结构体需要嵌入 *Watcher[T] 才能满足此接口。
    Watchable interface {
        // IsChanged 返回是否有字段发生变更
        IsChanged() bool
        // Changes 返回所有变更字段的详细列表
        Changes() []Change
    }

    // Watcher 是泛型变更监听器，T 必须是实现了 Watchable 接口的结构体类型。
    // 使用时将 *Watcher[T] 作为匿名字段嵌入到结构体 T 中。
    Watcher[T Watchable] struct {
        snapshot T            // 创建或 Reset 时的快照值（引用字段为深拷贝）
        target   *T           // 当前实际使用中的目标值指针
        meta     *watcherMeta // 按类型缓存的字段元数据
    }

    // Change 表示单个字段的变更信息
    Change struct {
        Field    string // 变更的字段名
        OldValue any    // 变更前的值
        NewValue any    // 变更后的值
    }
)

// watcherMeta 保存与具体 T 类型相关的元数据，按类型全局缓存，
// 避免每次 New 都通过反射重复构建。
type watcherMeta struct {
    watcherType reflect.Type // *Watcher[T] 类型，用于识别需跳过的嵌入字段
    skip        []bool       // 需要跳过比对与深拷贝的字段索引（嵌入的 Watcher 与 watch:"-" 字段）
    embedded    []int        // 需注入 Watcher 实例的嵌入字段索引
    numTracked  int          // 参与比对的字段数量（用于预分配）
}

const (
    watchTag       = "watch" // struct tag 键名
    watchTagIgnore = "-"     // 忽略字段，不参与变更检测与快照深拷贝
)

var (
    metaCache sync.Map // reflect.Type -> *watcherMeta
    refCache  sync.Map // reflect.Type -> bool，值中是否包含引用语义类型
    timeType  = reflect.TypeOf(time.Time{})
    // timeFastOK 确认 time.Time 的内部布局为 {wall uint64, ext int64, loc *Location}，
    // 布局不符时禁用表示比较快速路径，回退到装箱 Equal，保证向前兼容。
    timeFastOK = func() bool {
        if timeType.NumField() != 3 {
            return false
        }
        w, e, l := timeType.Field(0), timeType.Field(1), timeType.Field(2)
        return w.Name == "wall" && w.Type.Kind() == reflect.Uint64 &&
            e.Name == "ext" && e.Type.Kind() == reflect.Int64 &&
            l.Name == "loc" && l.Type.Kind() == reflect.Ptr
    }()
)

// timeRepIdentical 判断两个 time.Time 的内部表示是否完全一致。
// 表示一致则必为同一时刻（Equal 必返回 true），可在不装箱的情况下
// 判定相等；表示不同（如时区或单调时钟差异）仍需 Equal 精确判断。
func timeRepIdentical(a, b reflect.Value) bool {
    return a.Field(0).Uint() == b.Field(0).Uint() &&
        a.Field(1).Int() == b.Field(1).Int() &&
        a.Field(2).Pointer() == b.Field(2).Pointer()
}

// New 创建并初始化一个带变更监听的目标结构体。
// initial 为初始值，返回的指针可直接用于后续赋值和变更检测。
// 要求 T 必须是嵌入了 *Watcher[T] 的结构体，否则会 panic。
func New[T Watchable](initial T) *T {
    t, _ := newWatched(initial)
    return t
}

// newWatched 是 New 的内部实现，同时返回目标指针与 Watcher 实例，
// 供 SyncWatcher 等包装器持有并复用 Watcher 的方法。
func newWatched[T Watchable](initial T) (*T, *Watcher[T]) {
    meta := metaFor[T]()
    w := &Watcher[T]{
        target: new(T),
        meta:   meta,
    }
    *w.target = initial
    w.takeSnapshot()
    w.setEmbedded()
    return w.target, w
}

// Changes 返回自创建或上次 Reset 以来所有发生变更的字段列表。
// 如果接收器为 nil（未通过 New 创建），则 panic。
func (w *Watcher[T]) Changes() []Change {
    if w == nil {
        panic("structwatcher: method Changes called on nil Watcher, use structwatcher.New to create")
    }
    changes := make([]Change, 0, w.meta.numTracked)
    w.forEachField(func(name string, old, cur reflect.Value) bool {
        changes = append(changes, Change{
            Field:    name,
            OldValue: reportValue(old),
            NewValue: reportValue(cur),
        })
        return false
    })
    return changes
}

// IsChanged 返回是否有字段发生变更。
// 找到第一个变更字段即返回，不遍历剩余字段。
// 如果接收器为 nil（未通过 New 创建），则 panic。
func (w *Watcher[T]) IsChanged() bool {
    if w == nil {
        panic("structwatcher: method IsChanged called on nil Watcher, use structwatcher.New to create")
    }
    oldV := reflect.ValueOf(&w.snapshot).Elem()
    curV := reflect.ValueOf(w.target).Elem()
    t := oldV.Type()
    for i := 0; i < t.NumField(); i++ {
        if w.meta.skip[i] {
            continue
        }
        o, c := oldV.Field(i), curV.Field(i)
        // 未导出字段视为相等以跳过比较，避免 Interface() panic
        if !o.CanInterface() || !c.CanInterface() {
            continue
        }
        if !valueEqual(o, c) {
            return true
        }
    }
    return false
}

// Reset 将当前值设为新的快照（引用字段深拷贝），清空所有变更记录。
// 此后调用 Changes 将返回空，直到再次修改字段。
// 如果接收器为 nil，则 panic。
func (w *Watcher[T]) Reset() {
    if w == nil {
        panic("structwatcher: method Reset called on nil Watcher, use structwatcher.New to create")
    }
    w.takeSnapshot()
}

// History 返回最近一次 New 或 Reset 时的历史对象副本。
// 引用类型字段会做防御性深拷贝，修改返回值不会影响内部快照与后续变更检测；
// watch:"-" 忽略字段返回快照时刻的值，嵌入的 Watcher 会被设为原实例
// （返回值因此满足 Watchable 接口，但调用其方法操作的是原实例）。
// 如果接收器为 nil（未通过 New 创建），则 panic。
func (w *Watcher[T]) History() T {
    if w == nil {
        panic("structwatcher: method History called on nil Watcher, use structwatcher.New to create")
    }
    snap := reflect.ValueOf(&w.snapshot).Elem()
    cp := reflect.New(snap.Type()).Elem()
    cp.Set(snap)
    for i := 0; i < snap.NumField(); i++ {
        if w.meta.isEmbedded(i) {
            continue
        }
        f := cp.Field(i)
        if f.CanSet() && cachedContainsRef(f.Type()) {
            f.Set(deepCopyValue(snap.Field(i)))
        }
    }
    w.injectEmbedded(cp)
    return cp.Interface().(T)
}

// isEmbedded 判断字段索引 i 是否为嵌入的 *Watcher[T] 字段。
func (m *watcherMeta) isEmbedded(i int) bool {
    for _, e := range m.embedded {
        if e == i {
            return true
        }
    }
    return false
}

// metaFor 返回 T 的字段元数据，首次调用时构建并缓存。
// 若 T 不是结构体或未嵌入 *Watcher[T]，则 panic。
func metaFor[T Watchable]() *watcherMeta {
    t := reflect.TypeOf((*T)(nil)).Elem()
    if t.Kind() != reflect.Struct {
        panic(fmt.Sprintf("structwatcher: T must be a struct, got %s", t.Kind()))
    }
    if m, ok := metaCache.Load(t); ok {
        return m.(*watcherMeta)
    }
    watcherType := reflect.TypeOf(&Watcher[T]{})
    m := &watcherMeta{
        watcherType: watcherType,
        skip:        make([]bool, t.NumField()),
        numTracked:  t.NumField(),
    }
    embedded := false
    for i := 0; i < t.NumField(); i++ {
        f := t.Field(i)
        if f.Anonymous && f.Type == watcherType {
            m.skip[i] = true
            m.numTracked--
            m.embedded = append(m.embedded, i)
            embedded = true
            continue
        }
        if tag := f.Tag.Get(watchTag); tag != "" {
            if tag != watchTagIgnore {
                panic(fmt.Sprintf(
                    "structwatcher: invalid %s tag value %q on field %s.%s (only %q is supported)",
                    watchTag, tag, t.Name(), f.Name, watchTagIgnore))
            }
            m.skip[i] = true
            m.numTracked--
        }
    }
    if !embedded {
        panic(fmt.Sprintf("structwatcher: struct %s must embed *structwatcher.Watcher[T] as an anonymous field", t.Name()))
    }
    actual, _ := metaCache.LoadOrStore(t, m)
    return actual.(*watcherMeta)
}

// forEachField 遍历所有需要检查的字段，对每个不相等的字段调用 callback。
// 通过指针取 Elem() 获得零拷贝的字段视图，避免每次调用复制整个结构体。
// 回调返回 true 时停止遍历（用于短路优化），返回 false 继续遍历。
func (w *Watcher[T]) forEachField(cb func(name string, old, cur reflect.Value) bool) {
    oldV := reflect.ValueOf(&w.snapshot).Elem()
    curV := reflect.ValueOf(w.target).Elem()
    t := oldV.Type()
    for i := 0; i < t.NumField(); i++ {
        if w.meta.skip[i] {
            continue
        }
        o, c := oldV.Field(i), curV.Field(i)
        // 未导出字段视为相等以跳过比较，避免 Interface() panic
        if !o.CanInterface() || !c.CanInterface() {
            continue
        }
        if valueEqual(o, c) {
            continue
        }
        name := t.Field(i).Name
        if tagName := t.Field(i).Tag.Get("json"); tagName != "" {
            if idx := strings.IndexByte(tagName, ','); idx != -1 {
                name = tagName[:idx]
            } else {
                name = tagName
            }
        }
        if cb(name, o, c) {
            break
        }
    }
}

// takeSnapshot 将 target 的当前值保存为新的快照。
// 含引用语义类型的导出字段会做深拷贝，保证快照与目标不共享底层数据。
func (w *Watcher[T]) takeSnapshot() {
    cur := reflect.ValueOf(w.target).Elem()
    snap := reflect.ValueOf(&w.snapshot).Elem()
    snap.Set(cur)
    for i, skip := range w.meta.skip {
        if skip {
            continue
        }
        f := snap.Field(i)
        if f.CanSet() && cachedContainsRef(f.Type()) {
            f.Set(deepCopyValue(cur.Field(i)))
        }
    }
}

// setEmbedded 将 Watcher 实例设置到所有嵌入字段上。
// 这样目标结构体可以通过嵌入字段访问 Watcher 的公开方法。
func (w *Watcher[T]) setEmbedded() {
    w.injectEmbedded(reflect.ValueOf(w.target).Elem())
}

// injectEmbedded 将 Watcher 实例设置到 v 的所有嵌入字段上，v 须为 T 的可寻址视图。
func (w *Watcher[T]) injectEmbedded(v reflect.Value) {
    for _, i := range w.meta.embedded {
        if f := v.Field(i); f.CanSet() {
            f.Set(reflect.ValueOf(w))
        }
    }
}

// valueEqual 比较两个同类型的可导出值是否相等。
// 全程基于 reflect.Value 递归比较，仅在 Func/Chan 等罕见类型上
// 回退到 reflect.DeepEqual，避免 Interface() 装箱分配。
// 比较语义与 reflect.DeepEqual 一致，差异为：
//   - 两个 NaN 视为相等（含嵌套场景，语义在任意层级保持一致）；
//   - time.Time 使用 Equal 时刻语义（含嵌套场景）；
//   - 嵌套结构体内的未导出字段视为相等（与顶层字段规则一致）。
func valueEqual(a, b reflect.Value) bool {
    switch a.Kind() {
    case reflect.Bool:
        return a.Bool() == b.Bool()
    case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
        return a.Int() == b.Int()
    case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
        return a.Uint() == b.Uint()
    case reflect.Float32, reflect.Float64:
        return floatEqual(a.Float(), b.Float())
    case reflect.Complex64, reflect.Complex128:
        ca, cb := a.Complex(), b.Complex()
        return floatEqual(real(ca), real(cb)) && floatEqual(imag(ca), imag(cb))
    case reflect.String:
        return a.String() == b.String()
    case reflect.Ptr:
        if a.IsNil() || b.IsNil() {
            return a.IsNil() && b.IsNil()
        }
        if a.Pointer() == b.Pointer() {
            return true
        }
        return valueEqual(a.Elem(), b.Elem())
    case reflect.Interface:
        if a.IsNil() || b.IsNil() {
            return a.IsNil() && b.IsNil()
        }
        return valueEqual(a.Elem(), b.Elem())
    case reflect.Slice:
        if a.IsNil() != b.IsNil() {
            return false
        }
        if a.Len() != b.Len() {
            return false
        }
        if a.Pointer() == b.Pointer() {
            return true
        }
        for i := 0; i < a.Len(); i++ {
            if !valueEqual(a.Index(i), b.Index(i)) {
                return false
            }
        }
        return true
    case reflect.Array:
        for i := 0; i < a.Len(); i++ {
            if !valueEqual(a.Index(i), b.Index(i)) {
                return false
            }
        }
        return true
    case reflect.Map:
        if a.IsNil() != b.IsNil() {
            return false
        }
        if a.Len() != b.Len() {
            return false
        }
        if a.Len() == 0 || a.Pointer() == b.Pointer() {
            return true
        }
        iter := a.MapRange()
        for iter.Next() {
            v2 := b.MapIndex(iter.Key())
            if !v2.IsValid() || !valueEqual(iter.Value(), v2) {
                return false
            }
        }
        return true
    case reflect.Struct:
        if a.Type() == timeType {
            // time.Time 使用 Equal 语义，忽略单调时钟等内部差异。
            // 未变更的时间字段内部表示一致，走免装箱快速路径。
            if timeFastOK && timeRepIdentical(a, b) {
                return true
            }
            return a.Interface().(time.Time).Equal(b.Interface().(time.Time))
        }
        for i := 0; i < a.NumField(); i++ {
            af, bf := a.Field(i), b.Field(i)
            // 嵌套未导出字段视为相等，与顶层字段规则保持一致
            if !af.CanInterface() || !bf.CanInterface() {
                continue
            }
            if !valueEqual(af, bf) {
                return false
            }
        }
        return true
    case reflect.Func:
        // 非 nil 的 func 值无法可靠比较是否相等（DeepEqual 对非 nil
        // func 恒为 false，会导致永久误报），因此仅追踪 nil 状态的变化。
        return a.IsNil() == b.IsNil()
    case reflect.Chan:
        if a.IsNil() || b.IsNil() {
            return a.IsNil() && b.IsNil()
        }
        // 同一 channel（相同底层指针）视为相等
        return a.Pointer() == b.Pointer()
    case reflect.UnsafePointer:
        return a.Pointer() == b.Pointer()
    default:
        return reflect.DeepEqual(a.Interface(), b.Interface())
    }
}

// floatEqual 比较两个浮点数，两个 NaN 视为相等，
// 避免包含 NaN 的字段在 Reset 后仍被误报为变更。
func floatEqual(a, b float64) bool {
    return a == b || (math.IsNaN(a) && math.IsNaN(b))
}

// reportValue 返回用于 Change 报告的值。
// 非空指针按其指向的值报告，与比较语义（DeepEqual 解引用指针）保持一致，
// 避免报告中出现无意义的指针地址。
func reportValue(v reflect.Value) any {
    if v.Kind() == reflect.Ptr && !v.IsNil() {
        return v.Elem().Interface()
    }
    return v.Interface()
}

// cachedContainsRef 返回类型 t 的值中是否直接或嵌套包含
// 引用语义类型（slice/map/pointer/interface/func/chan）。
// 结果按类型缓存，纯值类型可跳过深拷贝。
func cachedContainsRef(t reflect.Type) bool {
    if v, ok := refCache.Load(t); ok {
        return v.(bool)
    }
    r := containsRef(t)
    refCache.Store(t, r)
    return r
}

func containsRef(t reflect.Type) bool {
    switch t.Kind() {
    case reflect.Slice, reflect.Map, reflect.Ptr, reflect.Interface,
        reflect.Func, reflect.Chan, reflect.UnsafePointer:
        return true
    case reflect.Array:
        return containsRef(t.Elem())
    case reflect.Struct:
        for i := 0; i < t.NumField(); i++ {
            if containsRef(t.Field(i).Type) {
                return true
            }
        }
    }
    return false
}

// deepCopyValue 返回 v 的深拷贝，用于构建快照。
// 纯值类型直接返回原值（无需拷贝）；引用类型递归复制，
// 使快照与目标不共享底层数据，从而能检测到原地修改。
// 注意：不支持自引用的循环数据结构。
func deepCopyValue(v reflect.Value) reflect.Value {
    if !v.IsValid() || !cachedContainsRef(v.Type()) {
        return v
    }
    switch v.Kind() {
    case reflect.Slice:
        if v.IsNil() {
            return v
        }
        cp := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
        if cachedContainsRef(v.Type().Elem()) {
            for i := 0; i < v.Len(); i++ {
                cp.Index(i).Set(deepCopyValue(v.Index(i)))
            }
        } else {
            reflect.Copy(cp, v)
        }
        return cp
    case reflect.Map:
        if v.IsNil() {
            return v
        }
        // 键按原样复制（map 键由 == 定位，深拷贝指针/接口键会破坏查找语义）
        cp := reflect.MakeMapWithSize(v.Type(), v.Len())
        iter := v.MapRange()
        for iter.Next() {
            cp.SetMapIndex(iter.Key(), deepCopyValue(iter.Value()))
        }
        return cp
    case reflect.Ptr:
        if v.IsNil() {
            return v
        }
        cp := reflect.New(v.Type().Elem())
        cp.Elem().Set(deepCopyValue(v.Elem()))
        return cp
    case reflect.Interface:
        if v.IsNil() {
            return v
        }
        cp := reflect.New(v.Type()).Elem()
        cp.Set(deepCopyValue(v.Elem()))
        return cp
    case reflect.Array:
        cp := reflect.New(v.Type()).Elem()
        if cachedContainsRef(v.Type().Elem()) {
            for i := 0; i < v.Len(); i++ {
                cp.Index(i).Set(deepCopyValue(v.Index(i)))
            }
        } else {
            cp.Set(v)
        }
        return cp
    case reflect.Struct:
        cp := reflect.New(v.Type()).Elem()
        cp.Set(v)
        for i := 0; i < v.NumField(); i++ {
            if cp.Field(i).CanSet() && cachedContainsRef(v.Type().Field(i).Type) {
                cp.Field(i).Set(deepCopyValue(v.Field(i)))
            }
        }
        return cp
    }
    return v
}
