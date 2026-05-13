package handler

// v1.59.0 个人中心 → 合思机器人 Tab "我的待审批" (只读)
// 当前登录用户 real_name 模糊匹配 hesi_flow.current_approver_name, 拿待审批清单

import (
	"database/sql"
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
	FlowID         string           `json:"flowId"`
	Code           string           `json:"code"`
	Title          string           `json:"title"`
	FormType       string           `json:"formType"`
	State          string           `json:"state"`
	StageName      *string          `json:"stageName"`
	ApproverName   *string          `json:"approverName"`   // 当前审批人 (可能多人, 含逗号/+)
	PayMoney       *float64         `json:"payMoney"`
	ExpenseMoney   *float64         `json:"expenseMoney"`
	LoanMoney      *float64         `json:"loanMoney"`
	SubmitDate     *int64           `json:"submitDate"`
	SubmitterId    *string          `json:"submitterId"`
	DepartmentId   *string          `json:"departmentId"`
	Suggestion     *AuditSuggestion `json:"suggestion,omitempty"` // v1.63 MVP 报销单审批建议
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

	var (
		rows *sql.Rows
		err  error
	)
	if queryStaffID != "" {
		// 精确匹配: current_approver_id 含 staffId (格式 corp:staff, LIKE %staff%)
		rows, err = h.DB.Query(`SELECT flow_id, code, IFNULL(title,''), form_type, state,
			current_stage_name, current_approver_name,
			pay_money, expense_money, loan_money, submit_date, submitter_id, department_id, IFNULL(raw_json,'')
			FROM hesi_flow
			WHERE active=1
			  AND state IN ('approving','paying','pending')
			  AND current_approver_id LIKE ?
			ORDER BY submit_date DESC, create_time DESC
			LIMIT 500`, "%"+queryStaffID+"%")
	} else {
		// 兜底 fallback: 姓名模糊
		rows, err = h.DB.Query(`SELECT flow_id, code, IFNULL(title,''), form_type, state,
			current_stage_name, current_approver_name,
			pay_money, expense_money, loan_money, submit_date, submitter_id, department_id, IFNULL(raw_json,'')
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
			&row.StageName, &row.ApproverName,
			&row.PayMoney, &row.ExpenseMoney, &row.LoanMoney, &row.SubmitDate,
			&row.SubmitterId, &row.DepartmentId, &rawJSON); err != nil {
			writeServerError(w, 500, "扫描失败", err)
			return
		}
		// v1.63 MVP: 仅对报销单跑审批建议规则
		if row.FormType == "expense" {
			row.Suggestion = AuditExpenseFlow(rawJSON)
		}
		items = append(items, row)
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
