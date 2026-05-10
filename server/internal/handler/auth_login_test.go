package handler

// auth_login_test.go — Login 全分支 + seed* + ensureDefaultAdmin + EnsureAuthSchemaAndSeed err 路径
// 已 Read auth.go (line 1098 Login, 989 seedPermissions, 1002 seedRoles, 1015 seedSuperAdminRolePermissions,
//   1036 seedRoleDefaultPermissions, 1058 ensureDefaultAdmin, 574 EnsureAuthSchemaAndSeed).

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"golang.org/x/crypto/bcrypt"
)

// 注入一个 verified captcha entry, 返回 captchaID + 应答 X
func injectVerifiedCaptcha(id string, x int) {
	captchaMu.Lock()
	captchaStore[id] = captchaEntry{
		targetX:   x,
		expiresAt: time.Now().Add(2 * time.Minute),
		verified:  true,
	}
	captchaMu.Unlock()
}

// ============ Login 分支 ============

func TestLoginBadJSON(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader([]byte(`bad json`)))
	(&DashboardHandler{DB: db}).Login(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json 应 400, got %d", rec.Code)
	}
}

func TestLoginEmptyUsername(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"username":"","password":"x","captchaId":"c1","captchaAnswer":1}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).Login(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("空 username 应 400, got %d", rec.Code)
	}
}

func TestLoginEmptyCaptchaID(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"username":"alice","password":"x","captchaId":"","captchaAnswer":1}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).Login(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("空 captchaId 应 400, got %d", rec.Code)
	}
}

func TestLoginCaptchaWrong(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	// 没注入 verified entry → verifyCaptcha 失败
	body := []byte(`{"username":"alice","password":"x","captchaId":"nonexistent","captchaAnswer":100}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).Login(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("无效 captcha 应 400, got %d", rec.Code)
	}
}

func TestLoginUserNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 准备一个 verified captcha
	injectVerifiedCaptcha("login_cap_1", 100)

	mock.ExpectQuery(`SELECT id, password_hash, real_name, status FROM users WHERE username`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ph", "rn", "st"}))

	body := []byte(`{"username":"nobody","password":"abc","captchaId":"login_cap_1","captchaAnswer":100}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).Login(rec, req)

	// 401 (rem > 0) 或 429 (locked)
	if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusTooManyRequests {
		t.Errorf("user 不存在应 401/429, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLoginAccountDisabled(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	injectVerifiedCaptcha("login_cap_2", 200)

	hash, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	mock.ExpectQuery(`SELECT id, password_hash, real_name, status FROM users WHERE username`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ph", "rn", "st"}).
			AddRow(int64(1), string(hash), nil, "disabled"))

	body := []byte(`{"username":"alice","password":"password","captchaId":"login_cap_2","captchaAnswer":200}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).Login(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("禁用账号应 403, got %d", rec.Code)
	}
}

func TestLoginPasswordWrong(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	injectVerifiedCaptcha("login_cap_3", 150)

	hash, _ := bcrypt.GenerateFromPassword([]byte("correctpass"), bcrypt.MinCost)
	mock.ExpectQuery(`SELECT id, password_hash, real_name, status FROM users WHERE username`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ph", "rn", "st"}).
			AddRow(int64(2), string(hash), "Alice", "active"))

	body := []byte(`{"username":"alice","password":"wrongpass","captchaId":"login_cap_3","captchaAnswer":150}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).Login(rec, req)

	if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusTooManyRequests {
		t.Errorf("密码错应 401/429, got %d", rec.Code)
	}
}

func TestLoginHappyPathSuperAdmin(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.MatchExpectationsInOrder(false)

	injectVerifiedCaptcha("login_cap_4", 250)

	hash, _ := bcrypt.GenerateFromPassword([]byte("rightpass"), bcrypt.MinCost)
	// 1. SELECT users
	mock.ExpectQuery(`SELECT id, password_hash, real_name, status FROM users WHERE username`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ph", "rn", "st"}).
			AddRow(int64(7), string(hash), "Alice", "active"))

	// 2. INSERT user_sessions
	mock.ExpectExec(`INSERT INTO user_sessions`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// 3. UPDATE last_login_at
	mock.ExpectExec(`UPDATE users SET last_login_at`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// 4. logAuditNoRequest goroutine INSERT audit_logs
	mock.ExpectExec(`INSERT INTO audit_logs`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// 5. loadAuthPayload: SELECT users (active) + roles
	mock.ExpectQuery(`SELECT id, username, real_name, must_change_password FROM users WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "u", "rn", "mcp"}).
			AddRow(int64(7), "alice", "Alice", false))
	mock.ExpectQuery(`FROM roles r\s+INNER JOIN user_roles ur`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code"}).
			AddRow(int64(1), "super_admin"))

	// super_admin shortcut → loadDataScopes 不走，不需要 mock data_scopes

	body := []byte(`{"username":"alice","password":"rightpass","captchaId":"login_cap_4","captchaAnswer":250,"remember":true}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	// 验证 cookie 设置
	cookies := rec.Result().Cookies()
	hasSession := false
	for _, c := range cookies {
		if c.Name == sessionCookieName && c.Value != "" {
			hasSession = true
		}
	}
	if !hasSession {
		t.Error("Login 成功应设 session cookie")
	}

	// 等 audit goroutine 异步 flush
	time.Sleep(50 * time.Millisecond)
}

// ============ Logout (with cookie + DB delete) ============

func TestLogoutWithCookie(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`DELETE FROM user_sessions WHERE token_hash`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "some-token-value"})
	(&DashboardHandler{DB: db}).Logout(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("应 200, got %d", rec.Code)
	}
}

// ============ seedPermissions ============

func TestSeedPermissionsHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 每条 seed 都 INSERT
	for range permissionSeeds {
		mock.ExpectExec(`INSERT INTO permissions`).WillReturnResult(sqlmock.NewResult(1, 1))
	}

	if err := seedPermissions(db); err != nil {
		t.Fatalf("err: %v", err)
	}
}

func TestSeedPermissionsDBError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`INSERT INTO permissions`).WillReturnError(errBoom)

	if err := seedPermissions(db); err == nil {
		t.Error("DB err 应返 err")
	}
}

// ============ seedRoles ============

func TestSeedRolesHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	for range roleSeeds {
		mock.ExpectExec(`INSERT INTO roles`).WillReturnResult(sqlmock.NewResult(1, 1))
	}

	if err := seedRoles(db); err != nil {
		t.Fatalf("err: %v", err)
	}
}

func TestSeedRolesDBError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`INSERT INTO roles`).WillReturnError(errBoom)
	if err := seedRoles(db); err == nil {
		t.Error("DB err 应返 err")
	}
}

// ============ seedSuperAdminRolePermissions / seedRoleDefaultPermissions DB err ============

func TestSeedSuperAdminRolePermissionsRoleNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 第 1 次 SELECT roles 返 ErrNoRows
	mock.ExpectQuery(`SELECT id FROM roles WHERE code = 'super_admin'`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	if err := seedSuperAdminRolePermissions(db); err == nil {
		t.Error("role 不存在应返 err")
	}
}

func TestSeedRoleDefaultPermissionsRoleNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 第一个 role 找不到就早返
	mock.ExpectQuery(`SELECT id FROM roles WHERE code = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	if err := seedRoleDefaultPermissions(db); err == nil {
		t.Error("role 不存在应返 err")
	}
}

// ============ ensureDefaultAdmin ============

func TestEnsureDefaultAdminUsersExist(t *testing.T) {
	// COUNT > 0 → 早返
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(5))

	if err := ensureDefaultAdmin(db); err != nil {
		t.Fatalf("已有用户应直接返 nil, got %v", err)
	}
}

func TestEnsureDefaultAdminCountError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users`).WillReturnError(errBoom)

	if err := ensureDefaultAdmin(db); err == nil {
		t.Error("COUNT err 应返 err")
	}
}

func TestEnsureDefaultAdminHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// COUNT = 0
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0))
	// INSERT users (含 lastInsertId)
	mock.ExpectExec(`INSERT INTO users \(username, password_hash, real_name, status, must_change_password\)`).
		WillReturnResult(sqlmock.NewResult(99, 1))
	// SELECT roleID super_admin
	mock.ExpectQuery(`SELECT id FROM roles WHERE code = 'super_admin'`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	// INSERT user_roles
	mock.ExpectExec(`INSERT IGNORE INTO user_roles \(user_id, role_id\)`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := ensureDefaultAdmin(db); err != nil {
		t.Fatalf("happy path err: %v", err)
	}
}

// ============ EnsureAuthSchemaAndSeed (第一个 CREATE TABLE 错就早返) ============

func TestEnsureAuthSchemaAndSeedFirstStmtError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 第一条 CREATE TABLE users 立即 err
	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS users`).WillReturnError(errBoom)

	if err := EnsureAuthSchemaAndSeed(db); err == nil {
		t.Error("第一条 SQL err 应返 err")
	}
}

// 防 race
var _ = sync.Mutex{}
