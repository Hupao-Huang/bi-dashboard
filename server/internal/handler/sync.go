package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	syncRunning bool
	syncMu      sync.Mutex
	syncLastLog string
	syncLastAt  string
)

const (
	dingWebhook = "https://oapi.dingtalk.com/robot/send"
	toolTimeout = 5 * time.Minute
)

type syncToolResult struct {
	Name         string
	Status       string
	Duration     time.Duration
	Output       string
	OutputDigest string
}

// SyncOps webhook接口：RPA抓完数据后调用，自动导入运营数据
// POST /api/webhook/sync-ops
// 可选参数: {"date":"20260325"} 不传则默认昨天
func (h *DashboardHandler) SyncOps(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, 405, "method not allowed")
		return
	}

	syncMu.Lock()
	if syncRunning {
		syncMu.Unlock()
		writeError(w, 409, "同步正在进行中，请稍后")
		return
	}
	syncRunning = true
	syncMu.Unlock()

	// 解析日期参数
	date := time.Now().AddDate(0, 0, -1).Format("20060102")
	if r.Body != nil {
		var req struct {
			Date string `json:"date"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Date != "" {
			date = req.Date
		}
	}

	// 异步执行导入
	go h.runSync(date)

	writeJSON(w, map[string]string{
		"status":  "started",
		"date":    date,
		"message": fmt.Sprintf("开始同步 %s 运营数据", date),
	})
}

// SyncStatus 查询同步状态
// GET /api/webhook/sync-status
func (h *DashboardHandler) SyncStatus(w http.ResponseWriter, r *http.Request) {
	syncMu.Lock()
	defer syncMu.Unlock()
	writeJSON(w, map[string]interface{}{
		"running":  syncRunning,
		"last_log": syncLastLog,
		"last_at":  syncLastAt,
	})
}

func (h *DashboardHandler) runSync(date string) {
	defer func() {
		syncMu.Lock()
		syncRunning = false
		syncMu.Unlock()
	}()

	exeDir := filepath.Dir(getExePath())
	tools := []string{
		"import-tmall.exe",
		"import-pdd.exe",
		"import-jd.exe",
		"import-customer.exe",
		"import-vip.exe",
		"import-tmallcs.exe",
		"import-promo.exe",
		"import-feigua.exe",
	}

	var results []string
	failCount := 0
	start := time.Now()
	log.Printf("[sync-ops] 开始同步 %s", date)

	for _, tool := range tools {
		result := runSyncTool(exeDir, tool, date)
		if result.Output != "" {
			log.Printf("[sync-ops] %s 输出:\n%s", tool, result.Output)
		}
		if result.Status != "成功" {
			failCount++
		}
		results = append(results, fmt.Sprintf("%s: %s (%s) %s", tool, result.Status, result.Duration, result.OutputDigest))
	}

	elapsed := time.Since(start).Round(time.Second)

	// 构建通知内容
	status := "全部成功"
	if failCount > 0 {
		status = fmt.Sprintf("%d个失败", failCount)
	}
	displayDate := fmt.Sprintf("%s-%s-%s", date[:4], date[4:6], date[6:8])
	summary := fmt.Sprintf("BI运营数据同步完成\n\n日期: %s\n状态: %s\n耗时: %s\n\n%s",
		displayDate, status, elapsed, strings.Join(results, "\n"))

	log.Printf("[sync-ops] %s", summary)

	// 发钉钉通知
	h.sendDingTalk(summary)

	syncMu.Lock()
	syncLastLog = fmt.Sprintf("同步完成 date=%s 耗时=%s %s", date, elapsed, status)
	syncLastAt = time.Now().Format("2006-01-02 15:04:05")
	syncMu.Unlock()
}

func runSyncTool(exeDir, tool, date string) syncToolResult {
	toolPath := filepath.Join(exeDir, tool)
	start := time.Now()
	log.Printf("[sync-ops] 开始执行 %s date=%s", tool, date)

	ctx, cancel := context.WithTimeout(context.Background(), toolTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, toolPath, date, date)
	cmd.Dir = exeDir
	outputBytes, err := cmd.CombinedOutput()
	duration := time.Since(start).Round(time.Second)
	output := strings.TrimSpace(string(outputBytes))
	digest := summarizeToolOutput(output)

	if err != nil {
		status := "失败"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = "超时"
			digest = fmt.Sprintf("执行超过 %s", toolTimeout)
		} else if digest == "" {
			digest = err.Error()
		} else {
			digest = fmt.Sprintf("%s; %v", digest, err)
		}
		log.Printf("[sync-ops] %s %s (%s): %s", tool, status, duration, digest)
		return syncToolResult{
			Name:         tool,
			Status:       status,
			Duration:     duration,
			Output:       output,
			OutputDigest: digest,
		}
	}

	if digest == "" {
		digest = "无输出"
	}
	log.Printf("[sync-ops] %s 成功 (%s): %s", tool, duration, digest)
	return syncToolResult{
		Name:         tool,
		Status:       "成功",
		Duration:     duration,
		Output:       output,
		OutputDigest: digest,
	}
}

func summarizeToolOutput(output string) string {
	if strings.TrimSpace(output) == "" {
		return ""
	}

	lines := []string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(strings.ReplaceAll(line, "\r", ""))
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return ""
	}

	lastLine := lines[len(lines)-1]
	if len(lines) == 1 {
		return lastLine
	}
	return fmt.Sprintf("%d行输出，最后一行: %s", len(lines), lastLine)
}

func (dh *DashboardHandler) sendDingTalk(content string) {
	if dh.DingToken == "" || dh.DingSecret == "" {
		return
	}
	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
	signStr := fmt.Sprintf("%s\n%s", timestamp, dh.DingSecret)
	h := hmac.New(sha256.New, []byte(dh.DingSecret))
	h.Write([]byte(signStr))
	sign := url.QueryEscape(base64.StdEncoding.EncodeToString(h.Sum(nil)))

	apiURL := fmt.Sprintf("%s?access_token=%s&timestamp=%s&sign=%s",
		dingWebhook, dh.DingToken, timestamp, sign)

	body := map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": content,
		},
	}
	jsonBytes, _ := json.Marshal(body)

	resp, err := http.Post(apiURL, "application/json", strings.NewReader(string(jsonBytes)))
	if err != nil {
		log.Printf("[钉钉通知] 发送失败: %v", err)
		return
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if errcode, ok := result["errcode"].(float64); ok && errcode != 0 {
		log.Printf("[钉钉通知] 返回错误: %v", result)
	} else {
		log.Println("[钉钉通知] 发送成功")
	}
}

func getExePath() string {
	p, _ := exec.LookPath("server.exe")
	if p == "" {
		p = `C:\Users\Administrator\bi-dashboard\server\server.exe`
	}
	abs, _ := filepath.Abs(p)
	return abs
}
