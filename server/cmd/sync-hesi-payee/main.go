// sync-hesi-payee: 同步合思收款方信息到 hesi_payee_info 表
// 调合思 OpenAPI /api/openapi/v2.1/payeeInfos 分页拉所有 (~2120 条),
// REPLACE INTO hesi_payee_info. 用于规则 3 (收款必须为银行账户)。
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const hesiAPIBase = "https://app.ekuaibao.com"

type config struct {
	Database struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		User     string `json:"user"`
		Password string `json:"password"`
		DBName   string `json:"dbname"`
	} `json:"database"`
	Hesi struct {
		AppKey string `json:"appkey"`
		Secret string `json:"secret"`
	} `json:"hesi"`
}

type payeeInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Sort     string `json:"sort"`
	Type     string `json:"type"`
	CardNo   string `json:"cardNo"`
	Bank     string `json:"bank"`
	StaffID  string `json:"staffId"`
	Active   bool   `json:"active"`
}

func main() {
	configPath := flag.String("config", "config.json", "config file path")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	token, err := getToken(cfg.Hesi.AppKey, cfg.Hesi.Secret)
	if err != nil {
		log.Fatalf("get token: %v", err)
	}
	log.Printf("got hesi token (len=%d)", len(token))

	all, err := fetchAllPayees(token)
	if err != nil {
		log.Fatalf("fetch payees: %v", err)
	}
	log.Printf("拉到 %d 条收款方信息", len(all))

	stats := countBySort(all)
	for s, n := range stats {
		log.Printf("  sort=%s -> %d", s, n)
	}

	db, err := openDB(cfg)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	n, err := upsertPayees(db, all)
	if err != nil {
		log.Fatalf("upsert: %v", err)
	}
	log.Printf("✅ 同步 %d 条收款方到 hesi_payee_info", n)
}

func loadConfig(path string) (*config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var c config
	if err := json.NewDecoder(f).Decode(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

func openDB(c *config) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true",
		c.Database.User, c.Database.Password, c.Database.Host, c.Database.Port, c.Database.DBName)
	return sql.Open("mysql", dsn)
}

func getToken(appKey, secret string) (string, error) {
	body := map[string]string{"appKey": appKey, "appSecurity": secret}
	b, _ := json.Marshal(body)
	resp, err := http.Post(hesiAPIBase+"/api/openapi/v1/auth/getAccessToken",
		"application/json", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("token http %d: %s", resp.StatusCode, string(data))
	}
	var parsed struct {
		Value struct {
			AccessToken string `json:"accessToken"`
		} `json:"value"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	if parsed.Value.AccessToken == "" {
		return "", fmt.Errorf("empty accessToken")
	}
	return parsed.Value.AccessToken, nil
}

// fetchAllPayees 分页拉所有 payee, count=1000/页
func fetchAllPayees(token string) ([]payeeInfo, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	var all []payeeInfo
	start := 0
	const pageSize = 1000
	for {
		url := fmt.Sprintf("%s/api/openapi/v2.1/payeeInfos?accessToken=%s&start=%d&count=%d",
			hesiAPIBase, token, start, pageSize)
		resp, err := client.Get(url)
		if err != nil {
			return nil, err
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("payeeInfos http %d: %s", resp.StatusCode, string(data[:min(200, len(data))]))
		}
		var parsed struct {
			Count int         `json:"count"`
			Items []payeeInfo `json:"items"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, err
		}
		all = append(all, parsed.Items...)
		log.Printf("  page start=%d 拉 %d 条 (total %d)", start, len(parsed.Items), parsed.Count)
		if len(parsed.Items) < pageSize {
			break
		}
		start += pageSize
	}
	return all, nil
}

func countBySort(items []payeeInfo) map[string]int {
	m := make(map[string]int)
	for _, it := range items {
		m[it.Sort]++
	}
	return m
}

func upsertPayees(db *sql.DB, items []payeeInfo) (int, error) {
	stmt, err := db.Prepare(`INSERT INTO hesi_payee_info (id, name, sort, type, card_no, bank, staff_id, active, gmt_sync)
	                          VALUES (?, ?, ?, ?, ?, ?, ?, ?, NOW())
	                          ON DUPLICATE KEY UPDATE name=VALUES(name), sort=VALUES(sort), type=VALUES(type),
	                          card_no=VALUES(card_no), bank=VALUES(bank), staff_id=VALUES(staff_id),
	                          active=VALUES(active), gmt_sync=NOW()`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	n := 0
	for _, it := range items {
		active := 0
		if it.Active {
			active = 1
		}
		if _, err := stmt.Exec(it.ID, it.Name, it.Sort, it.Type, it.CardNo, it.Bank, it.StaffID, active); err != nil {
			return n, fmt.Errorf("upsert %s: %w", it.ID, err)
		}
		n++
	}
	return n, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
