package handler

// push70_test.go — AdminUsersBatchImport dryRun + various low-coverage handlers
// 已 Read admin.go (line 1260+ AdminUsersBatchImport): 解析 Excel + 查重 + dryRun 不写 DB.

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/xuri/excelize/v2"
)

// ============ AdminUsersBatchImport dryRun + invalid rows ============

func buildBatchImportFull(t *testing.T, password, dryRun string, headers []string, dataRows [][]string) (*bytes.Buffer, string) {
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
	fw, _ := w.CreateFormFile("file", "users.xlsx")
	fw.Write(buf.Bytes())
	w.WriteField("password", password)
	w.WriteField("roleCodes", "[]")
	if dryRun != "" {
		w.WriteField("dryRun", dryRun)
	}
	w.Close()
	return body, w.FormDataContentType()
}

func TestAdminUsersBatchImportDryRunHappy(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 查重 (no existing users)
	mock.ExpectQuery(`SELECT username FROM users WHERE username IN`).
		WillReturnRows(sqlmock.NewRows([]string{"u"}))

	body, ct := buildBatchImportFull(t, "Goodpass1A", "true",
		[]string{"姓名", "手机号", "部门"},
		[][]string{
			{"张三", "13800000001", "电商"},
			{"李四", "13800000002", "社媒"},
		})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/batch-import", body)
	req.Header.Set("Content-Type", ct)
	(&DashboardHandler{DB: db}).AdminUsersBatchImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("dryRun 应 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminUsersBatchImportInvalidRows(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT username FROM users WHERE username IN`).
		WillReturnRows(sqlmock.NewRows([]string{"u"}))

	// 多个不合法行: 姓名空, 手机号空, 手机号短
	body, ct := buildBatchImportFull(t, "Goodpass1A", "true",
		[]string{"姓名", "手机号"},
		[][]string{
			{"", "13800000001"}, // 姓名空
			{"张三", ""},          // 手机号空
			{"李四", "12345"},     // 手机号短
			{"王五", "13800000002"}, // 合法
		})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/batch-import", body)
	req.Header.Set("Content-Type", ct)
	(&DashboardHandler{DB: db}).AdminUsersBatchImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("dryRun 含错应 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminUsersBatchImportAllInvalid(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	// 全不合法 + 非 dryRun → "没有可导入的有效数据"
	body, ct := buildBatchImportFull(t, "Goodpass1A", "false",
		[]string{"姓名", "手机号"},
		[][]string{
			{"", ""}, // 全空跳过
			{"x", "1"}, // 不合法
		})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/batch-import", body)
	req.Header.Set("Content-Type", ct)
	(&DashboardHandler{DB: db}).AdminUsersBatchImport(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("全不合法应 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ============ GetSpecialChannelAllotDetails happy path ============

func TestGetSpecialChannelAllotDetailsHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM allocate_details\s+WHERE allocate_no=\?`).
		WithArgs("ALL-202605-001").
		WillReturnRows(sqlmock.NewRows([]string{
			"gn", "sb", "gname", "sname", "sc", "oc", "ic", "ep", "ea", "sp", "ta", "ps",
		}).
			AddRow("G001", "SKU001", "酱油", "500ml", 100.0, 100.0, 100.0, 10.0, 1000.0, 9.5, 950.0, "API"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/special-channel-allot/details?allocate_no=ALL-202605-001", nil)
	(&DashboardHandler{DB: db}).GetSpecialChannelAllotDetails(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetSpecialChannelAllotDetailsDBError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM allocate_details`).WillReturnError(errBoom)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/special-channel-allot/details?allocate_no=A1", nil)
	(&DashboardHandler{DB: db}).GetSpecialChannelAllotDetails(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("DB err 应 500, got %d", rec.Code)
	}
}

// ============ DingtalkAuthURL ============

func TestDingtalkAuthURLEmpty(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/dingtalk-auth-url", nil)
	(&DashboardHandler{DingClientID: "", DB: db}).DingtalkAuthURL(rec, req)

	// 没配置 ClientID 也应不 panic
	if rec.Code != http.StatusOK && rec.Code != http.StatusBadRequest && rec.Code != http.StatusServiceUnavailable {
		t.Errorf("应有合理状态码, got %d", rec.Code)
	}
}

func TestDingtalkAuthURLConfigured(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/dingtalk-auth-url?redirect=http://localhost:3000", nil)
	(&DashboardHandler{DingClientID: "test_client", DB: db}).DingtalkAuthURL(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ============ AdminUserByPath bad path ============

func TestAdminUserByPathBadPath(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/wrong/prefix/123", nil)
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("非 /api/admin/users/ 前缀应 404, got %d", rec.Code)
	}
}

func TestAdminUserByPathBadAction(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/1/unknown_action", nil)
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("未知 action 应 404, got %d", rec.Code)
	}
}

// ============ adminUserDelete 自删除 (验证 admin_more 已有的 happy 不冲突) ============

func TestAdminUserDeleteOnlyHttpDeleteAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/1", nil)
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST 应 405, got %d", rec.Code)
	}
}

// ============ 其他 ============

func TestRequireAuthHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.MatchExpectationsInOrder(false)

	// session + loadAuthPayload (super admin)
	mock.ExpectQuery(`FROM user_sessions WHERE token_hash`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "lat"}).
			AddRow(int64(1), time.Now()))
	mock.ExpectExec(`UPDATE user_sessions SET last_active_at`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT id, username, real_name, must_change_password FROM users`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "u", "rn", "mcp"}).
			AddRow(int64(1), "admin", "Admin", false))
	mock.ExpectQuery(`SELECT r\.id, r\.code\s+FROM roles r`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code"}).AddRow(int64(1), "super_admin"))

	called := false
	next := func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		// 验证 ctx 里有 payload
		if p, ok := authPayloadFromContext(r); !ok || p == nil {
			t.Error("ctx 应注入 payload")
		}
	}

	handler := (&DashboardHandler{DB: db}).RequireAuth(next)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "valid-token"})
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("happy path 应 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !called {
		t.Error("next 应被调用")
	}
}

// 确保 import 不冲突
var _ = context.Background
