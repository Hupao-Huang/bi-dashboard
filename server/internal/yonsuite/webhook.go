package yonsuite

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"time"
)

// ClearBIServerCache 调 bi-server 的 webhook 清缓存接口, 让 UI 立即看到新数据
// 调用方应在 sync exe 完成所有 upsert 后调一次
// secret 来自 cfg.Webhook.Secret
func ClearBIServerCache(secret string) {
	url := "http://127.0.0.1:8080/api/webhook/clear-cache"
	req, err := http.NewRequest("POST", url, bytes.NewReader([]byte("{}")))
	if err != nil {
		log.Printf("[clear-cache] 构造请求失败: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Secret", secret)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[clear-cache] 调用失败 (bi-server 可能没启动): %v", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	log.Printf("[clear-cache] status=%d body=%s", resp.StatusCode, string(body))
}
