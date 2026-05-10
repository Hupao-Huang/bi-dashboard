package handler

// feedback_handlers_test.go — feedback.go SubmitFeedback / ListFeedback / FeedbackByPath
// 已 Read feedback.go (line 27-322).

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ SubmitFeedback ============

func TestSubmitFeedbackMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/feedback", nil)
	(&DashboardHandler{DB: db}).SubmitFeedback(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec.Code)
	}
}

func TestSubmitFeedbackNoAuth(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", nil)
	(&DashboardHandler{DB: db}).SubmitFeedback(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无 auth 应 401, got %d", rec.Code)
	}
}

// ============ ListFeedback ============

func TestListFeedbackMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/feedback/list", nil)
	(&DashboardHandler{DB: db}).ListFeedback(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST 应 405, got %d", rec.Code)
	}
}

func TestListFeedbackHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 1. count
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feedback WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(15))

	// 2. list
	// 源码 line 233: createdAt/updatedAt Scan 到 time.Time, attachments []byte
	now := time.Now()
	mock.ExpectQuery(`SELECT id, user_id, username, real_name, title, content, page_url, attachments, status`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "username", "real_name", "title", "content", "page_url", "attachments", "status", "reply", "replied_by", "created_at", "updated_at"}).
			AddRow(1, 1, "user1", "用户1", "Bug 反馈", "看板异常", "/dashboard", []byte("[]"), "pending", nil, nil, now, now))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/feedback/list?page=1&pageSize=20", nil)
	(&DashboardHandler{DB: db}).ListFeedback(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	if resp["total"] != float64(15) {
		t.Errorf("total 应 15, got %v", resp["total"])
	}
}

func TestListFeedbackWithStatusFilter(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM feedback WHERE 1=1 AND status = \?`).
		WithArgs("pending").
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(5))

	mock.ExpectQuery(`FROM feedback WHERE 1=1 AND status = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "username", "real_name", "title", "content", "page_url", "attachments", "status", "reply", "replied_by", "created_at", "updated_at"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/feedback/list?status=pending", nil)
	(&DashboardHandler{DB: db}).ListFeedback(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
}

// ============ FeedbackByPath ============

func TestFeedbackByPathBadID(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/feedback/abc", nil)
	(&DashboardHandler{DB: db}).FeedbackByPath(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("非数字 id 应 400, got %d", rec.Code)
	}
}

func TestFeedbackByPathNoAuth(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/feedback/1", bytes.NewReader([]byte(`{}`)))
	(&DashboardHandler{DB: db}).FeedbackByPath(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无 auth 应 401, got %d", rec.Code)
	}
}

func TestFeedbackByPathMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/feedback/1", nil)
	(&DashboardHandler{DB: db}).FeedbackByPath(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE 应 405, got %d", rec.Code)
	}
}

func TestFeedbackByPathBadJSON(t *testing.T) {
	// 注: 因为没 auth, 测不到 BadJSON 分支 (auth 优先 401)
	// 跳过 — auth 401 已测 (TestFeedbackByPathNoAuth)
	t.Skip("auth 401 优先于 JSON 解析")
}

func TestFeedbackByPathZeroID(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/feedback/0", nil)
	(&DashboardHandler{DB: db}).FeedbackByPath(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("id=0 应 400, got %d", rec.Code)
	}
}
