package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
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

	// A8: 明细行数/无票 — 付款单无明细→转人工; 预付款单可无明细→放行
	if rej, w := rulePaymentItemRows(raw, tmpl, codeOf(raw)); rej != "" || w != "" {
		if rej != "" {
			rejectReasons = append(rejectReasons, rej)
		}
		if w != "" {
			warnings = append(warnings, w)
		}
	}

	// A3: 客户多选 — 仅付款单 (预付款单无此字段)
	if tmpl == "payment" {
		if w := rulePaymentCustomer(raw); w != "" {
			warnings = append(warnings, w)
		}
	}

	// B1/B2: 收款方=开票方 + 购买方=所属公司 — 仅付款单 (预付款单无发票)
	if tmpl == "payment" {
		if rej, w := h.rulePaymentInvoiceParties(flowID, raw); len(rej) > 0 || len(w) > 0 {
			rejectReasons = append(rejectReasons, rej...)
			warnings = append(warnings, w...)
		}
	}

	// B4: 防重复付款 — 付款单+预付款单都做
	if w := h.rulePaymentDuplicate(flowID, raw); w != "" {
		warnings = append(warnings, w)
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

// ----- A8: 明细行数/无票 -----

// rulePaymentItemRows A8 明细行数: 预付款单(J)可无明细; 付款单(B)无明细→转人工。
// 返回 (reject, warn): reject=确定违规; warn=转人工提醒。
func rulePaymentItemRows(raw map[string]interface{}, tmpl, code string) (reject, warn string) {
	details, _ := raw["details"].([]interface{})
	if len(details) == 0 {
		if tmpl == "prepay" {
			return "", "" // 先款后票, 可无明细
		}
		return "", fmt.Sprintf("付款单 %s 无费用明细, 请人工确认 (A8)", code)
	}
	return "", ""
}

// ----- A3: 客户多选 (仅付款单) -----

// rulePaymentCustomer A3 客户多选: 空→转人工提醒, 不硬驳 (口径未完全定)。
func rulePaymentCustomer(raw map[string]interface{}) string {
	v, _ := raw["u_客户多选"].(string) // 存的是 JSON 字符串数组
	if strings.TrimSpace(v) == "" || v == "[]" || v == "null" {
		return "客户多选为空, 请确认是否应选虚拟客户 (A3)"
	}
	return ""
}

// ----- B1/B2: 收款方=开票方 + 购买方=所属公司 (仅付款单) -----

// payeeSellerMismatch B1 收款方=开票方: 收款方与任一发票开票方不一致→转人工。
// 缺数据时返回 "" (由 rulePaymentInvoiceParties 统一兜底)。
func payeeSellerMismatch(payee string, sellers []string) string {
	if payee == "" || len(sellers) == 0 {
		return ""
	}
	for _, s := range sellers {
		if s != "" && s == payee {
			return ""
		}
	}
	return "收款方与发票开票方不一致, 请人工核对 (B1)"
}

// buyerCompanyMismatch B2 购买方=所属公司: 全部发票购买方均须一致→否则驳回。
// 缺数据时返回 "" (由 rulePaymentInvoiceParties 统一兜底)。
func buyerCompanyMismatch(buyers []string, ownerCompany string) string {
	if ownerCompany == "" || len(buyers) == 0 {
		return ""
	}
	for _, b := range buyers {
		if b != "" && b != ownerCompany {
			return "发票购买方与所属公司不一致 (B2)"
		}
	}
	return ""
}

// invoiceParties 从 hesi_flow_invoice 查 flowID 对应的购买方/开票方列表 (5s 超时)。
// 查询失败/无数据/DB 为 nil 均返回 nil, nil (由调用方转人工兜底)。
func (h *DashboardHandler) invoiceParties(flowID string) (buyers, sellers []string) {
	if h.DB == nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := h.DB.QueryContext(ctx,
		`SELECT IFNULL(buyer_name,''), IFNULL(seller_name,'') FROM hesi_flow_invoice WHERE flow_id=?`, flowID)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()
	for rows.Next() {
		var b, s string
		if rows.Scan(&b, &s) == nil {
			if b != "" {
				buyers = append(buyers, b)
			}
			if s != "" {
				sellers = append(sellers, s)
			}
		}
	}
	_ = rows.Err()
	return buyers, sellers
}

// rulePaymentInvoiceParties B1+B2 发票方核验: 收款方=开票方 (转人工); 购买方=所属公司 (驳回)。
// 仅付款单调用; 预付款单无发票跳过。
func (h *DashboardHandler) rulePaymentInvoiceParties(flowID string, raw map[string]interface{}) (reject, warn []string) {
	buyers, sellers := h.invoiceParties(flowID)
	if len(buyers) == 0 && len(sellers) == 0 {
		warn = append(warn, "未识别到发票购买方/开票方, 请人工核对 (B1/B2)")
		return reject, warn
	}
	owner := h.LookupLegalEntityName(strOfPayment(raw["法人实体"]))
	if r := buyerCompanyMismatch(buyers, owner); r != "" {
		reject = append(reject, r)
	}
	if w := payeeSellerMismatch(h.payeeName(strOfPayment(raw["payeeId"])), sellers); w != "" {
		warn = append(warn, w)
	}
	return reject, warn
}

// ----- B4: 防重复付款 (付款单+预付款单) -----

// dupWarnText B4 重复付款提示文案 (纯逻辑, 便于单测)。
func dupWarnText(dupeCodes []string) string {
	if len(dupeCodes) == 0 {
		return ""
	}
	return "疑似重复付款 (同收款方+同金额), 历史单号: " + strings.Join(dupeCodes, ", ") + " (B4)"
}

// rulePaymentDuplicate B4 防重复付款: 同收款方+金额完全一致的历史单→转人工+提示原单号。
// 付款单+预付款单都做; 查询失败/DB 为 nil 时静默 (安全底线: 不误驳)。
func (h *DashboardHandler) rulePaymentDuplicate(flowID string, raw map[string]interface{}) string {
	if h.DB == nil {
		return ""
	}
	payeeID, _ := raw["payeeId"].(string)
	amount := toFloatPayment(raw["payMoney"])
	if payeeID == "" || amount <= 0 {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := h.DB.QueryContext(ctx,
		`SELECT code FROM hesi_flow
		 WHERE flow_id<>? AND active=1
		   AND JSON_UNQUOTE(JSON_EXTRACT(raw_json,'$.payeeId'))=?
		   AND ABS(IFNULL(pay_money,0)-?)<0.01
		   AND (specification_id LIKE 'ID01KgaO6dcZtR%' OR specification_id LIKE 'ID01FhdI9II9A3%')
		 LIMIT 5`,
		flowID, payeeID, amount)
	if err != nil {
		return ""
	}
	defer rows.Close()
	var codes []string
	for rows.Next() {
		var c string
		if rows.Scan(&c) == nil {
			codes = append(codes, c)
		}
	}
	_ = rows.Err()
	return dupWarnText(codes)
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

// codeOf 从 raw 取单据编号 (code 字段), 用于错误提示。
func codeOf(raw map[string]interface{}) string {
	s, _ := raw["code"].(string)
	return s
}
