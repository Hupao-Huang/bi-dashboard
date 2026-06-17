package handler

import (
	"strings"
	"testing"
)

func TestAuditPayment_EmptyRulesAgree(t *testing.T) {
	// 骨架测试: 非付款单模板 → agree (不评估)
	h := &DashboardHandler{}
	got := h.AuditPayment("F1", "ID01Other:abc", `{}`, 0)
	if got == nil || got.Action != "agree" {
		t.Fatalf("非付款单模板应 agree, got %+v", got)
	}
}

func TestPaymentTemplateType(t *testing.T) {
	if paymentTemplate("ID01KgaO6dcZtR:abc") != "payment" {
		t.Errorf("B开头 spec 应为 payment")
	}
	if paymentTemplate("ID01FhdI9II9A3:xyz") != "prepay" {
		t.Errorf("J开头 spec 应为 prepay")
	}
	if paymentTemplate("ID01Other") != "" {
		t.Errorf("其它 spec 应为空")
	}
}

// A1: 所属公司非空
func TestRulePaymentOwnerCompany(t *testing.T) {
	// 法人实体为空 → 驳回
	if rulePaymentOwnerCompany(map[string]interface{}{}) == "" {
		t.Error("法人实体为空应驳回")
	}
	// 有法人实体 → 通过
	if r := rulePaymentOwnerCompany(map[string]interface{}{"法人实体": "ID01X"}); r != "" {
		t.Errorf("有法人实体应通过, got %q", r)
	}
}

// A2: 申请部门末级 (转人工)
func TestAuditPayment_DeptLeafManual(t *testing.T) {
	// paymentDeptLeafWarn 把 ruleDeptLeaf 的非空结果原样返回 (转人工)
	msg := paymentDeptLeafWarn("非末级部门 (A2 申请部门)")
	if msg == "" {
		t.Error("非末级应产出转人工提醒")
	}
	// 空结果 (末级或不在树) → 无提醒
	if paymentDeptLeafWarn("") != "" {
		t.Error("末级应无提醒")
	}
}

// A6+A7: 防自付
func TestPaymentSelfPayDetect(t *testing.T) {
	// 收款方户名 == 所属公司名 → 驳回
	if r := selfPayReason("杭州松鲜鲜食品", "杭州松鲜鲜食品"); r == "" {
		t.Error("收款方=所属公司应驳回 (防自付)")
	}
	// 不同名 → 通过
	if r := selfPayReason("供应商A", "杭州松鲜鲜食品"); r != "" {
		t.Errorf("不同应通过, got %q", r)
	}
	// 任一为空 → 不误判
	if r := selfPayReason("", "杭州松鲜鲜食品"); r != "" {
		t.Errorf("收款方空时不应驳回, got %q", r)
	}
	if r := selfPayReason("供应商A", ""); r != "" {
		t.Errorf("公司名空时不应驳回, got %q", r)
	}
}

// A13: 阳光天际 → 悦伍公司
func TestSpecialSubject(t *testing.T) {
	// 事由含阳光天际 但所属公司名不含悦伍 → 驳回
	if r := sunshineRule("阳光天际项目款", "杭州松鲜鲜自然调味品"); r == "" {
		t.Error("阳光天际应要求悦伍公司 (A13)")
	}
	// 所属公司含悦伍 → 通过
	if r := sunshineRule("阳光天际项目款", "杭州松鲜鲜悦伍食品科技有限公司"); r != "" {
		t.Errorf("悦伍公司应通过, got %q", r)
	}
	// 事由不含阳光天际 → 通过
	if r := sunshineRule("普通供应商款", "杭州松鲜鲜自然调味品"); r != "" {
		t.Errorf("无阳光天际关键词应通过, got %q", r)
	}
}

// A18: 支付金额上限 (仅付款单)
func TestPaymentAmountCap(t *testing.T) {
	// 支付额 > 发票合计 → 转人工
	raw := map[string]interface{}{"payMoney": float64(1000)}
	if w := rulePaymentAmountCap(raw, 800); w == "" {
		t.Error("支付额超发票合计应转人工 (A18)")
	}
	// 支付额 == 发票合计 → 通过
	if w := rulePaymentAmountCap(raw, 1000); w != "" {
		t.Errorf("支付额=发票应通过, got %q", w)
	}
	// 支付额 < 发票合计 → 通过
	if w := rulePaymentAmountCap(raw, 1200); w != "" {
		t.Errorf("支付额<发票应通过, got %q", w)
	}
	// invoiceTotal = 0 (无发票) → 不误判
	if w := rulePaymentAmountCap(raw, 0); w != "" {
		t.Errorf("无发票时不应转人工, got %q", w)
	}
}

// A1 集成: AuditPayment 对空法人实体返回 reject
func TestAuditPayment_A1Reject(t *testing.T) {
	h := &DashboardHandler{}
	// 付款单, 法人实体为空
	got := h.AuditPayment("F2", "ID01KgaO6dcZtR:abc", `{"payMoney":500}`, 0)
	if got == nil || got.Action != "reject" {
		t.Fatalf("A1 应触发 reject, got %+v", got)
	}
}

// A1 集成: 预付款单也检查法人实体
func TestAuditPayment_A1PrepayReject(t *testing.T) {
	h := &DashboardHandler{}
	got := h.AuditPayment("F3", "ID01FhdI9II9A3:xyz", `{"loanMoney":200}`, 0)
	if got == nil || got.Action != "reject" {
		t.Fatalf("预付款单 A1 应触发 reject, got %+v", got)
	}
}

// A8: 明细行数/无票
func TestRulePaymentItemRows(t *testing.T) {
	// 预付款单 (prepay): 可无明细 → 通过
	if rej, _ := rulePaymentItemRows(map[string]interface{}{}, "prepay", "J26000001"); rej != "" {
		t.Errorf("预付款单可无明细, got rej=%q", rej)
	}
	// 付款单无明细 → 转人工 (warn 非空, reject 为空)
	rej, warn := rulePaymentItemRows(map[string]interface{}{}, "payment", "B26000001")
	if rej != "" {
		t.Errorf("付款单无明细应是 warn 不是 reject, got rej=%q", rej)
	}
	if warn == "" {
		t.Error("付款单无明细应转人工 (warn 非空)")
	}
	// 付款单有明细 → 通过
	raw := map[string]interface{}{
		"details": []interface{}{map[string]interface{}{"feeTypeId": "X"}},
	}
	if rej2, warn2 := rulePaymentItemRows(raw, "payment", "B26000002"); rej2 != "" || warn2 != "" {
		t.Errorf("有明细应通过, got rej=%q warn=%q", rej2, warn2)
	}
}

// A3: 客户多选 (仅付款单)
func TestRulePaymentCustomer(t *testing.T) {
	// 客户多选为空 → 转人工提醒
	if w := rulePaymentCustomer(map[string]interface{}{}); w == "" {
		t.Error("客户多选为空应提醒")
	}
	// 空 JSON 数组 → 提醒
	if w := rulePaymentCustomer(map[string]interface{}{"u_客户多选": "[]"}); w == "" {
		t.Error("u_客户多选=[] 应提醒")
	}
	// 有值 → 通过
	if w := rulePaymentCustomer(map[string]interface{}{"u_客户多选": `["ID01GU1fnDLSmb"]`}); w != "" {
		t.Errorf("有客户ID应通过, got %q", w)
	}
}

// B1+B2: 发票方核验 (纯逻辑)
func TestInvoicePartyMatch(t *testing.T) {
	sellers := []string{"唐山市艺诚广告有限公司"}
	// 收款方匹配任一开票方 → 通过
	if w := payeeSellerMismatch("唐山市艺诚广告有限公司", sellers); w != "" {
		t.Errorf("收款方=开票方应通过, got %q", w)
	}
	// 不匹配 → 转人工
	if w := payeeSellerMismatch("别的公司", sellers); w == "" {
		t.Error("收款方≠开票方应转人工 (B1)")
	}
	// 缺数据 → 不误判
	if w := payeeSellerMismatch("", sellers); w != "" {
		t.Errorf("收款方为空不误判, got %q", w)
	}
	if w := payeeSellerMismatch("公司X", nil); w != "" {
		t.Errorf("卖家列表空不误判, got %q", w)
	}

	// 购买方 != 所属公司 → 驳回
	if r := buyerCompanyMismatch([]string{"公司X"}, "公司Y"); r == "" {
		t.Error("购买方≠所属公司应驳回 (B2)")
	}
	// 购买方 = 所属公司 → 通过
	if r := buyerCompanyMismatch([]string{"公司X"}, "公司X"); r != "" {
		t.Errorf("购买方=所属公司应通过, got %q", r)
	}
	// 缺数据 → 不误判
	if r := buyerCompanyMismatch(nil, "公司X"); r != "" {
		t.Errorf("买家列表空不误判, got %q", r)
	}
	if r := buyerCompanyMismatch([]string{"公司X"}, ""); r != "" {
		t.Errorf("所属公司空不误判, got %q", r)
	}
}

// B4: 防重复付款 (纯逻辑)
func TestDuplicatePayment(t *testing.T) {
	// 找到历史单号 → 提示文案含原单号
	if w := dupWarnText([]string{"B26001234"}); w == "" || !strings.Contains(w, "B26001234") {
		t.Errorf("应提示原单号, got %q", w)
	}
	// 多个单号 → 都出现
	if w := dupWarnText([]string{"B26001111", "B26002222"}); !strings.Contains(w, "B26001111") || !strings.Contains(w, "B26002222") {
		t.Errorf("多个单号应全部出现, got %q", w)
	}
	// 无重复 → 空
	if w := dupWarnText(nil); w != "" {
		t.Errorf("无重复应空, got %q", w)
	}
	if w := dupWarnText([]string{}); w != "" {
		t.Errorf("空切片应空, got %q", w)
	}
}
