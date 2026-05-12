package handler

// v1.59.0 个人中心 → 合思机器人 Tab "我的待审批" (只读)
// 当前登录用户 real_name 模糊匹配 hesi_flow.current_approver_name, 拿待审批清单

import (
	"net/http"
	"strings"
)

type myHesiPendingRow struct {
	FlowID         string   `json:"flowId"`
	Code           string   `json:"code"`
	Title          string   `json:"title"`
	FormType       string   `json:"formType"`
	State          string   `json:"state"`
	StageName      *string  `json:"stageName"`
	ApproverName   *string  `json:"approverName"`   // 当前审批人 (可能多人, 含逗号/+)
	PayMoney       *float64 `json:"payMoney"`
	ExpenseMoney   *float64 `json:"expenseMoney"`
	LoanMoney      *float64 `json:"loanMoney"`
	SubmitDate     *int64   `json:"submitDate"`
	SubmitterId    *string  `json:"submitterId"`
	DepartmentId   *string  `json:"departmentId"`
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

	var realName string
	if err := h.DB.QueryRow(`SELECT IFNULL(real_name,'') FROM users WHERE id=?`, payload.User.ID).Scan(&realName); err != nil {
		writeServerError(w, 500, "查用户失败", err)
		return
	}
	realName = strings.TrimSpace(realName)
	if realName == "" {
		// 没填 real_name 直接返空, 提示前端
		writeJSON(w, map[string]interface{}{
			"realName": "",
			"items":    []myHesiPendingRow{},
			"warning":  "您账号未填真实姓名, 无法匹配合思待审批. 请到上方'个人信息'页填真实姓名.",
		})
		return
	}

	rows, err := h.DB.Query(`SELECT flow_id, code, IFNULL(title,''), form_type, state,
		current_stage_name, current_approver_name,
		pay_money, expense_money, loan_money, submit_date, submitter_id, department_id
		FROM hesi_flow
		WHERE active=1
		  AND state IN ('approving','paying','pending')
		  AND current_approver_name LIKE ?
		ORDER BY submit_date DESC, create_time DESC
		LIMIT 500`, "%"+realName+"%")
	if err != nil {
		writeServerError(w, 500, "查待审批失败", err)
		return
	}
	defer rows.Close()

	items := []myHesiPendingRow{}
	for rows.Next() {
		var row myHesiPendingRow
		if err := rows.Scan(&row.FlowID, &row.Code, &row.Title, &row.FormType, &row.State,
			&row.StageName, &row.ApproverName,
			&row.PayMoney, &row.ExpenseMoney, &row.LoanMoney, &row.SubmitDate,
			&row.SubmitterId, &row.DepartmentId); err != nil {
			writeServerError(w, 500, "扫描失败", err)
			return
		}
		items = append(items, row)
	}

	writeJSON(w, map[string]interface{}{
		"realName": realName,
		"items":    items,
		"count":    len(items),
	})
}
