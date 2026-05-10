package handler

// admin_access_handlers_test.go — adminUserAccessGet/Update + adminRoleUpdate + RequireAuth/RequirePermission 中间件
// 已 Read admin.go (line 442 AdminUserByPath 路由, 537 adminUserAccessGet, 549 adminUserAccessUpdate, 893 adminRoleUpdate).
// 已 Read auth.go (line 1320 RequireAuth, 1332 RequirePermission, 1343 RequireAnyPermission, 1360 RequireAllPermissions).

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ adminUserAccessGet (GET /api/admin/users/N/access) ============

func TestAdminUserAccessGetHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// loadAdminUserAccess 3 SQL chain
	mock.ExpectQuery(`SELECT id, username, IFNULL\(real_name,''\), status FROM users WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "u", "rn", "st"}).
			AddRow(int64(7), "alice", "Alice", "active"))
	mock.ExpectQuery(`FROM roles r\s+INNER JOIN user_roles ur`).
		WillReturnRows(sqlmock.NewRows([]string{"code"}))
	mock.ExpectQuery(`FROM data_scopes WHERE subject_type = 'user'`).
		WillReturnRows(sqlmock.NewRows([]string{"st", "sv"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/7/access", nil)
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminUserAccessGetNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// QueryRow().Scan() 在空 rows 上会返 sql.ErrNoRows
	mock.ExpectQuery(`SELECT id, username.*FROM users`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "u", "rn", "st"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/999/access", nil)
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("不存在用户应 404, got %d", rec.Code)
	}
}

func TestAdminUserAccessGetDBError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT id, username.*FROM users`).
		WillReturnError(errBoom)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/1/access", nil)
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("DB err 应 500, got %d", rec.Code)
	}
}

// ============ adminUserAccessUpdate (PUT /api/admin/users/N/access) ============

func TestAdminUserAccessUpdateBadJSON(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/users/1/access", bytes.NewReader([]byte(`bad json`)))
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json 应 400, got %d", rec.Code)
	}
}

func TestAdminUserAccessUpdateRemoveLastSuperAdmin(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// isUserSuperAdmin → true
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE id = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(1))
	mock.ExpectQuery(`FROM user_roles ur.*r\.code = 'super_admin'`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(1))

	// countOtherActiveSuperAdmins → 0 (没其他 super_admin)
	mock.ExpectQuery(`SELECT COUNT\(DISTINCT u\.id\)\s+FROM users u`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0))

	body := []byte(`{"roleCodes":["ops"],"dataScopes":{}}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/users/1/access", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("最后一个 super_admin 不能移除应 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminUserAccessUpdateUserNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// isUserSuperAdmin: COUNT=0 → ErrNoRows from sql lib
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE id = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0))

	body := []byte(`{"roleCodes":["ops"],"dataScopes":{}}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/users/999/access", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("user 不存在应 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// adminUserAccessUpdate access path 不允许 POST
func TestAdminUserAccessMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/1/access", nil)
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /access 应 405, got %d", rec.Code)
	}
}

// ============ adminRoleUpdate (PUT /api/admin/roles/N) ============

func TestAdminRoleUpdateBadJSON(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/roles/1", bytes.NewReader([]byte(`bad`)))
	(&DashboardHandler{DB: db}).AdminRoleByPath(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json 应 400, got %d", rec.Code)
	}
}

func TestAdminRoleUpdateMissingName(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"name":"","permissions":[]}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/roles/1", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).AdminRoleByPath(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("空 name 应 400, got %d", rec.Code)
	}
}

func TestAdminRoleUpdateRoleNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// loadAdminRoleDetail 第一查 roles → ErrNoRows (空 rows)
	mock.ExpectQuery(`SELECT id, code, name.*FROM roles WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name", "desc"}))

	body := []byte(`{"name":"运营","permissions":[]}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/roles/99", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).AdminRoleByPath(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("不存在 role 应 404, got %d", rec.Code)
	}
}

func TestAdminRoleUpdateSuperAdminReadOnly(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// roles SELECT 返 super_admin
	mock.ExpectQuery(`SELECT id, code, name.*FROM roles WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name", "desc"}).
			AddRow(int64(1), "super_admin", "超级管理员", ""))
	mock.ExpectQuery(`FROM permissions p`).
		WillReturnRows(sqlmock.NewRows([]string{"code"}))
	mock.ExpectQuery(`FROM data_scopes WHERE subject_type = 'role'`).
		WillReturnRows(sqlmock.NewRows([]string{"st", "sv"}))

	body := []byte(`{"name":"超级管理员","permissions":[]}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/roles/1", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).AdminRoleByPath(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("super_admin role 不能改应 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ============ RequireAuth 中间件 ============

func TestRequireAuthMissingCookie(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	called := false
	next := func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}

	handler := (&DashboardHandler{DB: db}).RequireAuth(next)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无 cookie 应 401, got %d", rec.Code)
	}
	if called {
		t.Error("next 不应被调用")
	}
}

func TestRequireAuthInvalidSession(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// session SELECT 返 ErrNoRows (token 无效, 空 rows)
	mock.ExpectQuery(`FROM user_sessions WHERE token_hash`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "last_active_at"}))

	called := false
	next := func(w http.ResponseWriter, r *http.Request) {
		called = true
	}

	handler := (&DashboardHandler{DB: db}).RequireAuth(next)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "invalid-token"})
	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无效 token 应 401, got %d", rec.Code)
	}
	if called {
		t.Error("next 不应被调用")
	}
}

// ============ RequirePermission ============

func TestRequirePermissionMissingCookie(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	called := false
	next := func(w http.ResponseWriter, r *http.Request) { called = true }

	handler := (&DashboardHandler{DB: db}).RequirePermission("write:notice", next)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无 cookie 应 401, got %d", rec.Code)
	}
	if called {
		t.Error("next 不应被调用")
	}
}

// ============ RequireAnyPermission ============

func TestRequireAnyPermissionMissingCookie(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	called := false
	next := func(w http.ResponseWriter, r *http.Request) { called = true }

	handler := (&DashboardHandler{DB: db}).RequireAnyPermission(next, "p1", "p2")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无 cookie 应 401, got %d", rec.Code)
	}
	if called {
		t.Error("next 不应被调用")
	}
}

// ============ RequireAllPermissions ============

func TestRequireAllPermissionsMissingCookie(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	called := false
	next := func(w http.ResponseWriter, r *http.Request) { called = true }

	handler := (&DashboardHandler{DB: db}).RequireAllPermissions(next, "p1", "p2")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无 cookie 应 401, got %d", rec.Code)
	}
	if called {
		t.Error("next 不应被调用")
	}
}
