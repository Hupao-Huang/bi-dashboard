package handler

import "encoding/json"

const (
	paymentSpecPrefix = "ID01KgaO6dcZtR" // 付款单 (票到付款/票到核销), B 开头
	prepaySpecPrefix  = "ID01FhdI9II9A3" // 预付款单 (先款后票), J 开头
)

// paymentTemplate 按 spec 前缀判模板类型: "payment"=付款单 / "prepay"=预付款单 / ""=不适用
func paymentTemplate(specID string) string {
	switch {
	case len(specID) >= len(paymentSpecPrefix) && specID[:len(paymentSpecPrefix)] == paymentSpecPrefix:
		return "payment"
	case len(specID) >= len(prepaySpecPrefix) && specID[:len(prepaySpecPrefix)] == prepaySpecPrefix:
		return "prepay"
	default:
		return ""
	}
}

// AuditPayment 张俊对外付款单审批规则引擎 (dry-run 建议态)。
// firstSubmitDate: 稳定首次提交时间 (同 AuditDailyExpense, 供发票时效用), 0=缺失。
func (h *DashboardHandler) AuditPayment(flowID, specID, rawJSON string, firstSubmitDate int64) *AuditSuggestion {
	tmpl := paymentTemplate(specID)
	if tmpl == "" {
		return &AuditSuggestion{Action: "agree", Reasons: []string{"非付款单模板, 不评估"}}
	}

	var raw map[string]interface{}
	if rawJSON != "" {
		_ = json.Unmarshal([]byte(rawJSON), &raw)
	}
	applyStableSubmitDate(firstSubmitDate, raw) // 复用 hesi_audit_rules.go 的稳定提交时间

	var rejectReasons []string
	var warnings []string

	// 后续任务在此追加 rule helper 调用 (按 tmpl 分支)
	_ = raw
	_ = flowID
	_ = tmpl

	if len(rejectReasons) > 0 {
		all := rejectReasons
		for _, w := range warnings {
			all = append(all, "[需人工核] "+w)
		}
		return &AuditSuggestion{Action: "reject", Reasons: all}
	}
	if len(warnings) > 0 {
		return &AuditSuggestion{Action: "manual", Reasons: warnings}
	}
	return &AuditSuggestion{Action: "agree", Reasons: []string{"所有规则通过"}}
}
