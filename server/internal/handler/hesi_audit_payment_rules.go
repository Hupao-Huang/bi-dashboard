package handler

import (
	"encoding/json"
	"fmt"
	"strings"
)

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
	if raw == nil {
		raw = make(map[string]interface{})
	}
	applyStableSubmitDate(firstSubmitDate, raw) // 复用 hesi_audit_rules.go 的稳定提交时间

	var rejectReasons []string
	var warnings []string

	// A1: 所属公司 (法人实体) 必须非空 — 付款单+预付款单都做
	if r := rulePaymentOwnerCompany(raw); r != "" {
		rejectReasons = append(rejectReasons, r)
	}

	// A2: 申请部门末级 — 付款单用 expenseDepartment; 预付款单优先 loanDepartment, 兜底 expenseDepartment
	// 裁定为"转人工提醒", 不硬驳 (Global Constraints: 数据缺失/拿不准 → manual)
	deptID, _ := raw["expenseDepartment"].(string)
	if tmpl == "prepay" {
		if ld, _ := raw["loanDepartment"].(string); ld != "" {
			deptID = ld
		}
	}
	if w := paymentDeptLeafWarn(h.ruleDeptLeaf(deptID, "申请部门 (A2)")); w != "" {
		warnings = append(warnings, w)
	}

	// A6+A7: 收款信息合规 + 防自付 — 付款单+预付款单都做
	if r := h.rulePaymentPayee(raw); r != "" {
		rejectReasons = append(rejectReasons, r)
	}

	// A13: 阳光天际 → 悦伍公司 — 付款单+预付款单都做 (sunshineRule only, NOT A14)
	leName := h.LookupLegalEntityName(strOfPayment(raw["法人实体"]))
	reason, _ := raw["description"].(string)
	if r := sunshineRule(reason, leName); r != "" {
		rejectReasons = append(rejectReasons, r)
	}

	// A18: 支付金额上限 — 仅付款单 (预付款单无发票, 跳过)
	if tmpl == "payment" {
		if w := rulePaymentAmountCap(raw, h.sumInvoiceTotal(flowID)); w != "" {
			warnings = append(warnings, w)
		}
	}

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

// ----- A1: 所属公司非空 -----

// rulePaymentOwnerCompany A1 所属公司 (法人实体) 必须非空。
// 与支付主体的实质一致性由 B2 (发票购买方=所属公司) 兜底。
func rulePaymentOwnerCompany(raw map[string]interface{}) string {
	if id, _ := raw["法人实体"].(string); id == "" {
		return "所属公司 (法人实体) 为空 (A1)"
	}
	return ""
}

// ----- A2: 申请部门末级 (转人工) -----

// paymentDeptLeafWarn A2 申请部门末级: 裁定为"转人工提醒", 不硬驳。
// 空输入 (末级或部门不在树) → 返回 "", 不产生提醒。
func paymentDeptLeafWarn(deptLeafReason string) string {
	return deptLeafReason // 非空即提醒文案, 由引擎塞入 warnings
}

// ----- A6+A7: 收款信息合规 + 防自付 -----

// selfPayReason A7 防自付: 收款方户名 = 所属公司名 → 驳回。
func selfPayReason(payeeName, ownerCompany string) string {
	if payeeName != "" && ownerCompany != "" && payeeName == ownerCompany {
		return "收款方与所属公司同名, 疑似自我付款 (A7)"
	}
	return ""
}

// rulePaymentPayee A6+A7 收款信息: 银行账户合规 (复用 rulePayeeBank) + 防自付。
func (h *DashboardHandler) rulePaymentPayee(raw map[string]interface{}) string {
	payeeID, _ := raw["payeeId"].(string)
	if payeeID == "" {
		return "收款信息为空 (A6)"
	}
	if r := rulePayeeBank(h.DB, payeeID); r != "" {
		return r // A6: 非银行账户等
	}
	leID, _ := raw["法人实体"].(string)
	return selfPayReason(h.payeeName(payeeID), h.LookupLegalEntityName(leID))
}

// ----- A13: 阳光天际 → 悦伍公司 -----

// sunshineRule A13 阳光天际→悦伍: 付款事由含「阳光天际」时, 所属公司必须含「悦伍」。
func sunshineRule(reason, ownerCompanyName string) string {
	if strings.Contains(reason, "阳光天际") && !strings.Contains(ownerCompanyName, "悦伍") {
		return "付款事由含「阳光天际」, 所属公司应为杭州松鲜鲜悦伍食品科技有限公司 (A13)"
	}
	return ""
}

// ----- A18: 支付金额上限 (仅付款单) -----

// rulePaymentAmountCap A18 支付金额 ≤ 发票合计。超出 → 转人工。
// invoiceTotal=0 表示无发票 (如无票费用) → 跳过, 不误驳。
func rulePaymentAmountCap(raw map[string]interface{}, invoiceTotal float64) string {
	pay := toFloatPayment(raw["payMoney"])
	if invoiceTotal > 0 && pay > invoiceTotal+0.01 {
		return fmt.Sprintf("支付额 ¥%.2f 超过发票合计 ¥%.2f, 请人工核对 (A18)", pay, invoiceTotal)
	}
	return ""
}

// ----- 内部 helpers -----

// toFloatPayment 安全转 float64 (JSON number 解析为 float64)
func toFloatPayment(v interface{}) float64 {
	f, _ := v.(float64)
	return f
}

// strOfPayment 安全转 string
func strOfPayment(v interface{}) string {
	s, _ := v.(string)
	return s
}
