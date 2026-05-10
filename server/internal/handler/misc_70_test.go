package handler

// misc_70_test.go — 推 70% 阶段三: AdminUsersBatchImport 边界 + adminUserPasswordUpdate + RunSyncStockOnce locked
//   + sendDingTalk 空 token + getExePath + notifyAdminsNewFeedback admin 不存在
// 已 Read admin.go (line 1260 AdminUsersBatchImport, 661 adminUserPasswordUpdate, 708 adminUserDelete).
// 已 Read sync.go (line 283 sendDingTalk, 444 getExePath).
// 已 Read feedback.go (line 138 notifyAdminsNewFeedback).
// 已 Read stock.go (line 76 RunSyncStockOnce).

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/xuri/excelize/v2"
)

// ============ AdminUsersBatchImport 边界 ============

func buildBatchImportMultipart(t *testing.T, password, roleCodesJSON string, headers []string, dataRows [][]string) (*bytes.Buffer, string) {
	t.Helper()

	tmp := t.TempDir()
	xlsxPath := filepath.Join(tmp, "users.xlsx")

	f := excelize.NewFile()
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellStr("Sheet1", cell, h)
	}
	for i, row := range dataRows {
		for j, v := range row {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+2)
			f.SetCellStr("Sheet1", cell, v)
		}
	}
	if err := f.SaveAs(xlsxPath); err != nil {
		t.Fatalf("save: %v", err)
	}
	f.Close()

	xf, err := excelize.OpenFile(xlsxPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	var buf bytes.Buffer
	xf.Write(&buf)
	xf.Close()

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, err := w.CreateFormFile("file", "users.xlsx")
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	fw.Write(buf.Bytes())
	if password != "" {
		w.WriteField("password", password)
	}
	if roleCodesJSON != "" {
		w.WriteField("roleCodes", roleCodesJSON)
	}
	w.Close()
	return body, w.FormDataContentType()
}

func TestAdminUsersBatchImportEmptyPassword(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body, ct := buildBatchImportMultipart(t, "", "", []string{"姓名", "手机号"}, [][]string{{"张三", "13800000000"}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/batch-import", body)
	req.Header.Set("Content-Type", ct)
	(&DashboardHandler{DB: db}).AdminUsersBatchImport(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("空密码应 400, got %d", rec.Code)
	}
}

func TestAdminUsersBatchImportWeakPassword(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body, ct := buildBatchImportMultipart(t, "abc", "", []string{"姓名", "手机号"}, [][]string{{"x", "13800000000"}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/batch-import", body)
	req.Header.Set("Content-Type", ct)
	(&DashboardHandler{DB: db}).AdminUsersBatchImport(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("弱密码应 400, got %d", rec.Code)
	}
}

func TestAdminUsersBatchImportBadRoleJSON(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body, ct := buildBatchImportMultipart(t, "Goodpass1", "not-json", []string{"姓名", "手机号"}, [][]string{{"x", "13800000000"}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/batch-import", body)
	req.Header.Set("Content-Type", ct)
	(&DashboardHandler{DB: db}).AdminUsersBatchImport(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("非 JSON role 应 400, got %d", rec.Code)
	}
}

func TestAdminUsersBatchImportMissingNameColumn(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body, ct := buildBatchImportMultipart(t, "Goodpass1", "[]", []string{"工号", "手机号"}, [][]string{{"E001", "13800000000"}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/batch-import", body)
	req.Header.Set("Content-Type", ct)
	(&DashboardHandler{DB: db}).AdminUsersBatchImport(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("无姓名列应 400, got %d", rec.Code)
	}
}

func TestAdminUsersBatchImportMissingPhoneColumn(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body, ct := buildBatchImportMultipart(t, "Goodpass1", "[]", []string{"姓名", "部门"}, [][]string{{"张三", "电商"}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/batch-import", body)
	req.Header.Set("Content-Type", ct)
	(&DashboardHandler{DB: db}).AdminUsersBatchImport(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("无手机号列应 400, got %d", rec.Code)
	}
}

func TestAdminUsersBatchImportEmptyExcel(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	// 只有 header 行
	body, ct := buildBatchImportMultipart(t, "Goodpass1", "[]", []string{"姓名", "手机号"}, [][]string{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/batch-import", body)
	req.Header.Set("Content-Type", ct)
	(&DashboardHandler{DB: db}).AdminUsersBatchImport(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("空数据 Excel 应 400, got %d", rec.Code)
	}
}

// ============ adminUserPasswordUpdate 缺漏分支 ============

func TestAdminUserPasswordUpdateInvalidPassword(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT username FROM users WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"u"}).AddRow("alice"))

	body := []byte(`{"password":"abc"}`) // 太短
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/users/1/password", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("弱密码应 400, got %d", rec.Code)
	}
}

// ============ adminUserDelete 自删除分支 ============

func TestAdminUserDeleteSelfDelete(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	payload := &authPayload{}
	payload.User.ID = 5

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/users/5", nil)
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req.WithContext(ctx))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("自删除应 400, got %d", rec.Code)
	}
}

// ============ RunSyncStockOnce locked ============

func TestRunSyncStockOnceLocked(t *testing.T) {
	// 先获取锁让下一次 TryLock 失败
	syncStockMu.Lock()
	defer syncStockMu.Unlock()

	_, _, locked, err := RunSyncStockOnce()
	if !locked {
		t.Error("已有锁应返 locked=true")
	}
	if err != nil {
		t.Errorf("locked 不应有 err, got %v", err)
	}
}

// ============ sendDingTalk 空 token ============

func TestSendDingTalkEmptyToken(t *testing.T) {
	// DingToken 空 → 立即 return, 不发 HTTP
	dh := &DashboardHandler{DingToken: "", DingSecret: ""}
	dh.sendDingTalk("test message") // 应静默返回
}

func TestSendDingTalkEmptySecret(t *testing.T) {
	dh := &DashboardHandler{DingToken: "abc", DingSecret: ""}
	dh.sendDingTalk("test message") // 也应静默返回
}

// ============ getExePath ============

func TestGetExePath(t *testing.T) {
	got := getExePath()
	if got == "" {
		t.Error("getExePath 不应空")
	}
	// 应是绝对路径
	if !filepath.IsAbs(got) {
		t.Errorf("应是绝对路径: %s", got)
	}
}

// ============ notifyAdminsNewFeedback ============

func TestNotifyAdminsNewFeedbackAdminNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT dingtalk_userid FROM users\s+WHERE username = 'admin' AND status = 'active'`).
		WillReturnError(errBoom)

	h := &DashboardHandler{DB: db}
	// 不应 panic, 内部 log + return
	h.notifyAdminsNewFeedback(1, "title", "content", "tester", "")
}

func TestNotifyAdminsNewFeedbackNoDingtalkBinding(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// admin 存在但没绑钉钉 (dingtalk_userid IS NULL)
	mock.ExpectQuery(`SELECT dingtalk_userid FROM users`).
		WillReturnRows(sqlmock.NewRows([]string{"did"}).AddRow(nil))

	h := &DashboardHandler{DB: db}
	h.notifyAdminsNewFeedback(1, "title", "content", "tester", "")
	// 不应 panic
}

// 防 race
var _ = sync.Mutex{}
