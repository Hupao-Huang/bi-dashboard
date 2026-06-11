package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// hesi 限流: 审批接口调用间隔 >= 60s, 单次最多 10 单
const (
	hesiCallInterval     = 65 * time.Second // 加 5s 余量
	hesiBatchMaxFlows    = 10
	approvalWorkerPolling = 10 * time.Second
)

type approvalQueueItem struct {
	ID        int64
	UserID    int64
	Username  string
	RealName  string
	FlowID    string
	FlowCode  string
	FlowTitle string
	ApproveID string
	Action    string
	Comment   string
}

// StartHesiApprovalWorker bi-server 启动时拉起的合思审批队列 worker (单 goroutine)
// 每 10s 轮询队列; 取 1 个 queued 当锚 → 同 approve_id+action 最多 10 单一批 → 合思 batch API
// 限流: 两次合思调用间隔 >= 65s
func (h *DashboardHandler) StartHesiApprovalWorker(stopCh <-chan struct{}) {
	// 启动时回滚 running → queued (防止上次重启遗留)
	if res, err := h.DB.Exec(`UPDATE hesi_approval_queue SET status='queued', started_at=NULL WHERE status='running'`); err == nil {
		if n, _ := res.RowsAffected(); n > 0 {
			log.Printf("[hesi-worker] 启动: 回滚 %d 条 running → queued", n)
		}
	}
	log.Println("[hesi-worker] 启动完成, 轮询间隔 10s, 合思调用间隔 65s")

	ticker := time.NewTicker(approvalWorkerPolling)
	defer ticker.Stop()
	var lastCallAt time.Time

	for {
		select {
		case <-stopCh:
			log.Println("[hesi-worker] 收到停止信号")
			return
		case <-ticker.C:
			// 限流: 距上次合思调用 < 65s 跳过
			if !lastCallAt.IsZero() && time.Since(lastCallAt) < hesiCallInterval {
				continue
			}
			batch, err := h.fetchNextApprovalBatch()
			if err != nil {
				log.Printf("[hesi-worker] 取批失败: %v", err)
				continue
			}
			if len(batch) == 0 {
				continue
			}
			h.processApprovalBatch(batch)
			lastCallAt = time.Now()
		}
	}
}

// fetchNextApprovalBatch 取下一批待处理: 最早的 queued 为锚, 同 approve_id+action 最多 10 单
func (h *DashboardHandler) fetchNextApprovalBatch() ([]approvalQueueItem, error) {
	// 1. 取最早的 queued 当锚
	var anchorApproveID, anchorAction string
	err := h.DB.QueryRow(`
		SELECT approve_id, action FROM hesi_approval_queue
		WHERE status='queued' ORDER BY created_at LIMIT 1`).Scan(&anchorApproveID, &anchorAction)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// 2. 取同 approve_id+action FIFO 最多 10 单
	rows, err := h.DB.Query(`
		SELECT id, user_id, username, real_name, flow_id, IFNULL(flow_code,''), IFNULL(flow_title,''),
		       approve_id, action, IFNULL(comment,'')
		FROM hesi_approval_queue
		WHERE status='queued' AND approve_id=? AND action=?
		ORDER BY created_at LIMIT ?`, anchorApproveID, anchorAction, hesiBatchMaxFlows)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var batch []approvalQueueItem
	for rows.Next() {
		var it approvalQueueItem
		if err := rows.Scan(&it.ID, &it.UserID, &it.Username, &it.RealName, &it.FlowID,
			&it.FlowCode, &it.FlowTitle, &it.ApproveID, &it.Action, &it.Comment); err != nil {
			return nil, err
		}
		batch = append(batch, it)
	}
	return batch, rows.Err()
}

// processApprovalBatch 处理一批: UPDATE running → 合思 API → 每单结果回写
func (h *DashboardHandler) processApprovalBatch(batch []approvalQueueItem) {
	if len(batch) == 0 {
		return
	}
	approver := batch[0].ApproveID
	action := batch[0].Action

	// 1. UPDATE status='running'
	ids := make([]interface{}, len(batch))
	for i, it := range batch {
		ids[i] = it.ID
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	// v1.70.6: status=running 写库失败必须中止, 否则下游合思 API 调用会执行但 DB 没标记, 重启后可能重复提交
	if _, err := h.DB.Exec(`UPDATE hesi_approval_queue SET status='running', started_at=NOW() WHERE id IN (`+placeholders+`)`, ids...); err != nil {
		log.Printf("[hesi-worker] 标 running 失败, 跳过批 approver=%s: %v", batch[0].ApproveID, err)
		return
	}

	log.Printf("[hesi-worker] 处理批 approveId=%s action=%s 单数=%d", approver, action, len(batch))

	// 2. 获取合思 token
	token, err := h.getHesiToken()
	if err != nil {
		h.failBatch(batch, fmt.Sprintf("获取合思授权失败: %v", err))
		return
	}

	// 3. 调合思 batch API: [flowId1,flowId2,...] 用方括号包逗号分隔
	flowIDs := make([]string, len(batch))
	for i, it := range batch {
		flowIDs[i] = it.FlowID
	}
	flowIDsParam := "[" + strings.Join(flowIDs, ",") + "]"
	encodedParam := strings.ReplaceAll(strings.ReplaceAll(flowIDsParam, "[", "%5B"), "]", "%5D")

	actionName := "freeflow.agree"
	if action == "reject" {
		actionName = "freeflow.reject"
	}
	hesiBody := map[string]interface{}{
		"approveId": approver,
		"action": map[string]interface{}{
			"name":    actionName,
			"comment": batch[0].Comment, // batch 内 comment 取第一个 (多单同 approver 一般同操作场景)
		},
	}
	if action == "reject" {
		hesiBody["action"].(map[string]interface{})["resubmitMethod"] = "resubmit"
	}
	bodyBytes, _ := json.Marshal(hesiBody)

	hesiURL := fmt.Sprintf("%s/api/openapi/v1/backlog/data/%s?accessToken=%s&messageCode=debug&powerCode=TICKET_AUDIT_switch",
		hesiAPIBase, encodedParam, token)
	httpReq, _ := http.NewRequest("POST", hesiURL, bytes.NewReader(bodyBytes))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := hesiHTTP.Do(httpReq)
	if err != nil {
		h.failBatch(batch, fmt.Sprintf("调用合思失败: %v", err))
		return
	}
	respData, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(respData)
		if len(snippet) > 300 {
			snippet = snippet[:300] + "..."
		}
		h.failBatch(batch, fmt.Sprintf("合思 HTTP %d: %s", resp.StatusCode, snippet))
		return
	}

	// 4. 解析合思响应 {value: {total, success, error, residue, errorMsg, ...}}
	var parsed struct {
		Value struct {
			Total    int    `json:"total"`
			Success  int    `json:"success"`
			Error    int    `json:"error"`
			Residue  int    `json:"residue"`
			ErrorMsg string `json:"errorMsg"`
		} `json:"value"`
	}
	// v1.71.0: 解析失败必须当整批失败处理 — 否则 Error=0 默认值会被误判成"全部成功", 引发幽灵审批
	if err := json.Unmarshal(respData, &parsed); err != nil {
		h.failBatch(batch, fmt.Sprintf("合思响应非 JSON 无法判断结果: %v response=%.500s", err, string(respData)))
		return
	}
	resultJSON := string(respData)
	if len(resultJSON) > 500 {
		resultJSON = resultJSON[:500] + "..."
	}

	// 5. 批量整批标记成功 (合思响应是聚合的, 无法精确归因每单)
	//    error>0 时整批标记 failed (保守, 防误标)
	if parsed.Value.Error > 0 {
		h.failBatch(batch, fmt.Sprintf("合思响应 error=%d errorMsg=%s response=%s", parsed.Value.Error, parsed.Value.ErrorMsg, resultJSON))
		return
	}
	// v1.71.0: 标 success 失败 → 重启时 running→queued 重复处理. log 关键
	if _, err := h.DB.Exec(`UPDATE hesi_approval_queue SET status='success', result=?, finished_at=NOW() WHERE id IN (`+placeholders+`)`,
		append([]interface{}{resultJSON}, ids...)...); err != nil {
		log.Printf("[hesi-worker] ⚠️ 标 success 写库失败! 重启时会重复处理 approver=%s ids=%v err=%v", approver, ids, err)
	}
	log.Printf("[hesi-worker] 批成功 单数=%d total=%d success=%d residue=%d",
		len(batch), parsed.Value.Total, parsed.Value.Success, parsed.Value.Residue)

	// 6. 批级通知: 被代审的审批人 (跑哥替樊雪娇审 → 通知樊雪娇, 一批一条汇总不刷屏)
	h.notifyDelegatedApprover(batch, action)

	// 7. 每单后处理: 刷新 hesi_flow + 钉钉通知操作人 + 钉钉通知下一环节审批人
	for _, it := range batch {
		stage, nextOps := h.refreshFlowAfterApproval(it.FlowID, action, token)
		h.notifyApprovalDone(it, action)
		if action == "agree" {
			h.notifyNextApprover(it, stage, nextOps)
		}
		h.writeAuditLogForApproval(it, action)
	}
}

// notifyDelegatedApprover 钉钉通知"被代审的审批人": 单据挂在他名下待审, 被别人通过 BI 代点了
// 整批同一审批人+同一动作 → 只发一条汇总。代审人就是审批人自己时不发 (自己审自己不用通知)
func (h *DashboardHandler) notifyDelegatedApprover(batch []approvalQueueItem, action string) {
	if h.Notifier == nil || len(batch) == 0 {
		return
	}
	approveID := batch[0].ApproveID

	// 点审批的人自己就是这个审批人 → 不通知自己
	var clickerHesiID string
	_ = h.DB.QueryRow(`SELECT IFNULL(hesi_staff_id,'') FROM users WHERE id=?`, batch[0].UserID).Scan(&clickerHesiID)
	if clickerHesiID != "" && clickerHesiID == approveID {
		return
	}

	var dingID, approverName string
	if err := h.DB.QueryRow(
		`SELECT dingtalk_userid, hesi_name FROM hesi_employee_contract_company
		 WHERE hesi_staff_id = ? AND dingtalk_userid <> '' LIMIT 1`, approveID).Scan(&dingID, &approverName); err != nil || dingID == "" {
		log.Printf("[hesi-worker] 被代审审批人 %s 未桥接钉钉, 跳过通知", approveID)
		return
	}

	label := "已同意"
	if action == "reject" {
		label = "已驳回"
	}
	operator := batch[0].RealName
	if operator == "" {
		operator = batch[0].Username
	}

	codes := make([]string, 0, len(batch))
	for _, it := range batch {
		codes = append(codes, it.FlowCode)
	}
	var msg string
	if len(batch) == 1 {
		msg = fmt.Sprintf("【合思审批】您的待审批单据 %s (%s) 已由 %s 代您审批: %s", batch[0].FlowCode, batch[0].FlowTitle, operator, label)
		if action == "reject" && batch[0].Comment != "" {
			msg += fmt.Sprintf(" (理由: %s)", batch[0].Comment)
		}
	} else {
		msg = fmt.Sprintf("【合思审批】您的 %d 张待审批单据已由 %s 代您审批: %s (%s)", len(batch), operator, label, strings.Join(codes, ", "))
	}
	h.Notifier.SendTextToStaffIDsAsync([]string{dingID}, msg)
	log.Printf("[hesi-worker] 已通知被代审审批人 %s(%s) 单数=%d", approverName, approveID, len(batch))
}

// failBatch 整批标记失败
func (h *DashboardHandler) failBatch(batch []approvalQueueItem, errMsg string) {
	ids := make([]interface{}, len(batch))
	for i, it := range batch {
		ids[i] = it.ID
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := append([]interface{}{errMsg}, ids...)
	// v1.71.0: 标 failed 失败 → 重启时 running→queued 重复处理. log 关键
	if _, err := h.DB.Exec(`UPDATE hesi_approval_queue SET status='failed', error_msg=?, finished_at=NOW() WHERE id IN (`+placeholders+`)`, args...); err != nil {
		log.Printf("[hesi-worker] ⚠️ 标 failed 写库失败! 重启时会重复处理 ids=%v err=%v", ids, err)
	}
	log.Printf("[hesi-worker] 批失败 单数=%d 错误=%s", len(batch), errMsg)
}

// hesiNextOperator 审批通过后下一环节的操作人 (通知用)
type hesiNextOperator struct {
	ID   string // 合思员工 ID (corp:innerId)
	Name string
}

// refreshFlowAfterApproval 审批成功后调合思 approveStates 拉实时状态并 UPDATE hesi_flow
// 返回下一环节名 + 操作人列表 (给 notifyNextApprover 用; 拉取/解析失败返回空)
func (h *DashboardHandler) refreshFlowAfterApproval(flowID, action, token string) (string, []hesiNextOperator) {
	url := fmt.Sprintf("%s/api/openapi/v2/approveStates/%%5B%s%%5D?accessToken=%s", hesiAPIBase, flowID, token)
	resp, err := hesiHTTP.Get(url)
	if err != nil {
		return "", nil
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
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
	if err := json.Unmarshal(data, &rs); err != nil || len(rs.Items) == 0 {
		if err != nil {
			log.Printf("[hesi-worker] refreshFlow 解析失败 flow=%s: %v", flowID, err)
		}
		return "", nil
	}
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
	newState := "approving"
	if len(it.Operators) == 0 {
		if action == "agree" {
			newState = "paying"
		} else {
			newState = "rejected"
		}
	}
	// v1.71.0: 刷新 hesi_flow 失败 → BI 看板状态陈旧, 下次 sync-hesi 会修正, 只 log
	if _, err := h.DB.Exec(
		`UPDATE hesi_flow SET state=?, current_stage_name=?, current_approver_id=?, current_approver_name=?, current_approver_code=? WHERE flow_id=?`,
		newState, nullableStr(it.StageName), nullableStr(opID), nullableStr(opName), nullableStr(opCode), flowID,
	); err != nil {
		log.Printf("[hesi-worker] refreshFlow UPDATE 失败 flow=%s: %v (BI 状态陈旧, 等下次 sync-hesi)", flowID, err)
	}

	ops := make([]hesiNextOperator, 0, len(it.Operators))
	for _, op := range it.Operators {
		ops = append(ops, hesiNextOperator{ID: op.ID, Name: op.Name})
	}
	return it.StageName, ops
}

// notifyNextApprover 审批同意后钉钉单聊通知下一环节操作人 (含出纳支付)
// 合思员工 → 钉钉 staffId 走花名册桥接表 hesi_employee_contract_company (优先 ID 精确, 兜底姓名)
// 桥接不到/未配 Notifier 只 log 不阻塞 — 通知是锦上添花, 不是审批主流程
func (h *DashboardHandler) notifyNextApprover(item approvalQueueItem, stageName string, ops []hesiNextOperator) {
	if h.Notifier == nil || len(ops) == 0 {
		return
	}
	seen := map[string]bool{}
	staffIDs := []string{}
	for _, op := range ops {
		var dingID string
		err := h.DB.QueryRow(
			`SELECT dingtalk_userid FROM hesi_employee_contract_company
			 WHERE hesi_staff_id = ? AND dingtalk_userid <> '' LIMIT 1`, op.ID).Scan(&dingID)
		if err == sql.ErrNoRows && op.Name != "" {
			err = h.DB.QueryRow(
				`SELECT dingtalk_userid FROM hesi_employee_contract_company
				 WHERE hesi_name = ? AND dingtalk_userid <> '' LIMIT 1`, op.Name).Scan(&dingID)
		}
		if err != nil || dingID == "" {
			log.Printf("[hesi-worker] 下一审批人 %s(%s) 未桥接钉钉, 跳过通知 flow=%s", op.Name, op.ID, item.FlowID)
			continue
		}
		if !seen[dingID] {
			seen[dingID] = true
			staffIDs = append(staffIDs, dingID)
		}
	}
	if len(staffIDs) == 0 {
		return
	}
	stageLabel := ""
	if stageName != "" {
		stageLabel = fmt.Sprintf(" (当前环节: %s)", stageName)
	}
	msg := fmt.Sprintf("【合思审批】单据 %s (%s) 已流转到您, 请及时处理%s", item.FlowCode, item.FlowTitle, stageLabel)
	h.Notifier.SendTextToStaffIDsAsync(staffIDs, msg)
	log.Printf("[hesi-worker] 已通知下一环节操作人 flow=%s stage=%s 人数=%d", item.FlowID, stageName, len(staffIDs))
}

// notifyApprovalDone 钉钉通知操作人
func (h *DashboardHandler) notifyApprovalDone(item approvalQueueItem, action string) {
	if h.Notifier == nil {
		return
	}
	var unionID sql.NullString
	// v1.71.0: 查 unionID 失败 → 钉钉通知降级跳过, 不影响审批主流程
	if err := h.DB.QueryRow(`SELECT dingtalk_userid FROM users WHERE id=?`, item.UserID).Scan(&unionID); err != nil && err != sql.ErrNoRows {
		log.Printf("[hesi-worker] 查 unionID 失败 user_id=%d: %v (跳过钉钉通知)", item.UserID, err)
		return
	}
	if !unionID.Valid || unionID.String == "" {
		return
	}
	label := "已同意"
	if action == "reject" {
		label = "已驳回"
	}
	msg := fmt.Sprintf("【合思审批】您提交的 %s (%s) %s", item.FlowCode, item.FlowTitle, label)
	h.Notifier.SendTextAsync([]string{unionID.String}, msg)
}

// writeAuditLogForApproval 写审计日志
func (h *DashboardHandler) writeAuditLogForApproval(item approvalQueueItem, action string) {
	label := "同意"
	if action == "reject" {
		label = "驳回"
	}
	// v1.71.0: 审计日志写入失败 → 缺一条审计记录, 不影响主流程, log 即可
	if _, err := h.DB.Exec(`
		INSERT INTO audit_logs (user_id, username, real_name, action, resource, detail, ip, user_agent)
		VALUES (?, ?, ?, 'hesi_approve', ?, ?, '', 'worker')`,
		item.UserID, item.Username, item.RealName, item.FlowID,
		fmt.Sprintf("%s 合思单据 %s [%s] (备注: %s)", label, item.FlowCode, item.FlowTitle, item.Comment),
	); err != nil {
		log.Printf("[hesi-worker] 写审计日志失败 user_id=%d flow=%s: %v", item.UserID, item.FlowID, err)
	}
}
