package handler

import "testing"

func TestAuditPayment_EmptyRulesAgree(t *testing.T) {
	h := &DashboardHandler{}
	raw := `{"法人实体":"X","payMoney":100}`
	got := h.AuditPayment("F1", "ID01KgaO6dcZtR:abc", raw, 0)
	if got == nil || got.Action != "agree" {
		t.Fatalf("空规则应 agree, got %+v", got)
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
