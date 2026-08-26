// 引用类型示例：演示对 slice/map/指针字段的原地修改也能被检测到，
// 因为 New 与 Reset 的快照对引用类型字段做了深拷贝。
package main

import (
	"fmt"

	"github.com/itnxs/structwatcher"
)

type Inventory struct {
	*structwatcher.Watcher[Inventory]
	Tags  []string
	Stock map[string]int
	Top   *int
}

func main() {
	top := 3
	inv := structwatcher.New(Inventory{
		Tags:  []string{"a", "b"},
		Stock: map[string]int{"apple": 1},
		Top:   &top,
	})

	// 原地修改：不是整体赋值，而是改动底层数据
	inv.Tags[0] = "x"
	inv.Stock["apple"] = 10
	*inv.Top = 99

	for _, c := range inv.Changes() {
		fmt.Printf("%s: %v -> %v\n", c.Field, c.OldValue, c.NewValue)
	}
	// 输出（map 打印顺序可能不同）:
	// Tags: [a b] -> [x b]
	// Stock: map[apple:1] -> map[apple:10]
	// Top: 3 -> 99   （非空指针在报告中自动解引用为指向的值）
}
