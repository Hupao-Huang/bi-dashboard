// probe-ys-voucher 一次性: 试用友 YS 凭证查询接口路径, 找 apicode
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"bi-dashboard/internal/config"
	"bi-dashboard/internal/yonsuite"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

func tryPath(token, baseURL, path string, body interface{}) {
	b, _ := json.Marshal(body)
	q := url.Values{}
	q.Set("access_token", token)
	full := baseURL + path + "?" + q.Encode()

	req, _ := http.NewRequest("POST", full, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Printf("  ❌ HTTP err: %v\n", err)
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	fmt.Printf("  HTTP %d | bytes=%d\n", resp.StatusCode, len(data))
	// 写文件供 Python 解析
	tmpFile := "C:/Users/Administrator/AppData/Local/Temp/ys_probe.json"
	if err := json.Indent(&bytes.Buffer{}, data, "", "  "); err == nil {
		_ = err
	}
	_ = data
	f, _ := os.Create(tmpFile)
	if f != nil {
		f.Write(data)
		f.Close()
		fmt.Printf("  saved → %s\n", tmpFile)
	}
}

func main() {
	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("load cfg: %v", err)
	}
	c := yonsuite.NewClient(cfg.YonSuite.AppKey, cfg.YonSuite.AppSecret, cfg.YonSuite.BaseURL)
	token, err := c.AccessToken()
	if err != nil {
		log.Fatalf("token: %v", err)
	}
	fmt.Printf("TOKEN ok (len=%d)\n", len(token))

	// 拉全量账簿用于映射
	candidates := []string{
		"/iuap-api-gateway/yonbip/fi/fipub/basedoc/querybd/accbook",
	}

	body := map[string]interface{}{
		"fields":    []string{"code", "name"},
		"pageIndex": 1,
		"pageSize":  1000,
	}

	for _, p := range candidates {
		fmt.Printf("\n--- %s ---\n", p)
		tryPath(token, cfg.YonSuite.BaseURL, p, body)
		time.Sleep(1200 * time.Millisecond) // YS 1.1s 限流
	}
}
