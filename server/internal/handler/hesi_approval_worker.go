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
	h.DB.Exec(`UPDATE hesi_approval_queue SET status='running', started_at=NOW() WHERE id IN (`+placeholders+`)`, ids...)

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
	json.Unmarshal(respData, &parsed)
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
	h.DB.Exec(`UPDATE hesi_approval_queue SET status='success', result=?, finished_at=NOW() WHERE id IN (`+placeholders+`)`,
		append([]interface{}{resultJSON}, ids...)...)
	log.Printf("[hesi-worker] 批成功 单数=%d total=%d success=%d residue=%d",
		len(batch), parsed.Value.Total, parsed.Value.Success, parsed.Value.Residue)

	// 6. 每单后处理: 刷新 hesi_flow + 钉钉通知
	for _, it := range batch {
		h.refreshFlowAfterApproval(it.FlowID, action, token)
		h.notifyApprovalDone(it, action)
		h.writeAuditLogForApproval(it, action)
	}
}

// failBatch 整批标记失败
func (h *DashboardHandler) failBatch(batch []approvalQueueItem, errMsg string) {
	ids := make([]interface{}, len(batch))
	for i, it := range batch {
		ids[i] = it.ID
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := append([]interface{}{errMsg}, ids...)
	h.DB.Exec(`UPDATE hesi_approval_queue SET status='failed', error_msg=?, finished_at=NOW() WHERE id IN (`+placeholders+`)`, args...)
	log.Printf("[hesi-worker] 批失败 单数=%d 错误=%s", len(batch), errMsg)
}

// refreshFlowAfterApproval 审批成功后调合思 approveStates 拉实时状态并 UPDATE hesi_flow
func (h *DashboardHandler) refreshFlowAfterApproval(flowID, action, token string) {
	url := fmt.Sprintf("%s/api/openapi/v2/approveStates/%%5B%s%%5D?accessToken=%s", hesiAPIBase, flowID, token)
	resp, err := hesiHTTP.Get(url)
	if err != nil {
		return
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
	if json.Unmarshal(data, &rs) != nil || len(rs.Items) == 0 {
		return
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
	h.DB.Exec(
		`UPDATE hesi_flow SET state=?, current_stage_name=?, current_approver_id=?, current_approver_name=?, current_approver_code=? WHERE flow_id=?`,
		newState, nullableStr(it.StageName), nullableStr(opID), nullableStr(opName), nullableStr(opCode), flowID,
	)
}

// notifyApprovalDone 钉钉通知操作人
func (h *DashboardHandler) notifyApprovalDone(item approvalQueueItem, action string) {
	if h.Notifier == nil {
		return
	}
	var unionID sql.NullString
	h.DB.QueryRow(`SELECT dingtalk_userid FROM users WHERE id=?`, item.UserID).Scan(&unionID)
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
	h.DB.Exec(`
		INSERT INTO audit_logs (user_id, username, real_name, action, resource, detail, ip, user_agent)
		VALUES (?, ?, ?, 'hesi_approve', ?, ?, '', 'worker')`,
		item.UserID, item.Username, item.RealName, item.FlowID,
		fmt.Sprintf("%s 合思单据 %s [%s] (备注: %s)", label, item.FlowCode, item.FlowTitle, item.Comment),
	)
}
