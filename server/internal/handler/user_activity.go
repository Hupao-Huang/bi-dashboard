package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
)

// user_activity.go — 用户操作活动
//   个人中心「我的活动」(看自己) + 系统设置「全员活动」(管理员看所有人)
// 数据全部来自 audit_logs（page_view/login/logout/export/permission_change 等），纯只读聚合。
// 动作/资源→中文文案的翻译交给前端用 navigation.tsx 的 pageTitleMap 做，后端只回原始字段。

// 活动统计：今日 / 近7天 / 近30天 / 总操作 / 活跃天数
type activityStats struct {
	Today      int `json:"today"`
	Last7d     int `json:"last7d"`
	Last30d    int `json:"last30d"`
	Total      int `json:"total"`
	ActiveDays int `json:"activeDays"`
}

// 热力图一格：某天 + 当天操作次数
type activityHeatCell struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// 一条操作记录（动作/资源/详情原文）
type activityRecord struct {
	Action    string `json:"action"`
	Resource  string `json:"resource"`
	Detail    string `json:"detail"`
	CreatedAt string `json:"createdAt"`
}

type userActivity struct {
	Stats   activityStats      `json:"stats"`
	Heatmap []activityHeatCell `json:"heatmap"`
	Recent  []activityRecord   `json:"recent"`
}

// buildUserActivity 算某个用户的活动数据（统计 + 近半年热力图 + 最近 15 条记录）。
// 出错时已写好响应，返回 ok=false；三个接口共用此函数，避免重复。
func (h *DashboardHandler) buildUserActivity(w http.ResponseWriter, r *http.Request, userID int64) (*userActivity, bool) {
	out := &userActivity{Heatmap: []activityHeatCell{}, Recent: []activityRecord{}}

	// 统计：今日 / 近7天 / 近30天 / 总操作 / 活跃天数
	statRows, ok := queryRowsOrWriteError(w, r, h.DB, `
		SELECT
			COUNT(*) AS total,
			COUNT(CASE WHEN created_at >= CURDATE() THEN 1 END) AS today,
			COUNT(CASE WHEN created_at >= DATE_SUB(CURDATE(), INTERVAL 6 DAY) THEN 1 END) AS d7,
			COUNT(CASE WHEN created_at >= DATE_SUB(CURDATE(), INTERVAL 29 DAY) THEN 1 END) AS d30,
			COUNT(DISTINCT DATE(created_at)) AS active_days
		FROM audit_logs WHERE user_id = ?`, userID)
	if !ok {
		return nil, false
	}
	if statRows.Next() {
		if err := statRows.Scan(&out.Stats.Total, &out.Stats.Today, &out.Stats.Last7d, &out.Stats.Last30d, &out.Stats.ActiveDays); err != nil {
			statRows.Close()
			writeError(w, http.StatusInternalServerError, "scan activity stats failed")
			return nil, false
		}
	}
	statRows.Close()

	// 近半年（182 天）每天操作次数 → 热力图（只回有操作的天，前端补零）
	heatRows, ok := queryRowsOrWriteError(w, r, h.DB, `
		SELECT DATE_FORMAT(created_at, '%Y-%m-%d') AS d, COUNT(*) AS c
		FROM audit_logs
		WHERE user_id = ? AND created_at >= DATE_SUB(CURDATE(), INTERVAL 181 DAY)
		GROUP BY d ORDER BY d`, userID)
	if !ok {
		return nil, false
	}
	for heatRows.Next() {
		var cell activityHeatCell
		if err := heatRows.Scan(&cell.Date, &cell.Count); err != nil {
			heatRows.Close()
			writeError(w, http.StatusInternalServerError, "scan activity heatmap failed")
			return nil, false
		}
		out.Heatmap = append(out.Heatmap, cell)
	}
	if err := heatRows.Err(); err != nil {
		heatRows.Close()
		writeError(w, http.StatusInternalServerError, "iterate activity heatmap failed")
		return nil, false
	}
	heatRows.Close()

	// 最近 15 条操作
	recentRows, ok := queryRowsOrWriteError(w, r, h.DB, `
		SELECT action, resource, IFNULL(detail, ''), DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:%s')
		FROM audit_logs WHERE user_id = ? ORDER BY id DESC LIMIT 15`, userID)
	if !ok {
		return nil, false
	}
	for recentRows.Next() {
		var rec activityRecord
		if err := recentRows.Scan(&rec.Action, &rec.Resource, &rec.Detail, &rec.CreatedAt); err != nil {
			recentRows.Close()
			writeError(w, http.StatusInternalServerError, "scan activity recent failed")
			return nil, false
		}
		out.Recent = append(out.Recent, rec)
	}
	if err := recentRows.Err(); err != nil {
		recentRows.Close()
		writeError(w, http.StatusInternalServerError, "iterate activity recent failed")
		return nil, false
	}
	recentRows.Close()

	return out, true
}

// UserActivity GET /api/user/activity — 当前登录用户看自己的活动（任何登录用户）。
// 安全红线：用户 ID 只从登录态取，绝不接受前端传入，防止越权看别人。
func (h *DashboardHandler) UserActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	act, ok := h.buildUserActivity(w, r, payload.User.ID)
	if !ok {
		return
	}
	writeJSON(w, act)
}

// AdminUserActivity GET /api/admin/user-activity?userId=X — 管理员看指定用户的活动（user.manage）。
func (h *DashboardHandler) AdminUserActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	userID, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("userId")), 10, 64)
	if err != nil || userID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid userId")
		return
	}
	act, ok := h.buildUserActivity(w, r, userID)
	if !ok {
		return
	}
	writeJSON(w, act)
}

// AdminUsersActivity GET /api/admin/users-activity — 全员活动汇总表（user.manage）。
// 每个用户一行统计，给「系统设置 → 在线用户 → 全员活动」那张表用；LEFT JOIN 保证没操作过的人也在列。
func (h *DashboardHandler) AdminUsersActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	rows, ok := queryRowsOrWriteError(w, r, h.DB, `
		SELECT u.id, u.username, IFNULL(u.real_name, ''), IFNULL(u.dingtalk_real_name, ''),
			IFNULL(u.department, ''), u.status,
			COUNT(a.id) AS total,
			COUNT(CASE WHEN a.created_at >= CURDATE() THEN 1 END) AS today,
			COUNT(CASE WHEN a.created_at >= DATE_SUB(CURDATE(), INTERVAL 6 DAY) THEN 1 END) AS d7,
			COUNT(CASE WHEN a.created_at >= DATE_SUB(CURDATE(), INTERVAL 29 DAY) THEN 1 END) AS d30,
			COUNT(DISTINCT DATE(a.created_at)) AS active_days,
			DATE_FORMAT(MAX(a.created_at), '%Y-%m-%d %H:%i:%s') AS last_active
		FROM users u
		LEFT JOIN audit_logs a ON a.user_id = u.id
		GROUP BY u.id, u.username, u.real_name, u.dingtalk_real_name, u.department, u.status
		ORDER BY total DESC, u.id`)
	if !ok {
		return
	}
	defer rows.Close()

	type userActivityRow struct {
		UserID           int64  `json:"userId"`
		Username         string `json:"username"`
		RealName         string `json:"realName"`
		DingtalkRealName string `json:"dingtalkRealName"`
		Department       string `json:"department"`
		Status           string `json:"status"`
		Total            int    `json:"total"`
		Today            int    `json:"today"`
		Last7d           int    `json:"last7d"`
		Last30d          int    `json:"last30d"`
		ActiveDays       int    `json:"activeDays"`
		LastActiveAt     string `json:"lastActiveAt"`
	}
	out := []userActivityRow{}
	for rows.Next() {
		var x userActivityRow
		var lastActive sql.NullString
		if err := rows.Scan(&x.UserID, &x.Username, &x.RealName, &x.DingtalkRealName, &x.Department, &x.Status,
			&x.Total, &x.Today, &x.Last7d, &x.Last30d, &x.ActiveDays, &lastActive); err != nil {
			writeError(w, http.StatusInternalServerError, "scan users activity failed")
			return
		}
		if lastActive.Valid {
			x.LastActiveAt = lastActive.String
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "iterate users activity failed")
		return
	}
	writeJSON(w, map[string]interface{}{"users": out, "count": len(out)})
}
