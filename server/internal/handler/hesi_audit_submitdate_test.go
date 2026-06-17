package handler

// 规则 8-4 / 14 发票时效用"首次提交时间": applyStableSubmitDate 用表 submit_date 覆盖 raw.submitDate (跑哥 2026-06-17)
// 根因: raw_json.submitDate 会被退回重提刷新, 把发票拖成超期误判; 表 submit_date 稳定不被覆盖。

import "testing"

const (
	firstSubmitMs = int64(1780062128665)   // 2026-05-29 首次提交(表 submit_date, 稳定)
	resubmitMs    = float64(1781180053459) // 2026-06-11 退回后重提(raw_json.submitDate, 会变)
)

func TestApplyStableSubmitDate_OverridesWhenTableEarlier(t *testing.T) {
	// 表 5/29(首次提交)比 raw 6/11(重提)早 → 覆盖为 5/29 → 发票时效按 5/29 算, 不再误判超期
	raw := map[string]interface{}{"submitDate": resubmitMs}
	applyStableSubmitDate(firstSubmitMs, raw)
	if got, _ := raw["submitDate"].(float64); got != float64(firstSubmitMs) {
		t.Errorf("表更早应覆盖为 %d, got %v", firstSubmitMs, got)
	}
}

func TestApplyStableSubmitDate_KeepsRawWhenTableLater(t *testing.T) {
	// 守卫: 表值因脏数据异常地比 raw 晚 → 不覆盖, 保留更早的 raw(宁严勿松, 防把发票距提交算短而漏判超期)
	laterTable := int64(resubmitMs) + 86400000 // 比 raw 晚一天
	raw := map[string]interface{}{"submitDate": resubmitMs}
	applyStableSubmitDate(laterTable, raw)
	if got, _ := raw["submitDate"].(float64); got != resubmitMs {
		t.Errorf("表值更晚应保留 raw 原值 %v, got %v", resubmitMs, got)
	}
}

func TestApplyStableSubmitDate_ZeroKeepsRaw(t *testing.T) {
	// firstSubmitDate=0(老单从没正常入库/没同步到)→ 不动 raw, 保留原值兜底
	raw := map[string]interface{}{"submitDate": resubmitMs}
	applyStableSubmitDate(0, raw)
	if got, _ := raw["submitDate"].(float64); got != resubmitMs {
		t.Errorf("0 应保留 raw 原值 %v, got %v", resubmitMs, got)
	}
}

func TestApplyStableSubmitDate_FillsWhenRawMissing(t *testing.T) {
	// raw 原本没有 submitDate(字段缺失)→ 用表值填(表是唯一可信来源)
	raw := map[string]interface{}{}
	applyStableSubmitDate(firstSubmitMs, raw)
	if got, _ := raw["submitDate"].(float64); got != float64(firstSubmitMs) {
		t.Errorf("raw 缺 submitDate 应填表值 %d, got %v", firstSubmitMs, got)
	}
}

func TestApplyStableSubmitDate_NilRawSafe(t *testing.T) {
	applyStableSubmitDate(firstSubmitMs, nil) // 不应 panic
}

func TestApplyStableSubmitDate_EqualKeepsRaw(t *testing.T) {
	// 表值与 raw 相等 → 不覆盖(无意义), 值不变
	raw := map[string]interface{}{"submitDate": float64(firstSubmitMs)}
	applyStableSubmitDate(firstSubmitMs, raw)
	if got, _ := raw["submitDate"].(float64); got != float64(firstSubmitMs) {
		t.Errorf("相等应保持 %d, got %v", firstSubmitMs, got)
	}
}
