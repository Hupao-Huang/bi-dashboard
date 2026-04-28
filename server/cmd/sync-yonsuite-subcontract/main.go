// sync-yonsuite-subcontract 拉取用友 YonBIP 委外订单到 ys_subcontract_orders
//
// 用法:
//
//	./sync-yonsuite-subcontract                    # 默认拉昨天 ~ 今天
//	./sync-yonsuite-subcontract 2026-01-01 2026-03-31  # 自定义日期范围 (vouchdate)
//
// 数据粒度: 订单行级 (一行 record = 一个委外订单 × 一个成品)
// UK: (id, order_product_id) 重复跑幂等
// 字段: 168 业务字段 (定义见 fields.go, 由 sql/gen_ys_subcontract.py 生成)
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
	pageSize   = 500
	maxRetries = 3
)

func main() {
	cfg, err := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	if cfg.YonSuite.AppKey == "" || cfg.YonSuite.AppSecret == "" || cfg.YonSuite.BaseURL == "" {
		log.Fatalf("config.json 缺少 yonsuite 配置")
	}

	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("DB open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("DB ping: %v", err)
	}

	now := time.Now()
	startDate := now.AddDate(0, 0, -1).Format("2006-01-02")
	endDate := now.Format("2006-01-02")
	if len(os.Args) >= 3 {
		startDate, endDate = os.Args[1], os.Args[2]
	}
	if _, err := time.Parse("2006-01-02", startDate); err != nil {
		log.Fatalf("startDate 格式错: %v", err)
	}
	if _, err := time.Parse("2006-01-02", endDate); err != nil {
		log.Fatalf("endDate 格式错: %v", err)
	}

	log.Printf("拉取范围: %s ~ %s (按天循环, pageSize=%d)", startDate, endDate, pageSize)

	client := yonsuite.NewClient(cfg.YonSuite.AppKey, cfg.YonSuite.AppSecret, cfg.YonSuite.BaseURL)

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
			},
		}

		var resp *yonsuite.PurchaseListResp
		var lastErr error
		for attempt := 1; attempt <= maxRetries; attempt++ {
			resp, lastErr = client.QuerySubcontractList(req)
			if lastErr == nil {
				break
			}
			log.Printf("[%s] page %d 第 %d 次失败: %v", day, pageIndex, attempt, lastErr)
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt) * 2 * time.Second)
			}
		}
		if lastErr != nil {
			log.Printf("[%s] 跳过本天: %v", day, lastErr)
			return dayIns, dayUpd, dayErr + 1
		}

		if len(resp.Data.RecordList) == 0 {
			break
		}

		for _, rec := range resp.Data.RecordList {
			ins, upd, err := upsertRecord(db, rec)
			if err != nil {
				dayErr++
				log.Printf("[%s] upsert 失败 id=%v line=%v: %v", day, rec["id"], rec["OrderProduct_id"], err)
				continue
			}
			if ins {
				dayIns++
			}
			if upd {
				dayUpd++
			}
		}

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

// upsertRecord 单条 record 入库 (动态构建 INSERT, 字段定义在 fields.go 的 ysSubcontractFields)
func upsertRecord(db *sql.DB, rec map[string]interface{}) (bool, bool, error) {
	rawJSON, _ := json.Marshal(rec)

	cols := make([]string, 0, len(ysSubcontractFields)+1)
	placeholders := make([]string, 0, len(ysSubcontractFields)+1)
	args := make([]interface{}, 0, len(ysSubcontractFields)+1)
	updates := make([]string, 0, len(ysSubcontractFields))
	for _, f := range ysSubcontractFields {
		cols = append(cols, f.col)
		placeholders = append(placeholders, "?")
		args = append(args, f.getter(rec, f.key))
		// UK (id, order_product_id) 不进 UPDATE
		if f.col != "id" && f.col != "order_product_id" {
			updates = append(updates, f.col+"=VALUES("+f.col+")")
		}
	}
	cols = append(cols, "raw_json")
	placeholders = append(placeholders, "?")
	args = append(args, nullableJSON(rawJSON))
	updates = append(updates, "raw_json=VALUES(raw_json)")

	sqlStmt := "INSERT INTO ys_subcontract_orders (" + strings.Join(cols, ",") +
		") VALUES (" + strings.Join(placeholders, ",") +
		") ON DUPLICATE KEY UPDATE " + strings.Join(updates, ",")

	res, err := db.Exec(sqlStmt, args...)
	if err != nil {
		return false, false, err
	}
	affected, _ := res.RowsAffected()
	return affected == 1, affected == 2, nil
}

// ============ 取值 helper (跟 sync-yonsuite-purchase 一致) ============

type ysField struct {
	col    string
	key    string
	getter func(map[string]interface{}, string) interface{}
}

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
	case json.Number:
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
	case json.Number:
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

func getTime(m map[string]interface{}, k string) interface{} {
	v, ok := m[k]
	if !ok || v == nil {
		return nil
	}
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return nil
	}
	formats := []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.Format("2006-01-02 15:04:05")
		}
	}
	return nil
}

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

func nullableJSON(b []byte) interface{} {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	return string(b)
}
