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
// 字段定义在 fields.go 的 ysPurchaseFields, 修改字段:
//  1. ALTER TABLE 加列  2. 重跑 sql/gen_ys_go_fields.py  3. go build
func upsertRecord(db *sql.DB, rec map[string]interface{}) (bool, bool, error) {
	rawJSON, _ := json.Marshal(rec)
	var headItemJSON []byte
	if hi, ok := rec["headItem"]; ok && hi != nil {
		headItemJSON, _ = json.Marshal(hi)
	}

	cols := make([]string, 0, len(ysPurchaseFields)+2)
	placeholders := make([]string, 0, len(ysPurchaseFields)+2)
	args := make([]interface{}, 0, len(ysPurchaseFields)+2)
	updates := make([]string, 0, len(ysPurchaseFields))
	for _, f := range ysPurchaseFields {
		cols = append(cols, f.col)
		placeholders = append(placeholders, "?")
		args = append(args, f.getter(rec, f.key))
		// UK (id, purchase_orders_id) 不进 UPDATE 子句
		if f.col != "id" && f.col != "purchase_orders_id" {
			updates = append(updates, f.col+"=VALUES("+f.col+")")
		}
	}
	cols = append(cols, "head_item_json", "raw_json")
	placeholders = append(placeholders, "?", "?")
	args = append(args, nullableJSON(headItemJSON), nullableJSON(rawJSON))
	updates = append(updates, "head_item_json=VALUES(head_item_json)", "raw_json=VALUES(raw_json)")

	sqlStmt := "INSERT INTO ys_purchase_orders (" + strings.Join(cols, ",") +
		") VALUES (" + strings.Join(placeholders, ",") +
		") ON DUPLICATE KEY UPDATE " + strings.Join(updates, ",")

	res, err := db.Exec(sqlStmt, args...)
	if err != nil {
		return false, false, err
	}
	affected, _ := res.RowsAffected()
	// MySQL: insert=1, update=2 (ON DUPLICATE KEY UPDATE 实际改了行)
	return affected == 1, affected == 2, nil
}

// getJSON 把 dict/list/嵌套结构序列化为 JSON 字符串 (用于 JSON 列)
func getJSON(m map[string]interface{}, k string) interface{} {
	v, ok := m[k]
	if !ok || v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil || len(b) == 0 || string(b) == "null" {
		return nil
	}
	return string(b)
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
