// 审计日志示例：用 Changes() 生成人类可读的字段级变更记录，
// 适合配置发布、工单编辑等需要留痕的场景。
package main

import (
	"fmt"
	"strings"

	"github.com/itnxs/structwatcher"
)

type GatewayConfig struct {
	*structwatcher.Watcher[GatewayConfig]
	Host    string
	Port    int
	Retries int
	Targets []string
}

func auditLog(c *GatewayConfig) string {
	changes := c.Changes()
	if len(changes) == 0 {
		return "无变更"
	}
	lines := make([]string, 0, len(changes))
	for _, ch := range changes {
		lines = append(lines, fmt.Sprintf("  %s: %v -> %v", ch.Field, ch.OldValue, ch.NewValue))
	}
	c.Reset()
	return strings.Join(lines, "\n")
}

func main() {
	cfg := structwatcher.New(GatewayConfig{
		Host:    "localhost",
		Port:    8080,
		Retries: 3,
		Targets: []string{"a", "b"},
	})

	// 第一轮变更
	cfg.Port = 9090
	cfg.Retries = 5
	fmt.Println("审计记录 #1:")
	fmt.Println(auditLog(cfg))

	// 第二轮变更（含 slice 原地修改）
	cfg.Targets[1] = "c"
	cfg.Host = "gateway.internal"
	fmt.Println("审计记录 #2:")
	fmt.Println(auditLog(cfg))

	// 第三轮：无变更
	fmt.Println("审计记录 #3:")
	fmt.Println(auditLog(cfg))
}
