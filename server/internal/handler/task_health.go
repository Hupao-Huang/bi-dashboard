// task_health.go - 定时任务健康巡检 + 失败钉钉告警
//
// 启动后 2 分钟首次巡检, 之后每 30 分钟一次。
// 状态变化才推送 (之前成功现在失败 = 新增失败), 同一个 fail 不会反复推。
// 接收人 = users.username='admin' 的 dingtalk_userid (跟 v1.14 反馈通知一致)。
package handler

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

const (
	healthCheckInterval = 30 * time.Minute
	firstCheckDelay     = 2 * time.Minute // 启动 2 分钟后再首次巡检, 避开启动期抢资源
)

var (
	healthLastSeen = make(map[string]string) // taskName → "lastRun|status|output" 状态指纹
	healthMu       sync.Mutex
)

// StartTaskHealthMonitor 启动定时任务健康巡检 goroutine
// 每 30 分钟巡一次, 发现新失败/卡死 → 钉钉推送给 admin
// Notifier 未配置时直接返回, 不影响 server 启动
func (h *DashboardHandler) StartTaskHealthMonitor() {
	if h.Notifier == nil {
		log.Println("[task-health] DingTalk Notifier 未配置, 跳过健康巡检")
		return
	}
	log.Printf("[task-health] 启动: 每 %v 巡检 BI-* 定时任务, 新增失败/卡死推送 admin", healthCheckInterval)

	time.Sleep(firstCheckDelay)
	h.runHealthCheck() // 首次巡检

	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()
	for range ticker.C {
		h.runHealthCheck()
	}
}

// runHealthCheck 单次巡检逻辑
func (h *DashboardHandler) runHealthCheck() {
	tasks, err := loadSchtasksStatus()
	if err != nil {
		log.Printf("[task-health] 巡检 schtasks 失败: %v", err)
		return
	}

	var newFails []TaskStatus
	healthMu.Lock()
	for _, ts := range tasks {
		if ts.Status != "failed" && ts.Status != "stuck" {
			// 状态恢复正常: 清掉旧记录, 下次再 fail 还会推
			delete(healthLastSeen, ts.Name)
			continue
		}
		// 失败/卡死任务: 用 lastRun + status + lastOutput 三件套做状态指纹
		key := fmt.Sprintf("%s|%s|%s", ts.LastRun, ts.Status, ts.LastOutput)
		if healthLastSeen[ts.Name] != key {
			newFails = append(newFails, ts)
			healthLastSeen[ts.Name] = key
		}
	}
	healthMu.Unlock()

	if len(newFails) == 0 {
		return
	}

	// 找 admin 钉钉
	var adminUnion sql.NullString
	err = h.DB.QueryRow(`
		SELECT dingtalk_userid FROM users
		WHERE username = 'admin' AND status = 'active' LIMIT 1
	`).Scan(&adminUnion)
	if err != nil || !adminUnion.Valid || adminUnion.String == "" {
		log.Printf("[task-health] admin 未绑定钉钉, %d 个失败任务无法推送", len(newFails))
		return
	}

	msg := buildHealthAlertMessage(newFails)
	h.Notifier.SendTextAsync([]string{adminUnion.String}, msg)
	log.Printf("[task-health] 推送 %d 个新失败/卡死任务给 admin", len(newFails))
}

// buildHealthAlertMessage 拼推送文本
func buildHealthAlertMessage(fails []TaskStatus) string {
	var sb strings.Builder
	sb.WriteString("【BI 看板·定时任务异常告警】\n\n")

	if len(fails) == 1 {
		ts := fails[0]
		statusLabel := "失败"
		if ts.Status == "stuck" {
			statusLabel = "卡死(运行超 1 小时)"
		}
		sb.WriteString(fmt.Sprintf("任务: %s\n", ts.Name))
		sb.WriteString(fmt.Sprintf("状态: %s\n", statusLabel))
		if ts.LastRun != "" {
			sb.WriteString(fmt.Sprintf("上次运行: %s\n", ts.LastRun))
		}
		if ts.LastOutput != "" {
			tail := ts.LastOutput
			// 限制 200 字防超长
			if len([]rune(tail)) > 200 {
				tail = string([]rune(tail)[:200]) + "..."
			}
			sb.WriteString(fmt.Sprintf("\n%s\n", tail))
		}
	} else {
		sb.WriteString(fmt.Sprintf("发现 %d 个异常任务:\n\n", len(fails)))
		for _, ts := range fails {
			statusLabel := "失败"
			if ts.Status == "stuck" {
				statusLabel = "卡死"
			}
			sb.WriteString(fmt.Sprintf("• %s [%s]\n", ts.Name, statusLabel))
			if ts.LastRun != "" {
				sb.WriteString(fmt.Sprintf("  上次: %s\n", ts.LastRun))
			}
		}
	}

	sb.WriteString("\n打开运维监控查看详情 → /system/ops")
	return sb.String()
}
