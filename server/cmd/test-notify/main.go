// 完整钉钉推送链路测试
//   cd server && go run ./cmd/test-notify
package main

import (
	"fmt"
	"os"

	"bi-dashboard/internal/dingtalk"
)

func main() {
	n := dingtalk.NewNotifier(
		"dingxfea1dc78fki8xcb",
		"dU2yZScYe4teGp73UYbVoKR5eZ1qD65YEpfDgIB7Imi6lsaAU68tVd1wGKqMAZj4",
		"dingxfea1dc78fki8xcb",
	)
	if n == nil {
		fmt.Println("[FAIL] notifier nil")
		os.Exit(1)
	}

	huPaoUnionID := "aOEluA2VmpLRPp4QlIVXTQiEiE"
	msg := "【BI看板·v1.13 反馈通知联调】\n跑哥你好，这是反馈管理通知闭环的链路测试。\n如果你看到这条消息，说明 chatbotToOne 全链路（access_token → UnionId 转 staffId → 主动消息）已通。"

	if err := n.SendText([]string{huPaoUnionID}, msg); err != nil {
		fmt.Println("[FAIL]", err)
		os.Exit(1)
	}
	fmt.Println("[OK] message sent successfully")
	fmt.Println("→ 请跑哥确认钉钉是否收到")
}
