package handler

// 合思机器人"刷新"升级: 现场去合思拉指定审批人的实时待办, 更新本地后再读列表。
// POST /api/profile/hesi-pending/sync?approver=xxx | ?staffId=xxx (语义同 GetMyHesiPending)
// 三步:
//   1. 合思员工待办接口拉该审批人当前真实待办集合 B (注意 URL 的 $ 是合思的字面字符, 去掉就404)
//   2. B 里的单: 不在本地的整单入库(同 sync-hesi 的 saveFlow 口径); 全部用 approveStates 刷环节/审批人
//   3. 本地挂在该审批人名下但不在 B 里的单(已被别处审掉/驳回): 用单据详情接口拉真实状态盖掉
// 数据量 = 一个人的待办(几十单), 合思读接口不限流, 秒级完成。

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// HesiPendingSync POST /api/profile/hesi-pending/sync
func (h *DashboardHandler) HesiPendingSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// 解析目标审批人 (优先级同待审批列表: staffId > approver 姓名 > 当前用户绑定)
	staffID, displayName, errMsg := h.resolvePendingSyncTarget(payload, r)
	if errMsg != "" {
		writeError(w, 400, errMsg)
		return
	}

	token, err := h.getHesiToken()
	if err != nil {
		writeServerError(w, 500, "获取合思授权失败", err)
		return
	}

	// 1. 拉实时待办集合 B (分页, 每页100)
	backlog, err := h.fetchHesiBacklog(staffID, token)
	if err != nil {
		writeServerError(w, 500, "拉取合思实时待办失败", err)
		return
	}
	backlogIDs := make(map[string]bool, len(backlog))
	added := 0
	for _, item := range backlog {
		fid, _ := item["id"].(string)
		if fid == "" {
			continue
		}
		backlogIDs[fid] = true
		var exists int
		_ = h.DB.QueryRow(`SELECT COUNT(*) FROM hesi_flow WHERE flow_id=?`, fid).Scan(&exists)
		if err := h.upsertHesiFlowFromItem(item); err != nil {
			log.Printf("[hesi-pending-sync] 入库失败 flow=%s: %v", fid, err)
			continue
		}
		if exists == 0 {
			added++
		}
	}

	// 2. B 里的单逐个刷环节/审批人 (approveStates; 待办最多几十单, 读接口不限流)
	refreshed := 0
	for fid := range backlogIDs {
		stage, ops, found, okFetch := h.fetchApproveState(fid, token)
		if okFetch && found && len(ops) > 0 {
			h.writeFlowStateAfterApproval(fid, "agree", stage, ops) // ops非空 → state=approving
			refreshed++
		}
	}

	// 3. 本地挂该审批人名下但已不在实时待办里的单 → 拉真实状态盖掉 (被别处审掉/驳回/支付了)
	staleRows, err := h.DB.Query(`SELECT flow_id FROM hesi_flow
		WHERE active=1 AND state IN ('approving','pending') AND current_approver_id LIKE ?`,
		"%"+staffID+"%")
	if err != nil {
		writeServerError(w, 500, "查本地待审批失败", err)
		return
	}
	staleIDs := []string{}
	for staleRows.Next() {
		var fid string
		if staleRows.Scan(&fid) == nil && !backlogIDs[fid] {
			staleIDs = append(staleIDs, fid)
		}
	}
	staleRows.Close()

	removed := 0
	for _, fid := range staleIDs {
		stage, ops, found, okFetch := h.fetchApproveState(fid, token)
		if !okFetch {
			continue
		}
		if found {
			// 还在审批管道, 只是换了环节/审批人 → 刷成真实归属
			h.writeFlowStateAfterApproval(fid, "agree", stage, ops)
			removed++
			continue
		}
		// 已离开审批管道 → 拉单据详情拿真实终态 (rejected/paying/paid/archived)
		state := h.fetchHesiFlowState(fid, token)
		if state == "" {
			continue
		}
		if _, err := h.DB.Exec(`UPDATE hesi_flow SET state=?, current_stage_name=NULL,
			current_approver_id=NULL, current_approver_name=NULL, current_approver_code=NULL WHERE flow_id=?`,
			state, fid); err == nil {
			removed++
		}
	}

	writeJSON(w, map[string]interface{}{
		"approver": displayName,
		"pulled":   len(backlogIDs), // 合思实时待办数
		"added":    added,           // 本地原来没有的新单
		"removed":  removed,         // 从该审批人名下撤走的单
		"refreshed": refreshed,
	})
}

// resolvePendingSyncTarget 解析要同步谁的待办 (返回合思 staffId + 展示名; errMsg 非空=失败)
func (h *DashboardHandler) resolvePendingSyncTarget(payload *authPayload, r *http.Request) (string, string, string) {
	isAdmin := hasPermission(payload, "user.manage")
	if isAdmin {
		if sid := strings.TrimSpace(r.URL.Query().Get("staffId")); sid != "" {
			return sid, sid, ""
		}
		if a := strings.TrimSpace(r.URL.Query().Get("approver")); a != "" {
			// 多人会签拼接名 "A+B" 取第一个人
			name := strings.SplitN(a, "+", 2)[0]
			var sid string
			err := h.DB.QueryRow(`SELECT hesi_staff_id FROM hesi_employee_contract_company
				WHERE hesi_name=? AND hesi_staff_id<>'' LIMIT 1`, name).Scan(&sid)
			if err == sql.ErrNoRows || sid == "" {
				return "", "", fmt.Sprintf("审批人[%s]在花名册里匹配不到合思员工, 暂不支持现场同步", name)
			}
			if err != nil {
				return "", "", "查花名册失败: " + err.Error()
			}
			return sid, name, ""
		}
	}
	// 看自己: 用绑定的合思工号
	var hesiStaffID, realName string
	_ = h.DB.QueryRow(`SELECT IFNULL(hesi_staff_id,''), IFNULL(real_name,'') FROM users WHERE id=?`, payload.User.ID).
		Scan(&hesiStaffID, &realName)
	if hesiStaffID == "" {
		return "", "", "您的账号未绑定合思员工, 无法现场同步"
	}
	return hesiStaffID, realName, ""
}

// fetchHesiBacklog 分页拉员工实时待办 (合思 docs/byFlowId 接口, $ 为字面字符)
func (h *DashboardHandler) fetchHesiBacklog(staffID, token string) ([]map[string]interface{}, error) {
	all := []map[string]interface{}{}
	for index := 0; ; index += 100 {
		url := fmt.Sprintf("%s/api/openapi/v1.1/docs/byFlowId/$%s?accessToken=%s&index=%d&count=100",
			hesiAPIBase, staffID, token, index)
		resp, err := hesiHTTP.Get(url)
		if err != nil {
			return nil, err
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("合思待办接口 HTTP %d: %.200s", resp.StatusCode, string(data))
		}
		var parsed struct {
			Count int                      `json:"count"`
			Items []map[string]interface{} `json:"items"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, err
		}
		all = append(all, parsed.Items...)
		if len(all) >= parsed.Count || len(parsed.Items) == 0 {
			return all, nil
		}
	}
}

// fetchHesiFlowState 拉单据详情只取 state (离开审批管道后的真实终态)
func (h *DashboardHandler) fetchHesiFlowState(flowID, token string) string {
	resp, err := hesiHTTP.Get(fmt.Sprintf("%s/api/openapi/v1.1/flowDetails?flowId=%s&accessToken=%s", hesiAPIBase, flowID, token))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var parsed struct {
		Value struct {
			State string `json:"state"`
		} `json:"value"`
	}
	if json.Unmarshal(data, &parsed) != nil {
		return ""
	}
	return parsed.Value.State
}

// ---------- 单据整单入库 (口径同 cmd/sync-hesi 的 saveFlow, 这里是 handler 侧移植版) ----------

func hpsStr(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func hpsInt64(m map[string]interface{}, key string) int64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	}
	return 0
}

func hpsMoney(m map[string]interface{}, key string) sql.NullFloat64 {
	v, ok := m[key]
	if !ok || v == nil {
		return sql.NullFloat64{}
	}
	moneyMap, ok := v.(map[string]interface{})
	if !ok {
		return sql.NullFloat64{}
	}
	std := hpsStr(moneyMap, "standard")
	if std == "" {
		return sql.NullFloat64{}
	}
	f, err := strconv.ParseFloat(std, 64)
	if err != nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: f, Valid: true}
}

func (h *DashboardHandler) upsertHesiFlowFromItem(item map[string]interface{}) error {
	flowID := hpsStr(item, "id")
	form, _ := item["form"].(map[string]interface{})
	if flowID == "" || form == nil {
		return fmt.Errorf("待办项缺 id/form")
	}
	active := 1
	if b, ok := item["active"].(bool); ok && !b {
		active = 0
	}
	deptID := hpsStr(form, "expenseDepartment")
	if deptID == "" {
		deptID = hpsStr(form, "loanDepartment")
	}
	rawJSON, _ := json.Marshal(form)
	_, err := h.DB.Exec(`INSERT INTO hesi_flow
		(flow_id, code, title, form_type, state, owner_id, owner_department,
		 submitter_id, department_id, pay_money, expense_money, loan_money, receipt_money,
		 create_time, update_time, submit_date, pay_date, flow_end_time,
		 voucher_no, voucher_status, corporation_id, specification_id, active, raw_json)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
			title=VALUES(title), state=VALUES(state), active=VALUES(active),
			pay_money=VALUES(pay_money), expense_money=VALUES(expense_money),
			loan_money=VALUES(loan_money), receipt_money=VALUES(receipt_money),
			update_time=VALUES(update_time), pay_date=VALUES(pay_date),
			flow_end_time=VALUES(flow_end_time),
			voucher_no=VALUES(voucher_no), voucher_status=VALUES(voucher_status),
			raw_json=VALUES(raw_json)`,
		flowID, hpsStr(form, "code"), hpsStr(form, "title"), hpsStr(item, "formType"), hpsStr(item, "state"),
		nullableStr(hpsStr(item, "ownerId")), nullableStr(hpsStr(item, "ownerDefaultDepartment")),
		nullableStr(hpsStr(form, "submitterId")), nullableStr(deptID),
		hpsMoney(form, "payMoney"), hpsMoney(form, "expenseMoney"), hpsMoney(form, "loanMoney"), hpsMoney(form, "receiptMoney"),
		nullableInt64(hpsInt64(item, "createTime")), nullableInt64(hpsInt64(item, "updateTime")),
		nullableInt64(hpsInt64(form, "submitDate")), nullableInt64(hpsInt64(form, "payDate")), nullableInt64(hpsInt64(form, "flowEndTime")),
		nullableStr(hpsStr(form, "voucherNo")), nullableStr(hpsStr(form, "voucherStatus")),
		nullableStr(hpsStr(item, "corporationId")), nullableStr(hpsStr(form, "specificationId")),
		active, string(rawJSON))
	return err
}

func nullableInt64(n int64) sql.NullInt64 {
	if n == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: n, Valid: true}
}
