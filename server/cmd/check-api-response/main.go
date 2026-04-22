package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/jackyun"
	"encoding/json"
	"fmt"
	"log"
	"os"
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
		PageSize:          50,
		IsCancelTrade:     "0",
		IsAssembly:        "2",
	}

	resp, err := client.Call("birc.report.salesGoodsSummary", query)
	if err != nil {
		log.Fatalf("API调用失败: %v", err)
	}

	// 输出完整响应
	outPath := "C:/Users/Administrator/Desktop/吉客云API_2026-03-01_响应.json"
	f, err := os.Create(outPath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	// 解析 result 并美化输出
	var wrapper struct {
		Data  json.RawMessage `json:"data"`
		Count interface{}     `json:"count"`
	}
	json.Unmarshal(resp.Result, &wrapper)

	var items []map[string]interface{}
	json.Unmarshal(wrapper.Data, &items)

	fullResp := map[string]interface{}{
		"code":      resp.Code,
		"msg":       resp.Msg,
		"subCode":   resp.SubCode,
		"dataCount": len(items),
		"data":      items,
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(fullResp); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("响应已保存到: %s\n", outPath)
	fmt.Printf("响应状态: code=%d, msg=%s\n", resp.Code, resp.Msg)
	fmt.Printf("本页返回 %d 条数据\n", len(items))

	// 控制台打印前2条示例
	if len(items) > 0 {
		fmt.Println("\n=== 第1条数据示例 ===")
		b, _ := json.MarshalIndent(items[0], "", "  ")
		fmt.Println(string(b))
	}
	if len(items) > 1 {
		fmt.Println("\n=== 第2条数据示例 ===")
		b, _ := json.MarshalIndent(items[1], "", "  ")
		fmt.Println(string(b))
	}
}
