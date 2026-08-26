// 脏检查示例：最常见的生产用法——只有字段实际变更时才执行持久化，
// 避免无变化的重复写库。IsChanged 热路径零分配，适合高频轮询。
package main

import (
	"fmt"

	"github.com/itnxs/structwatcher"
)

type Profile struct {
	*structwatcher.Watcher[Profile]
	Nickname string
	Avatar   string
	Bio      string
}

// save 模拟写库：仅当发生变更时执行，返回是否写入
func save(p *Profile) bool {
	if !p.IsChanged() {
		fmt.Println("跳过保存：无变更")
		return false
	}
	fmt.Printf("保存 %d 个字段变更到数据库\n", len(p.Changes()))
	p.Reset() // 落库成功后以当前状态为新基准
	return true
}

func main() {
	p := structwatcher.New(Profile{Nickname: "levi", Avatar: "a.png", Bio: "hi"})

	// 第一次保存：无变更，跳过
	save(p)

	// 修改两个字段
	p.Bio = "gopher"
	p.Avatar = "b.png"
	save(p)

	// 未再修改，再次跳过
	save(p)
}
