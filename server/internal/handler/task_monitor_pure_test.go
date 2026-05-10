package handler

// task_monitor_pure_test.go — buildTaskStatus / fillNextRun / cleanupOldTasks 纯函数 + 全局 map 测试
// 已 Read task_monitor.go (line 168 buildTaskStatus, 315 fillNextRun, 607 cleanupOldTasks).

import (
	"strings"
	"testing"
	"time"
)

// ============ buildTaskStatus ============

func TestBuildTaskStatusRunning(t *testing.T) {
	raw := schtasksRaw{
		TaskName:    "BI-SyncDailySummary",
		State:       "Running",
		LastRunTime: time.Now().Format("2006-01-02 15:04:05"),
	}
	ts := buildTaskStatus(raw)
	if ts.Status != "running" {
		t.Errorf("Running 状态应映射 running, got %s", ts.Status)
	}
	// 配了 meta → 中文 Name
	if ts.Name != "每日汇总帐同步" {
		t.Errorf("应替换为中文 Name, got %q", ts.Name)
	}
}

func TestBuildTaskStatusStuck(t *testing.T) {
	// LastRunTime > 1h 前 + Running → stuck
	raw := schtasksRaw{
		TaskName:    "BI-SyncStock",
		State:       "Running",
		LastRunTime: time.Now().Add(-2 * time.Hour).Format("2006-01-02 15:04:05"),
	}
	ts := buildTaskStatus(raw)
	if ts.Status != "stuck" {
		t.Errorf("> 1h Running 应 stuck, got %s", ts.Status)
	}
	if !strings.Contains(ts.LastOutput, "卡死") {
		t.Errorf("LastOutput 应含'卡死': %s", ts.LastOutput)
	}
}

func TestBuildTaskStatusSuccess(t *testing.T) {
	raw := schtasksRaw{
		TaskName:       "BI-SyncStock",
		State:          "Ready",
		LastTaskResult: "0",
	}
	ts := buildTaskStatus(raw)
	if ts.Status != "success" {
		t.Errorf("LastTaskResult=0 应 success, got %s", ts.Status)
	}
}

func TestBuildTaskStatusFailed(t *testing.T) {
	raw := schtasksRaw{
		TaskName:       "BI-SyncStock",
		LastTaskResult: "1",
	}
	ts := buildTaskStatus(raw)
	if ts.Status != "failed" {
		t.Errorf("非 0/267011/267014 应 failed, got %s", ts.Status)
	}
	if !strings.Contains(ts.LastOutput, "退出码 = 1") {
		t.Errorf("应含退出码: %s", ts.LastOutput)
	}
}

func TestBuildTaskStatusWaiting267011(t *testing.T) {
	raw := schtasksRaw{
		TaskName:       "BI-SyncStock",
		LastTaskResult: "267011",
	}
	ts := buildTaskStatus(raw)
	if ts.Status != "waiting" {
		t.Errorf("267011 应 waiting, got %s", ts.Status)
	}
}

func TestBuildTaskStatusWaiting267014(t *testing.T) {
	raw := schtasksRaw{
		TaskName:       "BI-SyncStock",
		LastTaskResult: "267014",
	}
	ts := buildTaskStatus(raw)
	if ts.Status != "waiting" {
		t.Errorf("267014 应 waiting, got %s", ts.Status)
	}
}

func TestBuildTaskStatusUnknownTask(t *testing.T) {
	// 不在 taskMetaByName 里的任务
	raw := schtasksRaw{
		TaskName:       "BI-UnknownTask",
		LastTaskResult: "0",
	}
	ts := buildTaskStatus(raw)
	if ts.Description != "（未配置中文描述）" {
		t.Errorf("无 meta 应有默认描述: %q", ts.Description)
	}
	if ts.Category != "other" {
		t.Errorf("无 meta 应 category=other, got %q", ts.Category)
	}
}

// ============ fillNextRun ============

func TestFillNextRun8AM(t *testing.T) {
	ts := &TaskStatus{}
	fillNextRun(ts, TaskConfig{Schedule: "每天 08:00"})
	// NextRun 必填且解析成功
	if ts.NextRun == "" {
		t.Error("应填 NextRun")
	}
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", ts.NextRun, time.Local)
	if err != nil {
		t.Errorf("parse next: %v", err)
	}
	if parsed.Hour() != 8 || parsed.Minute() != 0 {
		t.Errorf("应是 08:00, got %v:%v", parsed.Hour(), parsed.Minute())
	}
}

func TestFillNextRun830(t *testing.T) {
	ts := &TaskStatus{}
	fillNextRun(ts, TaskConfig{Schedule: "每天 08:30"})
	parsed, _ := time.ParseInLocation("2006-01-02 15:04:05", ts.NextRun, time.Local)
	if parsed.Hour() != 8 || parsed.Minute() != 30 {
		t.Errorf("应 08:30, got %v:%v", parsed.Hour(), parsed.Minute())
	}
}

func TestFillNextRunMultiTimes(t *testing.T) {
	// 9:00/15:00/21:00 三次, 取距今最近的下一个
	ts := &TaskStatus{}
	fillNextRun(ts, TaskConfig{Schedule: "每天 09:00/15:00/21:00"})
	parsed, _ := time.ParseInLocation("2006-01-02 15:04:05", ts.NextRun, time.Local)
	hour := parsed.Hour()
	if hour != 9 && hour != 15 && hour != 21 {
		t.Errorf("应是 9/15/21 之一, got %d", hour)
	}
}

func TestFillNextRun9AM(t *testing.T) {
	ts := &TaskStatus{}
	fillNextRun(ts, TaskConfig{Schedule: "每天 09:00"})
	parsed, _ := time.ParseInLocation("2006-01-02 15:04:05", ts.NextRun, time.Local)
	if parsed.Hour() != 9 || parsed.Minute() != 0 {
		t.Errorf("应 09:00, got %d:%d", parsed.Hour(), parsed.Minute())
	}
}

// ============ cleanupOldTasks ============

func TestCleanupOldTasksUnderThreshold(t *testing.T) {
	// 重置状态
	runningMu.Lock()
	runningTasks = make(map[string]*RunningTask)
	for i := 0; i < 10; i++ {
		runningTasks[generateTaskID()] = &RunningTask{Status: "completed"}
	}
	runningMu.Unlock()

	cleanupOldTasks()

	runningMu.Lock()
	count := len(runningTasks)
	runningMu.Unlock()
	if count != 10 {
		t.Errorf("≤ 20 应不动, got %d want 10", count)
	}
}

func TestCleanupOldTasksOverThreshold(t *testing.T) {
	runningMu.Lock()
	runningTasks = make(map[string]*RunningTask)
	// 25 个任务: 15 completed + 10 running
	for i := 0; i < 15; i++ {
		runningTasks[generateTaskID()] = &RunningTask{Status: "completed"}
	}
	for i := 0; i < 10; i++ {
		runningTasks[generateTaskID()] = &RunningTask{Status: "running"}
	}
	runningMu.Unlock()

	cleanupOldTasks()

	runningMu.Lock()
	count := len(runningTasks)
	// 删除 5 个 completed (25 - 20 = 5)
	runningCount := 0
	for _, t := range runningTasks {
		if t.Status == "running" {
			runningCount++
		}
	}
	runningMu.Unlock()

	if count != 20 {
		t.Errorf("应留 20, got %d", count)
	}
	if runningCount != 10 {
		t.Errorf("running 任务不应被删, got runningCount=%d want 10", runningCount)
	}
}

// ============ generateTaskID ============

func TestGenerateTaskIDUnique(t *testing.T) {
	id1 := generateTaskID()
	id2 := generateTaskID()
	if id1 == id2 {
		t.Errorf("两次生成应不同, got %s == %s", id1, id2)
	}
	if len(id1) != 16 { // 8 字节 hex = 16 字符
		t.Errorf("ID 长度应 16 hex, got %d", len(id1))
	}
}
