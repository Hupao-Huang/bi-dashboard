package handler

// 规则 7-1 交通及差旅费关联出差申请单 — 私车公用/过路费豁免 (跑哥 2026-06-17)

import (
	"strings"
	"testing"
)

func TestRule71PrivateCarTollExempt(t *testing.T) {
	// 只报私车公用/过路费 → 不触发规则7-1(无需关联出差申请单); step1 无 hit 直接返空, 不访问 DB
	h := &DashboardHandler{} // DB nil: 豁免路径走不到查库
	raw := map[string]interface{}{"details": []interface{}{
		map[string]interface{}{"feeTypeId": "ID01Fr2mX8KP2T"}, // 私车公用
		map[string]interface{}{"feeTypeId": "ID01KhLSijR8FV"}, // 过路费
	}}
	if r := h.ruleRequisitionLink(raw, 100, travelExpenseRequiringTrip, reqTripSpecPrefix, "交通及差旅费", "出差申请单", "规则 7-1"); r != "" {
		t.Errorf("私车公用/过路费应豁免规则7-1, 不应要求关联出差申请单, got %q", r)
	}
}

func TestRule71RealTravelStillRequiresLink(t *testing.T) {
	// 含真交通类型(火车)且未关联出差申请单 → 仍驳回(豁免不波及其他交通类型); 无 expenseLinks 在 step2 即返回, 不查库
	h := &DashboardHandler{}
	raw := map[string]interface{}{"details": []interface{}{
		map[string]interface{}{"feeTypeId": "ID01Fk0IZFCb03"}, // 火车
		map[string]interface{}{"feeTypeId": "ID01Fr2mX8KP2T"}, // 私车公用(豁免)
	}}
	r := h.ruleRequisitionLink(raw, 100, travelExpenseRequiringTrip, reqTripSpecPrefix, "交通及差旅费", "出差申请单", "规则 7-1")
	if !strings.Contains(r, "未关联") {
		t.Errorf("含火车票未关联出差申请单应驳回, got %q", r)
	}
}

func TestTravelExpenseRequiringTripExcludesExempt(t *testing.T) {
	// 派生触发集合: 私车公用/过路费 不在, 其余交通及差旅费在, 总数比全量少 2
	if _, ok := travelExpenseRequiringTrip["ID01Fr2mX8KP2T"]; ok {
		t.Error("私车公用不应在规则7-1触发集合里")
	}
	if _, ok := travelExpenseRequiringTrip["ID01KhLSijR8FV"]; ok {
		t.Error("过路费不应在规则7-1触发集合里")
	}
	if _, ok := travelExpenseRequiringTrip["ID01Fk0IZFCb03"]; !ok {
		t.Error("火车应仍在规则7-1触发集合里")
	}
	if len(travelExpenseRequiringTrip) != len(travelExpenseFeeTypes)-2 {
		t.Errorf("派生集合应比全量少2项, 全量%d 派生%d", len(travelExpenseFeeTypes), len(travelExpenseRequiringTrip))
	}
}
