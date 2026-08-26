// 基础示例：创建受监控的结构体、修改字段、检测变更并获取变更详情。
package main

import (
	"fmt"

	"github.com/itnxs/structwatcher"
)

type Person struct {
	*structwatcher.Watcher[Person]
	Name string
	Age  int
}

func main() {
	// 1. 创建受监控的实例
	p := structwatcher.New(Person{Name: "Alice", Age: 25})

	// 初始状态，无变化
	fmt.Println("初始化后是否有变更:", p.IsChanged()) // false

	// 2. 修改字段
	p.Name = "Bob"
	p.Age = 26

	// 3. 检测变更
	fmt.Println("修改后是否有变更:", p.IsChanged()) // true

	// 4. 打印变更详情（旧值 -> 新值）
	for _, c := range p.Changes() {
		fmt.Printf("字段 [%s]: %v -> %v\n", c.Field, c.OldValue, c.NewValue)
	}
	// 输出:
	// 字段 [Name]: Alice -> Bob
	// 字段 [Age]: 25 -> 26
}
