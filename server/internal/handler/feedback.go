package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	uploadDir      = `C:\Users\Administrator\bi-dashboard\server\uploads\feedback`
	maxUploadSize  = 10 << 20 // 10MB
	maxAttachments = 5
)

// SubmitFeedback 提交反馈
// POST /api/feedback (multipart/form-data)
func (h *DashboardHandler) SubmitFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, 405, "method not allowed")
		return
	}

	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, 401, "unauthorized")
		return
	}

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, 400, "请求过大或格式错误")
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	content := strings.TrimSpace(r.FormValue("content"))
	pageURL := strings.TrimSpace(r.FormValue("pageUrl"))

	if title == "" {
		writeError(w, 400, "标题不能为空")
		return
	}
	if content == "" {
		writeError(w, 400, "描述不能为空")
		return
	}

	// 处理文件上传
	var attachments []string
	if r.MultipartForm != nil && r.MultipartForm.File != nil {
		files := r.MultipartForm.File["files"]
		if len(files) > maxAttachments {
			writeError(w, 400, fmt.Sprintf("最多上传%d个文件", maxAttachments))
			return
		}

		os.MkdirAll(uploadDir, 0755)
		dateDir := time.Now().Format("20060102")
		dayDir := filepath.Join(uploadDir, dateDir)
		os.MkdirAll(dayDir, 0755)

		for _, fh := range files {
			if fh.Size > maxUploadSize {
				writeError(w, 400, "单个文件不能超过10MB")
				return
			}

			ext := filepath.Ext(fh.Filename)
			allowed := map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".bmp": true}
			if !allowed[strings.ToLower(ext)] {
				writeError(w, 400, "仅支持图片文件(png/jpg/gif/webp/bmp)")
				return
			}

			src, err := fh.Open()
			if err != nil {
				writeError(w, 500, "读取文件失败")
				return
			}

			randBytes := make([]byte, 8)
			rand.Read(randBytes)
			filename := hex.EncodeToString(randBytes) + ext
			dstPath := filepath.Join(dayDir, filename)

			dst, err := os.Create(dstPath)
			if err != nil {
				src.Close()
				writeError(w, 500, "保存文件失败")
				return
			}

			io.Copy(dst, src)
			src.Close()
			dst.Close()

			attachments = append(attachments, fmt.Sprintf("/api/uploads/feedback/%s/%s", dateDir, filename))
		}
	}

	var attachJSON []byte
	if len(attachments) > 0 {
		attachJSON, _ = json.Marshal(attachments)
	}

	result, err := h.DB.Exec(
		`INSERT INTO feedback (user_id, username, real_name, title, content, page_url, attachments)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		payload.User.ID, payload.User.Username, payload.User.RealName,
		title, content, pageURL, attachJSON,
	)
	if err != nil {
		writeServerError(w, 500, "保存反馈失败", err)
		return
	}

	id, _ := result.LastInsertId()

	// 通知管理员有新反馈（凭证未配置时跳过）
	if h.Notifier != nil {
		h.notifyAdminsNewFeedback(id, title, content, payload.User.RealName, pageURL)
	}

	writeJSON(w, map[string]interface{}{"id": id, "message": "反馈提交成功"})
}

// notifyAdminsNewFeedback 异步推送新反馈给主管理员（admin 账号）
// 跑哥决策：只推 admin 一人，不广播给所有 super_admin（v1.14）
func (h *DashboardHandler) notifyAdminsNewFeedback(feedbackID int64, title, content, submitter, pageURL string) {
	var adminUnionID sql.NullString
	err := h.DB.QueryRow(`
		SELECT dingtalk_userid FROM users
		WHERE username = 'admin' AND status = 'active'
		LIMIT 1
	`).Scan(&adminUnionID)
	if err != nil {
		log.Printf("[feedback-notify] query admin: %v", err)
		return
	}
	if !adminUnionID.Valid || adminUnionID.String == "" {
		log.Printf("[feedback-notify] admin has no dingtalk binding, skip")
		return
	}
	unionIDs := []string{adminUnionID.String}

	// 内容截断防超长
	preview := content
	if len([]rune(preview)) > 80 {
		preview = string([]rune(preview)[:80]) + "..."
	}
	pageHint := ""
	if pageURL != "" {
		pageHint = "\n来源页面：" + pageURL
	}
	msg := "【BI 看板·新反馈】\n" +
		submitter + " 提了一条反馈：\n\n" +
		"《" + title + "》\n" +
		preview + pageHint + "\n\n" +
		"前往反馈管理处理 → /system/feedback"

	h.Notifier.SendTextAsync(unionIDs, msg)
}

// ListFeedback 反馈列表（管理员）
// GET /api/feedback/list?status=&page=&pageSize=
func (h *DashboardHandler) ListFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeError(w, 405, "method not allowed")
		return
	}

	status := r.URL.Query().Get("status")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	where := "1=1"
	args := []interface{}{}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}

	var total int
	h.DB.QueryRow("SELECT COUNT(*) FROM feedback WHERE "+where, args...).Scan(&total)

	offset := (page - 1) * pageSize
	queryArgs := append(args, pageSize, offset)
	rows, err := h.DB.Query(
		"SELECT id, user_id, username, real_name, title, content, page_url, attachments, status, reply, replied_by, created_at, updated_at FROM feedback WHERE "+where+" ORDER BY created_at DESC LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		writeServerError(w, 500, "查询反馈失败", err)
		return
	}
	defer rows.Close()

	type FeedbackItem struct {
		ID          int64           `json:"id"`
		UserID      int64           `json:"userId"`
		Username    string          `json:"username"`
		RealName    string          `json:"realName"`
		Title       string          `json:"title"`
		Content     string          `json:"content"`
		PageURL     string          `json:"pageUrl"`
		Attachments json.RawMessage `json:"attachments"`
		Status      string          `json:"status"`
		Reply       *string         `json:"reply"`
		RepliedBy   *string         `json:"repliedBy"`
		CreatedAt   string          `json:"createdAt"`
		UpdatedAt   string          `json:"updatedAt"`
	}

	var list []FeedbackItem
	for rows.Next() {
		var item FeedbackItem
		var attachRaw []byte
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&item.ID, &item.UserID, &item.Username, &item.RealName,
			&item.Title, &item.Content, &item.PageURL, &attachRaw,
			&item.Status, &item.Reply, &item.RepliedBy, &createdAt, &updatedAt); err != nil {
			writeError(w, 500, "读取数据失败")
			return
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
	if list == nil {
		list = []FeedbackItem{}
	}

	writeJSON(w, map[string]interface{}{
		"list":     list,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// FeedbackByPath 单条反馈操作
// PUT /api/feedback/{id} — 更新状态/回复
func (h *DashboardHandler) FeedbackByPath(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/feedback/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, 400, "无效的反馈ID")
		return
	}

	switch r.Method {
	case "PUT":
		payload, ok := authPayloadFromContext(r)
		if !ok || payload == nil {
			writeError(w, 401, "unauthorized")
			return
		}

		var req struct {
			Status string `json:"status"`
			Reply  string `json:"reply"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, 400, "无效请求")
			return
		}

		validStatus := map[string]bool{"pending": true, "processing": true, "resolved": true, "closed": true}
		if req.Status != "" && !validStatus[req.Status] {
			writeError(w, 400, "无效状态")
			return
		}

		if req.Status != "" && req.Reply != "" {
			_, err = h.DB.Exec("UPDATE feedback SET status=?, reply=?, replied_by=? WHERE id=?",
				req.Status, req.Reply, payload.User.RealName, id)
		} else if req.Status != "" {
			_, err = h.DB.Exec("UPDATE feedback SET status=? WHERE id=?", req.Status, id)
		} else if req.Reply != "" {
			_, err = h.DB.Exec("UPDATE feedback SET reply=?, replied_by=? WHERE id=?",
				req.Reply, payload.User.RealName, id)
		} else {
			writeError(w, 400, "请提供状态或回复内容")
			return
		}

		if err != nil {
			writeServerError(w, 500, "更新反馈失败", err)
			return
		}

		// 反馈回复后通过钉钉推送给提交人（凭证未配置时跳过）
		if req.Reply != "" && h.Notifier != nil {
			h.notifyFeedbackReply(id, req.Reply, payload.User.RealName)
		}

		writeJSON(w, map[string]string{"message": "更新成功"})

	default:
		writeError(w, 405, "method not allowed")
	}
}

// notifyFeedbackReply 异步推送反馈回复给提交人
func (h *DashboardHandler) notifyFeedbackReply(feedbackID int64, reply, replier string) {
	var (
		title          string
		submitterUnion sql.NullString
		submitterName  sql.NullString
	)
	err := h.DB.QueryRow(`
		SELECT f.title, u.dingtalk_userid, u.real_name
		FROM feedback f
		LEFT JOIN users u ON u.id = f.user_id
		WHERE f.id = ?
	`, feedbackID).Scan(&title, &submitterUnion, &submitterName)
	if err != nil {
		log.Printf("[feedback-notify] query feedback %d: %v", feedbackID, err)
		return
	}
	if !submitterUnion.Valid || submitterUnion.String == "" {
		log.Printf("[feedback-notify] feedback %d: submitter has no dingtalk binding, skip", feedbackID)
		return
	}

	greeting := submitterName.String
	if greeting == "" {
		greeting = "你好"
	}
	content := greeting + "，你提交的反馈\"" + title + "\"已有新回复：\n\n" + reply + "\n\n— 来自 " + replier
	h.Notifier.SendTextAsync([]string{submitterUnion.String}, content)
}

