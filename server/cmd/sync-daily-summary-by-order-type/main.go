// sync-daily-summary-by-order-type: 按订单类型拆开的销售货品汇总账.
//
// 吉客云汇总账接口最多支持 3 个 SummaryType 维度. 这里继续使用 1,2,5
// (时间+销售渠道+仓库), 再按 OrderType 分多次拉取, 本地落 trade_order_type 维度.
package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"bi-dashboard/internal/jackyun"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	defaultConfigPath = `C:\Users\Administrator\bi-dashboard\server\config.json`
	targetTable       = "sales_goods_summary_by_order_type"
)

func main() {
	configPath := flag.String("config", defaultConfigPath, "BI config.json 路径")
	dryRun := flag.Bool("dry-run", false, "只拉取并统计, 不建表、不写库")
	orderTypesFlag := flag.String("order-types", "", "逗号分隔订单类型; 空则读 SYNC_ORDER_TYPES 或默认 1,2,7,8,9,10,12")
	flag.Parse()

	unlock := importutil.AcquireLock("sync-daily-summary-by-order-type")
	defer unlock()

	logFile, err := os.OpenFile(`C:\Users\Administrator\bi-dashboard\server\sync-daily-summary-by-order-type.log`, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		log.SetOutput(io.MultiWriter(logFile, os.Stdout))
		defer logFile.Close()
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	orderTypes := parseOrderTypes(*orderTypesFlag)
	if len(orderTypes) == 0 {
		log.Fatal("订单类型列表为空")
	}
	startDate, endDate := resolveDateRange()

	log.Printf("========== 开始按订单类型同步销售货品汇总账 ==========")
	log.Printf("日期范围: %s ~ %s", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	log.Printf("订单类型: %s", strings.Join(orderTypes, ","))
	log.Printf("模式: dryRun=%v", *dryRun)

	client := jackyun.NewClient(cfg.JackYun.AppKey, cfg.JackYun.Secret, cfg.JackYun.APIURL)

	var db *sql.DB
	deptMap := map[string]string{}
	if !*dryRun {
		db, err = sql.Open("mysql", cfg.Database.DSN())
		if err != nil {
			log.Fatalf("连接数据库失败: %v", err)
		}
		defer db.Close()
		db.SetMaxOpenConns(10)

		if err := ensureTargetTable(db); err != nil {
			log.Fatalf("初始化 %s 失败: %v", targetTable, err)
		}
		deptMap = loadDeptMap(db)
		log.Printf("已加载 %d 个渠道映射", len(deptMap))
	}

	totalWritten := 0
	totalFetched := 0
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		dayStr := d.Format("2006-01-02")
		for _, orderType := range orderTypes {
			items, err := fetchOneDayOrderType(client, dayStr, orderType)
			if err != nil {
				log.Printf("[%s type=%s] 拉取失败, 保留旧数据不动: %v", dayStr, orderType, err)
				continue
			}
			totalFetched += len(items)
			if *dryRun {
				log.Printf("[%s type=%s] dry-run 拉到 %d 条", dayStr, orderType, len(items))
				continue
			}
			written, deleted, err := replaceOneDayOrderType(db, dayStr, orderType, items, deptMap)
			if err != nil {
				log.Printf("[%s type=%s] 写入失败, 已回滚: %v", dayStr, orderType, err)
				continue
			}
			totalWritten += written
			log.Printf("[%s type=%s] 删除%d条, 新写入%d条", dayStr, orderType, deleted, written)
		}
	}

	log.Printf("同步完成: 拉取%d条, 写入%d条", totalFetched, totalWritten)
	if !*dryRun {
		notifyClearCache(cfg.Webhook.Secret)
	}
	log.Printf("========== 按订单类型同步销售货品汇总账结束 ==========")
}

func parseOrderTypes(flagValue string) []string {
	raw := flagValue
	if raw == "" {
		raw = os.Getenv("SYNC_ORDER_TYPES")
	}
	if raw == "" {
		raw = "1,2,7,8,9,10,12"
	}
	seen := map[string]bool{}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		v := strings.TrimSpace(p)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func resolveDateRange() (time.Time, time.Time) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	end := now.AddDate(0, 0, -1)
	if start.After(end) {
		start = end
	}
	if s := os.Getenv("SYNC_START_DATE"); s != "" {
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			log.Fatalf("SYNC_START_DATE 格式错误(应为 yyyy-MM-dd): %v", err)
		}
		start = d
	}
	if s := os.Getenv("SYNC_END_DATE"); s != "" {
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			log.Fatalf("SYNC_END_DATE 格式错误(应为 yyyy-MM-dd): %v", err)
		}
		end = d
	}
	if start.After(end) {
		log.Fatalf("开始日期不能晚于结束日期: %s > %s", start.Format("2006-01-02"), end.Format("2006-01-02"))
	}
	return start, end
}

func fetchOneDayOrderType(client *jackyun.Client, dayStr, orderType string) ([]jackyun.SalesSummaryItem, error) {
	query := jackyun.SalesSummaryQuery{
		TimeType:          3,
		StartTime:         dayStr,
		EndTime:           dayStr,
		FilterTimeType:    2,
		AssemblyDimension: 1,
		IsSkuStatistic:    1,
		SummaryType:       "1,2,5",
		PageIndex:         0,
		PageSize:          100,
		IsCancelTrade:     "0",
		IsAssembly:        "2",
		OrderType:         orderType,
	}
	var collected []jackyun.SalesSummaryItem
	err := client.FetchSalesSummary(query, func(items []jackyun.SalesSummaryItem) error {
		collected = append(collected, items...)
		return nil
	})
	return collected, err
}

func ensureTargetTable(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS ` + targetTable + ` LIKE sales_goods_summary`); err != nil {
		return err
	}
	if _, err := db.Exec(`ALTER TABLE ` + targetTable + ` COMMENT='销售货品汇总账-按订单类型拆分'`); err != nil {
		return err
	}
	if _, err := db.Exec(`ALTER TABLE ` + targetTable + `
		MODIFY trade_order_type VARCHAR(32) NOT NULL DEFAULT '' COMMENT '订单类型(按吉客云orderType过滤落库)'`); err != nil {
		return err
	}
	if _, err := db.Exec(`ALTER TABLE ` + targetTable + `
		MODIFY trade_order_type_name VARCHAR(64) DEFAULT NULL COMMENT '订单类型名称'`); err != nil {
		return err
	}
	if err := dropIndexIfExists(db, targetTable, "uk_daily_record"); err != nil {
		return err
	}
	if err := addIndexIfMissing(db, targetTable, "uk_daily_record_type",
		`ALTER TABLE `+targetTable+` ADD UNIQUE KEY uk_daily_record_type (stat_date, shop_id, goods_id, warehouse_id, sku_id, trade_order_type)`); err != nil {
		return err
	}
	if err := addIndexIfMissing(db, targetTable, "idx_date_type_goods",
		`ALTER TABLE `+targetTable+` ADD KEY idx_date_type_goods (stat_date, trade_order_type, goods_no)`); err != nil {
		return err
	}
	if err := addIndexIfMissing(db, targetTable, "idx_date_wh_type",
		`ALTER TABLE `+targetTable+` ADD KEY idx_date_wh_type (stat_date, warehouse_name, trade_order_type)`); err != nil {
		return err
	}
	return nil
}

func dropIndexIfExists(db *sql.DB, table, index string) error {
	if !indexExists(db, table, index) {
		return nil
	}
	_, err := db.Exec(`ALTER TABLE ` + table + ` DROP INDEX ` + index)
	return err
}

func addIndexIfMissing(db *sql.DB, table, index, ddl string) error {
	if indexExists(db, table, index) {
		return nil
	}
	_, err := db.Exec(ddl)
	return err
}

func indexExists(db *sql.DB, table, index string) bool {
	var n int
	err := db.QueryRow(`SELECT COUNT(*)
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND INDEX_NAME=?`, table, index).Scan(&n)
	return err == nil && n > 0
}

func replaceOneDayOrderType(db *sql.DB, dayStr, orderType string, items []jackyun.SalesSummaryItem, deptMap map[string]string) (written int, deleted int64, err error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	delRes, err := tx.Exec(`DELETE FROM `+targetTable+` WHERE stat_date=? AND trade_order_type=?`, dayStr, orderType)
	if err != nil {
		return 0, 0, err
	}
	deleted, _ = delRes.RowsAffected()
	for _, item := range items {
		shopID := item.ShopId.String()
		args := summaryInsertArgs(dayStr, orderType, item, deptMap[shopID])
		if _, err = tx.Exec(insertSQL(), args...); err != nil {
			return 0, deleted, fmt.Errorf("写入 %s/%s/type=%s: %w", item.GoodsNo.String(), item.ShopName.String(), orderType, err)
		}
		written++
	}
	if err = tx.Commit(); err != nil {
		return 0, deleted, err
	}
	return written, deleted, nil
}

func insertSQL() string {
	return `INSERT INTO ` + targetTable + `
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
			unique_sku_id=VALUES(unique_sku_id),
			department=VALUES(department)`
}

func summaryInsertArgs(dayStr, orderType string, item jackyun.SalesSummaryItem, dept string) []interface{} {
	storedType := strings.TrimSpace(orderType)
	if storedType == "" {
		storedType = strings.TrimSpace(item.TradeOrderType.String())
	}
	return []interface{}{
		dayStr, item.ShopId.String(), item.ShopName.String(), item.ShopCode.String(),
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
		item.SellerId.String(), item.SellerName.String(), storedType, item.TradeOrderTypeName.String(),
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
	}
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

func notifyClearCache(secret string) {
	if secret == "" {
		return
	}
	req, _ := http.NewRequest("POST", "http://127.0.0.1:8080/api/webhook/clear-cache", nil)
	req.Header.Set("X-Webhook-Secret", secret)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		log.Printf("通知 bi-server 清缓存失败: %v", err)
		return
	}
	resp.Body.Close()
	log.Println("已通知 bi-server 清缓存")
}
