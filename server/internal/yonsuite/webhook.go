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
// v1.72.0: 加 3 次重试, 防 bi-server 短暂繁忙导致 UI 看 30min 旧数据
func ClearBIServerCache(secret string) {
	url := "http://127.0.0.1:8080/api/webhook/clear-cache"
	client := &http.Client{Timeout: 10 * time.Second}
	const maxRetries = 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest("POST", url, bytes.NewReader([]byte("{}")))
		if err != nil {
			log.Printf("[clear-cache] 构造请求失败 (不重试): %v", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Webhook-Secret", secret)
		resp, err := client.Do(req)
		if err != nil {
			if attempt < maxRetries {
				log.Printf("[clear-cache] 第 %d 次失败, 2s 后重试: %v", attempt, err)
				time.Sleep(2 * time.Second)
				continue
			}
			log.Printf("[clear-cache] 重试 %d 次均失败 (bi-server 可能没启动), 放弃: %v", maxRetries, err)
			return
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		// 2xx 即成功, 退出重试
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			log.Printf("[clear-cache] 第 %d 次成功 status=%d body=%s", attempt, resp.StatusCode, string(body))
			return
		}
		if attempt < maxRetries {
			log.Printf("[clear-cache] 第 %d 次 HTTP %d body=%s, 2s 后重试", attempt, resp.StatusCode, string(body))
			time.Sleep(2 * time.Second)
			continue
		}
		log.Printf("[clear-cache] 重试 %d 次均 HTTP %d body=%s, 放弃 (UI 可能看 30min 旧数据)", maxRetries, resp.StatusCode, string(body))
		return
	}
}
