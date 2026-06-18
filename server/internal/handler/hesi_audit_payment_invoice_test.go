package handler

// 性能合并特征测试: 付款单原先 A18/B1·B2/B3 各查一次 hesi_flow_invoice (每单 3 次),
// 合并为 paymentInvoiceData 一次查 (3→1)。本测试锁住"一次查 + 派生值正确",
// 保证合并前后规则判定零变化。

import (
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// 多行: 验证 合计=SUM / 张数=行数 / 购买方·开票方过滤空串, 且恰好一次查询
func TestPaymentInvoiceData_Derive(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(`FROM hesi_flow_invoice WHERE flow_id=\?`).
		WithArgs("F-multi").
		WillReturnRows(sqlmock.NewRows([]string{"buyer_name", "seller_name", "total_amount"}).
			AddRow("买方甲", "卖方A", 100.50).
			AddRow("买方甲", "", 200.00). // 开票方空 → 不计入 sellers
			AddRow("", "卖方B", 50.00))   // 购买方空 → 不计入 buyers
	h := &DashboardHandler{DB: db}
	inv := h.paymentInvoiceData("F-multi")

	if inv.count != 3 {
		t.Errorf("张数应=行数 3 (原 COUNT(*)), got %d", inv.count)
	}
	if inv.total != 350.50 {
		t.Errorf("合计应=100.50+200+50=350.50 (原 SUM), got %v", inv.total)
	}
	if len(inv.buyers) != 2 {
		t.Errorf("购买方应过滤空串后剩 2, got %v", inv.buyers)
	}
	if len(inv.sellers) != 2 {
		t.Errorf("开票方应过滤空串后剩 2, got %v", inv.sellers)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("应恰好一次发票查询 (3→1 合并的核心): %v", err)
	}
}

// 无发票: total=0 / count=0 / 列表 nil (对齐原 sumInvoiceTotal 返 0 + invoiceParties 返 nil + COUNT=0)
func TestPaymentInvoiceData_Empty(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(`FROM hesi_flow_invoice WHERE flow_id=\?`).
		WillReturnRows(sqlmock.NewRows([]string{"buyer_name", "seller_name", "total_amount"}))
	h := &DashboardHandler{DB: db}
	inv := h.paymentInvoiceData("F-empty")
	if inv.count != 0 || inv.total != 0 || inv.buyers != nil || inv.sellers != nil {
		t.Errorf("无发票应 total=0 count=0 nil/nil, got %+v", inv)
	}
}

// nil DB: count=-1 (对齐原 invoiceCount 的"无法核验"语义, B3 据此转人工)
func TestPaymentInvoiceData_NilDB(t *testing.T) {
	h := &DashboardHandler{}
	inv := h.paymentInvoiceData("F")
	if inv.count != -1 {
		t.Errorf("nil DB 应 count=-1 (无法核验, 同旧 invoiceCount), got %d", inv.count)
	}
	if inv.total != 0 || inv.buyers != nil || inv.sellers != nil {
		t.Errorf("nil DB 应 total=0 nil/nil, got %+v", inv)
	}
}

// 查询出错: total=0 / count=-1 / 列表 nil (B1·B2 走兜底提醒, B3 转人工, A18 不误驳)
func TestPaymentInvoiceData_QueryError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(`FROM hesi_flow_invoice`).WillReturnError(fmt.Errorf("boom"))
	h := &DashboardHandler{DB: db}
	inv := h.paymentInvoiceData("F")
	if inv.count != -1 || inv.total != 0 || inv.buyers != nil || inv.sellers != nil {
		t.Errorf("查询出错应 total=0 count=-1 nil/nil, got %+v", inv)
	}
}

// rulePaymentTaxCountWith: 用预拉的发票张数核验 (替代原 rulePaymentTaxCount 内部再查一次)
func TestRulePaymentTaxCountWith(t *testing.T) {
	// 申报 0 / 未填 → 不判
	if r := rulePaymentTaxCountWith(map[string]interface{}{"u_WmLv_税额份数总计": "0"}, 5); r != "" {
		t.Errorf("申报0份应跳过, got %q", r)
	}
	if r := rulePaymentTaxCountWith(map[string]interface{}{}, 5); r != "" {
		t.Errorf("未填申报应跳过, got %q", r)
	}
	// 申报 3, 发票 3 → 通过
	if r := rulePaymentTaxCountWith(map[string]interface{}{"u_WmLv_税额份数总计": "3"}, 3); r != "" {
		t.Errorf("张数一致应通过, got %q", r)
	}
	// 申报 3, 发票 2 → 转人工
	if r := rulePaymentTaxCountWith(map[string]interface{}{"u_WmLv_税额份数总计": "3"}, 2); r == "" {
		t.Error("张数不符应转人工 (B3)")
	}
	// 申报 3, 发票无法核验 (count<0) → 转人工"无法核验"
	if r := rulePaymentTaxCountWith(map[string]interface{}{"u_WmLv_税额份数总计": "3"}, -1); r == "" {
		t.Error("申报存在但发票张数无法核验应转人工 (B3)")
	}
}
