// sync-yonsuite-purchase 拉取用友 YonBIP 采购订单到 ys_purchase_orders
//
// 用法:
//
//	./sync-yonsuite-purchase                    # 默认拉昨天 ~ 今天
//	./sync-yonsuite-purchase 2026-04-21 2026-04-28  # 自定义日期范围 (vouchdate)
//
// 数据粒度: 订单行级 (一行 record = 一个订单 × 一个商品)
// UK: (id, purchase_orders_id) 重复跑幂等
package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/yonsuite"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	// pageSize 每页 500 条 (YS 接口实际单页上限是 500，传更大也只返 500)
	// 关键: 必须按天分批 + pageSize 足以一页拿完, 避免翻页 bug
	// (实测 YS 接口翻页时同 UK 在多页重复返回, page_size=100 翻 7 页只能拿到 18% 真实数据)
	pageSize   = 500
	maxRetries = 3
)

func main() {
	cfg, err := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	if cfg.YonSuite.AppKey == "" || cfg.YonSuite.AppSecret == "" || cfg.YonSuite.BaseURL == "" {
		log.Fatalf("config.json 缺少 yonsuite 配置 (appkey/appsecret/base_url)")
	}

	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("DB open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("DB ping: %v", err)
	}

	// 解析日期范围
	now := time.Now()
	startDate := now.AddDate(0, 0, -1).Format("2006-01-02") // 默认昨天
	endDate := now.Format("2006-01-02")
	if len(os.Args) >= 3 {
		startDate, endDate = os.Args[1], os.Args[2]
	}
	if _, err := time.Parse("2006-01-02", startDate); err != nil {
		log.Fatalf("startDate 格式错误，应为 yyyy-MM-dd: %v", err)
	}
	if _, err := time.Parse("2006-01-02", endDate); err != nil {
		log.Fatalf("endDate 格式错误，应为 yyyy-MM-dd: %v", err)
	}

	log.Printf("拉取范围: %s ~ %s (按天循环, pageSize=%d)", startDate, endDate, pageSize)

	client := yonsuite.NewClient(cfg.YonSuite.AppKey, cfg.YonSuite.AppSecret, cfg.YonSuite.BaseURL)

	// 按天循环 — 避开 YS 接口翻页 bug (多页时同 UK 重复返回, 漏 80%+ 数据)
	// 每天单独 simpleVOs vouchdate between [day 00:00:00, day 23:59:59], pageSize=500 一页拿完
	totalInserted, totalUpdated, totalErrored := 0, 0, 0
	startT, _ := time.Parse("2006-01-02", startDate)
	endT, _ := time.Parse("2006-01-02", endDate)
	for d := startT; !d.After(endT); d = d.AddDate(0, 0, 1) {
		dayStr := d.Format("2006-01-02")
		dayIns, dayUpd, dayErr := syncOneDay(client, db, dayStr)
		totalInserted += dayIns
		totalUpdated += dayUpd
		totalErrored += dayErr
	}

	log.Printf("\n========== 完成 ==========")
	log.Printf("新增: %d / 更新: %d / 失败: %d", totalInserted, totalUpdated, totalErrored)
}

// syncOneDay 拉取单天数据 (vouchdate=day), 翻页直到 < pageSize
// 单天数据通常 < 500 条, 一页拿完; 极端场景 (大批量集中下单) 才会翻页
func syncOneDay(client *yonsuite.Client, db *sql.DB, day string) (int, int, int) {
	dayIns, dayUpd, dayErr := 0, 0, 0
	pageIndex := 1
	for {
		req := &yonsuite.PurchaseListReq{
			PageIndex: pageIndex,
			PageSize:  pageSize,
			IsSum:     false,
			SimpleVOs: []yonsuite.SimpleVO{
				{Field: "vouchdate", Op: "between", Value1: day + " 00:00:00", Value2: day + " 23:59:59"},
			},
			QueryOrders: []yonsuite.QueryOrder{
				{Field: "id", Order: "asc"},
				{Field: "purchaseOrders.id", Order: "asc"},
			},
		}

		var resp *yonsuite.PurchaseListResp
		var lastErr error
		for attempt := 1; attempt <= maxRetries; attempt++ {
			resp, lastErr = client.QueryPurchaseList(req)
			if lastErr == nil {
				break
			}
			log.Printf("[%s] page %d 第 %d 次失败: %v", day, pageIndex, attempt, lastErr)
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt) * 2 * time.Second)
			}
		}
		if lastErr != nil {
			log.Printf("[%s] page %d 重试 %d 次仍失败, 跳过本天: %v", day, pageIndex, maxRetries, lastErr)
			return dayIns, dayUpd, dayErr + 1
		}

		if len(resp.Data.RecordList) == 0 {
			break
		}

		for _, rec := range resp.Data.RecordList {
			ins, upd, err := upsertRecord(db, rec)
			if err != nil {
				dayErr++
				log.Printf("[%s] upsert 失败 id=%v line=%v: %v",
					day, rec["id"], rec["purchaseOrders_id"], err)
				continue
			}
			if ins {
				dayIns++
			}
			if upd {
				dayUpd++
			}
		}

		// 翻页结束条件: 单页 < pageSize 或 已超 pageCount
		if len(resp.Data.RecordList) < pageSize {
			break
		}
		if resp.Data.PageCount > 0 && pageIndex >= resp.Data.PageCount {
			break
		}
		pageIndex++
	}

	if dayIns+dayUpd+dayErr > 0 {
		log.Printf("[%s] 完成: +%d / 更新%d / 失败%d", day, dayIns, dayUpd, dayErr)
	}
	return dayIns, dayUpd, dayErr
}

// upsertRecord 单条 record 入库 (ON DUPLICATE KEY UPDATE 全字段)
// 返回 (inserted, updated, err) — MySQL ROW_COUNT()=1 是 insert，=2 是 update
func upsertRecord(db *sql.DB, rec map[string]interface{}) (bool, bool, error) {
	rawJSON, _ := json.Marshal(rec)

	// headItem 是嵌套 object，单独序列化
	var headItemJSON []byte
	if hi, ok := rec["headItem"]; ok && hi != nil {
		headItemJSON, _ = json.Marshal(hi)
	}

	args := []interface{}{
		getInt64(rec, "id"),
		getStr(rec, "code"),
		getStr(rec, "barCode"),
		getTime(rec, "vouchdate"),
		getStr(rec, "creator"),
		getTime(rec, "createTime"),
		getTime(rec, "pubts"),
		getInt64(rec, "purchaseOrders_id"),

		getInt64(rec, "product"),
		getStr(rec, "product_cCode"),
		getStr(rec, "product_cName"),
		getStr(rec, "product_model"),
		getStr(rec, "product_modelDescription"),
		getStr(rec, "product_defaultAlbumId"),
		getInt(rec, "realProductAttribute"),
		getInt(rec, "realProductAttributeType"),

		getInt64(rec, "vendor"),
		getStr(rec, "vendor_code"),
		getStr(rec, "vendor_name"),
		getInt64(rec, "invoiceVendor"),
		getStr(rec, "invoiceVendor_name"),

		getInt64(rec, "org"),
		getStr(rec, "org_name"),
		getInt64(rec, "demandOrg"),
		getStr(rec, "demandOrg_name"),
		getInt64(rec, "inOrg"),
		getStr(rec, "inOrg_name"),
		getInt64(rec, "inInvoiceOrg"),
		getStr(rec, "inInvoiceOrg_name"),

		getStr(rec, "purchaseOrders_warehouse_code"),

		getInt64(rec, "unit"),
		getStr(rec, "unit_name"),
		getStr(rec, "unit_code"),
		getInt(rec, "unit_Precision"),
		getInt64(rec, "priceUOM"),
		getStr(rec, "priceUOM_Name"),
		getStr(rec, "priceUOM_Code"),
		getInt(rec, "priceUOM_Precision"),
		getInt64(rec, "purchaseOrders_purUOM"),
		getInt(rec, "purUOM_Precision"),
		getFloat(rec, "purchaseOrders_invExchRate"),
		getFloat(rec, "purchaseOrders_invPriceExchRate"),

		getFloat(rec, "qty"),
		getFloat(rec, "subQty"),
		getFloat(rec, "totalQuantity"),
		getFloat(rec, "approvenum"),
		getFloat(rec, "storagenum"),
		getFloat(rec, "totalSendQty"),
		getFloat(rec, "totalRecieveQty"),
		getFloat(rec, "totalInQty"),

		getFloat(rec, "oriUnitPrice"),
		getFloat(rec, "oriTaxUnitPrice"),
		getFloat(rec, "moneysum"),
		getFloat(rec, "oriSum"),
		getFloat(rec, "listOriMoney"),
		getFloat(rec, "listOriSum"),
		getFloat(rec, "listOriTax"),
		getFloat(rec, "listTaxRate"),
		getStr(rec, "listdiscountTaxType"),
		getFloat(rec, "totalInTaxMoney"),
		getFloat(rec, "totalArrivedTaxMoney"),
		getFloat(rec, "listTotalPayApplyAmount"),
		getFloat(rec, "listTotalPayOriMoney"),
		getFloat(rec, "listTotalPayAmount"),
		getFloat(rec, "listTotalPayNATMoney"),

		getStr(rec, "currency"),
		getStr(rec, "currency_code"),
		getStr(rec, "currency_name"),
		getInt(rec, "currency_moneyDigit"),
		getInt(rec, "currency_priceDigit"),
		getStr(rec, "natCurrency"),
		getStr(rec, "natCurrency_code"),
		getInt(rec, "natCurrency_moneyDigit"),
		getInt(rec, "natCurrency_priceDigit"),

		getInt(rec, "status"),
		getInt(rec, "bizstatus"),
		getInt(rec, "modifyStatus"),
		getInt(rec, "receiveStatus"),
		getInt(rec, "purchaseOrders_arrivedStatus"),
		getInt(rec, "purchaseOrders_inWHStatus"),
		getInt(rec, "purchaseOrders_payStatus"),
		getInt(rec, "purchaseOrders_invoiceStatus"),

		getBool(rec, "isFlowCoreBill"),
		getBool(rec, "isWfControlled"),
		getBool(rec, "isContract"),
		getBool(rec, "bEffectStock"),
		getInt64(rec, "bustype"),
		getStr(rec, "bustype_name"),
		getStr(rec, "generalPurchaseOrderType"),
		getStr(rec, "bizFlow"),
		getStr(rec, "bizFlow_name"),
		getStr(rec, "bizFlow_version"),
		getBool(rec, "retailInvestors"),

		getBool(rec, "bmake_st_purinvoice"),
		getBool(rec, "bmake_st_purinvoice_red"),
		getBool(rec, "bmake_st_purinrecord"),
		getBool(rec, "bmake_st_purinrecord_red"),

		nullableJSON(headItemJSON),
		nullableJSON(rawJSON),
	}

	const sqlStmt = `INSERT INTO ys_purchase_orders (
		id, code, bar_code, vouchdate, creator, create_time, pubts, purchase_orders_id,
		product, product_c_code, product_c_name, product_model, product_model_description, product_default_album_id, real_product_attribute, real_product_attribute_type,
		vendor, vendor_code, vendor_name, invoice_vendor, invoice_vendor_name,
		org, org_name, demand_org, demand_org_name, in_org, in_org_name, in_invoice_org, in_invoice_org_name,
		purchase_orders_warehouse_code,
		unit, unit_name, unit_code, unit_precision, price_uom, price_uom_name, price_uom_code, price_uom_precision, purchase_orders_pur_uom, pur_uom_precision, purchase_orders_inv_exch_rate, purchase_orders_inv_price_exch_rate,
		qty, sub_qty, total_quantity, approvenum, storagenum, total_send_qty, total_recieve_qty, total_in_qty,
		ori_unit_price, ori_tax_unit_price, moneysum, ori_sum, list_ori_money, list_ori_sum, list_ori_tax, list_tax_rate, list_discount_tax_type, total_in_tax_money, total_arrived_tax_money, list_total_pay_apply_amount, list_total_pay_ori_money, list_total_pay_amount, list_total_pay_nat_money,
		currency, currency_code, currency_name, currency_money_digit, currency_price_digit, nat_currency, nat_currency_code, nat_currency_money_digit, nat_currency_price_digit,
		status, bizstatus, modify_status, receive_status, purchase_orders_arrived_status, purchase_orders_in_wh_status, purchase_orders_pay_status, purchase_orders_invoice_status,
		is_flow_core_bill, is_wf_controlled, is_contract, b_effect_stock, bustype, bustype_name, general_purchase_order_type, biz_flow, biz_flow_name, biz_flow_version, retail_investors,
		bmake_st_purinvoice, bmake_st_purinvoice_red, bmake_st_purinrecord, bmake_st_purinrecord_red,
		head_item_json, raw_json
	) VALUES (
		?,?,?,?,?,?,?,?,
		?,?,?,?,?,?,?,?,
		?,?,?,?,?,
		?,?,?,?,?,?,?,?,
		?,
		?,?,?,?,?,?,?,?,?,?,?,?,
		?,?,?,?,?,?,?,?,
		?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,
		?,?,?,?,?,?,?,?,?,
		?,?,?,?,?,?,?,?,
		?,?,?,?,?,?,?,?,?,?,?,
		?,?,?,?,
		?,?
	) ON DUPLICATE KEY UPDATE
		code=VALUES(code), bar_code=VALUES(bar_code), vouchdate=VALUES(vouchdate),
		creator=VALUES(creator), create_time=VALUES(create_time), pubts=VALUES(pubts),
		product=VALUES(product), product_c_code=VALUES(product_c_code), product_c_name=VALUES(product_c_name),
		product_model=VALUES(product_model), product_model_description=VALUES(product_model_description),
		product_default_album_id=VALUES(product_default_album_id), real_product_attribute=VALUES(real_product_attribute),
		real_product_attribute_type=VALUES(real_product_attribute_type),
		vendor=VALUES(vendor), vendor_code=VALUES(vendor_code), vendor_name=VALUES(vendor_name),
		invoice_vendor=VALUES(invoice_vendor), invoice_vendor_name=VALUES(invoice_vendor_name),
		org=VALUES(org), org_name=VALUES(org_name), demand_org=VALUES(demand_org), demand_org_name=VALUES(demand_org_name),
		in_org=VALUES(in_org), in_org_name=VALUES(in_org_name), in_invoice_org=VALUES(in_invoice_org), in_invoice_org_name=VALUES(in_invoice_org_name),
		purchase_orders_warehouse_code=VALUES(purchase_orders_warehouse_code),
		unit=VALUES(unit), unit_name=VALUES(unit_name), unit_code=VALUES(unit_code), unit_precision=VALUES(unit_precision),
		price_uom=VALUES(price_uom), price_uom_name=VALUES(price_uom_name), price_uom_code=VALUES(price_uom_code), price_uom_precision=VALUES(price_uom_precision),
		purchase_orders_pur_uom=VALUES(purchase_orders_pur_uom), pur_uom_precision=VALUES(pur_uom_precision),
		purchase_orders_inv_exch_rate=VALUES(purchase_orders_inv_exch_rate), purchase_orders_inv_price_exch_rate=VALUES(purchase_orders_inv_price_exch_rate),
		qty=VALUES(qty), sub_qty=VALUES(sub_qty), total_quantity=VALUES(total_quantity), approvenum=VALUES(approvenum),
		storagenum=VALUES(storagenum), total_send_qty=VALUES(total_send_qty), total_recieve_qty=VALUES(total_recieve_qty), total_in_qty=VALUES(total_in_qty),
		ori_unit_price=VALUES(ori_unit_price), ori_tax_unit_price=VALUES(ori_tax_unit_price), moneysum=VALUES(moneysum), ori_sum=VALUES(ori_sum),
		list_ori_money=VALUES(list_ori_money), list_ori_sum=VALUES(list_ori_sum), list_ori_tax=VALUES(list_ori_tax), list_tax_rate=VALUES(list_tax_rate),
		list_discount_tax_type=VALUES(list_discount_tax_type), total_in_tax_money=VALUES(total_in_tax_money), total_arrived_tax_money=VALUES(total_arrived_tax_money),
		list_total_pay_apply_amount=VALUES(list_total_pay_apply_amount), list_total_pay_ori_money=VALUES(list_total_pay_ori_money),
		list_total_pay_amount=VALUES(list_total_pay_amount), list_total_pay_nat_money=VALUES(list_total_pay_nat_money),
		currency=VALUES(currency), currency_code=VALUES(currency_code), currency_name=VALUES(currency_name),
		currency_money_digit=VALUES(currency_money_digit), currency_price_digit=VALUES(currency_price_digit),
		nat_currency=VALUES(nat_currency), nat_currency_code=VALUES(nat_currency_code),
		nat_currency_money_digit=VALUES(nat_currency_money_digit), nat_currency_price_digit=VALUES(nat_currency_price_digit),
		status=VALUES(status), bizstatus=VALUES(bizstatus), modify_status=VALUES(modify_status), receive_status=VALUES(receive_status),
		purchase_orders_arrived_status=VALUES(purchase_orders_arrived_status), purchase_orders_in_wh_status=VALUES(purchase_orders_in_wh_status),
		purchase_orders_pay_status=VALUES(purchase_orders_pay_status), purchase_orders_invoice_status=VALUES(purchase_orders_invoice_status),
		is_flow_core_bill=VALUES(is_flow_core_bill), is_wf_controlled=VALUES(is_wf_controlled), is_contract=VALUES(is_contract),
		b_effect_stock=VALUES(b_effect_stock), bustype=VALUES(bustype), bustype_name=VALUES(bustype_name),
		general_purchase_order_type=VALUES(general_purchase_order_type), biz_flow=VALUES(biz_flow), biz_flow_name=VALUES(biz_flow_name),
		biz_flow_version=VALUES(biz_flow_version), retail_investors=VALUES(retail_investors),
		bmake_st_purinvoice=VALUES(bmake_st_purinvoice), bmake_st_purinvoice_red=VALUES(bmake_st_purinvoice_red),
		bmake_st_purinrecord=VALUES(bmake_st_purinrecord), bmake_st_purinrecord_red=VALUES(bmake_st_purinrecord_red),
		head_item_json=VALUES(head_item_json), raw_json=VALUES(raw_json)`

	res, err := db.Exec(sqlStmt, args...)
	if err != nil {
		return false, false, err
	}
	affected, _ := res.RowsAffected()
	// MySQL: insert=1, update=2 (ON DUPLICATE KEY UPDATE 实际改了行)
	return affected == 1, affected == 2, nil
}

// ========== map[string]interface{} 安全取值 helper ==========

func getStr(m map[string]interface{}, k string) interface{} {
	v, ok := m[k]
	if !ok || v == nil {
		return nil
	}
	switch x := v.(type) {
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return nil
		}
		return s
	default:
		return fmt.Sprintf("%v", v)
	}
}

func getInt(m map[string]interface{}, k string) interface{} {
	v, ok := m[k]
	if !ok || v == nil {
		return nil
	}
	switch x := v.(type) {
	case json.Number: // UseNumber() 后的所有数字都是这个类型, 必须最先匹配
		if i, err := x.Int64(); err == nil {
			return int(i)
		}
		if f, err := x.Float64(); err == nil {
			return int(f)
		}
		return nil
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return x
	case string:
		if x == "" {
			return nil
		}
		n, err := strconv.Atoi(x)
		if err != nil {
			return nil
		}
		return n
	default:
		return nil
	}
}

func getInt64(m map[string]interface{}, k string) interface{} {
	v, ok := m[k]
	if !ok || v == nil {
		return nil
	}
	switch x := v.(type) {
	case json.Number: // 关键! 19 位 id 必须走 Int64() 不能走 Float64() 否则丢精度撞 UK
		if i, err := x.Int64(); err == nil {
			return i
		}
		return nil
	case float64:
		return int64(x)
	case int:
		return int64(x)
	case int64:
		return x
	case string:
		if x == "" {
			return nil
		}
		n, err := strconv.ParseInt(x, 10, 64)
		if err != nil {
			return nil
		}
		return n
	default:
		return nil
	}
}

func getFloat(m map[string]interface{}, k string) interface{} {
	v, ok := m[k]
	if !ok || v == nil {
		return nil
	}
	switch x := v.(type) {
	case json.Number:
		if f, err := x.Float64(); err == nil {
			return f
		}
		return nil
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		if x == "" {
			return nil
		}
		f, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return nil
		}
		return f
	default:
		return nil
	}
}

func getBool(m map[string]interface{}, k string) interface{} {
	v, ok := m[k]
	if !ok || v == nil {
		return nil
	}
	switch x := v.(type) {
	case bool:
		if x {
			return 1
		}
		return 0
	case string:
		if strings.EqualFold(x, "true") || x == "1" {
			return 1
		}
		if strings.EqualFold(x, "false") || x == "0" || x == "" {
			return 0
		}
		return nil
	case json.Number:
		if i, err := x.Int64(); err == nil {
			if i != 0 {
				return 1
			}
			return 0
		}
		return nil
	case float64:
		if x != 0 {
			return 1
		}
		return 0
	default:
		return nil
	}
}

// getTime 接收 "yyyy-MM-dd HH:mm:ss" 字符串，返回 SQL DATETIME 字符串
func getTime(m map[string]interface{}, k string) interface{} {
	v, ok := m[k]
	if !ok || v == nil {
		return nil
	}
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return nil
	}
	// 接受多种格式
	formats := []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.Format("2006-01-02 15:04:05")
		}
	}
	return nil
}

func nullableJSON(b []byte) interface{} {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	return string(b)
}
