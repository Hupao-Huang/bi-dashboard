// 影刀 RPA 触发: 点抽屉外面"立即同步本平台"按钮 → 调影刀 OpenAPI 启动子应用
//
// 5 个接口:
//   POST /api/admin/rpa/trigger                      跑哥点同步按钮触发同步
//   GET  /api/admin/rpa/job-status?trigger_id=N      前端轮询状态+日志 (5s 一次)
//   GET  /api/admin/rpa/platform-mapping             列出 11 平台 → 影刀子应用映射
//   POST /api/admin/rpa/platform-mapping/update      改映射 (后台维护用)
//   GET  /api/admin/yingdao/tasks                    拉影刀全量任务下拉选 (5min 缓存)
package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"bi-dashboard/internal/yingdao"
)

// ======== 内存缓存: 影刀任务列表 + 子应用 ========

var (
	yingdaoTasksMu       sync.Mutex
	yingdaoTasksCache    []yingdao.Schedule
	yingdaoTasksCachedAt time.Time
	yingdaoTasksCacheTTL = 5 * time.Minute

	yingdaoSubAppsMu       sync.Mutex
	yingdaoSubAppsCache    map[string][]yingdao.RobotInScheduleDetail // key=scheduleUuid
	yingdaoSubAppsCachedAt map[string]time.Time
)

// 默认 schedule: 集团数据看板 (跑哥唯一会用到的)
const defaultGroupDashboardScheduleUuid = "a146505a-1329-45b0-aa2a-c37d3620a8e4"

// ======== 1. POST /api/admin/rpa/trigger ========

// RPATriggerReq 触发请求
// Date 是业务日期 (YYYY-MM-DD), 传给影刀子应用的 run_data 入参, 应用按这个日期采数据
// Date 留空时用今天 (兼容旧调用方)
type RPATriggerReq struct {
	Platform string `json:"platform"`
	Date     string `json:"date,omitempty"`
}

// RPATriggerResp 触发响应
type RPATriggerResp struct {
	TriggerID    int64  `json:"trigger_id"`     // rpa_trigger_log.id, 前端轮询用
	JobUuid      string `json:"job_uuid"`       // 影刀返回的 jobUuid
	LogRequestID string `json:"log_request_id"` // 拉日志用 requestId
	RobotName    string `json:"robot_name"`     // 影刀子应用名 (展示)
	Platform     string `json:"platform"`
	RunDate      string `json:"run_date"`       // 实际传给影刀的业务日期
	StartedAt    string `json:"started_at"`
}

// TriggerRPASync POST /api/admin/rpa/trigger
// 流程: 查映射 → 调影刀 StartJob → 插 trigger_log → 异步拉 log requestId
func (dh *DashboardHandler) TriggerRPASync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	if dh.YingDao == nil || !dh.YingDao.Configured() {
		writeError(w, 500, "影刀 RPA 未配置 (config.json 缺 yingdao 段)")
		return
	}

	var req RPATriggerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Platform == "" {
		writeError(w, 400, "请传 {\"platform\":\"唯品会\",\"date\":\"2026-05-13\"}")
		return
	}
	// Date 缺省 = 今天
	runDate := req.Date
	if runDate == "" {
		runDate = time.Now().Format("2006-01-02")
	}
	// 校验日期格式
	if _, err := time.Parse("2006-01-02", runDate); err != nil {
		writeError(w, 400, "date 格式错误 (需 YYYY-MM-DD)")
		return
	}

	// 0. 防冲突: 查是否已有同 (platform, run_date, status=running) 的活跃任务
	// 避免影刀机器人重复跑同一份数据浪费时间
	var existingID int64
	var existingJobUuid, existingRobotName string
	err := dh.DB.QueryRowContext(r.Context(),
		`SELECT id, job_uuid, COALESCE(robot_name,'') FROM rpa_trigger_log
		 WHERE platform=? AND run_date=? AND status='running'
		 ORDER BY id DESC LIMIT 1`,
		req.Platform, runDate,
	).Scan(&existingID, &existingJobUuid, &existingRobotName)
	if err == nil && existingID > 0 {
		// 复用现有任务, 不调影刀
		writeJSON(w, RPATriggerResp{
			TriggerID:    existingID,
			JobUuid:      existingJobUuid,
			LogRequestID: "",
			RobotName:    existingRobotName,
			Platform:     req.Platform,
			RunDate:      runDate,
			StartedAt:    "",
		})
		return
	}

	// 1. 查映射
	var robotUuid, robotName, accountName string
	var enabled int
	err = dh.DB.QueryRowContext(r.Context(),
		`SELECT robot_uuid, COALESCE(robot_name,''), account_name, enabled
		 FROM rpa_platform_mapping WHERE platform=?`, req.Platform,
	).Scan(&robotUuid, &robotName, &accountName, &enabled)
	if err != nil {
		writeError(w, 404, fmt.Sprintf("平台 %q 未配置影刀任务映射 (RPA 文件映射 Tab 添加)", req.Platform))
		return
	}
	if enabled == 0 {
		writeError(w, 400, fmt.Sprintf("平台 %q 的影刀映射已禁用", req.Platform))
		return
	}

	// 2. 调影刀 StartJob (传 run_data 入参 = 业务日期)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	startResp, err := dh.YingDao.StartJob(ctx, yingdao.JobStartReq{
		RobotUuid:   robotUuid,
		AccountName: accountName,
		Params: []yingdao.JobParam{
			{Name: "run_data", Value: runDate, Type: "str"},
		},
	})
	if err != nil {
		writeServerError(w, 500, fmt.Sprintf("启动影刀应用失败: %v", err), err)
		return
	}

	// 3. 插 trigger_log
	user := getCurrentUserName(r)
	now := time.Now()
	res, err := dh.DB.ExecContext(r.Context(),
		`INSERT INTO rpa_trigger_log
		 (platform, robot_uuid, robot_name, job_uuid, trigger_user, run_date, status, started_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'running', ?)`,
		req.Platform, robotUuid, robotName, startResp.JobUuid, user, runDate, now,
	)
	if err != nil {
		writeServerError(w, 500, "记录触发日志失败", err)
		return
	}
	triggerID, _ := res.LastInsertId()

	// 4. 异步通知日志, 拿 requestId (60s 内有效, 后续轮询用)
	logReqID := ""
	notifyCtx, notifyCancel := context.WithTimeout(context.Background(), 10*time.Second)
	rid, notifyErr := dh.YingDao.NotifyLog(notifyCtx, startResp.JobUuid, 1, 100)
	notifyCancel()
	if notifyErr == nil {
		logReqID = rid
		_, _ = dh.DB.ExecContext(r.Context(),
			`UPDATE rpa_trigger_log SET log_request_id=? WHERE id=?`, rid, triggerID)
	}

	writeJSON(w, RPATriggerResp{
		TriggerID:    triggerID,
		JobUuid:      startResp.JobUuid,
		LogRequestID: logReqID,
		RobotName:    robotName,
		Platform:     req.Platform,
		RunDate:      runDate,
		StartedAt:    now.Format("2006-01-02 15:04:05"),
	})
}

// ======== 2. GET /api/admin/rpa/job-status?trigger_id=N ========

// RPAJobStatusResp 状态+日志
type RPAJobStatusResp struct {
	TriggerID      int64             `json:"trigger_id"`
	Platform       string            `json:"platform"`
	RobotName      string            `json:"robot_name"`
	JobUuid        string            `json:"job_uuid"`
	Status         string            `json:"status"`          // pending/waiting/running/finish/error/cancel
	StatusName     string            `json:"status_name"`     // 中文 (影刀 statusName)
	Remark         string            `json:"remark"`          // 影刀备注
	StartTime      string            `json:"start_time"`
	EndTime        string            `json:"end_time"`
	ElapsedSec     int               `json:"elapsed_sec"`     // 已运行秒数
	Logs           []yingdao.LogItem `json:"logs"`            // 最新日志
	LogRequestID   string            `json:"log_request_id"`
	Done           bool              `json:"done"`            // 是否终态
}

// GetRPAJobStatus GET /api/admin/rpa/job-status?trigger_id=N
// 前端 5s 轮询一次, 同时拿状态和日志
func (dh *DashboardHandler) GetRPAJobStatus(w http.ResponseWriter, r *http.Request) {
	if dh.YingDao == nil || !dh.YingDao.Configured() {
		writeError(w, 500, "影刀 RPA 未配置")
		return
	}
	idStr := r.URL.Query().Get("trigger_id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, 400, "trigger_id 必填")
		return
	}

	// 拿 trigger 记录
	var (
		platform, robotName, jobUuid, status, logReqID string
		startedAt                                       sql.NullTime
	)
	err = dh.DB.QueryRowContext(r.Context(),
		`SELECT platform, COALESCE(robot_name,''), job_uuid, status, COALESCE(log_request_id,''), started_at
		 FROM rpa_trigger_log WHERE id=?`, id,
	).Scan(&platform, &robotName, &jobUuid, &status, &logReqID, &startedAt)
	if err != nil {
		writeError(w, 404, "trigger_id 不存在")
		return
	}

	// 调影刀查状态
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	jobStatus, err := dh.YingDao.QueryJob(ctx, jobUuid)
	if err != nil {
		writeServerError(w, 500, fmt.Sprintf("查询影刀状态失败: %v", err), err)
		return
	}

	resp := RPAJobStatusResp{
		TriggerID:    id,
		Platform:     platform,
		RobotName:    robotName,
		JobUuid:      jobUuid,
		Status:       jobStatus.Status,
		StatusName:   jobStatus.StatusName,
		Remark:       jobStatus.Remark,
		StartTime:    jobStatus.StartTime,
		EndTime:      jobStatus.EndTime,
		LogRequestID: logReqID,
	}
	if startedAt.Valid {
		resp.ElapsedSec = int(time.Since(startedAt.Time).Seconds())
	}

	// 是否终态
	switch jobStatus.Status {
	case "finish", "error", "cancel", "fail":
		resp.Done = true
	}

	// 拉日志: 每次都重新 notify (影刀的 requestId 是某一刻的"日志快照",
	// 缓存旧的 requestId 会拿空——影刀不会"补回填"已经 notify 过的快照).
	// notify 后 sleep 短暂等影刀准备 + 重试 query 几次拿到日志.
	if rid, nerr := dh.YingDao.NotifyLog(ctx, jobUuid, 1, 100); nerr == nil && rid != "" {
		resp.LogRequestID = rid
		_, _ = dh.DB.ExecContext(r.Context(),
			`UPDATE rpa_trigger_log SET log_request_id=? WHERE id=?`, rid, id)
		// 影刀文档建议 100ms 轮询, 这里给 3 次重试 (300ms+600ms+900ms = 1.8s 内拿到)
		// 拿不到也无所谓——前端下一次 5s poll 会再来一次
		for attempt := 0; attempt < 3; attempt++ {
			time.Sleep(time.Duration(300*(attempt+1)) * time.Millisecond)
			logResp, qerr := dh.YingDao.QueryLog(ctx, rid)
			if qerr == nil && logResp != nil && len(logResp.Logs) > 0 {
				resp.Logs = logResp.Logs
				break
			}
		}
	}

	// 终态 → 更新 trigger_log + 发钉钉
	if resp.Done && status != "finish" && status != "error" && status != "cancel" {
		newDBStatus := jobStatus.Status
		if newDBStatus == "fail" {
			newDBStatus = "error"
		}
		resultMsg := jobStatus.Remark
		if resultMsg == "" {
			resultMsg = jobStatus.StatusName
		}
		_, _ = dh.DB.ExecContext(r.Context(),
			`UPDATE rpa_trigger_log SET status=?, result_msg=?, finished_at=NOW() WHERE id=?`,
			newDBStatus, resultMsg, id)

		// L4 钉钉通知
		go dh.notifyRPADone(platform, robotName, newDBStatus, resp.ElapsedSec, resultMsg)
	}

	writeJSON(w, resp)
}

// notifyRPADone 同步完成发钉钉
func (dh *DashboardHandler) notifyRPADone(platform, robotName, status string, elapsedSec int, msg string) {
	mins := elapsedSec / 60
	secs := elapsedSec % 60
	emoji := "✅"
	statusText := "同步成功"
	if status != "finish" {
		emoji = "❌"
		statusText = "同步失败"
	}
	content := fmt.Sprintf("%s RPA 数据同步 - %s\n平台：%s\n应用：%s\n耗时：%d分%d秒",
		emoji, statusText, platform, robotName, mins, secs)
	if msg != "" && status != "finish" {
		content += fmt.Sprintf("\n失败原因：%s", msg)
	}
	dh.sendDingTalk(content)
}

// ======== 3. GET /api/admin/rpa/platform-mapping ========

// RPAPlatformMappingRow 映射表项
type RPAPlatformMappingRow struct {
	Platform    string `json:"platform"`
	RobotUuid   string `json:"robot_uuid"`
	RobotName   string `json:"robot_name"`
	AccountName string `json:"account_name"`
	Enabled     int    `json:"enabled"`
	Remark      string `json:"remark"`
	UpdatedAt   string `json:"updated_at"`
}

// GetRPAPlatformMapping GET /api/admin/rpa/platform-mapping
func (dh *DashboardHandler) GetRPAPlatformMapping(w http.ResponseWriter, r *http.Request) {
	rows, err := dh.DB.QueryContext(r.Context(),
		`SELECT platform, robot_uuid, COALESCE(robot_name,''), account_name, enabled, COALESCE(remark,''), updated_at
		 FROM rpa_platform_mapping ORDER BY platform`)
	if err != nil {
		writeServerError(w, 500, "查询映射失败", err)
		return
	}
	defer rows.Close()
	var list []RPAPlatformMappingRow
	for rows.Next() {
		var r RPAPlatformMappingRow
		var t time.Time
		if err := rows.Scan(&r.Platform, &r.RobotUuid, &r.RobotName, &r.AccountName, &r.Enabled, &r.Remark, &t); err != nil {
			continue
		}
		r.UpdatedAt = t.Format("2006-01-02 15:04:05")
		list = append(list, r)
	}
	writeJSON(w, list)
}

// ======== 4. POST /api/admin/rpa/platform-mapping/update ========

// UpdateRPAPlatformMapping POST /api/admin/rpa/platform-mapping/update
// Body 同 RPAPlatformMappingRow (UpdatedAt 忽略)
func (dh *DashboardHandler) UpdateRPAPlatformMapping(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	var req RPAPlatformMappingRow
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Platform == "" {
		writeError(w, 400, "platform 必填")
		return
	}
	if req.AccountName == "" {
		req.AccountName = "lhx@sxx"
	}
	_, err := dh.DB.ExecContext(r.Context(),
		`INSERT INTO rpa_platform_mapping (platform, robot_uuid, robot_name, account_name, enabled, remark)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		   robot_uuid=VALUES(robot_uuid),
		   robot_name=VALUES(robot_name),
		   account_name=VALUES(account_name),
		   enabled=VALUES(enabled),
		   remark=VALUES(remark),
		   updated_at=NOW()`,
		req.Platform, req.RobotUuid, req.RobotName, req.AccountName, req.Enabled, req.Remark,
	)
	if err != nil {
		writeServerError(w, 500, "保存映射失败", err)
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}

// ======== 5. GET /api/admin/yingdao/tasks ========

// GetYingDaoTasks GET /api/admin/yingdao/tasks
// 拉影刀全量任务列表 (前端下拉选用), 5 min 内存缓存
func (dh *DashboardHandler) GetYingDaoTasks(w http.ResponseWriter, r *http.Request) {
	if dh.YingDao == nil || !dh.YingDao.Configured() {
		writeError(w, 500, "影刀未配置")
		return
	}
	yingdaoTasksMu.Lock()
	if r.URL.Query().Get("refresh") != "1" &&
		yingdaoTasksCache != nil &&
		time.Since(yingdaoTasksCachedAt) < yingdaoTasksCacheTTL {
		cache := yingdaoTasksCache
		yingdaoTasksMu.Unlock()
		writeJSON(w, cache)
		return
	}
	yingdaoTasksMu.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	list, err := dh.YingDao.ListSchedules(ctx, 1, 500)
	if err != nil {
		writeServerError(w, 500, "拉取影刀任务失败", err)
		return
	}
	yingdaoTasksMu.Lock()
	yingdaoTasksCache = list
	yingdaoTasksCachedAt = time.Now()
	yingdaoTasksMu.Unlock()
	writeJSON(w, list)
}

// ======== 6. GET /api/admin/rpa/active-tasks ========

// RPAActiveTask 后台仍在运行的同步任务摘要
type RPAActiveTask struct {
	TriggerID   int64  `json:"trigger_id"`
	Platform    string `json:"platform"`
	RobotName   string `json:"robot_name"`
	RunDate     string `json:"run_date"`
	TriggerUser string `json:"trigger_user"`
	StartedAt   string `json:"started_at"`
	ElapsedSec  int    `json:"elapsed_sec"`
}

// GetRPAActiveTasks GET /api/admin/rpa/active-tasks
// 返回所有 status='running' 的触发记录, 给"后台任务" Drawer 用
// 前端 5s 一次轮询这个接口, 知道哪些跑哪些没跑
func (dh *DashboardHandler) GetRPAActiveTasks(w http.ResponseWriter, r *http.Request) {
	rows, err := dh.DB.QueryContext(r.Context(),
		`SELECT id, platform, COALESCE(robot_name,''), COALESCE(run_date,''),
		        COALESCE(trigger_user,''), started_at
		 FROM rpa_trigger_log WHERE status='running'
		 ORDER BY started_at DESC LIMIT 50`)
	if err != nil {
		writeServerError(w, 500, "查询活跃任务失败", err)
		return
	}
	defer rows.Close()
	var list []RPAActiveTask
	for rows.Next() {
		var t RPAActiveTask
		var startedAt sql.NullTime
		var runDate sql.NullString
		if err := rows.Scan(&t.TriggerID, &t.Platform, &t.RobotName, &runDate, &t.TriggerUser, &startedAt); err != nil {
			continue
		}
		if runDate.Valid {
			t.RunDate = runDate.String
			// MySQL DATE 字段返回 "YYYY-MM-DDT00:00:00Z" 这种, 截取
			if len(t.RunDate) >= 10 {
				t.RunDate = t.RunDate[:10]
			}
		}
		if startedAt.Valid {
			t.StartedAt = startedAt.Time.Format("2006-01-02 15:04:05")
			t.ElapsedSec = int(time.Since(startedAt.Time).Seconds())
		}
		list = append(list, t)
	}
	writeJSON(w, list)
}

// ======== 7. GET /api/admin/yingdao/sub-apps?scheduleUuid=xxx ========

// GetYingDaoSubApps 拉某 schedule 下的子应用列表 (前端 platform mapping 下拉用)
// scheduleUuid 缺省 = 集团数据看板 (跑哥实际用的那个)
// 5 min 缓存, 加 ?refresh=1 强刷
func (dh *DashboardHandler) GetYingDaoSubApps(w http.ResponseWriter, r *http.Request) {
	if dh.YingDao == nil || !dh.YingDao.Configured() {
		writeError(w, 500, "影刀未配置")
		return
	}
	scheduleUuid := r.URL.Query().Get("scheduleUuid")
	if scheduleUuid == "" {
		scheduleUuid = defaultGroupDashboardScheduleUuid
	}
	refresh := r.URL.Query().Get("refresh") == "1"

	yingdaoSubAppsMu.Lock()
	if yingdaoSubAppsCache == nil {
		yingdaoSubAppsCache = make(map[string][]yingdao.RobotInScheduleDetail)
		yingdaoSubAppsCachedAt = make(map[string]time.Time)
	}
	if !refresh {
		if cached, ok := yingdaoSubAppsCache[scheduleUuid]; ok {
			if time.Since(yingdaoSubAppsCachedAt[scheduleUuid]) < yingdaoTasksCacheTTL {
				yingdaoSubAppsMu.Unlock()
				writeJSON(w, cached)
				return
			}
		}
	}
	yingdaoSubAppsMu.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	detail, err := dh.YingDao.GetScheduleDetail(ctx, scheduleUuid)
	if err != nil {
		writeServerError(w, 500, "拉影刀任务详情失败", err)
		return
	}
	yingdaoSubAppsMu.Lock()
	yingdaoSubAppsCache[scheduleUuid] = detail.RobotList
	yingdaoSubAppsCachedAt[scheduleUuid] = time.Now()
	yingdaoSubAppsMu.Unlock()
	writeJSON(w, detail.RobotList)
}

// ======== 后台状态巡检 (避免 trigger_log status 卡 running) ========

// StartYingDaoStatusReaper 后台 goroutine 每 30s 扫所有 running 任务,
// 主动调影刀查状态, 终态的更新 trigger_log + 发钉钉通知
// 解决问题: 跑哥批量触发后不打开 Modal, 状态没人刷, Badge 数字一直不降
func (dh *DashboardHandler) StartYingDaoStatusReaper() {
	if dh.YingDao == nil || !dh.YingDao.Configured() {
		return
	}
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		// 启动 5s 后先跑一次 (避免冷启动看到旧 running 数据)
		time.Sleep(5 * time.Second)
		dh.reapRunningRPATasks()
		for range ticker.C {
			dh.reapRunningRPATasks()
		}
	}()
}

// reapRunningRPATasks 扫所有 status='running' 任务, 调影刀更新终态
func (dh *DashboardHandler) reapRunningRPATasks() {
	defer func() {
		if r := recover(); r != nil {
			// 防 panic 影响其他任务
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// 只扫 6 小时内的 running 任务 (太老的认为僵死, 不再调影刀, 后面用 SQL 标 timeout)
	rows, err := dh.DB.QueryContext(ctx,
		`SELECT id, platform, COALESCE(robot_name,''), job_uuid, started_at
		 FROM rpa_trigger_log
		 WHERE status='running'
		   AND started_at > NOW() - INTERVAL 6 HOUR
		 ORDER BY id ASC LIMIT 50`)
	if err != nil {
		return
	}
	defer rows.Close()
	type task struct {
		ID        int64
		Platform  string
		RobotName string
		JobUuid   string
		StartedAt time.Time
	}
	var list []task
	for rows.Next() {
		var t task
		if err := rows.Scan(&t.ID, &t.Platform, &t.RobotName, &t.JobUuid, &t.StartedAt); err == nil {
			list = append(list, t)
		}
	}

	// 同时把 6 小时以上的 running 标 timeout (避免永远占着)
	_, _ = dh.DB.ExecContext(ctx,
		`UPDATE rpa_trigger_log SET status='timeout', finished_at=NOW(),
		 result_msg='超过 6 小时未拿到影刀终态'
		 WHERE status='running' AND started_at <= NOW() - INTERVAL 6 HOUR`)

	// 逐个查影刀状态
	for _, t := range list {
		js, err := dh.YingDao.QueryJob(ctx, t.JobUuid)
		if err != nil || js == nil {
			continue
		}
		switch js.Status {
		case "finish", "error", "cancel", "fail":
			newStatus := js.Status
			if newStatus == "fail" {
				newStatus = "error"
			}
			msg := js.Remark
			if msg == "" {
				msg = js.StatusName
			}
			_, _ = dh.DB.ExecContext(ctx,
				`UPDATE rpa_trigger_log SET status=?, result_msg=?, finished_at=NOW() WHERE id=? AND status='running'`,
				newStatus, msg, t.ID)
			// 算耗时发钉钉
			elapsed := int(time.Since(t.StartedAt).Seconds())
			go dh.notifyRPADone(t.Platform, t.RobotName, newStatus, elapsed, msg)
		}
		// 防止短时间打满影刀 API
		time.Sleep(200 * time.Millisecond)
	}
}

// ======== helpers ========

// getCurrentUserName 从 RequireAuth 注入的 authPayload 拿当前用户名
// 优先取 RealName (展示用), 退化到 Username, 都没有返回 unknown
func getCurrentUserName(r *http.Request) string {
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		return "unknown"
	}
	if payload.User.RealName != "" {
		return payload.User.RealName
	}
	if payload.User.Username != "" {
		return payload.User.Username
	}
	return "unknown"
}
