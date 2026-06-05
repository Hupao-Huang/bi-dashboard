// 回填火车票座位等出行信息 + 建列
// 合思发票主体(OCR)已含座位类型/车次/区间/乘车人, 之前没同步, 这里一次性补到 hesi_flow_invoice。
// 用法: cd server && go run ./cmd/backfill-train-seat
package main

import (
	"bi-dashboard/internal/config"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const apiBase = "https://app.ekuaibao.com"

// 要补的列: 列名 -> (类型, 中文注释)
var seatColumns = [][3]string{
	{"seat_type", "VARCHAR(20)", "座位类型(火车:二等座/一等座/商务座等, OCR)"},
	{"train_no", "VARCHAR(20)", "车次(火车票)"},
	{"carriage", "VARCHAR(20)", "车厢(火车票)"},
	{"seat_no", "VARCHAR(30)", "席位(火车票)"},
	{"from_station", "VARCHAR(50)", "上车车站(火车票)"},
	{"to_station", "VARCHAR(50)", "下车车站(火车票)"},
	{"passenger", "VARCHAR(50)", "乘车人姓名(火车票)"},
}

func main() {
	cfg, err := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()

	ensureColumns(db)

	token, err := getToken(cfg.Hesi.AppKey, cfg.Hesi.Secret)
	if err != nil {
		log.Fatalf("授权失败: %v", err)
	}

	// 拉所有火车票 invoice_id
	ids := loadTrainInvoiceIDs(db)
	log.Printf("待回填火车票 %d 张", len(ids))

	client := &http.Client{Timeout: 30 * time.Second}
	updated, withSeat := 0, 0
	for i := 0; i < len(ids); i += 100 {
		end := i + 100
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[i:end]
		items, err := fetchInvoices(client, token, batch)
		if err != nil {
			log.Printf("批 %d-%d 拉取失败: %v", i, end, err)
			continue
		}
		for _, inv := range items {
			id := getStr(inv, "id")
			if id == "" {
				continue
			}
			seat := pick(inv, "_座位类型")
			res, err := db.Exec(`UPDATE hesi_flow_invoice SET
				seat_type=?, train_no=?, carriage=?, seat_no=?, from_station=?, to_station=?, passenger=?
				WHERE invoice_id=?`,
				seat, pick(inv, "_车次"), pick(inv, "_车厢"), pick(inv, "_席位"),
				pick(inv, "_上车车站"), pick(inv, "_下车车站"), pick(inv, "_乘车人姓名"), id)
			if err != nil {
				log.Printf("UPDATE %s 失败: %v", id, err)
				continue
			}
			if n, _ := res.RowsAffected(); n > 0 {
				updated++
			}
			if seat != "" {
				withSeat++
			}
		}
		log.Printf("进度 %d/%d (已更新 %d, 含座位 %d)", end, len(ids), updated, withSeat)
		time.Sleep(300 * time.Millisecond) // 礼貌限速
	}
	log.Printf("===== 完成: 更新 %d 张, 其中 %d 张拿到座位类型 =====", updated, withSeat)
	fmt.Printf("回填完成: 更新 %d / 含座位 %d\n", updated, withSeat)
}

func ensureColumns(db *sql.DB) {
	for _, c := range seatColumns {
		name, typ, comment := c[0], c[1], c[2]
		var cnt int
		_ = db.QueryRow(`SELECT COUNT(*) FROM information_schema.COLUMNS
			WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='hesi_flow_invoice' AND COLUMN_NAME=?`, name).Scan(&cnt)
		if cnt > 0 {
			continue
		}
		stmt := fmt.Sprintf("ALTER TABLE hesi_flow_invoice ADD COLUMN %s %s DEFAULT NULL COMMENT '%s'", name, typ, comment)
		if _, err := db.Exec(stmt); err != nil {
			log.Fatalf("加列 %s 失败: %v", name, err)
		}
		log.Printf("已加列 %s", name)
	}
}

func loadTrainInvoiceIDs(db *sql.DB) []string {
	rows, err := db.Query(`SELECT invoice_id FROM hesi_flow_invoice WHERE invoice_type LIKE '%TRAIN%' AND invoice_id<>''`)
	if err != nil {
		log.Fatalf("查火车票失败: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil && id != "" {
			out = append(out, id)
		}
	}
	return out
}

func getToken(appKey, secret string) (string, error) {
	b, _ := json.Marshal(map[string]string{"appKey": appKey, "appSecurity": secret})
	resp, err := http.Post(apiBase+"/api/openapi/v1/auth/getAccessToken", "application/json", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var r struct {
		Value struct {
			AccessToken string `json:"accessToken"`
		} `json:"value"`
	}
	json.Unmarshal(data, &r)
	if r.Value.AccessToken == "" {
		return "", fmt.Errorf("token 为空: %s", string(data[:min(len(data), 200)]))
	}
	return r.Value.AccessToken, nil
}

func fetchInvoices(client *http.Client, token string, ids []string) ([]map[string]interface{}, error) {
	body, _ := json.Marshal(map[string]interface{}{"ids": ids})
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/openapi/v2/extension/INVOICE/object/invoice/search?accessToken=%s", apiBase, token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var r struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return r.Items, nil
}

func pick(m map[string]interface{}, suffix string) string {
	for k, v := range m {
		if strings.HasSuffix(k, suffix) {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

func getStr(m map[string]interface{}, k string) string {
	if s, ok := m[k].(string); ok {
		return s
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
