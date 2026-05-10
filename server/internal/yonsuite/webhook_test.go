package yonsuite

// webhook_test.go — ClearBIServerCache HTTP 调用边界
// 已 Read webhook.go (line 14 ClearBIServerCache).

import (
	"testing"
)

// HTTP 走 hardcoded 127.0.0.1:8080, 测试机大概率没监听 → 走 err 分支静默 log
func TestClearBIServerCacheNoServer(t *testing.T) {
	// 不应 panic, log 后返回
	ClearBIServerCache("any-secret")
}

func TestClearBIServerCacheEmptySecret(t *testing.T) {
	// 空 secret 也是合法输入 (取决于 bi-server 端校验)
	ClearBIServerCache("")
}
