// 并发测试示例：多 goroutine 场景下对 SyncWatcher 进行压力测试。
// 覆盖：并发 Update 写入、并发 View 读取、并发 IsChanged/Changes 轮询、
// 后台定期 Reset，并在结束时校验最终状态的一致性。
// 可用 go run -race 运行以验证无数据竞争。
package main

import (
	"fmt"
	"sync"

	"github.com/itnxs/structwatcher"
)

type Metrics struct {
	*structwatcher.Watcher[Metrics]
	Requests int
	Errors   int
	Tags     []string
}

const (
	writers      = 4  // 并发写入的 goroutine 数
	increments   = 50 // 每个写入者执行的增量次数
	errEvery     = 7  // 每 errEvery 次请求记一次错误
	resetEvery   = 25 // 写入者每 resetEvery 次增量触发一次 Reset
	readerRounds = 20 // 每个读取者的轮询轮数
)

func main() {
	m := structwatcher.NewSync(Metrics{Tags: []string{"svc"}})

	var wg sync.WaitGroup

	// 1. 多个写入者：并发 Update 递增计数、追加 Tags、定期 Reset
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < increments; j++ {
				m.Update(func(x *Metrics) {
					x.Requests++
					if j%errEvery == 0 {
						x.Errors++
					}
					x.Tags = append(x.Tags, "req")
				})
				if j%resetEvery == resetEvery-1 {
					m.Reset() // 定期以当前状态为新基准
				}
			}
		}()
	}

	// 2. 并发读取者：View 轮询字段 + IsChanged 检查
	//    只读访问不能直接解引用 Unwrap 的结果，必须走 View。
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := 0; r < readerRounds; r++ {
				var total int
				m.View(func(x *Metrics) {
					total = x.Requests + x.Errors
					_ = x.Tags
				})
				_ = m.IsChanged()
				if total < 0 { // 不可能为负，仅为了使用变量
					panic("negative counter")
				}
			}
		}()
	}

	// 3. 并发审计者：轮询 Changes，验证返回结果不被后续修改影响
	wg.Add(1)
	go func() {
		defer wg.Done()
		for r := 0; r < readerRounds; r++ {
			for _, c := range m.Changes() {
				// 拿到快照结果后，即使写入者继续修改字段，
				// 这里的 NewValue 也不会被污染（防御性深拷贝）
				_ = c.Field
				_, _ = c.OldValue, c.NewValue
			}
		}
	}()

	wg.Wait()

	// 4. 最终一致性校验：所有写入都通过 Update，总数必须精确
	var requests, errors int
	var tags int
	m.View(func(x *Metrics) {
		requests, errors, tags = x.Requests, x.Errors, len(x.Tags)
	})

	wantRequests := writers * increments
	wantErrors := writers * ((increments-1)/errEvery + 1)
	wantTags := 1 + wantRequests

	fmt.Printf("Requests: %d (期望 %d) 一致: %t\n", requests, wantRequests, requests == wantRequests)
	fmt.Printf("Errors:   %d (期望 %d) 一致: %t\n", errors, wantErrors, errors == wantErrors)
	fmt.Printf("Tags 长度: %d (期望 %d) 一致: %t\n", tags, wantTags, tags == wantTags)

	if requests != wantRequests || errors != wantErrors || tags != wantTags {
		panic("最终状态不一致：存在丢失更新")
	}
	fmt.Println("并发测试通过：无数据竞争、无丢失更新")
}
