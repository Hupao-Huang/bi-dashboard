package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// 生成月汇总账(sales_goods_summary_monthly)
// 数据源：从日表 sales_goods_summary 聚合生成（不再调吉客云按月API，因为有bug对不上后台）
// 用法：
//   sync-summary-monthly.exe                                          # 默认当月
//   REFRESH_LAST_MONTH=1 sync-summary-monthly.exe                     # 上月(每月7号)
//   SYNC_START_MONTH=2024-04 SYNC_END_MONTH=2025-12 sync-summary-monthly.exe  # 指定范围
func main() {
	unlock := importutil.AcquireLock("sync-summary-monthly")
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
	db.SetMaxOpenConns(5)

	currentMonthStr := time.Now().Format("2006-01")
	lastMonthStr := time.Now().AddDate(0, -1, 0).Format("2006-01")
	defaultMonth := currentMonthStr
	if os.Getenv("REFRESH_LAST_MONTH") == "1" {
		defaultMonth = lastMonthStr
	}
	startStr := os.Getenv("SYNC_START_MONTH")
	if startStr == "" {
		startStr = defaultMonth
	}
	endStr := os.Getenv("SYNC_END_MONTH")
	if endStr == "" {
		endStr = defaultMonth
	}

	startMonth, err := time.Parse("2006-01", startStr)
	if err != nil {
		log.Fatalf("开始月份格式错误(需yyyy-MM): %v", err)
	}
	endMonth, err := time.Parse("2006-01", endStr)
	if err != nil {
		log.Fatalf("结束月份格式错误(需yyyy-MM): %v", err)
	}

	fmt.Printf("月汇总账聚合(从日表): %s ~ %s\n", startStr, endStr)
	fmt.Println()

	totalRecords := int64(0)
	currentMonth := startMonth

	for !currentMonth.After(endMonth) {
		monthStr := currentMonth.Format("2006-01")
		monthStart := currentMonth.Format("2006-01-02")
		// 下月第一天减1天 = 当月最后一天
		monthEnd := currentMonth.AddDate(0, 1, 0).AddDate(0, 0, -1).Format("2006-01-02")

		fmt.Printf("[%s] 处理中...\n", monthStr)
		tStart := time.Now()

		// 先删除该月已有数据
		delRes, err := db.Exec(`DELETE FROM sales_goods_summary_monthly WHERE stat_month = ?`, monthStr)
		if err != nil {
			log.Fatalf("删除 %s 旧数据失败: %v", monthStr, err)
		}
		delCount, _ := delRes.RowsAffected()

		// 从日表聚合插入
		insRes, err := db.Exec(`
			INSERT INTO sales_goods_summary_monthly
			(stat_month, shop_id, shop_name, shop_code,
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
			SELECT
				? as stat_month,
				shop_id, MAX(shop_name), MAX(shop_code),
				warehouse_id, MAX(warehouse_name), MAX(warehouse_code),
				goods_id, MAX(goods_no), MAX(goods_name), MAX(goods_name_en), MAX(brand_name), MAX(cate_name),
				sku_id, MAX(sku_name), MAX(sku_barcode), MAX(unit), MAX(currency_code),
				SUM(goods_qty), SUM(goods_amt), SUM(local_goods_amt), SUM(goods_cost),
				SUM(tax_fee), SUM(tax_amt), SUM(gross_profit),
				CASE WHEN SUM(local_goods_amt)=0 THEN 0 ELSE LEAST(9999.9999, GREATEST(-9999.9999, ROUND(SUM(gross_profit)/SUM(local_goods_amt)*100, 4))) END,
				SUM(tax_gross_profit),
				CASE WHEN SUM(tax_amt)=0 THEN 0 ELSE LEAST(9999.9999, GREATEST(-9999.9999, ROUND(SUM(tax_gross_profit)/SUM(tax_amt)*100, 4))) END,
				MAX(tax_unit_price),
				SUM(fixed_cost), MAX(retail_price), SUM(so_qty),
				CASE WHEN SUM(goods_qty)=0 THEN 0 ELSE ROUND(SUM(local_goods_amt)/SUM(goods_qty), 4) END,
				SUM(sell_total), SUM(share_expense),
				MAX(seller_id), MAX(seller_name), MAX(trade_order_type), MAX(trade_order_type_name),
				MAX(cate_full_name), MAX(color_name), MAX(size_name), MAX(goods_alias), MAX(material_name),
				MAX(main_barcode), MAX(img_url), MAX(sku_no), MAX(sku_gmt_create), MAX(goods_gmt_create),
				MAX(shop_cate_name), MAX(shop_company_code), MAX(currency_name),
				SUM(local_share_expense), SUM(local_tax_fee),
				MAX(goods_extend_map), MAX(price_extend_map), MAX(sku_extend_map), MAX(assist_info),
				MAX(goods_flag_data), MAX(default_vend_name), MAX(estimate_weight),
				MAX(default_vend_id), MAX(unique_id), MAX(unique_sku_id),
				MAX(department)
			FROM sales_goods_summary
			WHERE stat_date BETWEEN ? AND ?
			GROUP BY shop_id, goods_id, warehouse_id, sku_id`,
			monthStr, monthStart, monthEnd)
		if err != nil {
			log.Fatalf("聚合 %s 失败: %v", monthStr, err)
		}
		insCount, _ := insRes.RowsAffected()

		fmt.Printf("  完成: 删除 %d 条, 新写入 %d 条, 耗时 %v\n", delCount, insCount, time.Since(tStart))
		totalRecords += insCount
		currentMonth = currentMonth.AddDate(0, 1, 0)
	}

	fmt.Printf("\n月汇总账聚合完成！共 %d 条\n", totalRecords)
}
