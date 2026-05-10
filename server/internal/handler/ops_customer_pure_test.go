package handler

// ops_customer_pure_test.go — ops_customer.go + distribution_customer.go 纯函数集合
// 已 Read ops_customer.go (line 80-155), distribution_customer.go (line 481-504).

import (
	"database/sql"
	"testing"
)

// ============ ops_customer 纯函数 ============

func TestNullFloat(t *testing.T) {
	if got := nullFloat(sql.NullFloat64{Valid: false, Float64: 100}); got != 0 {
		t.Errorf("invalid 应 0, got %v", got)
	}
	if got := nullFloat(sql.NullFloat64{Valid: true, Float64: 3.14}); got != 3.14 {
		t.Errorf("valid 应保留, got %v", got)
	}
}

func TestNormalizeRate(t *testing.T) {
	cases := map[float64]float64{
		0:       0,
		-0.5:    0,    // 负数 → 0
		0.5:     50,   // ≤ 1 视为小数 (×100)
		1.0:     100,
		0.95:    95,
		50:      50,   // > 1 已经是百分数
		87.5:    87.5,
		100:     100,
	}
	for in, want := range cases {
		if got := normalizeRate(in); got != want {
			t.Errorf("normalizeRate(%v)=%v want %v", in, got, want)
		}
	}
}

func TestRoundFloat(t *testing.T) {
	if got := roundFloat(3.14159, 2); got != 3.14 {
		t.Errorf("应 3.14, got %v", got)
	}
	if got := roundFloat(3.14159, 4); got != 3.1416 {
		t.Errorf("应 3.1416, got %v", got)
	}
	if got := roundFloat(2.5, 0); got != 3 {
		t.Errorf("应 3, got %v", got)
	}
	if got := roundFloat(123.456, -1); got != 123.456 {
		t.Errorf("digits<0 应原样, got %v", got)
	}
}

func TestCustomerMetricAggAdd(t *testing.T) {
	agg := &customerMetricAgg{}

	rec1 := customerMetricRecord{
		ConsultUsers: 100, InquiryUsers: 80, PayUsers: 30, SalesAmount: 5000,
		FirstRespSeconds: 5.5, ResponseSeconds: 8.0,
		SatisfactionRate: 0.95, ConvRate: 0.30,
	}
	agg.add(rec1)

	if agg.RecordCount != 1 {
		t.Errorf("RecordCount 应 1, got %d", agg.RecordCount)
	}
	if agg.ConsultUsers != 100 {
		t.Errorf("ConsultUsers 应累加")
	}
	if agg.FirstRespCount != 1 {
		t.Errorf("FirstRespSeconds > 0 应 +FirstRespCount")
	}
	// SatisfactionRate normalize: 0.95 → 95
	if agg.SatisfactionRate != 95 {
		t.Errorf("SatisfactionRate 应 normalize, got %v", agg.SatisfactionRate)
	}
	// ConvRate normalize: 0.30 → 30
	if agg.ConvRate != 30 {
		t.Errorf("ConvRate 应 normalize, got %v", agg.ConvRate)
	}

	// 第二条 — 无 first/response/satisfaction/conv (零值不计入)
	rec2 := customerMetricRecord{ConsultUsers: 50, SalesAmount: 1000}
	agg.add(rec2)

	if agg.RecordCount != 2 {
		t.Errorf("RecordCount 应 2, got %d", agg.RecordCount)
	}
	if agg.FirstRespCount != 1 {
		t.Errorf("零值不应 +FirstRespCount, got %d", agg.FirstRespCount)
	}
}

func TestCustomerMetricAggAvgWithZeroDivision(t *testing.T) {
	// 全空 agg → 各 avg* 应返 0 不 panic
	agg := &customerMetricAgg{}
	if agg.avgFirstRespSeconds() != 0 {
		t.Error("空 agg avgFirstRespSeconds 应 0")
	}
	if agg.avgResponseSeconds() != 0 {
		t.Error("空 agg avgResponseSeconds 应 0")
	}
	if agg.avgSatisfactionRate() != 0 {
		t.Error("空 agg avgSatisfactionRate 应 0")
	}
	if agg.avgConvRate() != 0 {
		t.Error("空 agg avgConvRate 应 0")
	}
}

func TestCustomerMetricAggAvgComputed(t *testing.T) {
	agg := &customerMetricAgg{
		FirstRespSeconds: 30, FirstRespCount: 3,
		ResponseSeconds: 60, ResponseCount: 4,
		SatisfactionRate: 270, SatisfactionCount: 3,
		ConvRate: 90, ConvCount: 3,
	}
	if got := agg.avgFirstRespSeconds(); got != 10 {
		t.Errorf("avgFirstResp=%v want 10", got)
	}
	if got := agg.avgResponseSeconds(); got != 15 {
		t.Errorf("avgResponse=%v want 15", got)
	}
	if got := agg.avgSatisfactionRate(); got != 90 {
		t.Errorf("avgSat=%v want 90", got)
	}
	if got := agg.avgConvRate(); got != 30 {
		t.Errorf("avgConv=%v want 30", got)
	}
}

// ============ distribution_customer 纯函数 ============

func TestMonthsBetween(t *testing.T) {
	// 同月
	got := monthsBetween("2026-04-01", "2026-04-30")
	if len(got) != 1 || got[0] != "202604" {
		t.Errorf("同月应 [202604], got %v", got)
	}

	// 跨多月
	got = monthsBetween("2026-04-15", "2026-06-10")
	want := []string{"202604", "202605", "202606"}
	if len(got) != 3 {
		t.Fatalf("跨 3 月应 3 个, got %d (%v)", len(got), got)
	}
	for i, m := range want {
		if got[i] != m {
			t.Errorf("got[%d]=%s want %s", i, got[i], m)
		}
	}

	// 跨年
	got = monthsBetween("2025-11-01", "2026-02-01")
	if len(got) != 4 {
		t.Errorf("跨年 4 月, got %d (%v)", len(got), got)
	}

	// 错误格式
	if got := monthsBetween("invalid", "2026-04-30"); got != nil {
		t.Error("无效 startDate 应返 nil")
	}
	if got := monthsBetween("2026-04-01", "invalid"); got != nil {
		t.Error("无效 endDate 应返 nil")
	}
}

func TestPreviousPeriod(t *testing.T) {
	// 一个月 (4-1 ~ 4-30, 30 天) → 上期 30 天
	prevStart, prevEnd := previousPeriod("2026-04-01", "2026-04-30")
	if prevEnd != "2026-03-31" {
		t.Errorf("prevEnd 应 2026-03-31, got %s", prevEnd)
	}
	if prevStart != "2026-03-02" {
		t.Errorf("prevStart 应 2026-03-02 (3-31 - 29), got %s", prevStart)
	}

	// 一周 (5-1 ~ 5-7, 7 天) → 上期 7 天 (4-24 ~ 4-30)
	prevStart, prevEnd = previousPeriod("2026-05-01", "2026-05-07")
	if prevEnd != "2026-04-30" {
		t.Errorf("prevEnd 应 2026-04-30, got %s", prevEnd)
	}
	if prevStart != "2026-04-24" {
		t.Errorf("prevStart 应 2026-04-24, got %s", prevStart)
	}

	// 单天 (5-10 ~ 5-10) → 上期单天 (5-9 ~ 5-9)
	prevStart, prevEnd = previousPeriod("2026-05-10", "2026-05-10")
	if prevStart != "2026-05-09" || prevEnd != "2026-05-09" {
		t.Errorf("单天上期应 5-9~5-9, got %s~%s", prevStart, prevEnd)
	}
}
