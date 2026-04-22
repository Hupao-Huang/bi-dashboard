package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/jackyun"
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	logFile, err := os.OpenFile(`C:\Users\Administrator\bi-dashboard\server\sync-daily-summary.log`, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}

	log.Println("========== 开始每日汇总帐同步 ==========")

	cfg, err := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)

	client := jackyun.NewClient(cfg.JackYun.AppKey, cfg.JackYun.Secret, cfg.JackYun.APIURL)

	deptMap := loadDeptMap(db)
	log.Printf("已加载 %d 个渠道映射", len(deptMap))

	// 同步范围：当月1号到昨天（先删后拉，保证与吉客云完全一致）
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	yesterday := now.AddDate(0, 0, -1)

	summaryTotal := 0
	totalDeleted := int64(0)
	for d := monthStart; !d.After(yesterday); d = d.AddDate(0, 0, 1) {
		dayStr := d.Format("2006-01-02")

		// 先删除该天旧数据（保证取消/合并的单据不残留）
		delResult, delErr := db.Exec(`DELETE FROM sales_goods_summary WHERE stat_date = ?`, dayStr)
		if delErr != nil {
			log.Printf("删除 %s 旧数据失败: %v", dayStr, delErr)
			continue
		}
		delCount, _ := delResult.RowsAffected()
		totalDeleted += delCount

		query := jackyun.SalesSummaryQuery{
			TimeType: 3, StartTime: dayStr, EndTime: dayStr,
			FilterTimeType: 2, AssemblyDimension: 1, IsSkuStatistic: 1,
			SummaryType: "1,2,5", PageIndex: 0, PageSize: 50,
			IsCancelTrade: "0", IsAssembly: "2",
		}

		dayCount := 0
		err = client.FetchSalesSummary(query, func(items []jackyun.SalesSummaryItem) error {
			for _, item := range items {
				shopId := item.ShopId.String()
				dept := deptMap[shopId]
				_, err := db.Exec(`INSERT INTO sales_goods_summary
					(stat_date, shop_id, shop_name, shop_code, warehouse_id, warehouse_name, warehouse_code,
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
					VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
					ON DUPLICATE KEY UPDATE
						goods_qty=VALUES(goods_qty), goods_amt=VALUES(goods_amt),
						local_goods_amt=VALUES(local_goods_amt), goods_cost=VALUES(goods_cost),
						gross_profit=VALUES(gross_profit), gross_profit_rate=VALUES(gross_profit_rate),
						tax_gross_profit=VALUES(tax_gross_profit), tax_gross_profit_rate=VALUES(tax_gross_profit_rate),
						tax_unit_price=VALUES(tax_unit_price), fixed_cost=VALUES(fixed_cost),
						retail_price=VALUES(retail_price), so_qty=VALUES(so_qty),
						avg_price=VALUES(avg_price), sell_total=VALUES(sell_total),
						share_expense=VALUES(share_expense), tax_fee=VALUES(tax_fee), tax_amt=VALUES(tax_amt),
						seller_id=VALUES(seller_id), seller_name=VALUES(seller_name),
						trade_order_type=VALUES(trade_order_type), trade_order_type_name=VALUES(trade_order_type_name),
						local_share_expense=VALUES(local_share_expense), local_tax_fee=VALUES(local_tax_fee),
						estimate_weight=VALUES(estimate_weight),
						goods_name_en=VALUES(goods_name_en),
						cate_full_name=VALUES(cate_full_name), color_name=VALUES(color_name),
						size_name=VALUES(size_name), goods_alias=VALUES(goods_alias),
						material_name=VALUES(material_name), main_barcode=VALUES(main_barcode),
						img_url=VALUES(img_url), sku_no=VALUES(sku_no),
						sku_gmt_create=VALUES(sku_gmt_create), goods_gmt_create=VALUES(goods_gmt_create),
						shop_cate_name=VALUES(shop_cate_name), shop_company_code=VALUES(shop_company_code),
						currency_name=VALUES(currency_name),
						goods_extend_map=VALUES(goods_extend_map), price_extend_map=VALUES(price_extend_map),
						sku_extend_map=VALUES(sku_extend_map), assist_info=VALUES(assist_info),
						goods_flag_data=VALUES(goods_flag_data), default_vend_name=VALUES(default_vend_name),
						default_vend_id=VALUES(default_vend_id), unique_id=VALUES(unique_id),
						unique_sku_id=VALUES(unique_sku_id)`,
					dayStr, shopId, item.ShopName.String(), item.ShopCode.String(),
					item.WarehouseId.String(), item.WarehouseName.String(), item.WarehouseCode.String(),
					item.GoodsId.String(), item.GoodsNo.String(), item.GoodsName.String(),
					item.GoodsNameEn.String(), item.BrandName.String(), item.CateName.String(),
					item.SkuId.String(), item.SkuName.String(), item.SkuBarcode.String(),
					item.Unit.String(), item.ChargeCurrencyCode.String(),
					item.GoodsQty.Float64(), item.GoodsAmt.Float64(), item.LocalCurrencyGoodsAmt.Float64(),
					item.GoodsCost.Float64(), item.TaxFee.Float64(), item.TaxAmt.Float64(),
					item.GrossProfit.Float64(), item.GrossProfitRate.Float64(),
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
					dept)
				if err != nil {
					log.Printf("汇总写入失败: %v", err)
				}
				dayCount++
			}
			return nil
		})
		if err != nil {
			log.Printf("汇总 %s 同步失败: %v", dayStr, err)
		} else {
			log.Printf("汇总 %s: 删除%d条, 新写入%d条", dayStr, delCount, dayCount)
		}
		summaryTotal += dayCount
	}
	// 清零"社媒-抖音-飞瓜"渠道的销售数据（达人样品发货，不计入销售）
	if res, err := db.Exec(`UPDATE sales_goods_summary SET
		goods_qty=0, goods_amt=0, local_goods_amt=0, goods_cost=0,
		tax_fee=0, tax_amt=0, gross_profit=0, gross_profit_rate=0,
		tax_gross_profit=0, tax_gross_profit_rate=0,
		sell_total=0, share_expense=0, local_share_expense=0, local_tax_fee=0,
		fixed_cost=0, so_qty=0, avg_price=0
		WHERE shop_id='2395831916807980288' AND stat_date BETWEEN ? AND ?`,
		monthStart.Format("2006-01-02"), yesterday.Format("2006-01-02")); err == nil {
		n, _ := res.RowsAffected()
		if n > 0 {
			log.Printf("已清零 社媒-抖音-飞瓜 渠道 %d 条记录的销售数据", n)
		}
	}

	log.Printf("汇总同步完成(当月%s至%s): 共删除%d条, 新写入%d条",
		monthStart.Format("2006-01-02"), yesterday.Format("2006-01-02"),
		totalDeleted, summaryTotal)

	// 通知 bi-server 清空缓存（让用户立即看到最新数据）
	if cfg.Webhook.Secret != "" {
		req, _ := http.NewRequest("POST", "http://127.0.0.1:8080/api/webhook/clear-cache", nil)
		req.Header.Set("X-Webhook-Secret", cfg.Webhook.Secret)
		if resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req); err == nil {
			resp.Body.Close()
			log.Println("已通知 bi-server 清缓存")
		} else {
			log.Printf("通知清缓存失败: %v", err)
		}
	}

	log.Println("========== 每日汇总帐同步结束 ==========")
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
