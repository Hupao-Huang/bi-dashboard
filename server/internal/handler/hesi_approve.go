package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// nullableStr 空字符串转 SQL NULL (用于 hesi_flow.current_* 字段)
func nullableStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// HesiApprove 提交合思审批请求 (v1.62.x 异步队列)
// POST /api/hesi-bot/approve
// Body: { flowId, action: "agree"|"reject", comment }
// 响应: { queueId, position, estimateSeconds }
func (h *DashboardHandler) HesiApprove(w http.ResponseWriter, r *http.Request) {
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
		FlowID  string `json:"flowId"`
		Action  string `json:"action"`
		Comment string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "无效请求")
		return
	}
	req.FlowID = strings.TrimSpace(req.FlowID)
	req.Action = strings.TrimSpace(req.Action)
	if req.FlowID == "" {
		writeError(w, 400, "flowId 不能为空")
		return
	}
	if req.Action != "agree" && req.Action != "reject" {
		writeError(w, 400, "action 必须是 agree 或 reject")
		return
	}
	if req.Action == "reject" && strings.TrimSpace(req.Comment) == "" {
		writeError(w, 400, "驳回必须填写备注理由")
		return
	}

	// 1. 查 hesi_flow 拿单据信息 + 审批人 ID
	var (
		currentApproverID sql.NullString
		flowCode, title, state string
		formType string
	)
	err := h.DB.QueryRow(`
		SELECT current_approver_id, code, IFNULL(title,''), state, form_type
		FROM hesi_flow WHERE flow_id=? AND active=1`, req.FlowID).
		Scan(&currentApproverID, &flowCode, &title, &state, &formType)
	if err == sql.ErrNoRows {
		writeError(w, 404, "单据不存在或已删除")
		return
	}
	if err != nil {
		writeServerError(w, 500, "查询单据失败", err)
		return
	}
	if !currentApproverID.Valid || currentApproverID.String == "" {
		writeError(w, 400, "单据当前没有审批人, 可能已完成或未到审批节点")
		return
	}
	if state != "approving" && state != "pending" {
		writeError(w, 400, fmt.Sprintf("单据当前状态[%s]不可审批", state))
		return
	}
	if !strings.Contains(currentApproverID.String, ":") {
		writeError(w, 500, fmt.Sprintf("合思审批人ID格式异常: %q", currentApproverID.String))
		return
	}

	// 2. 同 flowId 已在队列里 (queued/running) → 拒绝重复入队
	var existCount int
	h.DB.QueryRow(`SELECT COUNT(*) FROM hesi_approval_queue WHERE flow_id=? AND status IN ('queued','running')`, req.FlowID).Scan(&existCount)
	if existCount > 0 {
		writeError(w, 409, "该单据已在审批队列中, 请等待处理")
		return
	}

	// 3. INSERT 入队列
	res, err := h.DB.Exec(`
		INSERT INTO hesi_approval_queue
			(user_id, username, real_name, flow_id, flow_code, flow_title, action, comment, approve_id, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'queued')`,
		payload.User.ID, payload.User.Username, payload.User.RealName,
		req.FlowID, flowCode, title, req.Action, req.Comment, currentApproverID.String,
	)
	if err != nil {
		writeServerError(w, 500, "入队列失败", err)
		return
	}
	queueID, _ := res.LastInsertId()

	// 4. 估算等待时间: 前面 queued 数 × 65s
	var ahead int
	h.DB.QueryRow(`SELECT COUNT(*) FROM hesi_approval_queue WHERE status IN ('queued','running') AND id < ?`, queueID).Scan(&ahead)
	estimateSeconds := ahead * 65

	writeJSON(w, map[string]interface{}{
		"queueId":         queueID,
		"position":        ahead + 1,
		"estimateSeconds": estimateSeconds,
		"message":         fmt.Sprintf("已加入审批队列, 前面 %d 单, 预计 %d 秒后处理", ahead, estimateSeconds),
	})
}
