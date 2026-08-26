// 比较语义示例：演示 NaN、time.Time、指针、未导出字段的比较行为。
package main

import (
	"fmt"
	"math"
	"time"

	"github.com/itnxs/structwatcher"
)

type Event struct {
	*structwatcher.Watcher[Event]
	Name    string
	At      time.Time
	Score   float64
	private string // 未导出字段，不参与变更检测
}

func main() {
	// 1. 两个 NaN 视为相等，Reset 后不会误报变更
	e := structwatcher.New(Event{Name: "launch", Score: math.NaN()})
	fmt.Println("NaN 初始是否有变更:", e.IsChanged()) // false
	e.Reset()
	fmt.Println("NaN Reset 后是否有变更:", e.IsChanged()) // false

	// 2. time.Time 按时刻（Equal 语义）比较，单调时钟差异被忽略
	now := time.Now()
	e.At = now
	e.Reset()
	e.At = now.Round(0)                      // 去掉单调时钟，但时刻相同
	fmt.Println("相同时刻是否有变更:", e.IsChanged()) // false
	e.At = now.Add(time.Second)
	fmt.Println("不同时刻是否有变更:", e.IsChanged()) // true

	// 3. 未导出字段的修改不计入变更
	e.Reset()
	e.private = "hidden"
	fmt.Println("修改未导出字段后是否有变更:", e.IsChanged()) // false
}
