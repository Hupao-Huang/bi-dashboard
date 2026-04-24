package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// AuditEntry 记录一条审计日志（异步写入，不阻塞请求）
func (h *DashboardHandler) logAudit(r *http.Request, action, resource string, detail interface{}) {
	ip := clientIP(r)
	ua := truncateString(r.UserAgent(), 255)

	payload, ok := authPayloadFromContext(r)
	var userID int64
	var username, realName string
	if ok && payload != nil {
		userID = payload.User.ID
		username = payload.User.Username
		realName = payload.User.RealName
	}

	var detailStr string
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil {
			detailStr = string(b)
		}
	}

	go func() {
		_, _ = h.DB.Exec(
			`INSERT INTO audit_logs (user_id, username, real_name, action, resource, detail, ip, user_agent) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			userID, username, realName, action, resource, detailStr, ip, ua,
		)
	}()
}

// logAuditNoRequest 不依赖 http.Request 的审计记录（用于 Login/Logout 等场景）
func (h *DashboardHandler) logAuditNoRequest(userID int64, username, realName, action, resource, detail, ip, ua string) {
	go func() {
		_, _ = h.DB.Exec(
			`INSERT INTO audit_logs (user_id, username, real_name, action, resource, detail, ip, user_agent) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			userID, username, realName, action, resource, detail, ip, ua,
		)
	}()
}

// AuditLogPageView POST /api/audit/page-view — 前端页面切换时调用
func (h *DashboardHandler) AuditLogPageView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	h.logAudit(r, "page_view", req.Path, nil)
	writeJSON(w, map[string]string{"message": "ok"})
}

// AdminAuditLogs GET /api/admin/audit-logs — 管理端查询审计日志
func (h *DashboardHandler) AdminAuditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("pageSize"))
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}

	conditions := []string{}
	args := []interface{}{}

	if action := strings.TrimSpace(q.Get("action")); action != "" {
		conditions = append(conditions, "action = ?")
		args = append(args, action)
	}
	if username := strings.TrimSpace(q.Get("username")); username != "" {
		conditions = append(conditions, "username LIKE ?")
		args = append(args, "%"+username+"%")
	}
	if startDate := strings.TrimSpace(q.Get("startDate")); startDate != "" {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, startDate+" 00:00:00")
	}
	if endDate := strings.TrimSpace(q.Get("endDate")); endDate != "" {
		conditions = append(conditions, "created_at <= ?")
		args = append(args, endDate+" 23:59:59")
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM audit_logs %s", where)
	if err := h.DB.QueryRow(countSQL, args...).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, "query count failed")
		return
	}

	offset := (page - 1) * pageSize
	querySQL := fmt.Sprintf(
		"SELECT id, IFNULL(user_id,0), username, real_name, action, resource, IFNULL(detail,''), IFNULL(ip,''), IFNULL(user_agent,''), created_at FROM audit_logs %s ORDER BY id DESC LIMIT ? OFFSET ?",
		where,
	)
	args = append(args, pageSize, offset)
	rows, err := h.DB.Query(querySQL, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type auditLogEntry struct {
		ID        int64  `json:"id"`
		UserID    int64  `json:"userId"`
		Username  string `json:"username"`
		RealName  string `json:"realName"`
		Action    string `json:"action"`
		Resource  string `json:"resource"`
		Detail    string `json:"detail"`
		IP        string `json:"ip"`
		UserAgent string `json:"userAgent"`
		CreatedAt string `json:"createdAt"`
	}

	logs := []auditLogEntry{}
	for rows.Next() {
		var e auditLogEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.Username, &e.RealName, &e.Action, &e.Resource, &e.Detail, &e.IP, &e.UserAgent, &e.CreatedAt); err != nil {
			continue
		}
		logs = append(logs, e)
	}

	writeJSON(w, map[string]interface{}{
		"list":     logs,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}
