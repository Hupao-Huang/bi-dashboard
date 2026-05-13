package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// HesiApprove 手动审批合思单据
// POST /api/hesi-bot/approve
// Body: { flowId, action: "agree"|"reject", comment }
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
	// 驳回必须填备注
	if req.Action == "reject" && strings.TrimSpace(req.Comment) == "" {
		writeError(w, 400, "驳回必须填写备注理由")
		return
	}

	// 1. 查 hesi_flow 的 current_approver_id
	var currentApproverID sql.NullString
	var title, formType, state string
	err := h.DB.QueryRow(`
		SELECT current_approver_id, title, form_type, state
		FROM hesi_flow WHERE flow_id=? AND active=1`, req.FlowID).
		Scan(&currentApproverID, &title, &formType, &state)
	if err == sql.ErrNoRows {
		writeError(w, 404, "单据不存在或已删除")
		return
	}
	if err != nil {
		writeServerError(w, 500, "查询单据失败", err)
		return
	}
	if !currentApproverID.Valid || currentApproverID.String == "" {
		writeError(w, 400, "单据当前没有审批人，可能已完成或未到审批节点")
		return
	}
	// 状态校验：只能审批 approving / pending 状态的单据
	if state != "approving" && state != "pending" {
		writeError(w, 400, fmt.Sprintf("单据当前状态[%s]不可审批", state))
		return
	}

	// 2. 提取 approveId（current_approver_id 格式 "corpId:staffId"，取冒号后半部分）
	approveID := currentApproverID.String
	if idx := strings.Index(approveID, ":"); idx >= 0 {
		approveID = approveID[idx+1:]
	}
	if approveID == "" {
		writeError(w, 500, "解析合思审批人ID失败")
		return
	}

	// 3. 获取合思 accessToken
	token, err := h.getHesiToken()
	if err != nil {
		writeServerError(w, 500, "获取合思授权失败", err)
		return
	}

	// 4. 调合思审批接口
	actionName := "freeflow.agree"
	if req.Action == "reject" {
		actionName = "freeflow.reject"
	}
	hesiBody := map[string]interface{}{
		"approveId": approveID,
		"action": map[string]interface{}{
			"name":    actionName,
			"comment": req.Comment,
		},
	}
	if req.Action == "reject" {
		// reject 需要指定回退方式：resubmit=重新提交
		hesiBody["action"].(map[string]interface{})["rejectTo"] = ""
		hesiBody["action"].(map[string]interface{})["resubmitMethod"] = "resubmit"
	}
	bodyBytes, _ := json.Marshal(hesiBody)

	hesiURL := fmt.Sprintf(
		"%s/api/openapi/v1/backlog/data/%s?accessToken=%s&messageCode=debug&powerCode=TICKET_AUDIT_switch",
		hesiAPIBase, req.FlowID, token,
	)
	httpReq, _ := http.NewRequest("POST", hesiURL, bytes.NewReader(bodyBytes))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := hesiHTTP.Do(httpReq)
	if err != nil {
		writeServerError(w, 500, "调用合思审批接口失败", err)
		return
	}
	defer resp.Body.Close()
	respData, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 合思错误透传给前端
		writeError(w, 400, fmt.Sprintf("合思返回 HTTP %d: %s", resp.StatusCode, string(respData)))
		return
	}

	// 5. 成功 — 标记 hesi_flow 本地状态变更（等下次 sync 校准）
	if req.Action == "agree" {
		h.DB.Exec(`UPDATE hesi_flow SET state='paying' WHERE flow_id=?`, req.FlowID)
	} else {
		h.DB.Exec(`UPDATE hesi_flow SET state='rejected' WHERE flow_id=?`, req.FlowID)
	}

	// 6. 审计日志（写 audit_log，沿用现有表）
	actionLabel := "同意"
	if req.Action == "reject" {
		actionLabel = "驳回"
	}
	h.DB.Exec(`
		INSERT INTO audit_logs (user_id, username, real_name, action, resource, detail, ip, user_agent)
		VALUES (?, ?, ?, 'hesi_approve', ?, ?, ?, ?)`,
		payload.User.ID, payload.User.Username, payload.User.RealName, req.FlowID,
		fmt.Sprintf("%s 合思单据 %s (备注: %s)", actionLabel, title, req.Comment),
		clientIP(r), r.UserAgent(),
	)

	writeJSON(w, map[string]interface{}{
		"message":      "审批成功",
		"action":       req.Action,
		"flowId":       req.FlowID,
		"hesiResponse": string(respData),
	})
}
