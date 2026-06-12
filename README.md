# structwatcher

`structwatcher` 是一个基于 Go 泛型的结构体变更监听库。通过简单的泛型嵌入，您可以轻松追踪结构体字段自创建或最近一次重置以来的任何值变更。

## 功能特性

* **泛型支持**: 适用于任何结构体类型（Go 1.18+）。
* **极简嵌入**: 仅需将 `*Watcher[T]` 作为匿名字段嵌入结构体即可使用。
* **快照机制**: 自动维护字段的旧值快照，高效比对差异。
* **未导出字段自动忽略**: 自动跳过非导出字段的比对，确保安全。
* **并发说明**: 当前版本非并发安全，并发场景需自行处理锁。

## 安装

确保您的 Go 版本是 1.18 或更高：

```bash
go get github.com/itnxs/structwatcher
```

## 使用示例

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

### 4. 忽略未导出字段（Private Fields）

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

## API 参考

### 类型与方法

* `New[T Watchable](initial T) *T`: 
  创建被包装的结构体实例并返回结构体指针。要求 T 必须嵌入 `*Watcher[T]`。
* `IsChanged() bool`: 
  检查自创建或上次 Reset 以来是否有任何字段发生**值变更**。遇到第一个变更字段即返回（短路机制）。
* `Changes() []Change`: 
  返回包含所有变更字段的切片。`Change` 结构包含字段名 `Field`、旧值 `OldValue` 和新值 `NewValue`。
* `Reset()`: 
  将当前所有字段的值保存为新的快照（Snapshot），清空当前变更列表。

## 注意事项

* **性能**: 底层依赖 `reflect` 包进行字段比对。对于极其庞大的结构体，频繁调用 `IsChanged`/`Changes` 可能会产生一定开销。
* **并发**: 本包不是并发安全的。请勿在多个协程中同时读写被监听的结构体。
