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

// nullableStr 空字符串转 SQL NULL (用于 hesi_flow.current_* 字段)
func nullableStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

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

	// 2. approveId 用完整 "corpId:staffId" 格式（按官方文档示例 djg8LshfUkfM00:ID_3kpneISgylw）
	approveID := currentApproverID.String
	if approveID == "" || !strings.Contains(approveID, ":") {
		writeError(w, 500, fmt.Sprintf("合思审批人ID格式异常: %q", approveID))
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

	// 关键：合思要求 path 里的 flowId 用 [方括号] 字面字符包起来（不是占位符标识！）
	// URL-encode 方括号 → %5B / %5D
	hesiURL := fmt.Sprintf(
		"%s/api/openapi/v1/backlog/data/%%5B%s%%5D?accessToken=%s&messageCode=debug&powerCode=TICKET_AUDIT_switch",
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
		// 合思错误透传给前端 + 服务端日志带请求详情便于诊断
		bodySnippet := string(respData)
		if len(bodySnippet) > 500 {
			bodySnippet = bodySnippet[:500] + "...(truncated)"
		}
		writeError(w, 400, fmt.Sprintf(
			"合思返回 HTTP %d\n请求路径: /api/openapi/v1/backlog/data/%s\n请求 approveId: %s\n响应: %s",
			resp.StatusCode, req.FlowID, approveID, bodySnippet,
		))
		return
	}

	// 5. 成功 — 实时拉合思 approveStates 刷新本地"当前审批人/节点", 避免前端拿陈旧数据误点
	//    审批成功后单据已流转到下个节点 (同意) 或回退 (驳回), 新审批人变了
	//    不等下次 sync (最多 1h), 立即同步, 防止重复审批
	refreshURL := fmt.Sprintf(
		"%s/api/openapi/v2/approveStates/%%5B%s%%5D?accessToken=%s",
		hesiAPIBase, req.FlowID, token,
	)
	if refreshResp, refreshErr := hesiHTTP.Get(refreshURL); refreshErr == nil {
		refreshData, _ := io.ReadAll(refreshResp.Body)
		refreshResp.Body.Close()
		var rs struct {
			Items []struct {
				FlowID    string `json:"flowId"`
				StageName string `json:"stageName"`
				Operators []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
					Code string `json:"code"`
				} `json:"operators"`
			} `json:"items"`
		}
		if json.Unmarshal(refreshData, &rs) == nil && len(rs.Items) > 0 {
			it := rs.Items[0]
			var opID, opName, opCode string
			if len(it.Operators) > 0 {
				opID = it.Operators[0].ID
				opName = it.Operators[0].Name
				opCode = it.Operators[0].Code
				for j := 1; j < len(it.Operators); j++ {
					opName += "+" + it.Operators[j].Name
				}
			}
			// state 推断: 如果还有 operator 说明还在审批中; 没有 operator 说明流程结束
			newState := "approving"
			if len(it.Operators) == 0 {
				if req.Action == "agree" {
					newState = "paying" // 最终节点同意 → 进入支付
				} else {
					newState = "rejected"
				}
			}
			h.DB.Exec(
				`UPDATE hesi_flow SET state=?, current_stage_name=?, current_approver_id=?, current_approver_name=?, current_approver_code=? WHERE flow_id=?`,
				newState, nullableStr(it.StageName), nullableStr(opID), nullableStr(opName), nullableStr(opCode), req.FlowID,
			)
		}
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
