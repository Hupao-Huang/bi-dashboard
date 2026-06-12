package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// HesiRejectNodes GET /api/hesi-bot/reject-nodes?flowId=
// 返回该单据"当前这轮"已审批通过的节点清单, 给驳回弹窗的"驳回节点"下拉用
// (对应合思自家驳回弹窗的节点选项; "提交人"默认项由前端兜底, 不在返回里)
// 数据源: 合思单据详情 logs — 取最后一次提交之后的同意记录 (撤回重提的旧轮次不算)
func (h *DashboardHandler) HesiRejectNodes(w http.ResponseWriter, r *http.Request) {
	flowID := strings.TrimSpace(r.URL.Query().Get("flowId"))
	if flowID == "" {
		writeError(w, 400, "flowId 不能为空")
		return
	}
	token, err := h.getHesiToken()
	if err != nil {
		writeServerError(w, 500, "获取合思授权失败", err)
		return
	}
	resp, err := hesiHTTP.Get(fmt.Sprintf("%s/api/openapi/v1.1/flowDetails?flowId=%s&accessToken=%s", hesiAPIBase, flowID, token))
	if err != nil {
		writeServerError(w, 500, "查询合思单据失败", err)
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var parsed struct {
		Value struct {
			Logs []struct {
				Action     string `json:"action"`
				Attributes struct {
					NodeID   string `json:"nodeId"`
					NodeName string `json:"nodeName"`
				} `json:"attributes"`
			} `json:"logs"`
		} `json:"value"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		writeServerError(w, 500, "解析合思单据失败", err)
		return
	}

	lastSubmit := -1
	for i, lg := range parsed.Value.Logs {
		if lg.Action == "freeflow.submit" {
			lastSubmit = i
		}
	}
	type nodeItem struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	nodes := []nodeItem{}
	seen := map[string]bool{}
	for i := lastSubmit + 1; i >= 0 && i < len(parsed.Value.Logs); i++ {
		lg := parsed.Value.Logs[i]
		if lg.Action != "freeflow.agree" {
			continue
		}
		id, name := lg.Attributes.NodeID, lg.Attributes.NodeName
		if !strings.HasPrefix(id, "FLOW:") || name == "" || seen[id] {
			continue
		}
		if strings.Contains(name, "抄送") {
			continue // 抄送节点是自动通过的知会节点, 不能当驳回目标 (合思自家弹窗也不列)
		}
		seen[id] = true
		nodes = append(nodes, nodeItem{ID: id, Name: name})
	}
	writeJSON(w, map[string]interface{}{"nodes": nodes})
}

// HesiApprovalFlow GET /api/hesi/approval-flow?flowId= (费控) / 经 GetMyHesiApprovalFlow (机器人)
// 单据审批流时间线 (跑哥 2026-06-12: 详情里要能看审批流)
// 数据源: 合思 flowDetails v1.1 logs 实时拉 (本地 raw_json 没存 logs);
// 审批中的单补当前待审节点 (hesi_flow.current_* 本地同步字段)
func (h *DashboardHandler) HesiApprovalFlow(w http.ResponseWriter, r *http.Request) {
	flowID := strings.TrimSpace(r.URL.Query().Get("flowId"))
	if flowID == "" {
		writeError(w, 400, "flowId 不能为空")
		return
	}
	token, err := h.getHesiToken()
	if err != nil {
		writeServerError(w, 500, "获取合思授权失败", err)
		return
	}
	resp, err := hesiHTTP.Get(fmt.Sprintf("%s/api/openapi/v1.1/flowDetails?flowId=%s&accessToken=%s", hesiAPIBase, flowID, token))
	if err != nil {
		writeServerError(w, 500, "查询合思单据失败", err)
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var parsed struct {
		Value struct {
			State string `json:"state"`
			Logs  []struct {
				Action       string `json:"action"`
				Time         int64  `json:"time"`
				OperatorID   string `json:"operatorId"`
				ByDelegateID string `json:"byDelegateId"`
				Attributes   struct {
					NodeName string `json:"nodeName"`
					Comment  string `json:"comment"`
				} `json:"attributes"`
			} `json:"logs"`
		} `json:"value"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		writeServerError(w, 500, "解析合思单据失败", err)
		return
	}

	type step struct {
		Action   string `json:"action"`
		NodeName string `json:"nodeName"`
		Operator string `json:"operator"`
		Delegate string `json:"delegate,omitempty"` // 代审人
		Comment  string `json:"comment"`
		Time     int64  `json:"time"`
	}
	steps := make([]step, 0, len(parsed.Value.Logs))
	for _, lg := range parsed.Value.Logs {
		st := step{Action: lg.Action, NodeName: lg.Attributes.NodeName, Comment: lg.Attributes.Comment, Time: lg.Time}
		if lg.OperatorID != "" {
			st.Operator = h.LookupStaffName(lg.OperatorID)
			if st.Operator == "" {
				st.Operator = lg.OperatorID
			}
		}
		if lg.ByDelegateID != "" {
			st.Delegate = h.LookupStaffName(lg.ByDelegateID)
		}
		steps = append(steps, st)
	}

	var curNode, curApprover string
	_ = h.DB.QueryRow(`SELECT IFNULL(current_stage_name,''), IFNULL(current_approver_name,'') FROM hesi_flow WHERE flow_id=? LIMIT 1`, flowID).Scan(&curNode, &curApprover)
	writeJSON(w, map[string]interface{}{
		"state": parsed.Value.State, "logs": steps,
		"currentNode": curNode, "currentApprover": curApprover,
	})
}

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
		// v1.76.x 驳回选项 (对应合思驳回弹窗的两个模块):
		// RejectTo: 驳回目标节点ID(FLOW:x:y), 空=驳回至提交人
		// ResubmitMethod: TO_REJECTOR=重审从当前节点开始(默认) / FROM_START=重审从提交人重新走全流程
		RejectTo       string `json:"rejectTo"`
		ResubmitMethod string `json:"resubmitMethod"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "无效请求")
		return
	}
	req.FlowID = strings.TrimSpace(req.FlowID)
	req.Action = strings.TrimSpace(req.Action)
	req.RejectTo = strings.TrimSpace(req.RejectTo)
	req.ResubmitMethod = strings.TrimSpace(req.ResubmitMethod)
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
	if req.Action == "reject" {
		if req.ResubmitMethod == "" {
			req.ResubmitMethod = "TO_REJECTOR"
		}
		if req.ResubmitMethod != "TO_REJECTOR" && req.ResubmitMethod != "FROM_START" {
			writeError(w, 400, "重审路径只能是 TO_REJECTOR(从当前节点) 或 FROM_START(重走全流程)")
			return
		}
	} else {
		req.RejectTo, req.ResubmitMethod = "", "" // 同意不带驳回参数
	}

	// 1. 查 hesi_flow 拿单据信息 + 审批人 ID
	var (
		currentApproverID      sql.NullString
		flowCode, title, state string
		formType               string
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
			(user_id, username, real_name, flow_id, flow_code, flow_title, action, comment, approve_id, reject_to, resubmit_method, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'queued')`,
		payload.User.ID, payload.User.Username, payload.User.RealName,
		req.FlowID, flowCode, title, req.Action, req.Comment, currentApproverID.String,
		req.RejectTo, req.ResubmitMethod,
	)
	if err != nil {
		writeServerError(w, 500, "入队列失败", err)
		return
	}
	queueID, _ := res.LastInsertId()

	// 4. 估算等待时间: worker 按 (approve_id, action) 合批 (同组最多 10 单一次合思 API 调用)
	//    所以等待 = 前面有几个"不同 approve_id+action 组"× 65s + 自己这组 65s
	//    跑哥跟前面已有同 approver+action 的单会被合批, 不增加额外等待
	var groupsAhead int
	h.DB.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT approve_id, action
			FROM hesi_approval_queue
			WHERE status IN ('queued','running')
			GROUP BY approve_id, action
			HAVING MIN(created_at) < (
				SELECT MIN(created_at) FROM hesi_approval_queue
				WHERE status IN ('queued','running') AND approve_id=? AND action=?
			)
		) AS prior`, currentApproverID.String, req.Action).Scan(&groupsAhead)

	position := groupsAhead + 1
	estimateSeconds := position * 65

	writeJSON(w, map[string]interface{}{
		"queueId":         queueID,
		"position":        position,
		"estimateSeconds": estimateSeconds,
		"message":         fmt.Sprintf("已加入审批队列, 排第 %d 组, 预计 %d 秒后处理", position, estimateSeconds),
	})
}
