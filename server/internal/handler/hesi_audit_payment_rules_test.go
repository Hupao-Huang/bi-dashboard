package handler

import "testing"

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
