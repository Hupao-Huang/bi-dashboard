package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// 状态机：pending → accepted → scheduled → in_progress → done
//                ↓               ↓
//             rejected         shelved
var validReqStatus = map[string]bool{
	"pending": true, "accepted": true, "scheduled": true, "in_progress": true,
	"done": true, "shelved": true, "rejected": true,
}

var validReqPriority = map[string]bool{
	"P0": true, "P1": true, "P2": true, "P3": true,
}

type requirementItem struct {
	ID              int64           `json:"id"`
	Title           string          `json:"title"`
	Content         string          `json:"content"`
	SubmitterUserID int64           `json:"submitterUserId"`
	SubmitterName   string          `json:"submitterName"`
	SubmitterDept   string          `json:"submitterDept"`
	Priority        string          `json:"priority"`
	Status          string          `json:"status"`
	TargetVersion   string          `json:"targetVersion"`
	ExpectedDate    *string         `json:"expectedDate"`
	ActualDate      *string         `json:"actualDate"`
	Tag             string          `json:"tag"`
	AdminRemark     string          `json:"adminRemark"`
	Attachments     json.RawMessage `json:"attachments"`
	CreatedAt       string          `json:"createdAt"`
	UpdatedAt       string          `json:"updatedAt"`
}

// SubmitRequirement 提交需求（任何登录用户）
// POST /api/requirements
func (h *DashboardHandler) SubmitRequirement(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, 405, "method not allowed")
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, 401, "unauthorized")
		return
	}

	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
		Tag     string `json:"tag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "无效请求")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		writeError(w, 400, "需求标题不能为空")
		return
	}
	if len(req.Title) > 255 {
		writeError(w, 400, "标题过长（255 字以内）")
		return
	}

	// 提交人信息从 session 自动填
	dept := ""
	if len(payload.DataScopes.Depts) > 0 {
		dept = payload.DataScopes.Depts[0]
	}

	res, err := h.DB.Exec(
		`INSERT INTO requirements (title, content, submitter_user_id, submitter_name, submitter_dept, tag)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		req.Title, req.Content, payload.User.ID, payload.User.RealName, dept, req.Tag,
	)
	if err != nil {
		writeServerError(w, 500, "保存需求失败", err)
		return
	}
	id, _ := res.LastInsertId()

	// 钉钉通知跑哥/管理员有新需求
	if h.Notifier != nil {
		h.notifyNewRequirement(id, req.Title, payload.User.RealName)
	}

	writeJSON(w, map[string]interface{}{"id": id, "message": "需求已提交"})
}

// ListRequirements 列表（任何登录用户；非管理员只看自己提的）
// GET /api/requirements/list?status=&priority=&target_version=&page=&pageSize=&scope=mine|all
func (h *DashboardHandler) ListRequirements(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeError(w, 405, "method not allowed")
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, 401, "unauthorized")
		return
	}

	isAdmin := hasPermission(payload, "requirement.manage")
	scope := r.URL.Query().Get("scope") // mine | all
	status := r.URL.Query().Get("status")
	priority := r.URL.Query().Get("priority")
	targetVer := r.URL.Query().Get("target_version")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}

	where := "1=1"
	args := []interface{}{}
	if !isAdmin || scope == "mine" {
		where += " AND submitter_user_id = ?"
		args = append(args, payload.User.ID)
	}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	if priority != "" {
		where += " AND priority = ?"
		args = append(args, priority)
	}
	if targetVer != "" {
		where += " AND target_version = ?"
		args = append(args, targetVer)
	}

	var total int
	h.DB.QueryRow("SELECT COUNT(*) FROM requirements WHERE "+where, args...).Scan(&total)

	queryArgs := append(args, pageSize, (page-1)*pageSize)
	rows, err := h.DB.Query(
		`SELECT id, title, IFNULL(content,''), submitter_user_id, submitter_name, IFNULL(submitter_dept,''),
		        priority, status, IFNULL(target_version,''),
		        DATE_FORMAT(expected_date,'%Y-%m-%d'), DATE_FORMAT(actual_date,'%Y-%m-%d'),
		        IFNULL(tag,''), IFNULL(admin_remark,''), attachments,
		        created_at, updated_at
		 FROM requirements WHERE `+where+
			` ORDER BY FIELD(priority,'P0','P1','P2','P3'), created_at DESC LIMIT ? OFFSET ?`,
		queryArgs...,
	)
	if err != nil {
		writeServerError(w, 500, "查询需求失败", err)
		return
	}
	defer rows.Close()

	list := []requirementItem{}
	for rows.Next() {
		var item requirementItem
		var attachRaw []byte
		var expDate, actDate sql.NullString
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&item.ID, &item.Title, &item.Content,
			&item.SubmitterUserID, &item.SubmitterName, &item.SubmitterDept,
			&item.Priority, &item.Status, &item.TargetVersion,
			&expDate, &actDate, &item.Tag, &item.AdminRemark, &attachRaw,
			&createdAt, &updatedAt); err != nil {
			writeError(w, 500, "读取数据失败")
			return
		}
		if expDate.Valid {
			item.ExpectedDate = &expDate.String
		}
		if actDate.Valid {
			item.ActualDate = &actDate.String
		}
		item.CreatedAt = createdAt.Format("2006-01-02 15:04:05")
		item.UpdatedAt = updatedAt.Format("2006-01-02 15:04:05")
		if attachRaw != nil {
			item.Attachments = json.RawMessage(attachRaw)
		} else {
			item.Attachments = json.RawMessage("[]")
		}
		list = append(list, item)
	}

	writeJSON(w, map[string]interface{}{
		"list":     list,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
		"isAdmin":  isAdmin,
	})
}

// RequirementByPath PUT/DELETE 单条
// PUT /api/requirements/{id} — 更新状态/优先级/排期/备注（requirement.manage）
// DELETE /api/requirements/{id} — 删除（requirement.manage）
func (h *DashboardHandler) RequirementByPath(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/requirements/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, 400, "无效的需求ID")
		return
	}

	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, 401, "unauthorized")
		return
	}
	if !hasPermission(payload, "requirement.manage") {
		writeError(w, 403, "无权操作")
		return
	}

	switch r.Method {
	case "PUT":
		var req struct {
			Status        string  `json:"status"`
			Priority      string  `json:"priority"`
			TargetVersion string  `json:"targetVersion"`
			ExpectedDate  *string `json:"expectedDate"`
			Tag           string  `json:"tag"`
			AdminRemark   string  `json:"adminRemark"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, 400, "无效请求")
			return
		}
		if req.Status != "" && !validReqStatus[req.Status] {
			writeError(w, 400, "无效状态")
			return
		}
		if req.Priority != "" && !validReqPriority[req.Priority] {
			writeError(w, 400, "无效优先级")
			return
		}

		sets := []string{}
		args := []interface{}{}
		if req.Status != "" {
			sets = append(sets, "status=?")
			args = append(args, req.Status)
			if req.Status == "done" {
				sets = append(sets, "actual_date=CURDATE()")
			}
		}
		if req.Priority != "" {
			sets = append(sets, "priority=?")
			args = append(args, req.Priority)
		}
		if req.TargetVersion != "" {
			sets = append(sets, "target_version=?")
			args = append(args, req.TargetVersion)
		}
		if req.ExpectedDate != nil {
			if *req.ExpectedDate == "" {
				sets = append(sets, "expected_date=NULL")
			} else {
				sets = append(sets, "expected_date=?")
				args = append(args, *req.ExpectedDate)
			}
		}
		if req.Tag != "" {
			sets = append(sets, "tag=?")
			args = append(args, req.Tag)
		}
		if req.AdminRemark != "" {
			sets = append(sets, "admin_remark=?")
			args = append(args, req.AdminRemark)
		}
		if len(sets) == 0 {
			writeError(w, 400, "无可更新字段")
			return
		}
		args = append(args, id)

		_, err = h.DB.Exec("UPDATE requirements SET "+strings.Join(sets, ",")+" WHERE id=?", args...)
		if err != nil {
			writeServerError(w, 500, "更新需求失败", err)
			return
		}

		// 关键状态变更通知提需求人
		if req.Status == "accepted" || req.Status == "done" || req.Status == "rejected" || req.Status == "shelved" {
			if h.Notifier != nil {
				h.notifyRequirementStatus(id, req.Status, payload.User.RealName)
			}
		}

		writeJSON(w, map[string]string{"message": "更新成功"})

	case "DELETE":
		_, err = h.DB.Exec("DELETE FROM requirements WHERE id=?", id)
		if err != nil {
			writeServerError(w, 500, "删除需求失败", err)
			return
		}
		writeJSON(w, map[string]string{"message": "已删除"})

	default:
		writeError(w, 405, "method not allowed")
	}
}

// RequirementStats 状态/优先级计数（任何登录用户；非管理员只统计自己提的）
// GET /api/requirements/stats
func (h *DashboardHandler) RequirementStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeError(w, 405, "method not allowed")
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, 401, "unauthorized")
		return
	}

	isAdmin := hasPermission(payload, "requirement.manage")
	scope := r.URL.Query().Get("scope")

	where := "1=1"
	args := []interface{}{}
	if !isAdmin || scope == "mine" {
		where += " AND submitter_user_id = ?"
		args = append(args, payload.User.ID)
	}

	statusStats := map[string]int{
		"pending": 0, "accepted": 0, "scheduled": 0, "in_progress": 0,
		"done": 0, "shelved": 0, "rejected": 0,
	}
	rows, err := h.DB.Query("SELECT status, COUNT(*) FROM requirements WHERE "+where+" GROUP BY status", args...)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var s string
			var n int
			if err := rows.Scan(&s, &n); err == nil {
				statusStats[s] = n
			}
		}
	}

	priorityStats := map[string]int{"P0": 0, "P1": 0, "P2": 0, "P3": 0}
	rows2, err := h.DB.Query("SELECT priority, COUNT(*) FROM requirements WHERE "+where+" GROUP BY priority", args...)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var p string
			var n int
			if err := rows2.Scan(&p, &n); err == nil {
				priorityStats[p] = n
			}
		}
	}

	writeJSON(w, map[string]interface{}{
		"status":   statusStats,
		"priority": priorityStats,
	})
}

// RequirementGantt 甘特图数据（按目标版本号聚合，requirement.manage）
// GET /api/requirements/gantt
func (h *DashboardHandler) RequirementGantt(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeError(w, 405, "method not allowed")
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, 401, "unauthorized")
		return
	}
	if !hasPermission(payload, "requirement.manage") {
		writeError(w, 403, "无权访问")
		return
	}

	// 主轴：在排期或已上线的版本
	rows, err := h.DB.Query(`
		SELECT id, title, priority, status, IFNULL(target_version,''),
		       DATE_FORMAT(expected_date,'%Y-%m-%d'),
		       DATE_FORMAT(actual_date,'%Y-%m-%d'),
		       submitter_name
		FROM requirements
		WHERE status NOT IN ('rejected','shelved')
		ORDER BY target_version, FIELD(priority,'P0','P1','P2','P3'), created_at
	`)
	if err != nil {
		writeServerError(w, 500, "查询甘特数据失败", err)
		return
	}
	defer rows.Close()

	type ganttRow struct {
		ID            int64   `json:"id"`
		Title         string  `json:"title"`
		Priority      string  `json:"priority"`
		Status        string  `json:"status"`
		TargetVersion string  `json:"targetVersion"`
		ExpectedDate  *string `json:"expectedDate"`
		ActualDate    *string `json:"actualDate"`
		Submitter     string  `json:"submitter"`
	}
	main := []ganttRow{}
	for rows.Next() {
		var g ganttRow
		var expD, actD sql.NullString
		if err := rows.Scan(&g.ID, &g.Title, &g.Priority, &g.Status, &g.TargetVersion,
			&expD, &actD, &g.Submitter); err == nil {
			if expD.Valid {
				g.ExpectedDate = &expD.String
			}
			if actD.Valid {
				g.ActualDate = &actD.String
			}
			main = append(main, g)
		}
	}

	// 暂缓清单
	rows2, _ := h.DB.Query(`
		SELECT id, title, priority, status, IFNULL(target_version,''), submitter_name
		FROM requirements WHERE status IN ('shelved','rejected')
		ORDER BY status, FIELD(priority,'P0','P1','P2','P3'), created_at`)
	defer rows2.Close()
	shelved := []ganttRow{}
	for rows2.Next() {
		var g ganttRow
		if err := rows2.Scan(&g.ID, &g.Title, &g.Priority, &g.Status, &g.TargetVersion, &g.Submitter); err == nil {
			shelved = append(shelved, g)
		}
	}

	writeJSON(w, map[string]interface{}{
		"main":    main,
		"shelved": shelved,
	})
}

// notifyNewRequirement 新需求提交时通知所有有 requirement.manage 权限的人
func (h *DashboardHandler) notifyNewRequirement(reqID int64, title, submitter string) {
	rows, err := h.DB.Query(`
		SELECT DISTINCT u.dingtalk_userid FROM users u
		JOIN user_roles ur ON ur.user_id = u.id
		JOIN role_permissions rp ON rp.role_id = ur.role_id
		WHERE rp.permission_code = 'requirement.manage'
		  AND u.dingtalk_userid IS NOT NULL AND u.dingtalk_userid <> ''
		  AND u.status = 'active'`)
	if err != nil {
		return
	}
	defer rows.Close()
	var unionIDs []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err == nil && uid != "" {
			unionIDs = append(unionIDs, uid)
		}
	}
	if len(unionIDs) == 0 {
		return
	}
	msg := "【新需求】" + submitter + " 提交了一条需求：" + title
	h.Notifier.SendTextAsync(unionIDs, msg)
}

// notifyRequirementStatus 状态变更通知提需求人
func (h *DashboardHandler) notifyRequirementStatus(reqID int64, status, operator string) {
	var (
		title          string
		submitterUnion sql.NullString
	)
	err := h.DB.QueryRow(`
		SELECT r.title, u.dingtalk_userid
		FROM requirements r LEFT JOIN users u ON u.id = r.submitter_user_id
		WHERE r.id = ?`, reqID).Scan(&title, &submitterUnion)
	if err != nil || !submitterUnion.Valid || submitterUnion.String == "" {
		return
	}
	statusLabel := map[string]string{
		"accepted": "已接受", "done": "已完成", "rejected": "已拒绝", "shelved": "已搁置",
	}[status]
	if statusLabel == "" {
		return
	}
	msg := "【需求状态更新】您提的需求《" + title + "》" + statusLabel + "（操作人：" + operator + "）"
	h.Notifier.SendTextAsync([]string{submitterUnion.String}, msg)
}
