package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Notice struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Type      string `json:"type"`
	IsPinned  bool   `json:"isPinned"`
	IsActive  bool   `json:"isActive"`
	CreatedBy string `json:"createdBy"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// GetNotices 获取公告列表（所有登录用户可用，只返回启用的）
func (h *DashboardHandler) GetNotices(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(`
		SELECT id, title, content, type, is_pinned, is_active, created_by,
			DATE_FORMAT(created_at,'%Y-%m-%d %H:%i') as created_at,
			DATE_FORMAT(updated_at,'%Y-%m-%d %H:%i') as updated_at
		FROM notices WHERE is_active = 1
		ORDER BY is_pinned DESC, created_at DESC
		LIMIT 50`)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	defer rows.Close()

	var notices []Notice
	for rows.Next() {
		var n Notice
		if err := rows.Scan(&n.ID, &n.Title, &n.Content, &n.Type, &n.IsPinned, &n.IsActive, &n.CreatedBy, &n.CreatedAt, &n.UpdatedAt); err != nil {
			writeError(w, 500, err.Error())
			return
		}
		notices = append(notices, n)
	}
	if notices == nil {
		notices = []Notice{}
	}
	writeJSON(w, map[string]interface{}{"notices": notices})
}

// AdminListNotices 管理员获取所有公告（含禁用的）
func (h *DashboardHandler) AdminListNotices(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(`
		SELECT id, title, content, type, is_pinned, is_active, created_by,
			DATE_FORMAT(created_at,'%Y-%m-%d %H:%i') as created_at,
			DATE_FORMAT(updated_at,'%Y-%m-%d %H:%i') as updated_at
		FROM notices
		ORDER BY is_pinned DESC, created_at DESC`)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	defer rows.Close()

	var notices []Notice
	for rows.Next() {
		var n Notice
		if err := rows.Scan(&n.ID, &n.Title, &n.Content, &n.Type, &n.IsPinned, &n.IsActive, &n.CreatedBy, &n.CreatedAt, &n.UpdatedAt); err != nil {
			writeError(w, 500, err.Error())
			return
		}
		notices = append(notices, n)
	}
	if notices == nil {
		notices = []Notice{}
	}
	writeJSON(w, map[string]interface{}{"notices": notices})
}

// CreateNotice 创建公告
func (h *DashboardHandler) CreateNotice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	var req struct {
		Title    string `json:"title"`
		Content  string `json:"content"`
		Type     string `json:"type"`
		IsPinned bool   `json:"isPinned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid request")
		return
	}
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Content) == "" {
		writeError(w, 400, "标题和内容不能为空")
		return
	}
	if req.Type == "" {
		req.Type = "update"
	}

	// 获取当前用户名
	username := "admin"
	if payload, ok := authPayloadFromContext(r); ok && payload != nil {
		username = payload.User.Username
	}

	result, err := h.DB.Exec(`INSERT INTO notices (title, content, type, is_pinned, created_by) VALUES (?,?,?,?,?)`,
		req.Title, req.Content, req.Type, req.IsPinned, username)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	id, _ := result.LastInsertId()
	writeJSON(w, map[string]interface{}{"id": id, "message": "创建成功"})
}

// UpdateNotice 更新公告
func (h *DashboardHandler) UpdateNotice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(w, 405, "method not allowed")
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/api/admin/notices/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}

	var req struct {
		Title    *string `json:"title"`
		Content  *string `json:"content"`
		Type     *string `json:"type"`
		IsPinned *bool   `json:"isPinned"`
		IsActive *bool   `json:"isActive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid request")
		return
	}

	sets := []string{}
	args := []interface{}{}
	if req.Title != nil {
		sets = append(sets, "title=?")
		args = append(args, *req.Title)
	}
	if req.Content != nil {
		sets = append(sets, "content=?")
		args = append(args, *req.Content)
	}
	if req.Type != nil {
		sets = append(sets, "type=?")
		args = append(args, *req.Type)
	}
	if req.IsPinned != nil {
		sets = append(sets, "is_pinned=?")
		args = append(args, *req.IsPinned)
	}
	if req.IsActive != nil {
		sets = append(sets, "is_active=?")
		args = append(args, *req.IsActive)
	}
	if len(sets) == 0 {
		writeError(w, 400, "没有要更新的字段")
		return
	}

	args = append(args, time.Now(), id)
	_, err = h.DB.Exec("UPDATE notices SET "+strings.Join(sets, ",")+",updated_at=? WHERE id=?",
		args...)
	if err != nil {
		log.Printf("[notice] 更新公告失败 id=%d: %v", id, err)
		writeError(w, 500, "更新公告失败")
		return
	}
	writeJSON(w, map[string]interface{}{"message": "更新成功"})
}

// DeleteNotice 删除公告
func (h *DashboardHandler) DeleteNotice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, 405, "method not allowed")
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/api/admin/notices/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, 400, "invalid id")
		return
	}
	_, err = h.DB.Exec("DELETE FROM notices WHERE id=?", id)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"message": "删除成功"})
}

// NoticeByPath 路由分发（PUT/DELETE）
func (h *DashboardHandler) NoticeByPath(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut:
		h.UpdateNotice(w, r)
	case http.MethodDelete:
		h.DeleteNotice(w, r)
	default:
		writeError(w, 405, "method not allowed")
	}
}

