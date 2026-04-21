package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/jackyun"
	"encoding/json"
	"fmt"
	"log"
	"sort"
)

func main() {
	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	client := jackyun.NewClient(cfg.JackYun.AppKey, cfg.JackYun.Secret, cfg.JackYun.APIURL)

	query := jackyun.SalesSummaryQuery{
		TimeType:          3,
		StartTime:         "2026-03-01",
		EndTime:           "2026-03-01",
		FilterTimeType:    2,
		AssemblyDimension: 1,
		IsSkuStatistic:    1,
		SummaryType:       "1,2,5",
		PageIndex:         0,
		PageSize:          1,
		IsCancelTrade:     "0",
		IsAssembly:        "2",
	}

	resp, err := client.Call("birc.report.salesGoodsSummary", query)
	if err != nil {
		log.Fatalf("API调用失败: %v", err)
	}

	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	json.Unmarshal(resp.Result, &wrapper)

	var items []map[string]interface{}
	json.Unmarshal(wrapper.Data, &items)

	if len(items) == 0 {
		fmt.Println("无数据")
		return
	}

	item := items[0]
	keys := make([]string, 0, len(item))
	for k := range item {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Printf("共 %d 个字段\n\n", len(keys))
	fmt.Println("=== 金额相关字段 ===")
	for _, k := range keys {
		lo := k
		if contains(lo, "sell", "total", "amt", "local", "cost", "price", "profit", "expense", "fee", "tax") {
			fmt.Printf("  %-45s = %v\n", k, item[k])
		}
	}
	fmt.Println("\n=== 全部字段名 ===")
	for _, k := range keys {
		fmt.Println(k)
	}
}

func contains(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				match := true
				for j := 0; j < len(sub); j++ {
					c := s[i+j]
					if c >= 'A' && c <= 'Z' {
						c += 32
					}
					d := sub[j]
					if d >= 'A' && d <= 'Z' {
						d += 32
					}
					if c != d {
						match = false
						break
					}
				}
				if match {
					return true
				}
			}
		}
	}
	return false
}
