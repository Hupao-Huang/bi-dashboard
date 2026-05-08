// sync-yonsuite-stock 拉 YS 现存量到 ys_stock (空 body 全量, 翻页)
//
// 业务: YS 是包材/原料库存的金标准 (吉客云只同步成品+少部分包材)
// 用法: ./sync-yonsuite-stock  (全量)
// UK: (id) 单据行级 (product × warehouse × batchno × stockStatus)
package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"bi-dashboard/internal/yonsuite"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
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
	unlock := importutil.AcquireLock("sync-yonsuite-stock")
	defer unlock()

	cfg, err := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("DB open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("DB ping: %v", err)
	}

	client := yonsuite.NewClient(cfg.YonSuite.AppKey, cfg.YonSuite.AppSecret, cfg.YonSuite.BaseURL)

	totalIns, totalUpd, totalErr := 0, 0, 0
	page := 1
	for {
		req := &yonsuite.StockListReq{PageIndex: page, PageSize: pageSize}
		var resp *yonsuite.StockListResp
		var lastErr error
		for attempt := 1; attempt <= maxRetries; attempt++ {
			resp, lastErr = client.QueryStockList(req)
			if lastErr == nil {
				break
			}
			log.Printf("page %d 第 %d 次失败: %v", page, attempt, lastErr)
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt) * 2 * time.Second)
			}
		}
		if lastErr != nil {
			log.Fatalf("page %d 重试失败: %v", page, lastErr)
		}
		if len(resp.Data) == 0 {
			break
		}
		for _, rec := range resp.Data {
			ins, upd, err := upsertRecord(db, rec)
			if err != nil {
				totalErr++
				log.Printf("upsert 失败 id=%v: %v", rec["id"], err)
				continue
			}
			if ins {
				totalIns++
			}
			if upd {
				totalUpd++
			}
		}
		log.Printf("page %d: 拿到 %d 条 (累计 +%d 更新 %d 失败 %d)", page, len(resp.Data), totalIns, totalUpd, totalErr)
		if len(resp.Data) < pageSize {
			break
		}
		page++
	}
	log.Printf("\n========== 完成 ==========")
	log.Printf("新增 %d / 更新 %d / 失败 %d", totalIns, totalUpd, totalErr)
	yonsuite.ClearBIServerCache(cfg.Webhook.Secret)
}

func upsertRecord(db *sql.DB, rec map[string]interface{}) (bool, bool, error) {
	rawJSON, _ := json.Marshal(rec)
	cols := make([]string, 0, len(ysStockFields)+1)
	placeholders := make([]string, 0, len(ysStockFields)+1)
	args := make([]interface{}, 0, len(ysStockFields)+1)
	updates := make([]string, 0, len(ysStockFields))
	for _, f := range ysStockFields {
		cols = append(cols, f.col)
		placeholders = append(placeholders, "?")
		args = append(args, f.getter(rec, f.key))
		if f.col != "id" {
			updates = append(updates, f.col+"=VALUES("+f.col+")")
		}
	}
	cols = append(cols, "raw_json")
	placeholders = append(placeholders, "?")
	args = append(args, nullableJSON(rawJSON))
	updates = append(updates, "raw_json=VALUES(raw_json)")
	sqlStmt := "INSERT INTO ys_stock (" + strings.Join(cols, ",") +
		") VALUES (" + strings.Join(placeholders, ",") +
		") ON DUPLICATE KEY UPDATE " + strings.Join(updates, ",")
	res, err := db.Exec(sqlStmt, args...)
	if err != nil {
		return false, false, err
	}
	affected, _ := res.RowsAffected()
	return affected == 1, affected == 2, nil
}

// === helpers (跟其他 sync 一致) ===

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
