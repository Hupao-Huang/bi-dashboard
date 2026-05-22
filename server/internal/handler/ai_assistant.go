package handler

// BI 智能助手 HTTP handler (v1.73.0 W1-W2)
// 设计文档: docs/ai-assistant-design.md

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// AIAssistantAsk POST /api/ai-assistant/ask
// 请求体: {"question": "上月电商部销售多少", "sessionId": null}
// 响应: { code:200, data: { sessionId, messageId, answer, sourceType, sourceAPI, ... } }
func (h *DashboardHandler) AIAssistantAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.AIAssistant == nil {
		writeError(w, http.StatusServiceUnavailable, "AI 助手未启用 (config.json 里 ai_assistant.enabled=false 或未配 LLM key)")
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Question  string `json:"question"`
		SessionID *int64 `json:"sessionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	req.Question = strings.TrimSpace(req.Question)
	if req.Question == "" {
		writeError(w, http.StatusBadRequest, "请输入问题")
		return
	}
	if len(req.Question) > 500 {
		writeError(w, http.StatusBadRequest, "问题太长 (限 500 字)")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	result, err := h.AIAssistant.Ask(ctx, payload.User.ID, req.SessionID, req.Question)
	if err != nil {
		log.Printf("[ai-assistant] ask failed: user_id=%d question=%q err=%v", payload.User.ID, req.Question, err)
		writeServerError(w, 500, "AI 助手处理失败, 请稍后重试", err)
		return
	}

	writeJSON(w, result)
}

// AIAssistantSessions GET /api/ai-assistant/sessions
// 响应: { code:200, data: { items: [{id, title, createdAt, updatedAt, messageCount}] } }
// 只返当前用户的会话, 按 updated_at DESC, 最多 50 条
func (h *DashboardHandler) AIAssistantSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	rows, err := h.DB.Query(`
		SELECT s.id, IFNULL(s.title,''), s.created_at, s.updated_at,
		       (SELECT COUNT(*) FROM ai_chat_message m WHERE m.session_id=s.id) AS msg_count
		FROM ai_chat_session s
		WHERE s.user_id=?
		ORDER BY s.updated_at DESC
		LIMIT 50`, payload.User.ID)
	if writeDatabaseError(w, err) {
		return
	}
	defer rows.Close()

	type sessionItem struct {
		ID           int64  `json:"id"`
		Title        string `json:"title"`
		CreatedAt    string `json:"createdAt"`
		UpdatedAt    string `json:"updatedAt"`
		MessageCount int    `json:"messageCount"`
	}
	var items []sessionItem
	for rows.Next() {
		var it sessionItem
		var ct, ut time.Time
		if writeDatabaseError(w, rows.Scan(&it.ID, &it.Title, &ct, &ut, &it.MessageCount)) {
			return
		}
		it.CreatedAt = ct.Format("2006-01-02 15:04:05")
		it.UpdatedAt = ut.Format("2006-01-02 15:04:05")
		items = append(items, it)
	}
	if writeDatabaseError(w, rows.Err()) {
		return
	}
	writeJSON(w, map[string]interface{}{"items": items})
}

// AIAssistantMessages GET /api/ai-assistant/messages?sessionId=123
// 响应: { code:200, data: { items: [{id, role, question, answer, sourceAPI, confidence, createdAt}] } }
func (h *DashboardHandler) AIAssistantMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	sidStr := r.URL.Query().Get("sessionId")
	sid, err := strconv.ParseInt(sidStr, 10, 64)
	if err != nil || sid <= 0 {
		writeError(w, http.StatusBadRequest, "sessionId 无效")
		return
	}

	// verify session 属于本 user
	var ownerID int64
	if err := h.DB.QueryRow("SELECT user_id FROM ai_chat_session WHERE id=?", sid).Scan(&ownerID); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "会话不存在")
			return
		}
		writeServerError(w, 500, "查会话失败", err)
		return
	}
	if ownerID != payload.User.ID {
		writeError(w, http.StatusForbidden, "无权限访问该会话")
		return
	}

	rows, err := h.DB.Query(`
		SELECT id, role, IFNULL(question,''), IFNULL(answer,''), IFNULL(source_type,''), IFNULL(source_api,''),
		       IFNULL(confidence,0), IFNULL(llm_model,''), IFNULL(llm_tokens,0), IFNULL(duration_ms,0), IFNULL(warning,''),
		       created_at
		FROM ai_chat_message
		WHERE session_id=?
		ORDER BY created_at ASC, id ASC`, sid)
	if writeDatabaseError(w, err) {
		return
	}
	defer rows.Close()

	type msgItem struct {
		ID         int64   `json:"id"`
		Role       string  `json:"role"`
		Question   string  `json:"question,omitempty"`
		Answer     string  `json:"answer,omitempty"`
		SourceType string  `json:"sourceType,omitempty"`
		SourceAPI  string  `json:"sourceAPI,omitempty"`
		Confidence float64 `json:"confidence,omitempty"`
		LLMModel   string  `json:"llmModel,omitempty"`
		LLMTokens  int     `json:"llmTokens,omitempty"`
		DurationMs int     `json:"durationMs,omitempty"`
		Warning    string  `json:"warning,omitempty"`
		CreatedAt  string  `json:"createdAt"`
	}
	var items []msgItem
	for rows.Next() {
		var it msgItem
		var ct time.Time
		if writeDatabaseError(w, rows.Scan(&it.ID, &it.Role, &it.Question, &it.Answer, &it.SourceType, &it.SourceAPI,
			&it.Confidence, &it.LLMModel, &it.LLMTokens, &it.DurationMs, &it.Warning, &ct)) {
			return
		}
		it.CreatedAt = ct.Format("2006-01-02 15:04:05")
		items = append(items, it)
	}
	if writeDatabaseError(w, rows.Err()) {
		return
	}
	writeJSON(w, map[string]interface{}{"items": items, "sessionId": sid})
}

// AIAssistantFeedback POST /api/ai-assistant/feedback
// 请求体: { messageId: 123, thumb: 1 (👍) | -1 (👎), comment: "..." (可选) }
// UPSERT (同一 user 对同一 message 多次反馈, 最后一次为准)
func (h *DashboardHandler) AIAssistantFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		MessageID int64  `json:"messageId"`
		Thumb     int    `json:"thumb"`
		Comment   string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if req.MessageID <= 0 {
		writeError(w, http.StatusBadRequest, "messageId 无效")
		return
	}
	if req.Thumb != 1 && req.Thumb != -1 {
		writeError(w, http.StatusBadRequest, "thumb 必须是 1(👍) 或 -1(👎)")
		return
	}

	// verify message 存在 + 属于本 user 的 session
	var ownerID int64
	if err := h.DB.QueryRow(`
		SELECT s.user_id FROM ai_chat_message m
		JOIN ai_chat_session s ON s.id=m.session_id
		WHERE m.id=?`, req.MessageID).Scan(&ownerID); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "消息不存在")
			return
		}
		writeServerError(w, 500, "查消息失败", err)
		return
	}
	if ownerID != payload.User.ID {
		writeError(w, http.StatusForbidden, "无权限反馈他人会话")
		return
	}

	// UPSERT: 同 message + user 最后一次为准
	if _, err := h.DB.Exec(`
		INSERT INTO ai_chat_feedback (message_id, user_id, thumb, comment)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE thumb=VALUES(thumb), comment=VALUES(comment), created_at=NOW()`,
		req.MessageID, payload.User.ID, req.Thumb, req.Comment); err != nil {
		writeServerError(w, 500, "保存反馈失败", err)
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}
