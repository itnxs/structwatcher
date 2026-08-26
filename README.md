# structwatcher

`structwatcher` 是一个基于 Go 泛型的结构体变更监听库。通过简单的泛型嵌入，您可以轻松追踪结构体字段自创建或最近一次重置以来的任何值变更。

## 功能特性

* **泛型支持**: 适用于任何结构体类型（Go 1.18+）。
* **极简嵌入**: 仅需将 `*Watcher[T]` 作为匿名字段嵌入结构体即可使用。
* **深快照机制**: 快照对 slice/map/pointer/interface 字段做深拷贝，原地修改（如 `s[0]=x`、`m[k]=v`、`*p=v`）也能被检测到。
* **友好的比较语义**: 两个 `NaN` 视为相等；`time.Time` 按 `Equal` 时刻语义比较，忽略单调时钟差异；指针按指向的值比较并在报告中解引用。
* **未导出字段自动忽略**: 自动跳过非导出字段的比对，确保安全。
* **字段级忽略标签**: 通过 `watch:"-"` 可将时间戳、版本号等噪音字段排除在检测之外，同时跳过其快照深拷贝。
* **性能优化**: 字段元数据按类型缓存、零拷贝字段视图、标量类型免装箱快速比较，`IsChanged` 热路径零分配。
* **并发安全包装**: `SyncWatcher` 提供基于 RWMutex 的并发安全访问，写入走 `Update`、并发读取走 `View`，无需使用方自行加锁。

## 安装

确保您的 Go 版本是 1.18 或更高：

```bash
go get github.com/itnxs/structwatcher
```

## 使用示例

完整可运行示例见 [examples/](./examples/) 目录：

* [examples/basic](./examples/basic): 创建、修改、`IsChanged` 与 `Changes` 基础用法
* [examples/reftypes](./examples/reftypes): slice/map/指针字段的原地修改检测
* [examples/reset](./examples/reset): 使用 `Reset` 更新快照基准
* [examples/semantics](./examples/semantics): NaN、`time.Time`、指针、未导出字段的比较语义
* [examples/tagignore](./examples/tagignore): 使用 `watch:"-"` 忽略指定字段
* [examples/dirtycheck](./examples/dirtycheck): 脏检查实战——仅变更时落库
* [examples/audit](./examples/audit): 用 `Changes()` 生成审计日志
* [examples/nested](./examples/nested): 嵌套结构体、数组、多级引用的检测
* [examples/sync](./examples/sync): `SyncWatcher` 多 goroutine 并发读写

### 1. 基础示例：监听 int 与 string 变化

展示最基础的用法：创建、修改、检测变更（`IsChanged`）以及获取变更详情（`Changes`）。

```go
package main

import (
	"fmt"

	"github.com/itnxs/structwatcher"
)

type Person struct {
	*structwatcher.Watcher[Person] // 必须嵌入泛型 Watcher 指针
	Name string
	Age  int
}

func main() {
	// 1. 创建受监控的实例
	p := structwatcher.New(Person{Name: "Alice", Age: 25})

	// 初始状态，无变化
	if !p.IsChanged() {
		fmt.Println("初始化后尚未发生变更")
	}

	// 2. 修改字段
	p.Name = "Bob"
	p.Age = 26

	// 3. 检测变更
	fmt.Printf("是否有变更: %t\n", p.IsChanged()) // 输出: true

	// 4. 打印变更的详细信息 (旧值与新值)
	changes := p.Changes()
	for _, change := range changes {
		fmt.Printf("字段 [%s]: 旧值=%v -> 新值=%v\n", change.Field, change.OldValue, change.NewValue)
	}
}
```

### 2. 处理复杂类型：Slice 与 布尔值

库支持 `reflect.DeepEqual` 处理所有可比较类型，包括切片（Slice）和布尔值（Boolean）。

```go
type Config struct {
	*structwatcher.Watcher[Config]
	Title   string
	Enabled bool
	Tags    []string
}

func main() {
	c := structwatcher.New(Config{
		Title:   "My Config",
		Enabled: false,
		Tags:    []string{"a", "b"},
	})

	// 修改布尔值与切片内容
	c.Enabled = true
	c.Tags = append(c.Tags, "c")

	fmt.Println("Config 变更详情:")
	for _, ch := range c.Changes() {
		// Tags 变更时，会显示出完整的旧切片与新切片
		fmt.Printf("%s: %v -> %v\n", ch.Field, ch.OldValue, ch.NewValue)
	}
}
```

### 3. 使用 Reset 更新快照

调用 `Reset()` 会将当前状态设为新的基准。调用后，`Changes()` 将返回空列表，直到下一次修改。

```go
// User 定义略...

func main() {
	u := structwatcher.New(User{Name: "Tom", Level: 1})

	// 修改后变更
	u.Level = 2
	fmt.Println(u.IsChanged()) // true

	// 重置，接受当前状态为新的基准
	u.Reset()

	// 现在状态变为“干净”
	fmt.Println(u.IsChanged()) // false

	// 再次修改，仅报告 Reset 之后的变更
	u.Name = "Jerry"
	fmt.Println(u.Changes()) // 输出 {Name, Tom->Jerry}
}
```

### 4. 检测原地修改（In-place Mutation）

快照对引用类型字段做深拷贝，因此直接修改 slice 元素、map 条目或指针指向的值也能被检测到：

```go
type Inventory struct {
	*structwatcher.Watcher[Inventory]
	Tags []string
	Stock map[string]int
}

func main() {
	inv := structwatcher.New(Inventory{
		Tags:  []string{"a", "b"},
		Stock: map[string]int{"apple": 1},
	})

	// 原地修改，而非整体赋值
	inv.Tags[0] = "x"
	inv.Stock["apple"] = 10

	for _, ch := range inv.Changes() {
		// Tags: [a b] -> [x b]
		// Stock: map[apple:1] -> map[apple:10]
		fmt.Printf("%s: %v -> %v\n", ch.Field, ch.OldValue, ch.NewValue)
	}
}
```

### 5. 使用 `watch:"-"` 忽略指定字段

时间戳、版本号等每次写入都会变的"噪音"字段，可以通过 struct tag 排除在变更检测之外。被忽略的字段同样不参与快照深拷贝，忽略大型引用字段还能降低 `New`/`Reset` 开销。tag 仅支持 `"-"`，写成其他值会在 `New` 时 panic，及早暴露拼写错误。

```go
type Article struct {
	*structwatcher.Watcher[Article]
	Title     string    `watch:"-"` // 忽略：不追踪 Title 的变更
	UpdatedAt time.Time `watch:"-"` // 忽略：时间戳噪音
	Content   string              // 正常追踪
}

func main() {
	a := structwatcher.New(Article{Title: "t", Content: "hello"})

	a.Title = "new-title"
	a.UpdatedAt = time.Now()

	fmt.Println(a.IsChanged()) // false，被忽略字段不计入

	a.Content = "world"
	fmt.Println(a.IsChanged()) // true
}
```

### 6. 并发安全访问：SyncWatcher

`Watcher` 本身非并发安全。多 goroutine 场景下使用 `NewSync` 创建 `SyncWatcher`，它内部用 RWMutex 保护访问，使用方无需自行加锁：

```go
m := structwatcher.NewSync(Metrics{})

// 写：在写锁下修改字段
m.Update(func(x *Metrics) { x.Requests++ })

// 读：在读锁下只读访问字段
m.View(func(x *Metrics) { fmt.Println(x.Requests) })

// 变更检测与重置，语义同 Watcher
fmt.Println(m.IsChanged())
for _, c := range m.Changes() { /* ... */ }
m.Reset()
```

使用契约（库无法拦截对字段的直接赋值，违反契约仍会数据竞争）：

* 对字段的所有**写**操作必须通过 `Update` 进行；
* 并发**读**字段必须通过 `View` 进行；
* 不要在 `Update`/`View` 回调内调用 `SyncWatcher` 或内嵌 `Watcher` 的方法（锁不可重入，会死锁），也不要把回调中的 `*T` 持有到回调之外；
* `Changes()` 返回的 `NewValue` 做了防御性深拷贝，不受后续并发修改影响；
* 确定无并发时可通过 `Unwrap()` 取回原始 `*T` 直接操作。

### 7. 忽略未导出字段（Private Fields）

`structwatcher` 遵循 Go 的可见性规则，会自动忽略不可访问的字段。

```go
type SecureData struct {
	*structwatcher.Watcher[SecureData]
	PublicKey  string
	privateKey string // 小写开头，未导出字段
}

func main() {
	d := structwatcher.New(SecureData{
		PublicKey:  "public123",
		privateKey: "secret456",
	})

	// 修改未导出字段不会计入 Changes
	d.privateKey = "secret789"
	
	// 结果为 false，因为无法访问或比对 unexported 字段
	fmt.Println("是否有变更:", d.IsChanged()) 
}
```

## 比较语义

* **标量类型**（bool/整数/浮点/复数/字符串）直接按值比较，不走反射深比较。
* **NaN**: 两个 `NaN` 视为相等，避免包含 NaN 的字段在 `Reset` 后仍被误报为变更。
* **time.Time**: 使用 `Time.Equal` 时刻语义，同一时刻即视为相等（忽略单调时钟、时区内部表示差异）。
* **指针**: 与 `reflect.DeepEqual` 一致，按指向的值比较；`Change` 报告中也会解引用非空指针，显示实际的值而非地址。
* **func/chan**: func 字段仅追踪 nil 状态的变化（非 nil 的 func 无法比较是否相等）；chan 字段同一 channel 视为相等。
* **字段忽略**: `watch:"-"` 标记的字段完全跳过检测（含快照深拷贝），未导出字段也始终跳过。
* **其他类型**: 回退到 `reflect.DeepEqual`。

## API 参考

### 类型与方法

* `New[T Watchable](initial T) *T`: 
  创建被包装的结构体实例并返回结构体指针。要求 T 必须嵌入 `*Watcher[T]`，否则 panic。
* `NewSync[T Watchable](initial T) *SyncWatcher[T]`: 
  创建并发安全版本，语义同 `New`。
* `SyncWatcher.Update(fn func(t *T))`: 
  在写锁下执行 fn，fn 内可安全修改字段。
* `SyncWatcher.View(fn func(t *T))`: 
  在读锁下执行 fn，fn 内只读访问字段。
* `SyncWatcher.IsChanged() bool` / `Changes() []Change` / `Reset()`: 
  语义同 `Watcher` 的对应方法；`Changes` 的 `NewValue` 为防御性深拷贝。
* `SyncWatcher.Unwrap() *T`: 
  取回原始 `*T`，调用方需自行保证此后无并发访问。
* `IsChanged() bool`: 
  检查自创建或上次 Reset 以来是否有任何字段发生**值变更**。遇到第一个变更字段即返回（短路机制）。
* `Changes() []Change`: 
  返回包含所有变更字段的切片。`Change` 结构包含字段名 `Field`、旧值 `OldValue` 和新值 `NewValue`。
* `Reset()`: 
  将当前所有字段的值保存为新的快照（引用字段深拷贝），清空当前变更记录。

## 注意事项

* **测试覆盖率**: 单测覆盖率约 82%（watcher.go + sync.go）。
* **性能**（实测数量级见 `bench_large_test.go`，随机器而异）：
  * 小结构体（2~3 字段）：`IsChanged` 约 40ns、零分配；
  * 50+ 字段大结构体：纯标量/切片/时间字段约 1~1.5µs 且**零分配**；含 map 字段时约 3 allocs/条目（reflect 访问 map 的固有成本，`reflect.DeepEqual` 相同），可用 `watch:"-"` 忽略大 map；
  * `New`/`Reset` 对引用字段深拷贝，成本随数据体积线性增长（4 个万元素 slice + 2 个千条目 map 约 600µs）；
  * 字段元数据按类型缓存，标量与切片比较不经过装箱。
* **深拷贝成本**: `New`/`Reset` 会深拷贝含引用语义的字段，大 slice/map 的快照有一次复制开销；纯值类型结构体（不含 slice/map/指针/接口）的快照退化为一次浅拷贝，无额外开销。
* **循环数据结构**: 不支持自引用的循环数据结构（指针成环时深拷贝会栈溢出）。
* **值拷贝**: 结构体按值拷贝后（`p2 := *p`），副本仍共享原 Watcher，其变更会计入原快照。
* **并发**: `Watcher` 非并发安全。并发场景请使用 `SyncWatcher`（写入走 `Update`、读取走 `View`），或自行加锁。
