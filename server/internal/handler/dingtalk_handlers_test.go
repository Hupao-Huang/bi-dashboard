package handler

// dingtalk_handlers_test.go — DingtalkLogin/Bind 边界 + fill*TaskStatus + RequireAnyPermission happy
// 已 Read auth.go (line 1732 DingtalkLogin, 2034 DingtalkBind, 1343 RequireAnyPermission, 1360 RequireAllPermissions).
// 已 Read task_monitor.go (line 226 fillLogBasedTaskStatus, 263 fillOpsTaskStatus, 289 fillServiceTaskStatus).

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ DingtalkLogin 边界 ============

func TestDingtalkLoginMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/dingtalk-login", nil)
	(&DashboardHandler{DB: db}).DingtalkLogin(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec.Code)
	}
}

func TestDingtalkLoginBadJSON(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/dingtalk-login", bytes.NewReader([]byte(`bad`)))
	(&DashboardHandler{DB: db}).DingtalkLogin(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json 应 400, got %d", rec.Code)
	}
}

func TestDingtalkLoginEmptyCode(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"code":""}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/dingtalk-login", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).DingtalkLogin(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("空 code 应 400, got %d", rec.Code)
	}
}

func TestDingtalkLoginPendingTokenNotFound(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"pendingToken":"nonexistent","remark":"测试"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/dingtalk-login", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).DingtalkLogin(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("不存在 pendingToken 应 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "已过期") {
		t.Errorf("应提示过期, got %s", rec.Body.String())
	}
}

func TestDingtalkLoginPendingTokenExpired(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	// 注入一个过期的 pending
	dingtalkPendingUsers.Store("token-expired", &dingtalkPendingUser{
		Expires: time.Now().Add(-1 * time.Hour),
		Nick:    "测试",
	})
	defer dingtalkPendingUsers.Delete("token-expired")

	body := []byte(`{"pendingToken":"token-expired"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/dingtalk-login", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).DingtalkLogin(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("过期 pendingToken 应 400, got %d", rec.Code)
	}
}

func TestDingtalkLoginPendingTokenNewUserCreated(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 注入 valid pending
	dingtalkPendingUsers.Store("token-valid", &dingtalkPendingUser{
		Expires:    time.Now().Add(1 * time.Hour),
		Nick:       "新员工",
		UnionId:    "UID12345678901234",
		Mobile:     "13800000000",
		Department: "电商",
	})
	defer dingtalkPendingUsers.Delete("token-valid")

	mock.ExpectExec(`INSERT INTO users \(username, password_hash, real_name, phone, dingtalk_userid, status, remark\)`).
		WillReturnResult(sqlmock.NewResult(99, 1))

	body := []byte(`{"pendingToken":"token-valid","remark":"备注"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/dingtalk-login", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).DingtalkLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("注册申请提交应 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDingtalkLoginPendingTokenDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	dingtalkPendingUsers.Store("dup-token", &dingtalkPendingUser{
		Expires: time.Now().Add(1 * time.Hour),
		Nick:    "重复用户",
		Mobile:  "13800000001",
		UnionId: "UID-DUP",
	})
	defer dingtalkPendingUsers.Delete("dup-token")

	mock.ExpectExec(`INSERT INTO users`).WillReturnError(errBoom)

	body := []byte(`{"pendingToken":"dup-token"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/dingtalk-login", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).DingtalkLogin(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("INSERT err 应 409, got %d", rec.Code)
	}
}

// ============ DingtalkBind 边界 ============

func TestDingtalkBindMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/dingtalk-bind", nil)
	(&DashboardHandler{DB: db}).DingtalkBind(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec.Code)
	}
}

func TestDingtalkBindUnauthorized(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/dingtalk-bind", bytes.NewReader([]byte(`{}`)))
	(&DashboardHandler{DB: db}).DingtalkBind(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无 auth 应 401, got %d", rec.Code)
	}
}

func TestDingtalkBindBadJSON(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	payload := &authPayload{}
	payload.User.ID = 1
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/dingtalk-bind", bytes.NewReader([]byte(`bad`)))
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	(&DashboardHandler{DB: db}).DingtalkBind(rec, req.WithContext(ctx))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json 应 400, got %d", rec.Code)
	}
}

func TestDingtalkBindUnbind(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`UPDATE users SET dingtalk_userid = '' WHERE id`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	payload := &authPayload{}
	payload.User.ID = 5
	body := []byte(`{"action":"unbind"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/dingtalk-bind", bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	(&DashboardHandler{DB: db}).DingtalkBind(rec, req.WithContext(ctx))

	if rec.Code != http.StatusOK {
		t.Errorf("解绑应 200, got %d", rec.Code)
	}
}

func TestDingtalkBindEmptyCode(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	payload := &authPayload{}
	payload.User.ID = 1
	body := []byte(`{"action":"bind","code":""}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/dingtalk-bind", bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	(&DashboardHandler{DB: db}).DingtalkBind(rec, req.WithContext(ctx))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("空 code 应 400, got %d", rec.Code)
	}
}

// ============ fill*TaskStatus ============

func TestFillLogBasedTaskStatusEmptyLogFile(t *testing.T) {
	ts := &TaskStatus{}
	cfg := TaskConfig{LogFile: ""}
	fillLogBasedTaskStatus(ts, cfg)
	// 空 LogFile → 不动 ts
	if ts.Status != "" || ts.LastRun != "" {
		t.Errorf("空 LogFile 应不改 ts, got %+v", ts)
	}
}

func TestFillLogBasedTaskStatusFileNotExist(t *testing.T) {
	ts := &TaskStatus{}
	cfg := TaskConfig{LogFile: "definitely-nonexistent-12345.log"}
	fillLogBasedTaskStatus(ts, cfg)
	// 文件不存在 → 不动 ts
	if ts.Status != "" {
		t.Errorf("文件不存在应不改 ts, got %+v", ts)
	}
}

func TestFillOpsTaskStatusRunning(t *testing.T) {
	syncMu.Lock()
	syncRunning = true
	syncLastLog = ""
	syncLastAt = ""
	syncMu.Unlock()
	defer func() {
		syncMu.Lock()
		syncRunning = false
		syncMu.Unlock()
	}()

	ts := &TaskStatus{}
	(&DashboardHandler{}).fillOpsTaskStatus(ts)

	if ts.Status != "running" {
		t.Errorf("syncRunning=true 应 status=running, got %s", ts.Status)
	}
}

func TestFillOpsTaskStatusFailed(t *testing.T) {
	syncMu.Lock()
	syncRunning = false
	syncLastAt = "2026-05-10 03:00:00"
	syncLastLog = "同步失败：连接错误"
	syncMu.Unlock()
	defer func() {
		syncMu.Lock()
		syncLastAt = ""
		syncLastLog = ""
		syncMu.Unlock()
	}()

	ts := &TaskStatus{}
	(&DashboardHandler{}).fillOpsTaskStatus(ts)

	if ts.Status != "failed" {
		t.Errorf("含'失败'应 status=failed, got %s", ts.Status)
	}
}

func TestFillOpsTaskStatusSuccess(t *testing.T) {
	syncMu.Lock()
	syncRunning = false
	syncLastAt = "2026-05-10 03:00:00"
	syncLastLog = "同步完成"
	syncMu.Unlock()
	defer func() {
		syncMu.Lock()
		syncLastAt = ""
		syncLastLog = ""
		syncMu.Unlock()
	}()

	ts := &TaskStatus{}
	(&DashboardHandler{}).fillOpsTaskStatus(ts)

	if ts.Status != "success" {
		t.Errorf("含'完成'应 status=success, got %s", ts.Status)
	}
}

func TestFillServiceTaskStatusPortNotListening(t *testing.T) {
	// 默认 8080 不一定监听 (test 环境)
	// 不论 listening 与否, 都不应 panic
	ts := &TaskStatus{}
	cfg := TaskConfig{LogFile: ""}
	fillServiceTaskStatus(ts, cfg)
	// status 必填 (running 或 failed)
	if ts.Status != "running" && ts.Status != "failed" {
		t.Errorf("status 应 running 或 failed, got %s", ts.Status)
	}
}

// ============ RequireAnyPermission/RequireAllPermissions happy path ============

func TestRequireAnyPermissionMatched(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.MatchExpectationsInOrder(false)

	// authPayloadFromRequest: session + loadAuthPayload (super_admin shortcut)
	mock.ExpectQuery(`FROM user_sessions WHERE token_hash`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "lat"}).
			AddRow(int64(1), time.Now()))
	mock.ExpectExec(`UPDATE user_sessions SET last_active_at`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT id, username, real_name, must_change_password FROM users WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "u", "rn", "mcp"}).
			AddRow(int64(1), "admin", "Admin", false))
	mock.ExpectQuery(`SELECT r\.id, r\.code\s+FROM roles r`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code"}).AddRow(int64(1), "super_admin"))

	called := false
	next := func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}

	handler := (&DashboardHandler{DB: db}).RequireAnyPermission(next, "any_perm", "another")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "valid-token"})
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("super_admin 通过应 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !called {
		t.Error("next 应被调用")
	}
}

// ============ adminUserDelete 内部分支 (lastSuperAdmin path 已 admin_user_delete_test 覆盖) ============
