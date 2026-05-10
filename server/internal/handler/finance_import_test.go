package handler

// finance_import_test.go — finance_report.go ImportFinance 系列 + Export 边界
// 已 Read finance_report.go (line 611-727 ImportFinancePreview, 732-809 ImportFinanceConfirm).
// 已 Read internal/finance/parser.go (line 184 LoadSubjectDict 1 SQL, 768 ComputeDiff 1 SQL).

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/xuri/excelize/v2"
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

// buildFinanceMultipart 造 multipart form: file = 最小空 sheet xlsx, mode, [year]
func buildFinanceMultipart(t *testing.T, filename, mode string, extraFields map[string]string) (*bytes.Buffer, string) {
	t.Helper()

	tmp := t.TempDir()
	xlsxPath := filepath.Join(tmp, "fixture.xlsx")

	f := excelize.NewFile()
	// 默认 Sheet1 不在 SheetDeptMap，会被 skip → SheetCount=0, RowCount=0 (Happy path 不需要业务行)
	if err := f.SaveAs(xlsxPath); err != nil {
		t.Fatalf("save xlsx: %v", err)
	}
	f.Close()

	xlsxBytes, err := excelize.OpenFile(xlsxPath)
	if err != nil {
		t.Fatalf("open xlsx: %v", err)
	}
	var buf bytes.Buffer
	if err := xlsxBytes.Write(&buf); err != nil {
		t.Fatalf("write xlsx to buf: %v", err)
	}
	xlsxBytes.Close()

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(buf.Bytes()); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	w.WriteField("mode", mode)
	for k, v := range extraFields {
		w.WriteField(k, v)
	}
	w.Close()
	return body, w.FormDataContentType()
}

func TestImportFinancePreviewBadExtension(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body, ct := buildFinanceMultipart(t, "2026年报表.xls", "full", nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/finance/report/import/preview", body)
	req.Header.Set("Content-Type", ct)
	(&DashboardHandler{DB: db}).ImportFinancePreview(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf(".xls 应 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "xlsx") {
		t.Errorf("错误信息应提示 xlsx, got %s", rec.Body.String())
	}
}

func TestImportFinancePreviewBadMode(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body, ct := buildFinanceMultipart(t, "2026年报表.xlsx", "invalid_mode", nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/finance/report/import/preview", body)
	req.Header.Set("Content-Type", ct)
	(&DashboardHandler{DB: db}).ImportFinancePreview(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("非法 mode 应 400, got %d", rec.Code)
	}
}

func TestImportFinancePreviewYearInferFailed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	// 文件名不含 YYYY年, 也不传 year 参数
	body, ct := buildFinanceMultipart(t, "report.xlsx", "full", nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/finance/report/import/preview", body)
	req.Header.Set("Content-Type", ct)
	(&DashboardHandler{DB: db}).ImportFinancePreview(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("无法推断年份应 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "年份") {
		t.Errorf("错误应提到年份, got %s", rec.Body.String())
	}
}

func TestImportFinancePreviewYearOutOfRange(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	// year 1900 < 2000
	body, ct := buildFinanceMultipart(t, "report.xlsx", "full", map[string]string{"year": "1900"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/finance/report/import/preview", body)
	req.Header.Set("Content-Type", ct)
	(&DashboardHandler{DB: db}).ImportFinancePreview(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("年份越界应 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "1900") {
		t.Errorf("错误应含 1900, got %s", rec.Body.String())
	}
}

func TestImportFinancePreviewLoadDictDBError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT subject_code, subject_name.*FROM finance_subject_dict`).
		WillReturnError(errBoom)

	body, ct := buildFinanceMultipart(t, "2026年财务管理报表.xlsx", "full", nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/finance/report/import/preview", body)
	req.Header.Set("Content-Type", ct)
	(&DashboardHandler{DB: db}).ImportFinancePreview(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("LoadSubjectDict err 应 500, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// Happy path: 上传 .xlsx (没有已知 sheet name) → ParseFile 返空 result → ComputeDiff 跑空 → 200 + token
func TestImportFinancePreviewHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 1. LoadSubjectDict 返空
	mock.ExpectQuery(`SELECT subject_code, subject_name, subject_category, subject_level, parent_code, aliases FROM finance_subject_dict`).
		WillReturnRows(sqlmock.NewRows([]string{"code", "name", "cat", "lvl", "parent", "aliases"}))

	// 2. ComputeDiff (line 798): SELECT department, month, SUM... FROM finance_report WHERE year=?
	mock.ExpectQuery(`FROM finance_report\s+WHERE year = \?\s+GROUP BY department, month`).
		WithArgs(2026).
		WillReturnRows(sqlmock.NewRows([]string{"dept", "month", "rev", "rows"}))

	body, ct := buildFinanceMultipart(t, "2026年财务管理报表.xlsx", "full", nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/finance/report/import/preview", body)
	req.Header.Set("Content-Type", ct)
	(&DashboardHandler{DB: db}).ImportFinancePreview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var env map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal env: %v", err)
	}
	resp, _ := env["data"].(map[string]interface{})
	token, _ := resp["token"].(string)
	if len(token) != 32 {
		t.Errorf("token 应 32 hex, got len=%d", len(token))
	}
	if y, _ := resp["year"].(float64); int(y) != 2026 {
		t.Errorf("year=%v want 2026", resp["year"])
	}
	if m, _ := resp["mode"].(string); m != "full" {
		t.Errorf("mode=%q want full", m)
	}
}

// year 参数覆盖文件名推断
func TestImportFinancePreviewYearOverride(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM finance_subject_dict`).
		WillReturnRows(sqlmock.NewRows([]string{"code", "name", "cat", "lvl", "parent", "aliases"}))
	mock.ExpectQuery(`FROM finance_report\s+WHERE year = \?`).
		WithArgs(2025).
		WillReturnRows(sqlmock.NewRows([]string{"dept", "month", "rev", "rows"}))

	// 文件名 2026 但 year 参数 2025 → 用 2025
	body, ct := buildFinanceMultipart(t, "2026年报表.xlsx", "incremental", map[string]string{"year": "2025"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/finance/report/import/preview", body)
	req.Header.Set("Content-Type", ct)
	(&DashboardHandler{DB: db}).ImportFinancePreview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	if y, _ := resp["year"].(float64); int(y) != 2025 {
		t.Errorf("year override 失败, got %v want 2025", resp["year"])
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
