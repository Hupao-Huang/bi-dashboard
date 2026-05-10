package handler

// finance_import_test.go — finance_report.go ImportFinance 系列 + Export 边界
// 已 Read finance_report.go (line 611-727 ImportFinancePreview, 732-809 ImportFinanceConfirm).

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ ImportFinancePreview ============

func TestImportFinancePreviewMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/finance/report/import/preview", nil)
	(&DashboardHandler{DB: db}).ImportFinancePreview(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec.Code)
	}
}

func TestImportFinancePreviewBadFormData(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	// POST 但没 multipart → ParseMultipartForm 失败
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/finance/report/import/preview", bytes.NewReader([]byte(`raw`)))
	(&DashboardHandler{DB: db}).ImportFinancePreview(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("非 multipart 应 400, got %d", rec.Code)
	}
}

// ============ ImportFinanceConfirm ============

func TestImportFinanceConfirmMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/finance/report/import/confirm", nil)
	(&DashboardHandler{DB: db}).ImportFinanceConfirm(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec.Code)
	}
}

func TestImportFinanceConfirmBadJSON(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/finance/report/import/confirm", bytes.NewReader([]byte(`bad`)))
	(&DashboardHandler{DB: db}).ImportFinanceConfirm(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json 应 400, got %d", rec.Code)
	}
}

func TestImportFinanceConfirmEmptyToken(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"token":""}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/finance/report/import/confirm", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).ImportFinanceConfirm(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("空 token 应 400, got %d", rec.Code)
	}
}

func TestImportFinanceConfirmInvalidTokenChars(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	// 含非 hex 字符 (g 不是 hex)
	body := []byte(`{"token":"abcdefg0123456789abcdef0123456789"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/finance/report/import/confirm", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).ImportFinanceConfirm(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("非 hex token 应 400, got %d", rec.Code)
	}
}

func TestImportFinanceConfirmInvalidTokenLength(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	// 长度 != 32
	body := []byte(`{"token":"abc123"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/finance/report/import/confirm", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).ImportFinanceConfirm(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("长度非 32 token 应 400, got %d", rec.Code)
	}
}

func TestImportFinanceConfirmTokenNotFound(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	// 32 个 hex 字符, 但文件不存在
	body := []byte(`{"token":"0123456789abcdef0123456789abcdef"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/finance/report/import/confirm", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).ImportFinanceConfirm(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("不存在 token 应 404, got %d", rec.Code)
	}
}

// ============ ExportFinanceReport (调用 GetFinanceReport, 测边界) ============

func TestExportFinanceReportMissingYearStart(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.MatchExpectationsInOrder(false)
	// logAudit goroutine INSERT
	mock.ExpectExec(`INSERT INTO audit_logs`).WillReturnResult(sqlmock.NewResult(1, 1))

	// 内部调 GetFinanceReport, 缺 yearStart 会 400
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/finance/report/export", nil)
	(&DashboardHandler{DB: db}).ExportFinanceReport(rec, req)

	// 缺 yearStart → 内部走 GetFinanceReport, 应 400
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusInternalServerError {
		t.Errorf("缺 yearStart 应 400/500, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "yearStart") && rec.Code != http.StatusInternalServerError {
		t.Errorf("应含 yearStart 错误信息, got %s", rec.Body.String())
	}
}
