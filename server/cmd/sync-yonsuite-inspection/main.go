// sync-yonsuite-inspection 拉取用友 YonBIP 来料检验单到 ys_inspection
//
// 用法:
//
//	./sync-yonsuite-inspection                       # 默认拉最近 3 天 ~ 今天 (覆盖更新, 捕捉晚审批)
//	./sync-yonsuite-inspection 2025-01-01 2026-06-04 # 自定义检验日期范围 (历史全量回灌)
//
// 粒度: 一行 record = 一张检验单(表头). UK=id. 按检验日期 inspectDate 按天循环, 避开 YS 翻页 bug。
// 注意: 来料检验全是安徽香松供货, 此处不屏蔽香松 (屏蔽=数据全没)。
package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
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
	unlock := importutil.AcquireLock("sync-yonsuite-inspection")
	defer unlock()

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
	startDate := now.AddDate(0, 0, -3).Format("2006-01-02") // 默认最近3天(覆盖晚审批)
	endDate := now.Format("2006-01-02")
	if len(os.Args) >= 3 {
		startDate, endDate = os.Args[1], os.Args[2]
	}
	startT, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		log.Fatalf("startDate 格式错误 yyyy-MM-dd: %v", err)
	}
	endT, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		log.Fatalf("endDate 格式错误 yyyy-MM-dd: %v", err)
	}

	log.Printf("拉取来料检验单: %s ~ %s (按检验日期按天循环)", startDate, endDate)
	client := yonsuite.NewClient(cfg.YonSuite.AppKey, cfg.YonSuite.AppSecret, cfg.YonSuite.BaseURL)

	totalIns, totalUpd, totalErr := 0, 0, 0
	for d := startT; !d.After(endT); d = d.AddDate(0, 0, 1) {
		day := d.Format("2006-01-02")
		ins, upd, e := syncOneDay(client, db, day)
		totalIns += ins
		totalUpd += upd
		totalErr += e
	}

	log.Printf("\n========== 完成 ==========")
	log.Printf("新增: %d / 更新: %d / 失败: %d", totalIns, totalUpd, totalErr)
	yonsuite.ClearBIServerCache(cfg.Webhook.Secret)
}

func syncOneDay(client *yonsuite.Client, db *sql.DB, day string) (int, int, int) {
	ins, upd, errCnt := 0, 0, 0
	pageIndex := 1
	for {
		req := &yonsuite.InspectionListReq{
			Billnum:   "qms_incominspectorder_list",
			PageIndex: pageIndex,
			PageSize:  pageSize,
			Simple: &yonsuite.InspectionSimple{
				OpenInspectDateBegin: day,
				OpenInspectDateEnd:   day,
			},
			QueryOrders: []yonsuite.QueryOrder{{Field: "id", Order: "asc"}},
		}

		var resp *yonsuite.PurchaseListResp
		var lastErr error
		for attempt := 1; attempt <= maxRetries; attempt++ {
			resp, lastErr = client.QueryInspectionList(req)
			if lastErr == nil {
				break
			}
			log.Printf("[%s] page %d 第 %d 次失败: %v", day, pageIndex, attempt, lastErr)
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt)*2*time.Second + 1100*time.Millisecond)
			}
		}
		if lastErr != nil {
			log.Printf("[%s] page %d 重试%d次仍失败, 跳过本天: %v", day, pageIndex, maxRetries, lastErr)
			return ins, upd, errCnt + 1
		}
		if len(resp.Data.RecordList) == 0 {
			break
		}

		for _, rec := range resp.Data.RecordList {
			isIns, isUpd, e := upsert(db, rec)
			if e != nil {
				errCnt++
				log.Printf("[%s] upsert 失败 id=%v: %v", day, rec["id"], e)
				continue
			}
			if isIns {
				ins++
			}
			if isUpd {
				upd++
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
	if ins+upd+errCnt > 0 {
		log.Printf("[%s] +%d / 更新%d / 失败%d", day, ins, upd, errCnt)
	}
	return ins, upd, errCnt
}

func upsert(db *sql.DB, rec map[string]interface{}) (bool, bool, error) {
	rawJSON, _ := json.Marshal(rec)
	cols := []string{
		"id", "code", "billtype", "trantype_name", "inspect_date",
		"pk_material_code", "pk_material_name", "pk_batchcode",
		"pk_outsupplier", "pk_outsupplier_name", "pk_org_name",
		"verifystate", "inspect_result", "pk_stockstatus_statusname", "handle_type_name",
		"inspectnum", "qnum", "nqnum", "nnum", "qrate",
		"manufacture_date", "validity_date", "vsourcecode", "source_order_code", "source_bill_type",
		"create_time", "modify_time", "pubts", "raw_json",
	}
	args := []interface{}{
		getInt64(rec, "id"), getStr(rec, "code"), getStr(rec, "billtype"), getStr(rec, "trantype_name"), getTime(rec, "inspectDate"),
		getStr(rec, "pk_material_code"), getStr(rec, "pk_material_name"), getStr(rec, "pk_batchcode"),
		getInt64(rec, "pk_outsupplier"), getStr(rec, "pk_outsupplier_name"), getStr(rec, "pk_org_name"),
		getInt(rec, "verifystate"), getStr(rec, "inspectResult"), getStr(rec, "pk_stockstatus_statusName"), getStr(rec, "handleType_name"),
		getFloat(rec, "inspectnum"), getFloat(rec, "qnum"), getFloat(rec, "nqnum"), getFloat(rec, "nnum"), getFloat(rec, "qrate"),
		getTime(rec, "manufacture_date"), getTime(rec, "validityDate"), getStr(rec, "vsourcecode"),
		getStr(rec, "sourceOrderCode"), getStr(rec, "sourcebilltype"),
		getTime(rec, "createTime"), getTime(rec, "modifyTime"), getStr(rec, "pubts"), nullableJSON(rawJSON),
	}
	placeholders := make([]string, len(cols))
	updates := make([]string, 0, len(cols))
	for i, c := range cols {
		placeholders[i] = "?"
		if c != "id" {
			updates = append(updates, c+"=VALUES("+c+")")
		}
	}
	stmt := "INSERT INTO ys_inspection (" + strings.Join(cols, ",") + ") VALUES (" +
		strings.Join(placeholders, ",") + ") ON DUPLICATE KEY UPDATE " + strings.Join(updates, ",")
	res, err := db.Exec(stmt, args...)
	if err != nil {
		return false, false, err
	}
	aff, _ := res.RowsAffected()
	return aff == 1, aff == 2, nil
}

// ===== map 安全取值 (UseNumber 后数字都是 json.Number) =====

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
	case json.Number:
		return x.String()
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

func getTime(m map[string]interface{}, k string) interface{} {
	v, ok := m[k]
	if !ok || v == nil {
		return nil
	}
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return nil
	}
	for _, f := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"} {
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
