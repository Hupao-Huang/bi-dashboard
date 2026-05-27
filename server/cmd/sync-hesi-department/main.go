// sync-hesi-department: 同步合思部门树到 hesi_department 表
// 调合思 OpenAPI /api/openapi/v2/departments 拉所有部门 (511 个左右),
// 计算每个部门 has_child (反向扫 parentId 引用), REPLACE INTO hesi_department.
// 用于规则引擎判定"提交人部门是否末级"。
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

type hesiDept struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parentId"`
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
	if len(token) >= 30 {
		log.Printf("got hesi token: %s...", token[:30])
	} else {
		log.Printf("got hesi token (len=%d)", len(token))
	}

	depts, err := fetchAllDepts(token)
	if err != nil {
		log.Fatalf("fetch depts: %v", err)
	}
	log.Printf("拉到 %d 个部门", len(depts))

	hasChildSet := computeHasChild(depts)
	log.Printf("含子部门 = %d / 末级 = %d", len(hasChildSet), len(depts)-len(hasChildSet))

	db, err := openDB(cfg)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	n, err := upsertDepts(db, depts, hasChildSet)
	if err != nil {
		log.Fatalf("upsert: %v", err)
	}
	log.Printf("✅ 同步 %d 个部门到 hesi_department", n)
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
		return "", fmt.Errorf("empty accessToken: %s", string(data))
	}
	return parsed.Value.AccessToken, nil
}

func fetchAllDepts(token string) ([]hesiDept, error) {
	url := fmt.Sprintf("%s/api/openapi/v2/departments?accessToken=%s&start=0&count=1000",
		hesiAPIBase, token)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("departments http %d: %s", resp.StatusCode, string(data))
	}
	var parsed struct {
		Count int        `json:"count"`
		Items []hesiDept `json:"items"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	if parsed.Count > 1000 {
		log.Printf("⚠️ 部门数 %d > 1000, 需分页 (TODO)", parsed.Count)
	}
	return parsed.Items, nil
}

// computeHasChild 反向扫所有 parentId, 收集"被引用过的 ID 集合" = 含子部门
func computeHasChild(depts []hesiDept) map[string]struct{} {
	set := make(map[string]struct{})
	for _, d := range depts {
		if d.ParentID != "" {
			set[d.ParentID] = struct{}{}
		}
	}
	return set
}

func upsertDepts(db *sql.DB, depts []hesiDept, hasChildSet map[string]struct{}) (int, error) {
	stmt, err := db.Prepare(`INSERT INTO hesi_department (id, name, parent_id, has_child, active, gmt_sync)
	                          VALUES (?, ?, ?, ?, ?, NOW())
	                          ON DUPLICATE KEY UPDATE name=VALUES(name), parent_id=VALUES(parent_id),
	                          has_child=VALUES(has_child), active=VALUES(active), gmt_sync=NOW()`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	n := 0
	for _, d := range depts {
		hasChild := 0
		if _, ok := hasChildSet[d.ID]; ok {
			hasChild = 1
		}
		active := 0
		if d.Active {
			active = 1
		}
		var parent interface{}
		if d.ParentID == "" {
			parent = nil
		} else {
			parent = d.ParentID
		}
		if _, err := stmt.Exec(d.ID, d.Name, parent, hasChild, active); err != nil {
			return n, fmt.Errorf("upsert %s: %w", d.ID, err)
		}
		n++
	}
	return n, nil
}
