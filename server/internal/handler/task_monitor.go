package handler

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TaskConfig 任务配置
type TaskConfig struct {
	Name        string
	Description string
	Schedule    string
	TaskName    string
	LogFile     string
	Category    string
}

// TaskStatus 任务状态返回结构
type TaskStatus struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Schedule    string `json:"schedule"`
	Category    string `json:"category"`
	Status      string `json:"status"`     // success / failed / running / waiting
	LastRun     string `json:"lastRun"`    // 上次运行时间
	LastFinish  string `json:"lastFinish"` // 上次结束时间
	Duration    string `json:"duration"`   // 耗时
	LastOutput  string `json:"lastOutput"` // 最后几行日志
	NextRun     string `json:"nextRun"`    // 下次运行时间
}

var taskConfigs = []TaskConfig{
	{
		Name:        "每日汇总帐同步",
		Description: "销售货品汇总帐(最近7天覆盖)",
		Schedule:    "每天 08:00",
		TaskName:    "BI-SyncDailySummary",
		LogFile:     "sync-daily-summary.log",
		Category:    "sync",
	},
	{
		Name:        "库存同步",
		Description: "库存分页查询 + 历史明细快照(每天3次)",
		Schedule:    "每天 09:00/15:00/21:00",
		TaskName:    "BI-SyncStock-09",
		LogFile:     "sync-stock.log",
		Category:    "stock",
	},
	{
		Name:        "批次库存同步",
		Description: "按仓库同步批次库存",
		Schedule:    "每天 09:05",
		TaskName:    "BI-SyncBatchStock",
		LogFile:     "sync-batch-stock.log",
		Category:    "stock",
	},
	{
		Name:        "合思费控同步",
		Description: "合思费用单据 + 报销单 + 流程明细",
		Schedule:    "每天 10:30",
		TaskName:    "BI-SyncHesi",
		LogFile:     "sync-hesi.log",
		Category:    "sync",
	},
	{
		Name:        "运营数据导入",
		Description: "天猫/京东/拼多多/唯品会/天猫超市/推广/飞瓜/客服专项",
		Schedule:    "RPA webhook触发",
		TaskName:    "",
		LogFile:     "",
		Category:    "ops",
	},
	{
		Name:        "API服务",
		Description: "后端HTTP API服务(8080端口)",
		Schedule:    "开机自启",
		TaskName:    "BI-APIServer",
		LogFile:     "server-stderr.log",
		Category:    "service",
	},
}

const (
	logBaseDir = `C:\Users\Administrator\bi-dashboard\server`
	timeFmt    = "2006-01-02 15:04:05"
)

// GetTaskStatus 返回所有定时任务和同步工具的运行状态
// GET /api/admin/tasks
func (h *DashboardHandler) GetTaskStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeError(w, 405, "method not allowed")
		return
	}

	var tasks []TaskStatus
	for _, cfg := range taskConfigs {
		ts := TaskStatus{
			Name:        cfg.Name,
			Description: cfg.Description,
			Schedule:    cfg.Schedule,
			Category:    cfg.Category,
			Status:      "waiting",
		}

		switch cfg.Category {
		case "ops":
			h.fillOpsTaskStatus(&ts)
		case "service":
			fillServiceTaskStatus(&ts, cfg)
		default:
			fillLogBasedTaskStatus(&ts, cfg)
		}

		fillNextRun(&ts, cfg)
		tasks = append(tasks, ts)
	}

	writeJSON(w, tasks)
}

// fillLogBasedTaskStatus 通过日志文件判断任务状态
func fillLogBasedTaskStatus(ts *TaskStatus, cfg TaskConfig) {
	if cfg.LogFile == "" {
		return
	}

	logPath := logBaseDir + `\` + cfg.LogFile
	info, err := os.Stat(logPath)
	if err != nil {
		return
	}

	modTime := info.ModTime()
	ts.LastRun = modTime.Format(timeFmt)

	lines := readLastNLines(logPath, 10)
	if len(lines) > 0 {
		ts.LastOutput = strings.Join(lines, "\n")
		ts.Status = parseStatusFromLog(strings.Join(lines, "\n"))

		// 用文件修改时间作为结束时间参考
		ts.LastFinish = modTime.Format(timeFmt)

		// 如果文件在最近5分钟内修改过，可能正在运行
		if time.Since(modTime) < 5*time.Minute {
			// 检查最后几行是否有明确的完成/失败标志
			lastContent := strings.Join(lines, "\n")
			if !strings.Contains(lastContent, "完成") &&
				!strings.Contains(lastContent, "成功") &&
				!strings.Contains(lastContent, "失败") &&
				!strings.Contains(lastContent, "error") {
				ts.Status = "running"
			}
		}
	}
}

// fillOpsTaskStatus 运营数据导入状态（从内存中的sync状态获取）
func (h *DashboardHandler) fillOpsTaskStatus(ts *TaskStatus) {
	syncMu.Lock()
	running := syncRunning
	lastLog := syncLastLog
	lastAt := syncLastAt
	syncMu.Unlock()

	if running {
		ts.Status = "running"
		ts.LastRun = time.Now().Format(timeFmt)
		return
	}

	if lastAt != "" {
		ts.LastRun = lastAt
		ts.LastFinish = lastAt
		if strings.Contains(lastLog, "失败") {
			ts.Status = "failed"
		} else if strings.Contains(lastLog, "完成") || strings.Contains(lastLog, "成功") {
			ts.Status = "success"
		}
		ts.LastOutput = lastLog
	}
}

// fillServiceTaskStatus API服务状态（检查端口监听）
func fillServiceTaskStatus(ts *TaskStatus, cfg TaskConfig) {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:8080", 2*time.Second)
	if err == nil {
		conn.Close()
		ts.Status = "running"
		ts.LastRun = "服务运行中"
	} else {
		ts.Status = "failed"
		ts.LastOutput = "8080端口未监听"
	}

	// 读取服务日志
	if cfg.LogFile != "" {
		logPath := logBaseDir + `\` + cfg.LogFile
		lines := readLastNLines(logPath, 10)
		if len(lines) > 0 {
			ts.LastOutput = strings.Join(lines, "\n")
			info, err := os.Stat(logPath)
			if err == nil {
				ts.LastFinish = info.ModTime().Format(timeFmt)
			}
		}
	}
}

// fillNextRun 计算下次运行时间
func fillNextRun(ts *TaskStatus, cfg TaskConfig) {
	now := time.Now()
	loc := now.Location()

	switch {
	case cfg.Schedule == "每天 08:00":
		next := time.Date(now.Year(), now.Month(), now.Day(), 8, 0, 0, 0, loc)
		if now.After(next) {
			next = next.AddDate(0, 0, 1)
		}
		ts.NextRun = next.Format(timeFmt)
	case cfg.Schedule == "每天 08:30":
		next := time.Date(now.Year(), now.Month(), now.Day(), 8, 30, 0, 0, loc)
		if now.After(next) {
			next = next.AddDate(0, 0, 1)
		}
		ts.NextRun = next.Format(timeFmt)
	case cfg.Schedule == "每天 09:00/15:00/21:00":
		candidates := []time.Time{
			time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, loc),
			time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, loc),
			time.Date(now.Year(), now.Month(), now.Day(), 21, 0, 0, 0, loc),
		}
		var next time.Time
		for _, candidate := range candidates {
			if !now.After(candidate) {
				next = candidate
				break
			}
		}
		if next.IsZero() {
			next = candidates[0].AddDate(0, 0, 1)
		}
		ts.NextRun = next.Format(timeFmt)
	case cfg.Schedule == "每天 09:00":
		next := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, loc)
		if now.After(next) {
			next = next.AddDate(0, 0, 1)
		}
		ts.NextRun = next.Format(timeFmt)
	case cfg.Schedule == "每天 15:00":
		next := time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, loc)
		if now.After(next) {
			next = next.AddDate(0, 0, 1)
		}
		ts.NextRun = next.Format(timeFmt)
	case cfg.Schedule == "每天 21:00":
		next := time.Date(now.Year(), now.Month(), now.Day(), 21, 0, 0, 0, loc)
		if now.After(next) {
			next = next.AddDate(0, 0, 1)
		}
		ts.NextRun = next.Format(timeFmt)
	case cfg.Schedule == "每天 09:05":
		next := time.Date(now.Year(), now.Month(), now.Day(), 9, 5, 0, 0, loc)
		if now.After(next) {
			next = next.AddDate(0, 0, 1)
		}
		ts.NextRun = next.Format(timeFmt)
	case cfg.Schedule == "每天 10:30":
		next := time.Date(now.Year(), now.Month(), now.Day(), 10, 30, 0, 0, loc)
		if now.After(next) {
			next = next.AddDate(0, 0, 1)
		}
		ts.NextRun = next.Format(timeFmt)
	default:
		ts.NextRun = "-"
	}
}

// parseStatusFromLog 从日志内容解析状态
func parseStatusFromLog(content string) string {
	lower := strings.ToLower(content)
	if strings.Contains(content, "失败") || strings.Contains(lower, "error") || strings.Contains(lower, "fatal") {
		return "failed"
	}
	if strings.Contains(content, "完成") || strings.Contains(content, "成功") {
		return "success"
	}
	return "waiting"
}

// readLastNLines 倒序读取文件最后N行，避免全文加载
func readLastNLines(filePath string, n int) []string {
	f, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return nil
	}

	// 从文件末尾向前读取，每次读一个块
	const blockSize = 4096
	fileSize := info.Size()
	var lines []string
	var leftover string
	offset := fileSize

	for offset > 0 && len(lines) < n+1 {
		readSize := int64(blockSize)
		if readSize > offset {
			readSize = offset
		}
		offset -= readSize

		buf := make([]byte, readSize)
		_, err := f.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			break
		}

		chunk := string(buf) + leftover
		leftover = ""

		parts := strings.Split(chunk, "\n")
		// 第一个 part 可能是不完整的行，保留给下一轮
		if offset > 0 {
			leftover = parts[0]
			parts = parts[1:]
		}

		// 反向收集非空行
		for i := len(parts) - 1; i >= 0; i-- {
			line := strings.TrimSpace(strings.ReplaceAll(parts[i], "\r", ""))
			if line == "" {
				continue
			}
			lines = append(lines, line)
			if len(lines) >= n {
				break
			}
		}
	}

	// 如果还有剩余的 leftover 且行数不够
	if leftover != "" && len(lines) < n {
		line := strings.TrimSpace(strings.ReplaceAll(leftover, "\r", ""))
		if line != "" {
			lines = append(lines, line)
		}
	}

	// lines 是倒序的，翻转为正序
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}

	// 最多取 n 行
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	return lines
}

// readLastNLinesScanner 备用方案：用 Scanner 顺序读取（小文件适用）
func readLastNLinesScanner(filePath string, n int) []string {
	f, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var ring []string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		ring = append(ring, line)
		if len(ring) > n {
			ring = ring[1:]
		}
	}

	return ring
}

// formatDuration 格式化耗时显示
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%d时%d分%d秒", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%d分%d秒", m, s)
	}
	return fmt.Sprintf("%d秒", s)
}

// ==================== 手动任务触发功能 ====================

// ManualTaskConfig 手动任务配置
type ManualTaskConfig struct {
	Key         string        `json:"key"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Exe         string        `json:"exe"`
	Params      []ParamConfig `json:"params"`
}

// ParamConfig 参数配置
type ParamConfig struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Default  string `json:"default"`
	EnvVar   string `json:"envVar"`
}

// RunningTask 运行中任务
type RunningTask struct {
	ID        string            `json:"id"`
	Key       string            `json:"key"`
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	StartedAt time.Time         `json:"startedAt"`
	EndedAt   *time.Time        `json:"endedAt,omitempty"`
	Params    map[string]string `json:"params"`
	LogFile   string            `json:"logFile"`
	Cmd       *exec.Cmd         `json:"-"`
}

var manualTaskConfigs = []ManualTaskConfig{
	{
		Key: "sync-trades", Name: "销售单补拉", Description: "按日期范围补拉销售单+明细+包裹",
		Exe: "sync-trades-v2.exe",
		Params: []ParamConfig{
			{Key: "startDate", Label: "开始日期", Type: "date", Required: true, EnvVar: "TRADE_SYNC_START_DATE"},
			{Key: "endDate", Label: "结束日期", Type: "date", Required: true, EnvVar: "TRADE_SYNC_END_DATE"},
		},
	},
	{
		Key: "sync-summary", Name: "汇总帐补拉", Description: "按日期范围补拉销售货品汇总帐",
		Exe: "sync-summary.exe",
		Params: []ParamConfig{
			{Key: "startDate", Label: "开始日期", Type: "date", Required: true, EnvVar: "SYNC_START_DATE"},
			{Key: "endDate", Label: "结束日期", Type: "date", Required: true, EnvVar: "SYNC_END_DATE"},
		},
	},
	{
		Key: "sync-stock", Name: "库存同步", Description: "同步库存分页数据+历史快照",
		Exe:    "sync-stock.exe",
		Params: []ParamConfig{},
	},
	{
		Key: "sync-batch-stock", Name: "批次库存同步", Description: "按仓库同步批次库存",
		Exe:    "sync-batch-stock.exe",
		Params: []ParamConfig{},
	},
	{
		Key: "sync-channels", Name: "渠道同步", Description: "同步吉客云销售渠道",
		Exe:    "sync-channels.exe",
		Params: []ParamConfig{},
	},
	{
		Key: "sync-goods", Name: "货品档案同步", Description: "同步吉客云货品档案",
		Exe:    "sync-goods.exe",
		Params: []ParamConfig{},
	},
	{
		Key: "import-ops", Name: "运营数据导入", Description: "导入天猫/京东/拼多多/唯品会/天猫超市/推广/飞瓜",
		Exe: "",
		Params: []ParamConfig{
			{Key: "date", Label: "日期", Type: "date", Required: false, Default: "", EnvVar: ""},
		},
	},
}

var (
	runningTasks = make(map[string]*RunningTask)
	runningMu    sync.Mutex
)

// generateTaskID 生成随机任务ID
func generateTaskID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// cleanupOldTasks 清理已完成的旧任务，保留最近20个
func cleanupOldTasks() {
	if len(runningTasks) <= 20 {
		return
	}
	// 找出已完成的任务，按结束时间排序删除最旧的
	var completedIDs []string
	for id, t := range runningTasks {
		if t.Status != "running" {
			completedIDs = append(completedIDs, id)
		}
	}
	// 删除多余的已完成任务
	excess := len(runningTasks) - 20
	for i := 0; i < excess && i < len(completedIDs); i++ {
		delete(runningTasks, completedIDs[i])
	}
}

// RunManualTask 手动触发任务
// POST /api/admin/tasks/run
func (h *DashboardHandler) RunManualTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, 405, "method not allowed")
		return
	}

	var req struct {
		Task   string            `json:"task"`
		Params map[string]string `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}

	// 查找任务配置
	var config *ManualTaskConfig
	for i := range manualTaskConfigs {
		if manualTaskConfigs[i].Key == req.Task {
			config = &manualTaskConfigs[i]
			break
		}
	}
	if config == nil {
		writeError(w, 400, "unknown task: "+req.Task)
		return
	}

	// 检查必填参数
	for _, p := range config.Params {
		if p.Required {
			val := req.Params[p.Key]
			if val == "" {
				writeError(w, 400, "missing required param: "+p.Label)
				return
			}
		}
	}

	// 检查文件锁（跨定时任务和面板的互锁）
	lockFile := filepath.Join(logBaseDir, config.Key+".lock")
	if _, err := os.Stat(lockFile); err == nil {
		writeError(w, 409, config.Name+"正在运行中（文件锁存在）")
		return
	}

	// 检查是否已有同类任务在运行
	runningMu.Lock()
	for _, t := range runningTasks {
		if t.Key == req.Task && t.Status == "running" {
			runningMu.Unlock()
			writeError(w, 409, "task already running: "+config.Name)
			return
		}
	}
	runningMu.Unlock()

	taskID := generateTaskID()
	now := time.Now()

	task := &RunningTask{
		ID:        taskID,
		Key:       config.Key,
		Name:      config.Name,
		Status:    "running",
		StartedAt: now,
		Params:    req.Params,
	}

	// import-ops 通过 webhook 触发
	if config.Key == "import-ops" {
		task.LogFile = ""
		runningMu.Lock()
		cleanupOldTasks()
		runningTasks[taskID] = task
		runningMu.Unlock()

		go func() {
			dateParam := req.Params["date"]
			if dateParam == "" {
				dateParam = time.Now().AddDate(0, 0, -1).Format("20060102")
			} else {
				// 把 2026-03-25 转成 20260325
				dateParam = strings.ReplaceAll(dateParam, "-", "")
			}

			body := fmt.Sprintf(`{"date":"%s"}`, dateParam)
			resp, err := http.Post("http://localhost:8080/api/webhook/sync-ops", "application/json", bytes.NewBufferString(body))

			endTime := time.Now()
			runningMu.Lock()
			defer runningMu.Unlock()
			task.EndedAt = &endTime
			if err != nil {
				task.Status = "failed"
			} else {
				resp.Body.Close()
				if resp.StatusCode == 200 {
					task.Status = "completed"
				} else {
					task.Status = "failed"
				}
			}
		}()

		writeJSON(w, map[string]string{"taskId": taskID})
		return
	}

	// 普通 exe 任务
	logFileName := fmt.Sprintf("manual-%s-%s.log", config.Key, now.Format("20060102-150405"))
	logPath := filepath.Join(logBaseDir, logFileName)
	task.LogFile = logPath

	logFile, err := os.Create(logPath)
	if err != nil {
		writeError(w, 500, "create log file failed: "+err.Error())
		return
	}

	// 构造环境变量
	var envVars []string
	for _, p := range config.Params {
		if p.EnvVar == "" {
			continue
		}
		val := req.Params[p.Key]
		if val == "" {
			val = p.Default
		}
		if val != "" {
			envVars = append(envVars, p.EnvVar+"="+val)
		}
	}

	cmd := exec.Command(filepath.Join(logBaseDir, config.Exe))
	cmd.Dir = logBaseDir
	cmd.Env = append(os.Environ(), envVars...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	task.Cmd = cmd

	if err := cmd.Start(); err != nil {
		logFile.Close()
		writeError(w, 500, "start task failed: "+err.Error())
		return
	}

	runningMu.Lock()
	cleanupOldTasks()
	runningTasks[taskID] = task
	runningMu.Unlock()

	// 后台等待完成（锁文件由exe自己管理）
	go func() {
		err := cmd.Wait()
		logFile.Close()
		endTime := time.Now()
		runningMu.Lock()
		defer runningMu.Unlock()
		task.EndedAt = &endTime
		if err != nil {
			task.Status = "failed"
		} else {
			task.Status = "completed"
		}
	}()

	writeJSON(w, map[string]string{"taskId": taskID})
}

// GetRunningTasks 获取运行中和最近完成的任务
// GET /api/admin/tasks/running
func (h *DashboardHandler) GetRunningTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeError(w, 405, "method not allowed")
		return
	}

	// 先在锁内 snapshot 字段，释放锁后再读日志文件，避免持锁做磁盘 I/O 阻塞其他任务操作
	type taskSnap struct {
		id, key, name, status, startedAt, endedAt, logFile string
		params                                             map[string]string
		hasEnd                                             bool
	}
	runningMu.Lock()
	snaps := make([]taskSnap, 0, len(runningTasks))
	for id, t := range runningTasks {
		s := taskSnap{
			id:        id,
			key:       t.Key,
			name:      t.Name,
			status:    t.Status,
			startedAt: t.StartedAt.Format(timeFmt),
			params:    t.Params,
			logFile:   t.LogFile,
		}
		if t.EndedAt != nil {
			s.endedAt = t.EndedAt.Format(timeFmt)
			s.hasEnd = true
		}
		snaps = append(snaps, s)
	}
	runningMu.Unlock()

	result := make(map[string]interface{}, len(snaps))
	for _, s := range snaps {
		entry := map[string]interface{}{
			"id":        s.id,
			"key":       s.key,
			"name":      s.name,
			"status":    s.status,
			"startedAt": s.startedAt,
			"params":    s.params,
		}
		if s.hasEnd {
			entry["endedAt"] = s.endedAt
		}
		if s.logFile != "" {
			lines := readLastNLines(s.logFile, 20)
			if len(lines) > 0 {
				entry["log"] = strings.Join(lines, "\n")
			}
		}
		result[s.id] = entry
	}

	writeJSON(w, map[string]interface{}{
		"configs": manualTaskConfigs,
		"running": result,
	})
}

// StopManualTask 停止手动任务
// POST /api/admin/tasks/stop
func (h *DashboardHandler) StopManualTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, 405, "method not allowed")
		return
	}

	var req struct {
		TaskID string `json:"taskId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}

	// 检查 + kill + 状态改写在同一把锁内完成，避免 TOCTOU
	// Process.Kill() 只发送信号不阻塞，持锁安全
	runningMu.Lock()
	task, ok := runningTasks[req.TaskID]
	if !ok {
		runningMu.Unlock()
		writeError(w, 404, "task not found")
		return
	}
	if task.Status != "running" {
		runningMu.Unlock()
		writeError(w, 400, "task is not running")
		return
	}
	if task.Cmd != nil && task.Cmd.Process != nil {
		_ = task.Cmd.Process.Kill()
	}
	endTime := time.Now()
	task.Status = "failed"
	task.EndedAt = &endTime
	runningMu.Unlock()

	writeJSON(w, map[string]string{"status": "stopped"})
}
