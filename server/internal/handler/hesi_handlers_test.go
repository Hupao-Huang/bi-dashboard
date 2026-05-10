package handler

// hesi_handlers_test.go — hesi.go (合思费控) 主路径 sqlmock + 边界
// 已 Read hesi.go:
//   - GetHesiFlowDetail (246): 4 SQL (主表 + details + invoices + attachments)
//   - GetHesiAttachmentURLs (406): missing flowId 边界
//   - GetHesiStats (453): 11 个 QueryRow + 3 Query (全是 COUNT/分布)
//   - GetHesiFlows (53): 复杂分页 + 多个 batch SQL, 跳过 happy 只测边界

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ GetHesiFlowDetail ============

func TestGetHesiFlowDetailMissingFlowId(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/hesi/flow", nil)
	(&DashboardHandler{DB: db}).GetHesiFlowDetail(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("缺 flowId 应 400, got %d", rec.Code)
	}
}

func TestGetHesiFlowDetailNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 主表查询返空 → ErrNoRows → 404
	mock.ExpectQuery(`SELECT flow_id, code, title, form_type, state.*FROM hesi_flow WHERE flow_id=\?`).
		WillReturnRows(sqlmock.NewRows([]string{"flow_id", "code", "title", "form_type", "state",
			"owner", "dept", "submitter", "pay", "expense", "loan",
			"ct", "ut", "sd", "pd", "fet", "vno", "vstatus", "raw"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/hesi/flow?flowId=NOTEXIST", nil)
	(&DashboardHandler{DB: db}).GetHesiFlowDetail(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("不存在 flowId 应 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetHesiFlowDetailHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 1. 主表
	mock.ExpectQuery(`FROM hesi_flow WHERE flow_id=\?`).
		WillReturnRows(sqlmock.NewRows([]string{"flow_id", "code", "title", "form_type", "state",
			"owner", "dept", "submitter", "pay", "expense", "loan",
			"ct", "ut", "sd", "pd", "fet", "vno", "vstatus", "raw"}).
			AddRow("F001", "EX-001", "差旅报销", "expense", "paid",
				"u001", "d001", "u002", 100.0, 100.0, 0.0,
				1234567890000, 1234567890000, 1234567890000, 1234567890000, 1234567890000,
				"V001", "approved", "{}"))

	// 2. details
	mock.ExpectQuery(`FROM hesi_flow_detail WHERE flow_id=\?`).
		WillReturnRows(sqlmock.NewRows([]string{"detail_id", "detail_no", "fee_type", "amount", "fee_date", "ic", "is", "reasons"}).
			AddRow("D001", 1, "T001", 100.0, 1234567890000, 1, "exist", "出差").
			AddRow("D002", 2, "T002", 50.0, 1234567890000, 0, "noExist", ""))

	// 3. invoices
	mock.ExpectQuery(`FROM hesi_flow_invoice WHERE flow_id=\?`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "num", "code", "date", "amt", "total", "tax", "approve",
			"is", "type", "buyer", "buyer_tax", "seller", "seller_tax", "verified"}).
			AddRow("I001", "12345678", "0001", 1234567890000, 100.0, 100.0, 13.0, 100.0,
				"exist", "VAT", "我司", "TAX", "供应商", "TAX2", 1))

	// 4. attachments (源码 line 370/378: 6 列 attachment_type/file_id/file_name/is_invoice/invoice_number/invoice_code)
	mock.ExpectQuery(`FROM hesi_flow_attachment WHERE flow_id=\?`).
		WillReturnRows(sqlmock.NewRows([]string{"attachment_type", "file_id", "file_name", "is_invoice", "invoice_number", "invoice_code"}).
			AddRow("invoice", "F001", "发票.jpg", 1, "12345678", "0001"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/hesi/flow?flowId=F001", nil)
	(&DashboardHandler{DB: db}).GetHesiFlowDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ============ GetHesiAttachmentURLs ============

func TestGetHesiAttachmentURLsMissingFlowId(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/hesi/attachment", nil)
	(&DashboardHandler{DB: db}).GetHesiAttachmentURLs(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("缺 flowId 应 400, got %d", rec.Code)
	}
}

// ============ GetHesiStats ============

func TestGetHesiStatsHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 11 个 COUNT QueryRow 按顺序消费
	// 1. totalFlows
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM hesi_flow WHERE active=1$`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(500))
	// 2. totalExpense
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM hesi_flow WHERE active=1 AND form_type='expense'`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(300))
	// 3. totalLoan
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM hesi_flow WHERE active=1 AND form_type='loan'`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(50))
	// 4. totalRequisition
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM hesi_flow WHERE active=1 AND form_type='requisition'`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(100))
	// 5. totalCustom
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM hesi_flow WHERE active=1 AND form_type='custom'`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(50))
	// 6. paidNoInvoice (JOIN)
	mock.ExpectQuery(`SELECT COUNT\(DISTINCT f\.flow_id\) FROM hesi_flow f\s+JOIN hesi_flow_detail d.*invoice_status='noExist'`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(20))
	// 7. approving
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM hesi_flow WHERE active=1 AND state='approving'`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(10))
	// 8. paying
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM hesi_flow WHERE active=1 AND state='paying'`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(5))
	// 9. totalAttachments
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM hesi_flow_attachment a\s+JOIN hesi_flow f.*WHERE f\.active=1$`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(800))
	// 10. totalInvoiceFiles
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM hesi_flow_attachment a\s+JOIN hesi_flow f.*a\.is_invoice=1`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(600))
	// 11. state distribution
	mock.ExpectQuery(`SELECT state, COUNT\(\*\) as cnt FROM hesi_flow WHERE active=1 GROUP BY state`).
		WillReturnRows(sqlmock.NewRows([]string{"state", "cnt"}).
			AddRow("paid", 400).AddRow("approving", 10).AddRow("paying", 5))
	// 12. type distribution
	mock.ExpectQuery(`SELECT form_type, COUNT\(\*\) as cnt FROM hesi_flow WHERE active=1 GROUP BY form_type`).
		WillReturnRows(sqlmock.NewRows([]string{"form_type", "cnt"}).
			AddRow("expense", 300).AddRow("loan", 50).AddRow("requisition", 100).AddRow("custom", 50))
	// 13. daily trend (近 30 天)
	mock.ExpectQuery(`FROM hesi_flow WHERE active=1 AND create_time >= \?\s+GROUP BY dt`).
		WillReturnRows(sqlmock.NewRows([]string{"dt", "cnt"}).
			AddRow("2026-05-08", 5).AddRow("2026-05-09", 8).AddRow("2026-05-10", 3))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/hesi/stats", nil)
	(&DashboardHandler{DB: db}).GetHesiStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ============ GetHesiFlows ============

func TestGetHesiFlowsEmptyResult(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 1. count = 0
	mock.ExpectQuery(`SELECT COUNT\(DISTINCT f\.flow_id\) FROM hesi_flow f WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0))

	// 2. data 空
	mock.ExpectQuery(`SELECT DISTINCT f\.flow_id, f\.code, f\.title.*FROM hesi_flow f WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"flow_id", "code", "title", "form_type", "state",
			"owner", "dept", "submitter", "pay", "expense", "loan",
			"ct", "ut", "sd", "pd", "fet", "vno", "vstatus"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/hesi/flows?page=1&pageSize=20", nil)
	(&DashboardHandler{DB: db}).GetHesiFlows(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("空结果应 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}
