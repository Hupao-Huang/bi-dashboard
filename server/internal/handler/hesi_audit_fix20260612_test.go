package handler

// 2026-06-12 全仓审查修复回归测试 (4 个审批规则 bug)
// ① 规则 12-1 金额 0/解析失败不再静默通过  ② 规则 7-3 feeDatePeriod 零值守卫
// ③ 规则 8/10/15 发票读取不完整降级转人工  ④ 规则 10 费用类型缺失不误驳

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// ===== ① 规则 12-1: 金额 0 / 解析失败 → 转人工 (修前: 0 ≤ 补助恒成立, 静默通过) =====

func TestRule121ZeroAmountManual(t *testing.T) {
	restore := seedDriveRecCache(map[string]*DriveRecord{
		"REC1": {Mileage: "97.68", Standard: "0.70", Subsidy: "68.38"},
	})
	defer restore()
	h := &DashboardHandler{}
	rej, warn := h.ruleDriveRecordCheck(mkDriveRaw("0", "REC1"))
	if len(rej) != 0 {
		t.Errorf("金额 0 不应驳回, got %v", rej)
	}
	if !strings.Contains(strings.Join(warn, "; "), "金额为 0 或解析失败") {
		t.Errorf("金额 0 应转人工核金额, got %v", warn)
	}
}

func TestRule121UnparsableAmountManual(t *testing.T) {
	// 金额字段是非数字字符串 (解析失败置 0), 真实金额可能不是 0 — 必须人工核
	restore := seedDriveRecCache(map[string]*DriveRecord{
		"REC1": {Mileage: "97.68", Standard: "0.70", Subsidy: "68.38"},
	})
	defer restore()
	h := &DashboardHandler{}
	rej, warn := h.ruleDriveRecordCheck(mkDriveRaw("abc", "REC1"))
	if len(rej) != 0 || !strings.Contains(strings.Join(warn, "; "), "人工核") {
		t.Errorf("金额解析失败应转人工: rej=%v warn=%v", rej, warn)
	}
}

// ===== ② 规则 7-3: feeDatePeriod 零值守卫 =====

func seedCityTierCache(m map[string]string) func() {
	cityTierCacheMu.Lock()
	old, oldAt := cityTierCache, cityTierCacheAt
	cityTierCache = m
	cityTierCacheAt = time.Now()
	cityTierCacheMu.Unlock()
	return func() {
		cityTierCacheMu.Lock()
		cityTierCache, cityTierCacheAt = old, oldAt
		cityTierCacheMu.Unlock()
	}
}

func mkHotelRaw(amt string, startMs, endMs int64) map[string]interface{} {
	form := map[string]interface{}{
		"detailNo": float64(1),
		"amount":   map[string]interface{}{"standard": amt},
	}
	if startMs != 0 || endMs != 0 {
		form["feeDatePeriod"] = map[string]interface{}{"start": float64(startMs), "end": float64(endMs)}
	}
	return map[string]interface{}{"details": []interface{}{
		map[string]interface{}{"feeTypeId": hotelFeeTypeID, "feeTypeForm": form},
	}}
}

func TestRule73MissingStartGoesManual(t *testing.T) {
	// 修前: start=0 + end 正常 → days ≈ 2 万+ → cap 天文数字 → 超标住宿静默过审
	// 修后: 区间异常 → 间夜数无法核算 → 转人工
	restore := seedCityTierCache(map[string]string{})
	defer restore()
	h := &DashboardHandler{}
	rej, warn := h.ruleAccommodationStandard(mkHotelRaw("999999", 0, r15June1), "", false)
	if rej != "" {
		t.Errorf("日期区间异常不应直接驳回, got %q", rej)
	}
	if !strings.Contains(warn, "间夜数无法核算") {
		t.Errorf("日期区间异常应转人工, got %q", warn)
	}
}

func TestRule73ValidPeriodStillJudges(t *testing.T) {
	// 正常 2 天区间: 间夜数照算, 超标照提 (SSC 未匹配 fallback 走 warn)
	restore := seedCityTierCache(map[string]string{})
	defer restore()
	h := &DashboardHandler{}
	rej, warn := h.ruleAccommodationStandard(mkHotelRaw("999", r15June1, r15June1+r15Day), "", false)
	combined := rej + warn
	if !strings.Contains(combined, "×2晚") {
		t.Errorf("正常区间应算出 2 晚并判超标, rej=%q warn=%q", rej, warn)
	}
}

// ===== ③④ 规则 8/10: 发票读取容错 + 费用类型缺失不误驳 =====

const invSelectPattern = `FROM hesi_flow_invoice i`

func invCols() []string {
	return []string{"detail_id", "detail_no", "buyer_name", "buyer_tax_no", "invoice_date", "total_amount", "approve_amount"}
}

func TestRule10MissingFeeTypeManualNotReject(t *testing.T) {
	// detail 顶层缺 feeTypeId: 修前被判 "无发票不豁免" 误驳, 修后转人工
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(invSelectPattern).WillReturnRows(sqlmock.NewRows(invCols()))
	h := &DashboardHandler{DB: db}
	raw := map[string]interface{}{"details": []interface{}{
		map[string]interface{}{ // 故意不带 feeTypeId
			"feeTypeForm": map[string]interface{}{
				"detailId": "D1", "detailNo": float64(1),
				"amount": map[string]interface{}{"standard": "100"},
			},
		},
	}}
	rej, warn := h.ruleInvoiceChecks(raw, "", "F1")
	if len(rej) != 0 {
		t.Errorf("费用类型缺失不应误驳, got %v", rej)
	}
	if !strings.Contains(strings.Join(warn, "; "), "费用类型缺失") {
		t.Errorf("费用类型缺失应转人工, got %v", warn)
	}
}

func TestRule8InvoiceReadBrokenManual(t *testing.T) {
	// 发票行读取中断: 修前规则跑在不完整数据上 (漏驳/误驳), 修后整体转人工
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	broken := sqlmock.NewRows(invCols()).
		AddRow("D1", 1, "公司A", "TAX1", int64(1780272000000), 100.0, 100.0).
		RowError(0, errors.New("connection lost"))
	mock.ExpectQuery(invSelectPattern).WillReturnRows(broken)
	h := &DashboardHandler{DB: db}
	raw := map[string]interface{}{"details": []interface{}{}}
	rej, warn := h.ruleInvoiceChecks(raw, "", "F1")
	if len(rej) != 0 {
		t.Errorf("读取中断不应输出驳回, got %v", rej)
	}
	if !strings.Contains(strings.Join(warn, "; "), "规则 8/10 未自动判定") {
		t.Errorf("读取中断应整体转人工, got %v", warn)
	}
}

func TestRule15InvoiceReadBrokenManual(t *testing.T) {
	// 规则 15: invSum 不完整会把合法单误驳 (油费合计偏低), 修后整体转人工
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	broken := sqlmock.NewRows([]string{"detail_id", "invoice_type", "total_amount", "approve_amount"}).
		AddRow("D1", "普票", 50.0, 50.0).
		RowError(0, errors.New("connection lost"))
	mock.ExpectQuery(`SELECT IFNULL\(detail_id,''\), IFNULL\(invoice_type,''\)`).WillReturnRows(broken)
	h := &DashboardHandler{DB: db}
	raw := rawOf(mkSubsidyDetail(1, "60", "1", r15June1, r15June1))
	rej, warn := h.ruleOfflineExtras(raw, "F1", "")
	if len(rej) != 0 {
		t.Errorf("读取中断不应输出驳回, got %v", rej)
	}
	if !strings.Contains(strings.Join(warn, "; "), "规则 15 未自动判定") {
		t.Errorf("读取中断应整体转人工, got %v", warn)
	}
}
