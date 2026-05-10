package handler

// auth_loadpayload_test.go — loadAuthPayload + loadDataScopes 全分支
// 已 Read auth.go (line 1420 loadAuthPayload, 1508 loadDataScopes).

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ loadAuthPayload super_admin shortcut ============

func TestLoadAuthPayloadSuperAdminShortcut(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 1. SELECT users
	mock.ExpectQuery(`SELECT id, username, real_name, must_change_password FROM users WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "u", "rn", "mcp"}).
			AddRow(int64(1), "admin", "Admin", false))
	// 2. SELECT roles → super_admin
	mock.ExpectQuery(`SELECT r\.id, r\.code\s+FROM roles r`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code"}).
			AddRow(int64(1), "super_admin"))
	// super_admin shortcut → 不查 permissions/data_scopes

	h := &DashboardHandler{DB: db}
	payload, err := h.loadAuthPayload(1)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !payload.IsSuperAdmin {
		t.Error("应识别为 super_admin")
	}
	if len(payload.Permissions) == 0 {
		t.Error("super_admin 应有全部 permissions (allPermissionCodes)")
	}
}

func TestLoadAuthPayloadRegularUser(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT id, username, real_name, must_change_password FROM users WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "u", "rn", "mcp"}).
			AddRow(int64(2), "alice", "Alice", false))
	mock.ExpectQuery(`SELECT r\.id, r\.code\s+FROM roles r`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code"}).
			AddRow(int64(3), "ops"))
	// permissions
	mock.ExpectQuery(`SELECT DISTINCT p\.code\s+FROM permissions p`).
		WillReturnRows(sqlmock.NewRows([]string{"code"}).
			AddRow("read:trade").AddRow("write:notice"))
	// loadDataScopes
	mock.ExpectQuery(`FROM data_scopes WHERE \(subject_type = 'user'`).
		WillReturnRows(sqlmock.NewRows([]string{"st", "sv"}).
			AddRow("dept", "电商").AddRow("platform", "tmall"))

	h := &DashboardHandler{DB: db}
	payload, err := h.loadAuthPayload(2)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if payload.IsSuperAdmin {
		t.Error("非 super_admin")
	}
	if len(payload.Permissions) != 2 {
		t.Errorf("Permissions len=%d want 2", len(payload.Permissions))
	}
	if len(payload.DataScopes.Depts) != 1 || payload.DataScopes.Depts[0] != "电商" {
		t.Errorf("Depts wrong: %v", payload.DataScopes.Depts)
	}
}

func TestLoadAuthPayloadUserNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT id, username.*FROM users WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "u", "rn", "mcp"}))

	h := &DashboardHandler{DB: db}
	_, err = h.loadAuthPayload(99)
	if err == nil {
		t.Error("user 不存在应返 err")
	}
}

func TestLoadAuthPayloadRolesError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT id, username.*FROM users`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "u", "rn", "mcp"}).
			AddRow(int64(1), "x", "X", false))
	mock.ExpectQuery(`SELECT r\.id, r\.code\s+FROM roles r`).
		WillReturnError(errBoom)

	h := &DashboardHandler{DB: db}
	_, err = h.loadAuthPayload(1)
	if err == nil {
		t.Error("roles err 应返 err")
	}
}

func TestLoadAuthPayloadPermissionsError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT id, username.*FROM users`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "u", "rn", "mcp"}).
			AddRow(int64(1), "x", "X", false))
	mock.ExpectQuery(`FROM roles r`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code"}).
			AddRow(int64(2), "ops"))
	mock.ExpectQuery(`FROM permissions p`).
		WillReturnError(errBoom)

	h := &DashboardHandler{DB: db}
	_, err = h.loadAuthPayload(1)
	if err == nil {
		t.Error("permissions err 应返 err")
	}
}

// ============ loadDataScopes ============

func TestLoadDataScopesUserOnlyNoRoles(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 没 roles → 只查 user 部分 (无 OR ... role 子句)
	mock.ExpectQuery(`FROM data_scopes WHERE \(subject_type = 'user' AND subject_id = \?\) ORDER BY scope_type, scope_value`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"st", "sv"}).
			AddRow("dept", "电商").
			AddRow("dept", "社媒"). // 重复 → uniqueSortedStrings 后保留
			AddRow("shop", "天猫旗舰店").
			AddRow("warehouse", "华东仓"))

	h := &DashboardHandler{DB: db}
	scopes, err := h.loadDataScopes(7, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(scopes.Depts) != 2 {
		t.Errorf("Depts len=%d want 2 (sorted)", len(scopes.Depts))
	}
	if len(scopes.Shops) != 1 || scopes.Shops[0] != "天猫旗舰店" {
		t.Errorf("Shops wrong: %v", scopes.Shops)
	}
	if len(scopes.Warehouses) != 1 {
		t.Errorf("Warehouses len=%d want 1", len(scopes.Warehouses))
	}
}

func TestLoadDataScopesUserAndRoles(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM data_scopes.*subject_type = 'role' AND subject_id IN`).
		WillReturnRows(sqlmock.NewRows([]string{"st", "sv"}).
			AddRow("platform", "tmall").
			AddRow("platform", "jd").
			AddRow("domain", "trade"))

	h := &DashboardHandler{DB: db}
	scopes, err := h.loadDataScopes(1, []int64{2, 3})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(scopes.Platforms) != 2 {
		t.Errorf("Platforms len=%d want 2", len(scopes.Platforms))
	}
	if len(scopes.Domains) != 1 {
		t.Errorf("Domains len=%d want 1", len(scopes.Domains))
	}
}

func TestLoadDataScopesQueryError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM data_scopes`).WillReturnError(errBoom)

	h := &DashboardHandler{DB: db}
	_, err = h.loadDataScopes(1, nil)
	if err == nil {
		t.Error("DB err 应返 err")
	}
}

// ============ authPayloadFromRequest 经路径 ============

func TestAuthPayloadFromRequestNoCookie(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	h := &DashboardHandler{DB: db}
	_, err := h.authPayloadFromRequest(req)
	if err == nil {
		t.Error("无 cookie 应返 err")
	}
}

func TestAuthPayloadFromRequestSessionNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM user_sessions WHERE token_hash`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "lat"}))

	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "fake-token"})
	h := &DashboardHandler{DB: db}
	_, err = h.authPayloadFromRequest(req)
	if err == nil {
		t.Error("session 不存在应返 err")
	}
}
