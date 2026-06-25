package handler

// 规则 8 (发票抬头/税号/金额/开票时间) + 规则 10 无票豁免 的特征测试 (characterization)。
// 2026-06-25 拆 ruleInvoiceChecks 前补网: 锁定 8-1抬头 / 8-2税号 / 8-4开票时效 及
// 豁免 A研发样品 / B补贴 / E外币 的现有判定 —— 这些分支原来没有直接测试覆盖。
// 拆函数(编排器+子函数)时靠这套网证明行为不变。invSelectPattern/invCols 复用 fix20260612_test.go。

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const r8Month = int64(30 * 24 * 3600 * 1000)
const r8Day = int64(86400000)
const r8SubmitMs = int64(1780000000000) // 固定提交时间戳 (2026 年中), 避免依赖当前时间

const dictPattern = `hesi_legal_entity_invoice_info`

func mkInvMock(t *testing.T) (*DashboardHandler, sqlmock.Sqlmock, func()) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	return &DashboardHandler{DB: db}, mock, func() { _ = db.Close() }
}

// invDetail 造一条费用明细 (feeType + detailId + 行号 + 金额)
func invDetail(feeType, detailID string, no int, amount string) map[string]interface{} {
	return map[string]interface{}{
		"feeTypeId": feeType,
		"feeTypeForm": map[string]interface{}{
			"detailId": detailID,
			"detailNo": float64(no),
			"amount":   map[string]interface{}{"standard": amount},
		},
	}
}

// invRaw 造单据 raw_json (法人实体/提交时间可选 + 明细列表)
func invRaw(legalEntity string, submitMs int64, dets ...map[string]interface{}) map[string]interface{} {
	ds := make([]interface{}, len(dets))
	for i, d := range dets {
		ds[i] = d
	}
	raw := map[string]interface{}{"details": ds}
	if legalEntity != "" {
		raw["法人实体"] = legalEntity
	}
	if submitMs != 0 {
		raw["submitDate"] = float64(submitMs)
	}
	return raw
}

// ===== 规则 8-1 / 8-2: 发票抬头 + 税号 vs 法人实体开票字典 =====

func TestRule81WrongTitleRejects(t *testing.T) {
	h, mock, done := mkInvMock(t)
	defer done()
	mock.ExpectQuery(invSelectPattern).WillReturnRows(
		sqlmock.NewRows(invCols()).AddRow("D1", 1, "错抬头公司", "TAX_OK", int64(0), 100.0, 0.0))
	mock.ExpectQuery(dictPattern).WillReturnRows(
		sqlmock.NewRows([]string{"invoice_title", "tax_no"}).AddRow("正确公司", "TAX_OK"))
	raw := invRaw("LE1", 0, invDetail("ID01OTHER", "D1", 1, "100"))
	rej, _ := h.ruleInvoiceChecks(raw, "", "F1", false)
	joined := strings.Join(rej, "; ")
	if !strings.Contains(joined, "规则 8-1") || !strings.Contains(joined, "发票抬头") {
		t.Errorf("抬头不符应驳回 8-1, got %v", rej)
	}
	if strings.Contains(joined, "规则 8-2") {
		t.Errorf("税号一致不应驳 8-2, got %v", rej)
	}
}

func TestRule82WrongTaxRejects(t *testing.T) {
	h, mock, done := mkInvMock(t)
	defer done()
	mock.ExpectQuery(invSelectPattern).WillReturnRows(
		sqlmock.NewRows(invCols()).AddRow("D1", 1, "正确公司", "错税号", int64(0), 100.0, 0.0))
	mock.ExpectQuery(dictPattern).WillReturnRows(
		sqlmock.NewRows([]string{"invoice_title", "tax_no"}).AddRow("正确公司", "TAX_OK"))
	raw := invRaw("LE1", 0, invDetail("ID01OTHER", "D1", 1, "100"))
	rej, _ := h.ruleInvoiceChecks(raw, "", "F1", false)
	joined := strings.Join(rej, "; ")
	if !strings.Contains(joined, "规则 8-2") || !strings.Contains(joined, "发票税号") {
		t.Errorf("税号不符应驳回 8-2, got %v", rej)
	}
	if strings.Contains(joined, "规则 8-1") {
		t.Errorf("抬头一致不应驳 8-1, got %v", rej)
	}
}

func TestRule812CorrectPasses(t *testing.T) {
	h, mock, done := mkInvMock(t)
	defer done()
	mock.ExpectQuery(invSelectPattern).WillReturnRows(
		sqlmock.NewRows(invCols()).AddRow("D1", 1, "正确公司", "TAX_OK", int64(0), 100.0, 0.0))
	mock.ExpectQuery(dictPattern).WillReturnRows(
		sqlmock.NewRows([]string{"invoice_title", "tax_no"}).AddRow("正确公司", "TAX_OK"))
	raw := invRaw("LE1", 0, invDetail("ID01OTHER", "D1", 1, "100"))
	rej, _ := h.ruleInvoiceChecks(raw, "", "F1", false)
	if len(rej) != 0 {
		t.Errorf("抬头税号金额都对不应驳回, got %v", rej)
	}
}

func TestRule812DictMissingManual(t *testing.T) {
	h, mock, done := mkInvMock(t)
	defer done()
	mock.ExpectQuery(invSelectPattern).WillReturnRows(
		sqlmock.NewRows(invCols()).AddRow("D1", 1, "任意公司", "任意税号", int64(0), 100.0, 0.0))
	mock.ExpectQuery(dictPattern).WillReturnRows(
		sqlmock.NewRows([]string{"invoice_title", "tax_no"})) // 字典查无
	raw := invRaw("LE_UNKNOWN", 0, invDetail("ID01OTHER", "D1", 1, "100"))
	rej, warn := h.ruleInvoiceChecks(raw, "", "F1", false)
	if len(rej) != 0 {
		t.Errorf("字典查不到不应驳回, got %v", rej)
	}
	if !strings.Contains(strings.Join(warn, "; "), "未在开票资料字典内") {
		t.Errorf("字典查不到应转人工, got %v", warn)
	}
}

// ===== 规则 8-4: 开票时间距提交 ≤1月过 / 1-3月人工 / >3月驳 =====

func TestRule84Over3MonthsRejects(t *testing.T) {
	h, mock, done := mkInvMock(t)
	defer done()
	mock.ExpectQuery(invSelectPattern).WillReturnRows(
		sqlmock.NewRows(invCols()).AddRow("D1", 1, "", "", r8SubmitMs-4*r8Month, 100.0, 0.0))
	raw := invRaw("", r8SubmitMs, invDetail("ID01OTHER", "D1", 1, "100"))
	rej, _ := h.ruleInvoiceChecks(raw, "", "F1", false)
	if !strings.Contains(strings.Join(rej, "; "), "开票时间 > 3 个月") {
		t.Errorf(">3 个月应驳回 8-4, got %v", rej)
	}
}

func TestRule84Between1And3MonthsManual(t *testing.T) {
	h, mock, done := mkInvMock(t)
	defer done()
	mock.ExpectQuery(invSelectPattern).WillReturnRows(
		sqlmock.NewRows(invCols()).AddRow("D1", 1, "", "", r8SubmitMs-2*r8Month, 100.0, 0.0))
	raw := invRaw("", r8SubmitMs, invDetail("ID01OTHER", "D1", 1, "100"))
	rej, warn := h.ruleInvoiceChecks(raw, "", "F1", false)
	if len(rej) != 0 {
		t.Errorf("1-3 个月不应驳回, got %v", rej)
	}
	if !strings.Contains(strings.Join(warn, "; "), "1-3 个月") {
		t.Errorf("1-3 个月应转人工, got %v", warn)
	}
}

func TestRule84Within1MonthOK(t *testing.T) {
	h, mock, done := mkInvMock(t)
	defer done()
	mock.ExpectQuery(invSelectPattern).WillReturnRows(
		sqlmock.NewRows(invCols()).AddRow("D1", 1, "", "", r8SubmitMs-10*r8Day, 100.0, 0.0))
	raw := invRaw("", r8SubmitMs, invDetail("ID01OTHER", "D1", 1, "100"))
	rej, warn := h.ruleInvoiceChecks(raw, "", "F1", false)
	if strings.Contains(strings.Join(rej, "; ")+strings.Join(warn, "; "), "规则 8-4") {
		t.Errorf("1 个月内不应有 8-4 提示, rej=%v warn=%v", rej, warn)
	}
}

// ===== 规则 10: 无票豁免 A 研发样品 / B 补贴 / E 外币 =====

func TestRule10SubsidyNoInvoiceExempt(t *testing.T) {
	h, mock, done := mkInvMock(t)
	defer done()
	mock.ExpectQuery(invSelectPattern).WillReturnRows(sqlmock.NewRows(invCols())) // 无发票
	raw := invRaw("", 0, invDetail("ID01Fk0MQBAAQ7", "D1", 1, "100"))             // 出差补贴
	rej, _ := h.ruleInvoiceChecks(raw, "", "F1", false)
	if len(rej) != 0 {
		t.Errorf("补贴类无票应豁免(规则10-B), got %v", rej)
	}
}

func TestRule10ForeignNoInvoiceExempt(t *testing.T) {
	h, mock, done := mkInvMock(t)
	defer done()
	mock.ExpectQuery(invSelectPattern).WillReturnRows(sqlmock.NewRows(invCols()))
	raw := invRaw("", 0, invDetail("ID01OTHER", "D1", 1, "100"))
	rej, _ := h.ruleInvoiceChecks(raw, "", "F1", true) // isForeign=true
	if len(rej) != 0 {
		t.Errorf("外币(国外)无票应豁免(规则10-E), got %v", rej)
	}
}

func TestRule10ResearchSampleNoInvoiceExempt(t *testing.T) {
	restore := seedhesiDeptTreeCache(map[string]hesiDeptNode{
		"D_RD": {name: "产品研发中心", parentID: "", active: true},
	})
	defer restore()
	h, mock, done := mkInvMock(t)
	defer done()
	mock.ExpectQuery(invSelectPattern).WillReturnRows(sqlmock.NewRows(invCols()))
	raw := invRaw("", 0, invDetail("ID01Fk0FsIqhDV", "D1", 1, "100")) // 赠品及样品
	rej, _ := h.ruleInvoiceChecks(raw, "D_RD", "F1", false)
	if len(rej) != 0 {
		t.Errorf("研发部门样品无票应豁免(规则10-A), got %v", rej)
	}
}
