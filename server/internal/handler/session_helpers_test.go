package handler

// session_helpers_test.go — createSessionAndRespond 直接测试 + getDingtalkDepartment 错误路径
// 已 Read auth.go (line 2002 createSessionAndRespond, 1905 getDingtalkDepartment).

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCreateSessionAndRespondHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.MatchExpectationsInOrder(false)

	// INSERT user_sessions
	mock.ExpectExec(`INSERT INTO user_sessions`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	// UPDATE last_login_at
	mock.ExpectExec(`UPDATE users SET last_login_at`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// loadAuthPayload (super_admin shortcut, 2 SQL)
	mock.ExpectQuery(`SELECT id, username, real_name, must_change_password FROM users WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "u", "rn", "mcp"}).
			AddRow(int64(7), "alice", "Alice", false))
	mock.ExpectQuery(`SELECT r\.id, r\.code\s+FROM roles r`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code"}).
			AddRow(int64(1), "super_admin"))
	// audit
	mock.ExpectExec(`INSERT INTO audit_logs`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/dingtalk-login", nil)
	(&DashboardHandler{DB: db}).createSessionAndRespond(rec, req, 7)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	// 验 cookie
	cookies := rec.Result().Cookies()
	hasSession := false
	for _, c := range cookies {
		if c.Name == sessionCookieName && c.Value != "" {
			hasSession = true
		}
	}
	if !hasSession {
		t.Error("应设 session cookie")
	}

	// audit goroutine flush
	time.Sleep(50 * time.Millisecond)
}

func TestCreateSessionAndRespondInsertError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`INSERT INTO user_sessions`).WillReturnError(errBoom)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/dingtalk-login", nil)
	(&DashboardHandler{DB: db}).createSessionAndRespond(rec, req, 1)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("INSERT err 应 500, got %d", rec.Code)
	}
}

func TestCreateSessionAndRespondLoadPayloadError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`INSERT INTO user_sessions`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE users SET last_login_at`).WillReturnResult(sqlmock.NewResult(0, 1))
	// loadAuthPayload users 查不到
	mock.ExpectQuery(`SELECT id, username, real_name, must_change_password FROM users WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "u", "rn", "mcp"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/dingtalk-login", nil)
	(&DashboardHandler{DB: db}).createSessionAndRespond(rec, req, 99)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("loadAuthPayload err 应 500, got %d", rec.Code)
	}
}
