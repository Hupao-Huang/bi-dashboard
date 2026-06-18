package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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

	// 付款单的 A18 / B1·B2 / B3 三条规则都要发票数据, 一次查替代原先各查一次 (3→1 性能合并)。
	// 预付款单无发票, 不查 (inv 零值: total=0/列表 nil/count=0, 这三条规则本就跳过预付款单)。
	var inv paymentInvoice
	if tmpl == "payment" {
		inv = h.paymentInvoiceData(flowID)
	}

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
	// Bug 1 fix: 付款单事由在 details[].feeTypeForm.consumptionReasons (逐行), 不在 description。
	// 预付款单事由在 description。两者都 belt-and-suspenders 拼入 reasonText。
	leName := h.LookupLegalEntityName(strOfPayment(raw["法人实体"]))
	if r := sunshineRule(extractPaymentReasonText(raw, tmpl), leName); r != "" {
		rejectReasons = append(rejectReasons, r)
	}

	// A18: 支付金额上限 — 仅付款单 (预付款单无发票, 跳过)
	if tmpl == "payment" {
		if w := rulePaymentAmountCap(raw, inv.total); w != "" {
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
	// 用预拉的 inv.buyers/inv.sellers, 不再单独查发票表 (合并进 paymentInvoiceData)。
	if tmpl == "payment" {
		owner := h.LookupLegalEntityName(strOfPayment(raw["法人实体"]))
		payee := h.payeeName(strOfPayment(raw["payeeId"]))
		if rej, w := rulePaymentInvoicePartiesLogic(inv.buyers, inv.sellers, payee, owner); len(rej) > 0 || len(w) > 0 {
			rejectReasons = append(rejectReasons, rej...)
			warnings = append(warnings, w...)
		}
	}

	// B4: 防重复付款 — 付款单+预付款单都做
	if w := h.rulePaymentDuplicate(flowID, raw); w != "" {
		warnings = append(warnings, w)
	}

	// A5: 事由必填 + 不含合计/小计 — 付款单+预付款单都做
	if r := rulePaymentReason(raw, tmpl); r != "" {
		rejectReasons = append(rejectReasons, r)
	}

	// A17: 大额(>2万)缺附件 → 转人工提醒 — 付款单+预付款单都做
	if w := rulePaymentLargeContract(raw, tmpl); w != "" {
		warnings = append(warnings, w)
	}

	// B3: 税额份数对账 — 仅付款单 (预付款单无发票跳过)
	if tmpl == "payment" {
		if w := rulePaymentTaxCountWith(raw, inv.count); w != "" {
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
// u_客户多选 经 json.Unmarshal 后为 []interface{}, 不是 string。
// 原 .(string) 断言必失败 → 永远当空 → 全部误报 (A3 假阳性 70/70)。
func rulePaymentCustomer(raw map[string]interface{}) string {
	arr, ok := raw["u_客户多选"].([]interface{})
	if !ok || len(arr) == 0 {
		return "客户多选为空, 请确认是否应选虚拟客户 (A3)"
	}
	return ""
}

// ----- B1/B2: 收款方=开票方 + 购买方=所属公司 (仅付款单) -----

// payeeSellerMismatch B1 收款方=开票方: 收款方与任一发票开票方不一致→转人工。
// 缺数据时返回 "" (由 rulePaymentInvoicePartiesLogic 统一兜底)。
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
// 缺数据时返回 "" (由 rulePaymentInvoicePartiesLogic 统一兜底)。
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

// paymentInvoice 一张付款单的发票派生数据 (一次查 hesi_flow_invoice 得出, 供 A18/B1·B2/B3 共用)。
type paymentInvoice struct {
	total   float64  // 价税合计 (原 sumInvoiceTotal): 无票/查不到 = 0
	buyers  []string // 购买方列表, 已过滤空串 (原 invoiceParties)
	sellers []string // 开票方列表, 已过滤空串 (原 invoiceParties)
	count   int      // 发票张数 (原 invoiceCount): -1 = DB nil / 查询失败, 无法核验
}

// paymentInvoiceData 一次查 hesi_flow_invoice 取该单所有发票行, 派生出
// 价税合计 / 购买方·开票方列表 / 张数, 替代原先 A18(SUM) + B1·B2(买卖方) + B3(COUNT) 各查一次 (3→1)。
// 各派生值的缺数据语义逐一对齐被替代的三个旧函数 (5s 超时):
// 无票→total=0·列表 nil·count=0; DB nil 或查询失败→total=0·列表 nil·count=-1(无法核验)。
func (h *DashboardHandler) paymentInvoiceData(flowID string) paymentInvoice {
	if h.DB == nil {
		return paymentInvoice{count: -1}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := h.DB.QueryContext(ctx,
		`SELECT IFNULL(buyer_name,''), IFNULL(seller_name,''), IFNULL(total_amount,0) FROM hesi_flow_invoice WHERE flow_id=?`, flowID)
	if err != nil {
		return paymentInvoice{count: -1}
	}
	defer rows.Close()
	var inv paymentInvoice
	for rows.Next() {
		var b, s string
		var amt float64
		if rows.Scan(&b, &s, &amt) == nil {
			inv.count++
			inv.total += amt
			if b != "" {
				inv.buyers = append(inv.buyers, b)
			}
			if s != "" {
				inv.sellers = append(inv.sellers, s)
			}
		}
	}
	_ = rows.Err()
	return inv
}

// rulePaymentInvoicePartiesLogic B1+B2 纯逻辑层 (便于单测, 不依赖 DB/handler)。
// Bug 3 fix: 原代码仅在 buyers==0 && sellers==0 时才转人工, 漏掉了仅一方缺失的情况。
// 修复: buyers 为空单独产生 B2 提醒; sellers 为空单独产生 B1 提醒; 两者均空合并为 B1/B2 兜底。
func rulePaymentInvoicePartiesLogic(buyers, sellers []string, payeeName, owner string) (reject, warn []string) {
	if len(buyers) == 0 && len(sellers) == 0 {
		warn = append(warn, "未识别到发票购买方/开票方, 请人工核对 (B1/B2)")
		return reject, warn
	}
	if len(buyers) == 0 {
		warn = append(warn, "未识别到发票购买方, 请人工核对 (B2)")
	} else {
		if r := buyerCompanyMismatch(buyers, owner); r != "" {
			reject = append(reject, r)
		}
	}
	if len(sellers) == 0 {
		warn = append(warn, "未识别到发票开票方, 请人工核对 (B1)")
	} else {
		if w := payeeSellerMismatch(payeeName, sellers); w != "" {
			warn = append(warn, w)
		}
	}
	return reject, warn
}

// 说明: B1/B2 的发票方核验已在 AuditPayment 内联 (用预拉的 inv.buyers/inv.sellers 调
// rulePaymentInvoicePartiesLogic), 不再单独包装查询函数 —— 见 3→1 性能合并。

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
	// Bug 2 fix: 预付款单金额存 loanMoney，payMoney 为 0，原代码在 amount<=0 时直接返回。
	// 修复: 先取 payMoney，为 0 时再取 loanMoney (extractDupCheckAmount)。
	amount := extractDupCheckAmount(raw)
	if payeeID == "" || amount <= 0 {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Bug 2 fix: SQL 也要同时检查 pay_money 和 loan_money，覆盖历史预付款单记录。
	rows, err := h.DB.QueryContext(ctx,
		`SELECT code FROM hesi_flow
		 WHERE flow_id<>? AND active=1
		   AND JSON_UNQUOTE(JSON_EXTRACT(raw_json,'$.payeeId'))=?
		   AND (ABS(IFNULL(pay_money,0)-?)<0.01 OR ABS(IFNULL(loan_money,0)-?)<0.01)
		   AND (specification_id LIKE 'ID01KgaO6dcZtR%' OR specification_id LIKE 'ID01FhdI9II9A3%')
		 LIMIT 5`,
		flowID, payeeID, amount, amount)
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

// toFloatPayment 安全转 float64。
// 合思金额字段为 {"standard":"25844.00",...} 结构, 复用 getStandardAmount 解析。
// 普通 float64 (测试用) 也能正确处理。
func toFloatPayment(v interface{}) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	if f, ok := getStandardAmount(v); ok {
		return f
	}
	return 0
}

// extractPaymentReasonText Bug 1 fix: 按模板类型提取「付款事由」文本。
// 付款单 (payment): 遍历 details[].feeTypeForm.consumptionReasons 并拼接; 同时 belt-and-suspenders 包含 description。
// 预付款单 (prepay): 直接取 description。
// A13 sunshineRule 用此函数取 reason，避免付款单永远读空 description。
func extractPaymentReasonText(raw map[string]interface{}, tmpl string) string {
	desc, _ := raw["description"].(string)
	if tmpl != "payment" {
		return desc
	}
	// 付款单: 从所有明细行 consumptionReasons 拼接
	var parts []string
	if desc != "" {
		parts = append(parts, desc)
	}
	details, _ := raw["details"].([]interface{})
	for _, d := range details {
		dm, _ := d.(map[string]interface{})
		form, _ := dm["feeTypeForm"].(map[string]interface{})
		s, _ := form["consumptionReasons"].(string)
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

// extractDupCheckAmount Bug 2 fix: B4 防重复付款的金额提取。
// 优先取 payMoney (付款单); 为 0 时取 loanMoney (预付款单)。
func extractDupCheckAmount(raw map[string]interface{}) float64 {
	amount := toFloatPayment(raw["payMoney"])
	if amount <= 0 {
		amount = toFloatPayment(raw["loanMoney"])
	}
	return amount
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

// ----- A5: 事由必填 + 不含合计/小计 -----

var vagueReasonWords = []string{"合计", "小计"}

// rulePaymentReason A5 事由校验:
// 预付款单 → 单据级 description 必填 + 不含模糊词;
// 付款单 → 逐行 details[].feeTypeForm.consumptionReasons 必填 + 不含模糊词。
// 两模板都做, 违规 → reject。
func rulePaymentReason(raw map[string]interface{}, tmpl string) string {
	checkReason := func(s, where string) string {
		s = strings.TrimSpace(s)
		if s == "" {
			return where + "为空 (A5)"
		}
		for _, w := range vagueReasonWords {
			if strings.Contains(s, w) {
				return where + "含模糊词「" + w + "」(A5)"
			}
		}
		return ""
	}
	if tmpl == "prepay" {
		s, _ := raw["description"].(string)
		return checkReason(s, "付款事由")
	}
	// 付款单: 遍历 details[].feeTypeForm.consumptionReasons
	details, _ := raw["details"].([]interface{})
	if len(details) == 0 {
		return "" // 无明细由 A8 处理, 此处不重复驳
	}
	for i, d := range details {
		dm, _ := d.(map[string]interface{})
		form, _ := dm["feeTypeForm"].(map[string]interface{})
		s, _ := form["consumptionReasons"].(string)
		if r := checkReason(s, fmt.Sprintf("第%d行消费事由", i+1)); r != "" {
			return r
		}
	}
	return ""
}

// ----- A17: 大额付款缺附件 → 提醒 (两模板都做) -----

// rulePaymentLargeContract A17 对外付款>2万元未见附件 → 转人工提醒 (非 reject)。
// 两模板都做: 付款单取 payMoney, 预付款单取 loanMoney。
func rulePaymentLargeContract(raw map[string]interface{}, tmpl string) string {
	var amount float64
	if tmpl == "prepay" {
		amount = toFloatPayment(raw["loanMoney"])
	} else {
		amount = toFloatPayment(raw["payMoney"])
	}
	if amount <= 20000 {
		return ""
	}
	atts, _ := raw["attachments"].([]interface{})
	if len(atts) == 0 {
		return fmt.Sprintf("对外付款 ¥%.2f 超2万元未见附件, 请确认是否已附盖章合同 (A17)", amount)
	}
	return ""
}

// ----- B3: 税额份数对账 (仅付款单) -----

// taxCountMismatch B3 纯逻辑: 发票实际张数 vs 申报张数, 不一致 → 转人工。
// 0 份/空申报 → 不判 (安全底线: 不误驳)。
func taxCountMismatch(invCount int, declCount int) string {
	if declCount == 0 {
		return "" // 申报 0 或未填, 跳过
	}
	if invCount != declCount {
		return fmt.Sprintf("发票张数(%d)与申报份数(%d)不符, 请人工核对 (B3)", invCount, declCount)
	}
	return ""
}

// rulePaymentTaxCountWith B3 税额份数对账: 发票实际张数(预拉 invCount) vs raw_json 申报张数, 不符 → 转人工。
// invCount<0 表示无法获取发票张数 (DB nil/查询失败) → 申报存在时转人工。
// 纯函数 (发票张数由 paymentInvoiceData 预拉传入, 不再单独查发票表), 便于单测。仅付款单调用。
func rulePaymentTaxCountWith(raw map[string]interface{}, invCount int) string {
	declRaw, _ := raw["u_WmLv_税额份数总计"].(string)
	declRaw = strings.TrimSpace(declRaw)
	if declRaw == "" || declRaw == "0" {
		return "" // 未填或申报0份, 不判
	}
	declCount, err := strconv.Atoi(declRaw)
	if err != nil || declCount == 0 {
		return "" // 解析失败 → 不误驳, 安全底线
	}
	if invCount < 0 {
		return "税额份数申报字段存在但无法核验发票张数, 请人工核对 (B3)"
	}
	return taxCountMismatch(invCount, declCount)
}
