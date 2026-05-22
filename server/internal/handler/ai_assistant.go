package handler

// BI 智能助手 HTTP handler (v1.73.0 W1 demo)
// 设计文档: docs/ai-assistant-design.md

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

// AIAssistantAsk POST /api/ai-assistant/ask
// 请求体: {"question": "上月电商部销售多少"}
// 响应: { code:200, data: { answer, sourceType, sourceAPI, sourceParams, rawData, confidence, durationMs, intent, llmTokens } }
func (h *DashboardHandler) AIAssistantAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.AIAssistant == nil {
		writeError(w, http.StatusServiceUnavailable, "AI 助手未启用 (config.json 里 ai_assistant.enabled=false 或未配 LLM key)")
		return
	}

	var req struct {
		Question string `json:"question"`
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

	// W1 demo: 写死 60s timeout (覆盖 LLM 2 次调用 + 内部 SQL 查询)
	// W2 加 config 控制
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	result, err := h.AIAssistant.Ask(ctx, req.Question)
	if err != nil {
		log.Printf("[ai-assistant] ask failed: question=%q err=%v", req.Question, err)
		writeServerError(w, 500, "AI 助手处理失败, 请稍后重试", err)
		return
	}

	writeJSON(w, result)
}
