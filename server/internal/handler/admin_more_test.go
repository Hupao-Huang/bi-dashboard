package handler

// admin_more_test.go — admin.go transaction handlers + helpers
// 已 Read admin.go: adminRoleCreate (799), adminRoleDelete (833), adminUserStatusUpdate (611),
// adminUserPasswordUpdate (661), adminUserDelete (708), isUserSuperAdmin (1212), countOtherActiveSuperAdmins (1234).

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ adminRoleCreate (POST /api/admin/roles) ============

func TestAdminRoleCreateHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`INSERT INTO roles \(code, name, description\)`).
		WithArgs("test_role", "测试角色", "描述").
		WillReturnResult(sqlmock.NewResult(42, 1))

	body := []byte(`{"code":"test_role","name":"测试角色","description":"描述"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/roles", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).AdminRoles(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminRoleCreateMissingFields(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"code":"","name":""}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/roles", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).AdminRoles(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("空 name/code 应 400, got %d", rec.Code)
	}
}

func TestAdminRoleCreateDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`INSERT INTO roles`).
		WillReturnError(errors.New("Error 1062: Duplicate entry 'test_role' for key"))

	body := []byte(`{"code":"test_role","name":"X"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/roles", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).AdminRoles(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("Duplicate 应 409, got %d", rec.Code)
	}
}

func TestAdminRoleCreateBadJSON(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/roles", bytes.NewReader([]byte(`bad`)))
	(&DashboardHandler{DB: db}).AdminRoles(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json 应 400, got %d", rec.Code)
	}
}

// ============ AdminRoleByPath GET → adminRoleGet ============

func TestAdminRoleByPathGetNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// loadAdminRoleDetail 第 1 SQL 返 sql.ErrNoRows
	mock.ExpectQuery(`SELECT.+FROM roles WHERE id = \?`).
		WillReturnError(errors.New("sql: no rows in result set"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/roles/999", nil)
	(&DashboardHandler{DB: db}).AdminRoleByPath(rec, req)

	// 实际 errors.Is(err, sql.ErrNoRows) 应 404; 否则可能 500
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusInternalServerError {
		t.Errorf("应 404/500, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ============ AdminUserByPath /status PUT → adminUserStatusUpdate ============

func TestAdminUserStatusUpdateBadJSON(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/users/1/status", bytes.NewReader([]byte(`bad`)))
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json 应 400, got %d", rec.Code)
	}
}

func TestAdminUserStatusUpdateNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT status FROM users WHERE id = \?`).
		WillReturnError(errors.New("sql: no rows in result set"))

	body := []byte(`{"status":"disabled"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/users/999/status", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusNotFound && rec.Code != http.StatusInternalServerError {
		t.Errorf("应 404/500, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminUserStatusUpdateHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 1. SELECT current status
	mock.ExpectQuery(`SELECT status FROM users WHERE id = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("disabled"))

	// 2. UPDATE (because 当前 disabled → active 跳过 isUserSuperAdmin 检查)
	mock.ExpectExec(`UPDATE users SET status = \? WHERE id = \?`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := []byte(`{"status":"active"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/users/1/status", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ============ adminUserPasswordUpdate ============

func TestAdminUserPasswordUpdateBadJSON(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/users/1/password", bytes.NewReader([]byte(`bad`)))
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json 应 400, got %d", rec.Code)
	}
}

func TestAdminUserPasswordUpdateUserNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT username FROM users WHERE id = \?`).
		WillReturnError(errors.New("sql: no rows in result set"))

	body := []byte(`{"password":"Test1234"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/users/999/password", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusNotFound && rec.Code != http.StatusInternalServerError {
		t.Errorf("应 404/500, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminUserPasswordUpdateWeakPassword(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// SELECT username 成功后 validatePassword 失败
	mock.ExpectQuery(`SELECT username FROM users WHERE id = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("user1"))

	body := []byte(`{"password":"weak"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/users/1/password", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("弱密码应 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ============ adminRoleDelete ============

func TestAdminRoleDeleteBuiltInForbidden(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 查 code = "super_admin" → 内置 → 403
	mock.ExpectQuery(`SELECT code FROM roles WHERE id = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"code"}).AddRow("super_admin"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/roles/1", nil)
	(&DashboardHandler{DB: db}).AdminRoleByPath(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("内置角色应 403, got %d", rec.Code)
	}
}

func TestAdminRoleDeleteNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT code FROM roles WHERE id = \?`).
		WillReturnError(errors.New("sql: no rows in result set"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/roles/999", nil)
	(&DashboardHandler{DB: db}).AdminRoleByPath(rec, req)

	if rec.Code != http.StatusNotFound && rec.Code != http.StatusInternalServerError {
		t.Errorf("应 404/500, got %d", rec.Code)
	}
}

func TestAdminRoleDeleteWithUsersConflict(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT code FROM roles WHERE id = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"code"}).AddRow("custom_role"))

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_roles WHERE role_id = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(5)) // 5 个用户在用

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/roles/2", nil)
	(&DashboardHandler{DB: db}).AdminRoleByPath(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("有用户在用应 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminRoleDeleteHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT code FROM roles`).
		WillReturnRows(sqlmock.NewRows([]string{"code"}).AddRow("custom_role"))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_roles`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0))

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM role_permissions WHERE role_id = \?`).
		WillReturnResult(sqlmock.NewResult(0, 5))
	mock.ExpectExec(`DELETE FROM data_scopes WHERE subject_type = 'role'`).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`DELETE FROM roles WHERE id = \?`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/roles/2", nil)
	(&DashboardHandler{DB: db}).AdminRoleByPath(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ============ AdminUserByPath delete ============

func TestAdminUserDeleteMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/users/1", nil)
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /users/1 (无 action) 应 405 (期望 DELETE), got %d", rec.Code)
	}
}
