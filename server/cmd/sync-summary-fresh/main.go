package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"bi-dashboard/internal/jackyun"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// 按天全量重拉销售货品汇总账（日表 sales_goods_summary）
// 每天先DELETE旧数据再INSERT，保证和吉客云完全一致
// 用于解决吉客云订单取消/合并后我们库里残留记录的问题
//
// 用法：SYNC_START_DATE=2026-03-01 SYNC_END_DATE=2026-03-31 sync-summary-fresh.exe
func main() {
	unlock := importutil.AcquireLock("sync-summary-fresh")
	defer unlock()

	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(20)

	client := jackyun.NewClient(cfg.JackYun.AppKey, cfg.JackYun.Secret, cfg.JackYun.APIURL)

	deptMap := loadDeptMap(db)
	fmt.Printf("已加载 %d 个渠道的部门映射\n", len(deptMap))

	startStr := os.Getenv("SYNC_START_DATE")
	endStr := os.Getenv("SYNC_END_DATE")
	if startStr == "" || endStr == "" {
		log.Fatalf("必须指定 SYNC_START_DATE 和 SYNC_END_DATE（格式：yyyy-MM-dd）")
	}

	startDate, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		log.Fatalf("开始日期格式错误: %v", err)
	}
	endDate, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		log.Fatalf("结束日期格式错误: %v", err)
	}

	fmt.Printf("按天全量重拉销售货品汇总账: %s ~ %s\n", startStr, endStr)
	fmt.Println("维度: 按日 + 渠道 + 仓库 + 单品")
	fmt.Println("条件: 发货时间, 排除已取消, 非组装, 排除特殊单")
	fmt.Println("策略: 每天先DELETE旧数据再INSERT")
	fmt.Println()

	totalRecords := 0
	totalDeleted := int64(0)
	currentDate := startDate

	for !currentDate.After(endDate) {
		dateStr := currentDate.Format("2006-01-02")
		fmt.Printf("[%s] 处理中...\n", dateStr)

		// 先删除该天旧数据
		delResult, err := db.Exec(`DELETE FROM sales_goods_summary WHERE stat_date = ?`, dateStr)
		if err != nil {
			log.Fatalf("删除 %s 旧数据失败: %v", dateStr, err)
		}
		delCount, _ := delResult.RowsAffected()
		if delCount > 0 {
			fmt.Printf("  删除旧数据: %d 条\n", delCount)
			totalDeleted += delCount
		}

		dayRecords := 0

		query := jackyun.SalesSummaryQuery{
			TimeType:          3,
			StartTime:         dateStr,
			EndTime:           dateStr,
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
				shopId := item.ShopId.String()
				dept := deptMap[shopId]

				_, err := db.Exec(`
					INSERT INTO sales_goods_summary
						(stat_date, shop_id, shop_name, shop_code,
						 warehouse_id, warehouse_name, warehouse_code,
						 goods_id, goods_no, goods_name, goods_name_en, brand_name, cate_name,
						 sku_id, sku_name, sku_barcode, unit, currency_code,
						 goods_qty, goods_amt, local_goods_amt, goods_cost,
						 tax_fee, tax_amt, gross_profit, gross_profit_rate,
						 tax_gross_profit, tax_gross_profit_rate, tax_unit_price,
						 fixed_cost, retail_price, so_qty,
						 avg_price, sell_total, share_expense,
						 seller_id, seller_name, trade_order_type, trade_order_type_name,
						 cate_full_name, color_name, size_name, goods_alias, material_name,
						 main_barcode, img_url, sku_no, sku_gmt_create, goods_gmt_create,
						 shop_cate_name, shop_company_code, currency_name,
						 local_share_expense, local_tax_fee,
						 goods_extend_map, price_extend_map, sku_extend_map, assist_info,
						 goods_flag_data, default_vend_name, estimate_weight,
						 default_vend_id, unique_id, unique_sku_id,
						 department)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
					ON DUPLICATE KEY UPDATE
						goods_qty=VALUES(goods_qty), goods_amt=VALUES(goods_amt),
						local_goods_amt=VALUES(local_goods_amt), goods_cost=VALUES(goods_cost),
						gross_profit=VALUES(gross_profit), gross_profit_rate=VALUES(gross_profit_rate),
						tax_gross_profit=VALUES(tax_gross_profit), tax_gross_profit_rate=VALUES(tax_gross_profit_rate),
						tax_unit_price=VALUES(tax_unit_price), fixed_cost=VALUES(fixed_cost),
						retail_price=VALUES(retail_price), so_qty=VALUES(so_qty),
						avg_price=VALUES(avg_price), sell_total=VALUES(sell_total),
						share_expense=VALUES(share_expense), tax_fee=VALUES(tax_fee), tax_amt=VALUES(tax_amt),
						local_share_expense=VALUES(local_share_expense), local_tax_fee=VALUES(local_tax_fee)`,
					dateStr, shopId, item.ShopName.String(), item.ShopCode.String(),
					item.WarehouseId.String(), item.WarehouseName.String(), item.WarehouseCode.String(),
					item.GoodsId.String(), item.GoodsNo.String(), item.GoodsName.String(),
					item.GoodsNameEn.String(), item.BrandName.String(), item.CateName.String(),
					item.SkuId.String(), item.SkuName.String(), item.SkuBarcode.String(),
					item.Unit.String(), item.ChargeCurrencyCode.String(),
					item.GoodsQty.Float64(), item.GoodsAmt.Float64(), item.LocalCurrencyGoodsAmt.Float64(),
					item.GoodsCost.Float64(),
					item.TaxFee.Float64(), item.TaxAmt.Float64(), item.GrossProfit.Float64(),
					item.GrossProfitRate.Float64(),
					item.TaxGrossProfit.Float64(), item.TaxGrossProfitRate.Float64(),
					item.TaxUnitPrice.Float64(),
					item.FixedCost.Float64(), item.RetailPrice.Float64(), item.SoQty.Float64(),
					item.AvgPrice.Float64(), item.SellTotal.Float64(), item.ShareSalesExpense.Float64(),
					item.SellerId.String(), item.SellerName.String(),
					item.TradeOrderType.String(), item.TradeOrderTypeName.String(),
					item.CateFullName.String(), item.ColorName.String(), item.SizeName.String(),
					item.GoodsAlias.String(), item.MaterialName.String(),
					item.MainBarcode.String(), item.ImgUrl.String(), item.SkuNo.String(),
					item.SkuGmtCreate.String(), item.GoodsGmtCreate.String(),
					item.ShopCateName.String(), item.ShopCompanyCode.String(), item.CurrencyName.String(),
					item.LocalCurrencyShareSalesExpense.Float64(), item.LocalCurrencyTaxFee.Float64(),
					item.GoodsExtendMap.String(), item.PriceExtendMap.String(),
					item.SkuExtendMap.String(), item.AssistInfo.String(),
					item.GoodsFlagData.String(), item.DefaultVendName.String(),
					item.EstimateWeight.Float64(),
					item.DefaultVendId.String(), item.UniqueId.String(), item.UniqueSkuId.String(),
					dept,
				)
				if err != nil {
					return fmt.Errorf("写入 %s/%s 失败: %w", item.GoodsNo.String(), item.ShopName.String(), err)
				}
				dayRecords++
			}
			return nil
		})

		if err != nil {
			fmt.Printf("  失败: %s\n", err.Error())
		} else {
			fmt.Printf("  完成 (%d 条)\n", dayRecords)
		}

		totalRecords += dayRecords
		currentDate = currentDate.Add(24 * time.Hour)
		time.Sleep(300 * time.Millisecond)
	}

	// 清零"社媒-抖音-飞瓜"渠道的销售数据（达人样品发货，不计入销售）
	if res, err := db.Exec(`UPDATE sales_goods_summary SET
		goods_qty=0, goods_amt=0, local_goods_amt=0, goods_cost=0,
		tax_fee=0, tax_amt=0, gross_profit=0, gross_profit_rate=0,
		tax_gross_profit=0, tax_gross_profit_rate=0,
		sell_total=0, share_expense=0, local_share_expense=0, local_tax_fee=0,
		fixed_cost=0, so_qty=0, avg_price=0
		WHERE shop_id='2395831916807980288' AND stat_date BETWEEN ? AND ?`,
		startStr, endStr); err == nil {
		n, _ := res.RowsAffected()
		if n > 0 {
			fmt.Printf("已清零 社媒-抖音-飞瓜 渠道 %d 条记录的销售数据\n", n)
		}
	}

	fmt.Printf("\n重拉完成！共删除 %d 条旧数据，新写入 %d 条\n", totalDeleted, totalRecords)
}

func loadDeptMap(db *sql.DB) map[string]string {
	m := make(map[string]string)
	rows, err := db.Query("SELECT channel_id, department FROM sales_channel WHERE department IS NOT NULL AND department != ''")
	if err != nil {
		return m
	}
	defer rows.Close()
	for rows.Next() {
		var id, dept string
		rows.Scan(&id, &dept)
		m[id] = dept
	}
	return m
}
