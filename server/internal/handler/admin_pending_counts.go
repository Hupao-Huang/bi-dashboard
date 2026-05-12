package handler

import (
	"net/http"
)

// AdminPendingCounts 返回当前管理员关心的待办数（用户管理/反馈管理）。
// 没有对应权限的字段返回 0，前端用于菜单徽标显示。
func (h *DashboardHandler) AdminPendingCounts(w http.ResponseWriter, r *http.Request) {
	payload, _ := authPayloadFromContext(r)

	resp := map[string]int{"users": 0, "feedback": 0}

	if hasPermission(payload, "user.manage") {
		var n int
		if err := h.DB.QueryRow(`SELECT COUNT(*) FROM users WHERE status='pending'`).Scan(&n); err == nil {
			resp["users"] = n
		}
	}

	if hasPermission(payload, "feedback.manage") {
		var n int
		if err := h.DB.QueryRow(`SELECT COUNT(*) FROM feedback WHERE status='pending'`).Scan(&n); err == nil {
			resp["feedback"] = n
		}
	}

	writeJSON(w, resp)
}
