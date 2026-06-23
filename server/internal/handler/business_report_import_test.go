package handler

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func multipartBody(t *testing.T, filename, mode string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("file", filename)
	fw.Write([]byte("dummy"))
	w.WriteField("mode", mode)
	w.Close()
	return body, w.FormDataContentType()
}

func TestImportBusinessReportPreviewRejectsNonXlsx(t *testing.T) {
	h := &DashboardHandler{}
	body, ct := multipartBody(t, "2026年04月业务预决算报表.csv", "full")
	req := httptest.NewRequest(http.MethodPost, "/api/finance/business-report/import/preview", body)
	req.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()
	h.ImportBusinessReportPreview(rr, req)
	if rr.Code != 400 {
		t.Errorf("非 xlsx 应 400, got %d", rr.Code)
	}
}

func TestImportBusinessReportPreviewRejectsNoMonth(t *testing.T) {
	h := &DashboardHandler{}
	body, ct := multipartBody(t, "2026年业务预决算报表.xlsx", "full") // 文件名无月份且不传 snapshotMonth
	req := httptest.NewRequest(http.MethodPost, "/api/finance/business-report/import/preview", body)
	req.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()
	h.ImportBusinessReportPreview(rr, req)
	if rr.Code != 400 {
		t.Errorf("无快照月份应 400, got %d", rr.Code)
	}
}
