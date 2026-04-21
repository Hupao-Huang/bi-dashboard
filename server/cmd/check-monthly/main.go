package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/jackyun"
	"fmt"
	"log"
)

func main() {
	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	client := jackyun.NewClient(cfg.JackYun.AppKey, cfg.JackYun.Secret, cfg.JackYun.APIURL)

	var totalQty, totalAmt, totalSell float64
	var totalRows int

	query := jackyun.SalesSummaryQuery{
		TimeType:          2,
		StartTime:         "2026-03",
		EndTime:           "2026-03",
		FilterTimeType:    2,
		AssemblyDimension: 1,
		IsSkuStatistic:    1,
		SummaryType:       "1,2,5",
		PageIndex:         0,
		PageSize:          50,
		IsCancelTrade:     "0",
		IsAssembly:        "2",
	}

	err = client.FetchSalesSummary(query, func(items []jackyun.SalesSummaryItem) error {
		for _, item := range items {
			totalQty += item.GoodsQty.Float64()
			totalAmt += item.LocalCurrencyGoodsAmt.Float64()
			totalSell += item.SellTotal.Float64()
			totalRows++
		}
		return nil
	})
	if err != nil {
		log.Fatalf("失败: %v", err)
	}

	fmt.Printf("按月统计 2026年3月:\n")
	fmt.Printf("  行数:     %d\n", totalRows)
	fmt.Printf("  数量:     %.2f\n", totalQty)
	fmt.Printf("  销售额:   %.2f\n", totalAmt)
	fmt.Printf("  货款金额: %.2f\n", totalSell)
}
