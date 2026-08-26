// 嵌套与复杂类型示例：演示嵌套结构体、数组、多级引用的检测行为。
// 快照的深拷贝是递归的——嵌套在结构体/数组/slice 内部的引用类型
// 同样会被复制，原地修改任何一层都能被检测到。
package main

import (
	"fmt"

	"github.com/itnxs/structwatcher"
)

type Endpoint struct {
	Host string
	Port int
}

type Service struct {
	*structwatcher.Watcher[Service]
	Name    string
	Replica Endpoint       // 嵌套结构体（值类型，整体比较）
	Nodes   [2]Endpoint    // 数组
	Meta    map[string]any // map，值为接口（可放任意引用类型）
	Labels  *[]string      // 指向 slice 的指针（多级引用）
}

func main() {
	labels := []string{"core", "v1"}
	s := structwatcher.New(Service{
		Name:    "api",
		Replica: Endpoint{Host: "10.0.0.1", Port: 80},
		Nodes:   [2]Endpoint{{Host: "n1", Port: 81}, {Host: "n2", Port: 82}},
		Meta:    map[string]any{"owner": "team-a"},
		Labels:  &labels,
	})

	// 1. 修改嵌套结构体的字段
	s.Replica.Port = 8080

	// 2. 修改数组元素（数组元素为值类型，视为整体的一个字段变更）
	s.Nodes[1].Host = "n2-new"

	// 3. 原地修改 map 的接口值
	s.Meta["owner"] = "team-b"

	// 4. 多级引用：修改指针指向的 slice 内容
	(*s.Labels)[1] = "v2"

	for _, c := range s.Changes() {
		fmt.Printf("%s: %v -> %v\n", c.Field, c.OldValue, c.NewValue)
	}
	// 输出（map 打印顺序可能不同）:
	// Name 未变不出现；四个字段各报告一条
	// Replica: {10.0.0.1 80} -> {10.0.0.1 8080}
	// Nodes: [{n1 81} {n2 82}] -> [{n1 81} {n2-new 82}]
	// Meta: map[owner:team-a] -> map[owner:team-b]
	// Labels: [core v1] -> [core v2]
}
