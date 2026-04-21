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

	for _, skuMode := range []int{1, 2} {
		label := "区分规格"
		if skuMode == 2 {
			label = "按商品汇总"
		}

		var totalQty, totalAmt, totalSell float64
		var totalRows int

		query := jackyun.SalesSummaryQuery{
			TimeType:          3,
			StartTime:         "2026-03-01",
			EndTime:           "2026-03-01",
			FilterTimeType:    2,
			AssemblyDimension: 1,
			IsSkuStatistic:    skuMode,
			SummaryType:       "1,2,5",
			PageIndex:         0,
			PageSize:          50,
			IsCancelTrade:     "0",
			IsAssembly:        "2",
		}

		err := client.FetchSalesSummary(query, func(items []jackyun.SalesSummaryItem) error {
			for _, item := range items {
				totalQty += item.GoodsQty.Float64()
				totalAmt += item.LocalCurrencyGoodsAmt.Float64()
				totalSell += item.SellTotal.Float64()
				totalRows++
			}
			return nil
		})
		if err != nil {
			log.Printf("%s 失败: %v", label, err)
			continue
		}

		fmt.Printf("[%s] 行数=%d  数量=%.2f  销售额=%.2f  货款金额=%.2f\n",
			label, totalRows, totalQty, totalAmt, totalSell)
	}
}
