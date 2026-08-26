// Reset 示例：演示如何以当前状态为新基准，仅报告 Reset 之后的变更。
package main

import (
	"fmt"

	"github.com/itnxs/structwatcher"
)

type User struct {
	*structwatcher.Watcher[User]
	Name  string
	Level int
}

func main() {
	u := structwatcher.New(User{Name: "Tom", Level: 1})

	u.Level = 2
	fmt.Println("修改后是否有变更:", u.IsChanged()) // true

	// 重置：接受当前状态为新的基准（引用字段会再次深拷贝）
	u.Reset()
	fmt.Println("Reset 后是否有变更:", u.IsChanged()) // false

	// 再次修改，仅报告 Reset 之后的变更
	u.Name = "Jerry"
	for _, c := range u.Changes() {
		fmt.Printf("字段 [%s]: %v -> %v\n", c.Field, c.OldValue, c.NewValue)
	}
	// 输出:
	// 字段 [Name]: Tom -> Jerry
}
