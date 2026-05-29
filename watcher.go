// Package structwatcher 提供基于泛型的结构体变更监听功能。
// 通过嵌入 Watcher 指针到目标结构体中，
// 可以自动追踪结构体字段自创建或 Reset 以来的变更。
//
// 并发安全：本包不是并发安全的。
// 不要在多个 goroutine 中同时修改字段和调用 Changes/IsChanged/Reset。
// 如需并发访问，请自行加锁保护。
package structwatcher

import (
	"fmt"
	"reflect"
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
		snapshot    T            // 创建或 Reset 时的快照值
		target      *T           // 当前实际使用中的目标值指针
		watcherType reflect.Type // Watcher[T] 类型，用于识别需跳过的嵌入字段
		skipSet     map[int]bool // 需要跳过比对的字段索引集合
	}

	// Change 表示单个字段的变更信息
	Change struct {
		Field    string // 变更的字段名
		OldValue any    // 变更前的值
		NewValue any    // 变更后的值
	}
)

// New 创建并初始化一个带变更监听的目标结构体。
// initial 为初始值，返回的指针可直接用于后续赋值和变更检测。
// 要求 T 结构体必须嵌入 *Watcher[T]，否则会 panic。
func New[T Watchable](initial T) *T {
	watcherType := reflect.TypeOf(&Watcher[T]{})
	validateWatchable(initial, watcherType)
	w := &Watcher[T]{
		snapshot:    initial,
		target:      new(T),
		watcherType: watcherType,
	}
	*w.target = initial
	w.buildSkipSet()
	w.setEmbedded()
	return w.target
}

// Changes 返回自创建或上次 Reset 以来所有发生变更的字段列表。
// 如果接收器为 nil（未通过 New 创建），则 panic。
func (w *Watcher[T]) Changes() []Change {
	if w == nil {
		panic("structwatcher: method Changes called on nil Watcher, use structwatcher.New to create")
	}
	var changes []Change
	w.forEachField(func(name string, old, cur reflect.Value) bool {
		changes = append(changes, Change{
			Field:    name,
			OldValue: old.Interface(),
			NewValue: cur.Interface(),
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
	found := false
	w.forEachField(func(name string, old, cur reflect.Value) bool {
		found = true
		return true // 短路，停止遍历
	})
	return found
}

// Reset 将当前值设为新的快照，清空所有变更记录。
// 此后调用 Changes 将返回空，直到再次修改字段。
// 如果接收器为 nil，则 panic。
func (w *Watcher[T]) Reset() {
	if w == nil {
		panic("structwatcher: method Reset called on nil Watcher, use structwatcher.New to create")
	}
	w.snapshot = *w.target
}

// forEachField 遍历所有需要检查的字段，对每个不相等的字段调用 callback。
// 回调返回 true 时停止遍历（用于短路优化），返回 false 继续遍历。
func (w *Watcher[T]) forEachField(cb func(name string, old, cur reflect.Value) bool) {
	oldV := reflect.ValueOf(w.snapshot)
	curV := reflect.ValueOf(*w.target)
	t := oldV.Type()
	for i := 0; i < t.NumField(); i++ {
		if w.skipSet[i] {
			continue
		}
		if isFieldEqual(oldV, curV, i) {
			continue
		}
		if cb(t.Field(i).Name, oldV.Field(i), curV.Field(i)) {
			break
		}
	}
}

// buildSkipSet 构建需要跳过比对的字段索引集合。
// 当前会跳过类型为 *Watcher[T] 的匿名嵌入字段。
func (w *Watcher[T]) buildSkipSet() {
	v := reflect.ValueOf(w.target).Elem()
	t := v.Type()
	w.skipSet = make(map[int]bool, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous && f.Type == w.watcherType {
			w.skipSet[i] = true
		}
	}
}

// setEmbedded 将 Watcher 实例设置到所有需要跳过的嵌入字段上。
// 这样目标结构体可以通过嵌入字段访问 Watcher 的公开方法。
func (w *Watcher[T]) setEmbedded() {
	v := reflect.ValueOf(w.target).Elem()
	for i := range w.skipSet {
		if v.Field(i).CanSet() {
			v.Field(i).Set(reflect.ValueOf(w))
		}
	}
}

// isFieldEqual 比较指定索引处的两个字段值是否相等。
// 如果字段未导出（小写开头），直接视为相等以跳过比较，
// 避免调用 Interface() 导致 panic。
func isFieldEqual(oldV, curV reflect.Value, i int) bool {
	if !oldV.Field(i).CanInterface() || !curV.Field(i).CanInterface() {
		return true // 跳过未导出字段的比较
	}
	return reflect.DeepEqual(oldV.Field(i).Interface(), curV.Field(i).Interface())
}

// validateWatchable 校验 T 结构体是否正确嵌入了 *Watcher[T]。
// watcherType 为 reflect.TypeOf(&Watcher[T]{})，由 New 传入避免重复计算。
func validateWatchable[T Watchable](initial T, watcherType reflect.Type) {
	t := reflect.TypeOf(initial)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous && f.Type == watcherType {
			return
		}
	}
	panic(fmt.Sprintf("structwatcher: struct %s must embed *structwatcher.Watcher[T] as an anonymous field", t.Name()))
}
