package handler

// v1.59.0 个人中心 → 合思机器人 Tab "我的待审批" (只读)
// 当前登录用户 real_name 模糊匹配 hesi_flow.current_approver_name, 拿待审批清单
// v1.62.x: 字段对齐费控管理 (单据模板/创建时间/明细发票/附件统计)

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

// GetHesiApprovers GET /api/profile/hesi-approvers
// 返回所有当前有待审批的审批人列表 (distinct), 仅管理员可调
func (h *DashboardHandler) GetHesiApprovers(w http.ResponseWriter, r *http.Request) {
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !hasPermission(payload, "user.manage") {
		writeError(w, http.StatusForbidden, "无权限")
		return
	}
	rows, err := h.DB.Query(`SELECT current_approver_name, COUNT(*) AS cnt
		FROM hesi_flow
		WHERE active=1 AND state IN ('approving','paying','pending') AND current_approver_name IS NOT NULL AND current_approver_name<>''
		GROUP BY current_approver_name
		ORDER BY cnt DESC, current_approver_name`)
	if err != nil {
		writeServerError(w, 500, "查询审批人失败", err)
		return
	}
	defer rows.Close()
	type approverItem struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	var items []approverItem
	for rows.Next() {
		var it approverItem
		if err := rows.Scan(&it.Name, &it.Count); err == nil {
			items = append(items, it)
		}
	}
	writeJSON(w, map[string]interface{}{"items": items, "count": len(items)})
}

type myHesiPendingRow struct {
	FlowID              string           `json:"flowId"`
	Code                string           `json:"code"`
	Title               string           `json:"title"`
	FormType            string           `json:"formType"`
	State               string           `json:"state"`
	StageName           *string          `json:"stageName"`
	ApproverName        *string          `json:"approverName"` // 当前审批人 (可能多人, 含逗号/+)
	CurrentApproverCode *string          `json:"currentApproverCode"`
	PayMoney            *float64         `json:"payMoney"`
	ExpenseMoney        *float64         `json:"expenseMoney"`
	LoanMoney           *float64         `json:"loanMoney"`
	CreateTime          *int64           `json:"createTime"`
	UpdateTime          *int64           `json:"updateTime"`
	SubmitDate          *int64           `json:"submitDate"`
	SubmitterId         *string          `json:"submitterId"`
	DepartmentId        *string          `json:"departmentId"`
	PreApprovedNode     *string          `json:"preApprovedNode"`
	PreApprovedTime     *string          `json:"preApprovedTime"`
	SpecificationId     *string          `json:"specificationId"`
	SpecificationName   string           `json:"specificationName"`
	DetailCount         int              `json:"detailCount"`
	InvoiceExist        int              `json:"invoiceExist"`
	InvoiceMissing      int              `json:"invoiceMissing"`
	AttachmentCount     int              `json:"attachmentCount"`
	Suggestion          *AuditSuggestion `json:"suggestion,omitempty"` // v1.63 MVP 报销单审批建议
}

// GetMyHesiPending GET /api/profile/hesi-pending
// 返回登录用户当前待审批的合思单据 (按 real_name 模糊匹配 current_approver_name)
func (h *DashboardHandler) GetMyHesiPending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// v1.60.1: 优先用 hesi_staff_id 精确匹配, 没绑定再 fallback real_name
	var realName, hesiStaffID, hesiRealName string
	if err := h.DB.QueryRow(`SELECT IFNULL(real_name,''), IFNULL(hesi_staff_id,''), IFNULL(hesi_real_name,'') FROM users WHERE id=?`, payload.User.ID).Scan(&realName, &hesiStaffID, &hesiRealName); err != nil {
		writeServerError(w, 500, "查用户失败", err)
		return
	}
	realName = strings.TrimSpace(realName)
	hesiStaffID = strings.TrimSpace(hesiStaffID)
	hesiRealName = strings.TrimSpace(hesiRealName)

	// v1.59.3: 管理员可以传 ?approver=xxx (姓名) 查别人的待审批; 或 ?staffId=xxx 精确查
	// 优先级: ?staffId > ?approver > 当前用户的 hesi_staff_id > 当前用户的 real_name
	isAdmin := hasPermission(payload, "user.manage")
	queryStaffID := hesiStaffID
	queryName := realName
	displayName := hesiRealName
	if displayName == "" {
		displayName = realName
	}
	if isAdmin {
		if sid := strings.TrimSpace(r.URL.Query().Get("staffId")); sid != "" {
			queryStaffID = sid
			queryName = ""
			displayName = sid
		} else if a := strings.TrimSpace(r.URL.Query().Get("approver")); a != "" {
			queryStaffID = ""
			queryName = a
			displayName = a
		}
	}

	if queryStaffID == "" && queryName == "" {
		writeJSON(w, map[string]interface{}{
			"realName":  realName,
			"queryName": "",
			"isAdmin":   isAdmin,
			"items":     []myHesiPendingRow{},
			"warning":   "您账号未绑定合思员工, 无法匹配待审批单据. 请联系管理员绑定合思真名/工号.",
		})
		return
	}

	// v1.63 MVP: AI 审批建议规则当前仅适用于张俊 (Excel 私有定义), 其他审批人不跑
	enableAuditSuggestion := strings.Contains(displayName, "张俊") ||
		strings.Contains(queryName, "张俊") ||
		strings.Contains(hesiRealName, "张俊")

	// v1.62.x: SELECT 字段对齐费控管理 (含 specification_id / create_time / update_time / preApproved)
	selectFields := `flow_id, code, IFNULL(title,''), form_type, state,
			current_stage_name, current_approver_name, current_approver_code,
			pay_money, expense_money, loan_money,
			create_time, update_time, submit_date, submitter_id, department_id,
			JSON_UNQUOTE(JSON_EXTRACT(raw_json, '$.preApprovedNodeName')) AS pre_approved_node,
			JSON_UNQUOTE(JSON_EXTRACT(raw_json, '$.preNodeApprovedTime')) AS pre_approved_time,
			specification_id, IFNULL(raw_json,'')`

	var (
		rows *sql.Rows
		err  error
	)
	if queryStaffID != "" {
		// 精确匹配: current_approver_id 含 staffId (格式 corp:staff, LIKE %staff%)
		rows, err = h.DB.Query(`SELECT `+selectFields+`
			FROM hesi_flow
			WHERE active=1
			  AND state IN ('approving','paying','pending')
			  AND current_approver_id LIKE ?
			ORDER BY submit_date DESC, create_time DESC
			LIMIT 500`, "%"+queryStaffID+"%")
	} else {
		// 兜底 fallback: 姓名模糊
		rows, err = h.DB.Query(`SELECT `+selectFields+`
			FROM hesi_flow
			WHERE active=1
			  AND state IN ('approving','paying','pending')
			  AND current_approver_name LIKE ?
			ORDER BY submit_date DESC, create_time DESC
			LIMIT 500`, "%"+queryName+"%")
	}
	if err != nil {
		writeServerError(w, 500, "查待审批失败", err)
		return
	}
	defer rows.Close()

	items := []myHesiPendingRow{}
	for rows.Next() {
		var row myHesiPendingRow
		var rawJSON string
		if err := rows.Scan(&row.FlowID, &row.Code, &row.Title, &row.FormType, &row.State,
			&row.StageName, &row.ApproverName, &row.CurrentApproverCode,
			&row.PayMoney, &row.ExpenseMoney, &row.LoanMoney,
			&row.CreateTime, &row.UpdateTime, &row.SubmitDate,
			&row.SubmitterId, &row.DepartmentId,
			&row.PreApprovedNode, &row.PreApprovedTime,
			&row.SpecificationId, &rawJSON); err != nil {
			writeServerError(w, 500, "扫描失败", err)
			return
		}
		if row.SpecificationId != nil && *row.SpecificationId != "" {
			row.SpecificationName = h.LookupSpecName(*row.SpecificationId)
		}
		// v1.63 MVP: 仅对张俊作为审批人的报销单跑审批建议规则 (规则源自张俊 Excel)
		if enableAuditSuggestion && row.FormType == "expense" {
			row.Suggestion = AuditExpenseFlow(rawJSON)
		}
		items = append(items, row)
	}

	// v1.62.x: 批量明细统计 (count/已开票/未开票) + 附件统计, 一次性查避免 N+1
	if len(items) > 0 {
		flowIDs := make([]interface{}, len(items))
		ph := make([]string, len(items))
		for i := range items {
			flowIDs[i] = items[i].FlowID
			ph[i] = "?"
		}
		phJoin := strings.Join(ph, ",")

		type ds struct{ count, exist, missing int }
		detailMap := make(map[string]ds, len(items))
		drows, derr := h.DB.Query(fmt.Sprintf(
			`SELECT flow_id, COUNT(*), COALESCE(SUM(invoice_status='exist'),0), COALESCE(SUM(invoice_status='noExist'),0)
			 FROM hesi_flow_detail WHERE flow_id IN (%s) GROUP BY flow_id`, phJoin), flowIDs...)
		if derr == nil {
			for drows.Next() {
				var fid string
				var s ds
				if e := drows.Scan(&fid, &s.count, &s.exist, &s.missing); e == nil {
					detailMap[fid] = s
				}
			}
			drows.Close()
		}

		attachMap := make(map[string]int, len(items))
		arows, aerr := h.DB.Query(fmt.Sprintf(
			`SELECT flow_id, COUNT(*) FROM hesi_flow_attachment WHERE flow_id IN (%s) GROUP BY flow_id`, phJoin), flowIDs...)
		if aerr == nil {
			for arows.Next() {
				var fid string
				var c int
				if e := arows.Scan(&fid, &c); e == nil {
					attachMap[fid] = c
				}
			}
			arows.Close()
		}

		for i := range items {
			if s, ok := detailMap[items[i].FlowID]; ok {
				items[i].DetailCount = s.count
				items[i].InvoiceExist = s.exist
				items[i].InvoiceMissing = s.missing
			}
			items[i].AttachmentCount = attachMap[items[i].FlowID]
		}
	}

	writeJSON(w, map[string]interface{}{
		"realName":  realName,     // BI 看板 real_name (昵称)
		"queryName": displayName,  // 实际查询展示名 (优先 hesi_real_name, 管理员切人时为对方名)
		"staffId":   queryStaffID, // 实际查询用的合思 staffId (空=用姓名兜底)
		"isAdmin":   isAdmin,      // 前端据此显示/隐藏切换控件
		"items":     items,
		"count":     len(items),
	})
}

// authorizeFlowAccess 鉴权: 单据当前审批人本人 / 提交人 / 管理员可看
// 返回 (allowed, errMsg)
func (h *DashboardHandler) authorizeFlowAccess(r *http.Request, flowID string) (bool, string) {
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		return false, "unauthorized"
	}
	if hasPermission(payload, "user.manage") {
		return true, ""
	}
	if hasPermission(payload, "finance.expense:view") {
		return true, ""
	}
	var approverID, approverName, submitterID sql.NullString
	if err := h.DB.QueryRow(
		`SELECT current_approver_id, current_approver_name, submitter_id FROM hesi_flow WHERE flow_id=?`,
		flowID,
	).Scan(&approverID, &approverName, &submitterID); err != nil {
		return false, "单据不存在"
	}
	var realName, hesiStaffID string
	_ = h.DB.QueryRow(`SELECT IFNULL(real_name,''), IFNULL(hesi_staff_id,'') FROM users WHERE id=?`, payload.User.ID).Scan(&realName, &hesiStaffID)
	realName = strings.TrimSpace(realName)
	hesiStaffID = strings.TrimSpace(hesiStaffID)

	if hesiStaffID != "" && approverID.Valid && strings.Contains(approverID.String, hesiStaffID) {
		return true, ""
	}
	if hesiStaffID != "" && submitterID.Valid && strings.Contains(submitterID.String, hesiStaffID) {
		return true, ""
	}
	if realName != "" && approverName.Valid && strings.Contains(approverName.String, realName) {
		return true, ""
	}
	return false, "您不是此单据的审批人/提交人, 无权查看"
}

// GetMyHesiFlowDetail GET /api/profile/hesi-flow-detail?flowId=xxx
// 个人中心/合思机器人页用的单据详情, 鉴权: 审批人本人 / 提交人 / 管理员
func (h *DashboardHandler) GetMyHesiFlowDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	flowID := strings.TrimSpace(r.URL.Query().Get("flowId"))
	if flowID == "" {
		writeError(w, 400, "缺少flowId参数")
		return
	}
	ok, msg := h.authorizeFlowAccess(r, flowID)
	if !ok {
		if msg == "单据不存在" {
			writeError(w, 404, msg)
		} else {
			writeError(w, http.StatusForbidden, msg)
		}
		return
	}
	h.GetHesiFlowDetail(w, r)
}

// GetMyHesiAttachmentURLs GET /api/profile/hesi-attachment-urls?flowId=xxx
// 同上, 鉴权后 delegate
func (h *DashboardHandler) GetMyHesiAttachmentURLs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	flowID := strings.TrimSpace(r.URL.Query().Get("flowId"))
	if flowID == "" {
		writeError(w, 400, "缺少flowId参数")
		return
	}
	ok, msg := h.authorizeFlowAccess(r, flowID)
	if !ok {
		if msg == "单据不存在" {
			writeError(w, 404, msg)
		} else {
			writeError(w, http.StatusForbidden, msg)
		}
		return
	}
	h.GetHesiAttachmentURLs(w, r)
}
