// 字段忽略示例：使用 watch:"-" 标签将时间戳、版本号等噪音字段
// 排除在变更检测之外。被忽略的字段同样不参与快照深拷贝。
package main

import (
	"fmt"
	"time"

	"github.com/itnxs/structwatcher"
)

type Article struct {
	*structwatcher.Watcher[Article]
	Title     string    `watch:"-"` // 忽略：外部同步字段，不追踪
	Version   int       `watch:"-"` // 忽略：版本号噪音
	UpdatedAt time.Time `watch:"-"` // 忽略：时间戳噪音
	Content   string    // 正常追踪
	History   []string  // 正常追踪（含原地修改检测）
}

func main() {
	a := structwatcher.New(Article{
		Title:   "t",
		Version: 1,
		Content: "hello",
		History: []string{"init"},
	})

	// 修改被忽略的字段：不计入变更
	a.Title = "new-title"
	a.Version = 2
	a.UpdatedAt = time.Now()

	fmt.Println("修改忽略字段后是否有变更:", a.IsChanged()) // false

	// 修改正常追踪的字段：正常检测
	a.Content = "world"
	a.History[0] = "modified" // 原地修改同样检测

	for _, c := range a.Changes() {
		fmt.Printf("字段 [%s]: %v -> %v\n", c.Field, c.OldValue, c.NewValue)
	}
	// 输出:
	// 字段 [Content]: hello -> world
	// 字段 [History]: [init] -> [modified]
}
