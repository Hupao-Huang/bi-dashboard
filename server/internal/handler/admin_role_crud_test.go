package handler

// admin_role_crud_test.go — adminRoleGet/Create/Delete + adminUserStatusUpdate 全分支
// 已 Read admin.go (line 611 adminUserStatusUpdate, 787 adminRoleGet, 799 adminRoleCreate, 833 adminRoleDelete).

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ adminRoleGet (GET /api/admin/roles/N) ============

func TestAdminRoleGetHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// loadAdminRoleDetail 3 SQL chain
	mock.ExpectQuery(`SELECT id, code, name, IFNULL\(description,''\) FROM roles WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name", "desc"}).
			AddRow(int64(2), "ops", "运营", "运营角色"))
	mock.ExpectQuery(`FROM permissions p`).
		WillReturnRows(sqlmock.NewRows([]string{"code"}))
	mock.ExpectQuery(`FROM data_scopes WHERE subject_type = 'role'`).
		WillReturnRows(sqlmock.NewRows([]string{"st", "sv"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/roles/2", nil)
	(&DashboardHandler{DB: db}).AdminRoleByPath(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminRoleGetNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT id, code, name.*FROM roles WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name", "desc"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/roles/99", nil)
	(&DashboardHandler{DB: db}).AdminRoleByPath(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("不存在 role 应 404, got %d", rec.Code)
	}
}

// ============ adminRoleCreate DB error (其余 case 已在 admin_more_test.go) ============

func TestAdminRoleCreateDBError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`INSERT INTO roles`).WillReturnError(errBoom)

	body := []byte(`{"name":"x","code":"x"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/roles", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).AdminRoles(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("DB err 应 500, got %d", rec.Code)
	}
}

// ============ adminRoleDelete (DELETE /api/admin/roles/N) ============

func TestAdminRoleDeleteRoleNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT code FROM roles WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"code"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/roles/99", nil)
	(&DashboardHandler{DB: db}).AdminRoleByPath(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("不存在 role 应 404, got %d", rec.Code)
	}
}

func TestAdminRoleDeleteBuiltinRole(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT code FROM roles WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"code"}).AddRow("super_admin"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/roles/1", nil)
	(&DashboardHandler{DB: db}).AdminRoleByPath(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("内置角色应 403, got %d", rec.Code)
	}
}

func TestAdminRoleDeleteHasUsers(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 自定义角色
	mock.ExpectQuery(`SELECT code FROM roles WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"code"}).AddRow("custom_role"))
	// 有 5 个用户在用
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_roles WHERE role_id`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(5))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/roles/2", nil)
	(&DashboardHandler{DB: db}).AdminRoleByPath(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("有用户使用应 409, got %d", rec.Code)
	}
}

// ============ adminUserStatusUpdate 缺漏分支 ============

func TestAdminUserStatusUpdateSelfDisable(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	// payload 是用户自己 + status != active
	payload := &authPayload{}
	payload.User.ID = 5
	body := []byte(`{"status":"disabled"}`)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/users/5/status", bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req.WithContext(ctx))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("自己禁自己应 400, got %d", rec.Code)
	}
}

func TestAdminUserStatusUpdateUserNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT status FROM users WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"st"}))

	body := []byte(`{"status":"active"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/users/99/status", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("不存在 user 应 404, got %d", rec.Code)
	}
}

func TestAdminUserStatusUpdateDisableLastSuperAdmin(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// existingStatus = active
	mock.ExpectQuery(`SELECT status FROM users WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"st"}).AddRow("active"))
	// isUserSuperAdmin = true
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE id = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(1))
	mock.ExpectQuery(`FROM user_roles ur.*super_admin`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(1))
	// 没其他 super_admin
	mock.ExpectQuery(`SELECT COUNT\(DISTINCT u\.id\)`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0))

	body := []byte(`{"status":"disabled"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/users/1/status", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("禁最后 super_admin 应 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

