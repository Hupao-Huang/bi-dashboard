package main

import (
	"bi-dashboard/internal/importutil"
	"bi-dashboard/internal/jackyun"
	"encoding/json"
	"fmt"
	"log"
)

func main() {
	unlock := importutil.AcquireLock("fetch-channels")
	defer unlock()

	client := jackyun.NewClient(
		"56462534",
		"53aa471225b64ea28d0b8809c2555b38",
		"https://open.jackyun.com/open/openapi/do",
	)

	fmt.Println("正在拉取销售渠道数据...")

	var allChannels []jackyun.Channel
	err := client.FetchChannels(func(channels []jackyun.Channel) error {
		allChannels = append(allChannels, channels...)
		fmt.Printf("  已获取 %d 条\n", len(allChannels))
		return nil
	})
	if err != nil {
		log.Fatalf("拉取失败: %v", err)
	}

	fmt.Printf("\n共获取 %d 个渠道\n\n", len(allChannels))

	// 打印每个渠道的关键信息
	fmt.Printf("%-20s %-30s %-10s %-15s %-15s %-20s\n",
		"渠道ID", "渠道名称", "渠道类型", "渠道分类", "负责部门", "平台")
	fmt.Println("----------------------------------------------------------------------------------------------------------------------")

	for _, ch := range allChannels {
		typeName := jackyun.ChannelTypeName[ch.ChannelType.String()]
		if typeName == "" {
			typeName = ch.ChannelType.String()
		}
		fmt.Printf("%-20s %-30s %-10s %-15s %-15s %-20s\n",
			ch.ChannelId.String(),
			truncate(ch.ChannelName, 28),
			typeName,
			truncate(ch.CateName, 13),
			truncate(ch.ChannelDepartName, 13),
			truncate(ch.OnlinePlatTypeName, 18),
		)
	}

	// 也输出 JSON 方便查看
	data, _ := json.MarshalIndent(allChannels, "", "  ")
	fmt.Printf("\n\n=== JSON 完整数据 ===\n%s\n", string(data))
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + ".."
	}
	return s
}
