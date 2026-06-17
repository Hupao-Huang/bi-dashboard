package handler

// 规则 7-1 交通及差旅费关联出差申请单 (跑哥 2026-06-17 最终口径)
// 集团: 全部交通差旅费(含私车公用/过路费)都要关联出差申请单 + 金额≤额度;
// 线下: 整体豁免 —— 在 AuditDailyExpense 的 `!isOfflineFlow` 门挡掉, 不在本函数。
// 故本文件直接调 ruleRequisitionLink = 测集团路径。

import (
	"strings"
	"testing"
)

func TestRule71PrivateCarTollNowRequiresLink(t *testing.T) {
	// 集团下: 私车公用/过路费 未关联出差申请单 → 驳回 (旧口径是豁免, 6/17 改为集团要关联)
	h := &DashboardHandler{} // 无 expenseLinks 在 step2 即返回, 不查库
	for _, ft := range []struct{ id, name string }{
		{"ID01Fr2mX8KP2T", "私车公用"},
		{"ID01KhLSijR8FV", "过路费"},
	} {
		raw := map[string]interface{}{"details": []interface{}{
			map[string]interface{}{"feeTypeId": ft.id},
		}}
		r := h.ruleRequisitionLink(raw, 100, travelExpenseFeeTypes, reqTripSpecPrefix, "交通及差旅费", "出差申请单", "规则 7-1")
		if !strings.Contains(r, "未关联") {
			t.Errorf("%s 集团下未关联出差申请单应驳回, got %q", ft.name, r)
		}
	}
}

func TestRule71TrainRequiresLink(t *testing.T) {
	// 火车未关联 → 驳回 (一直如此, 回归保护)
	h := &DashboardHandler{}
	raw := map[string]interface{}{"details": []interface{}{
		map[string]interface{}{"feeTypeId": "ID01Fk0IZFCb03"}, // 火车
	}}
	r := h.ruleRequisitionLink(raw, 100, travelExpenseFeeTypes, reqTripSpecPrefix, "交通及差旅费", "出差申请单", "规则 7-1")
	if !strings.Contains(r, "未关联") {
		t.Errorf("含火车票未关联出差申请单应驳回, got %q", r)
	}
}

func TestRule71TravelSetCoversPrivateCarToll(t *testing.T) {
	// 规则7-1 触发集合 = 全量 travelExpenseFeeTypes, 现含私车公用/过路费 + 常规交通类型
	for _, id := range []string{"ID01Fr2mX8KP2T", "ID01KhLSijR8FV", "ID01Fk0IZFCb03", "ID01Fk0IZFCaJx"} {
		if _, ok := travelExpenseFeeTypes[id]; !ok {
			t.Errorf("fee_type %s 应在规则7-1触发集合(全量交通差旅费)里", id)
		}
	}
}

func TestRule71NonTravelNoTrigger(t *testing.T) {
	// 非交通差旅费明细 → 不触发7-1 (step1 无 hit 直接返空, 不查库)
	h := &DashboardHandler{}
	raw := map[string]interface{}{"details": []interface{}{
		map[string]interface{}{"feeTypeId": "ID01OTHER_NOT_TRAVEL"},
	}}
	if r := h.ruleRequisitionLink(raw, 100, travelExpenseFeeTypes, reqTripSpecPrefix, "交通及差旅费", "出差申请单", "规则 7-1"); r != "" {
		t.Errorf("非交通差旅费不应触发7-1, got %q", r)
	}
}
