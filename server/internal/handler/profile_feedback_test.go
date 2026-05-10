package handler

// profile_feedback_test.go — GetProfile/UpdateProfile/UploadAvatar + SubmitFeedback 完整覆盖
// 已 Read profile.go (line 17 GetProfile, 48 UpdateProfile, 98 UploadAvatar).
// 已 Read feedback.go (line 27 SubmitFeedback).

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ GetProfile ============

func TestGetProfileHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	loginAt := "2026-05-10 09:00"
	mock.ExpectQuery(`SELECT IFNULL\(real_name,''\), IFNULL\(avatar,''\)`).
		WillReturnRows(sqlmock.NewRows([]string{"rn", "av", "p", "e", "lat", "did", "ph"}).
			AddRow("Alice", "/avatar/1.jpg", "13800000000", "a@b.com", &loginAt, "DT123", "$2a$..."))

	payload := &authPayload{}
	payload.User.ID = 1
	payload.User.Username = "alice"
	payload.Roles = []string{"ops"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/profile", nil)
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	(&DashboardHandler{DB: db}).GetProfile(rec, req.WithContext(ctx))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "alice") || !strings.Contains(body, "Alice") {
		t.Errorf("应含用户信息: %s", body)
	}
	if !strings.Contains(body, "dingtalkBound") || !strings.Contains(body, "true") {
		t.Errorf("dingtalkBound 应 true: %s", body)
	}
}

func TestGetProfileUnauthorized(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/profile", nil)
	(&DashboardHandler{DB: db}).GetProfile(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无 auth 应 401, got %d", rec.Code)
	}
}

func TestGetProfileDBError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT IFNULL\(real_name,''\)`).WillReturnError(errBoom)

	payload := &authPayload{}
	payload.User.ID = 1

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/profile", nil)
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	(&DashboardHandler{DB: db}).GetProfile(rec, req.WithContext(ctx))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("DB err 应 500, got %d", rec.Code)
	}
}

// ============ UpdateProfile ============

func TestUpdateProfileUnauthorized(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/profile", bytes.NewReader([]byte(`{}`)))
	(&DashboardHandler{DB: db}).UpdateProfile(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无 auth 应 401, got %d", rec.Code)
	}
}

func TestUpdateProfileBadJSON(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	payload := &authPayload{}
	payload.User.ID = 1

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/profile", bytes.NewReader([]byte(`bad`)))
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	(&DashboardHandler{DB: db}).UpdateProfile(rec, req.WithContext(ctx))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json 应 400, got %d", rec.Code)
	}
}

func TestUpdateProfileNothingToUpdate(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	payload := &authPayload{}
	payload.User.ID = 1

	body := []byte(`{}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/profile", bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	(&DashboardHandler{DB: db}).UpdateProfile(rec, req.WithContext(ctx))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("无字段更新应 400, got %d", rec.Code)
	}
}

func TestUpdateProfileHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`UPDATE users SET .*WHERE id`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	payload := &authPayload{}
	payload.User.ID = 1

	body := []byte(`{"realName":"新名字","phone":"13900000000"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/profile", bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	(&DashboardHandler{DB: db}).UpdateProfile(rec, req.WithContext(ctx))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ============ UploadAvatar ============

func TestUploadAvatarMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/profile/avatar", nil)
	(&DashboardHandler{DB: db}).UploadAvatar(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec.Code)
	}
}

func TestUploadAvatarUnauthorized(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/profile/avatar", nil)
	(&DashboardHandler{DB: db}).UploadAvatar(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无 auth 应 401, got %d", rec.Code)
	}
}

func TestUploadAvatarNoFile(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	payload := &authPayload{}
	payload.User.ID = 1

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/profile/avatar", nil)
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	(&DashboardHandler{DB: db}).UploadAvatar(rec, req.WithContext(ctx))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("无文件应 400, got %d", rec.Code)
	}
}

func TestUploadAvatarBadExtension(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("avatar", "evil.exe")
	fw.Write([]byte("evil bytes"))
	w.Close()

	payload := &authPayload{}
	payload.User.ID = 1

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/profile/avatar", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	(&DashboardHandler{DB: db}).UploadAvatar(rec, req.WithContext(ctx))

	if rec.Code != http.StatusBadRequest {
		t.Errorf(".exe 应 400, got %d", rec.Code)
	}
}

// ============ SubmitFeedback (method/auth 边界已 feedback_handlers_test) ============

func TestSubmitFeedbackUnauthorized(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", nil)
	(&DashboardHandler{DB: db}).SubmitFeedback(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无 auth 应 401, got %d", rec.Code)
	}
}

func TestSubmitFeedbackEmptyTitle(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	w.WriteField("title", "")
	w.WriteField("content", "内容")
	w.Close()

	payload := &authPayload{}
	payload.User.ID = 1

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	(&DashboardHandler{DB: db}).SubmitFeedback(rec, req.WithContext(ctx))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("空 title 应 400, got %d", rec.Code)
	}
}

func TestSubmitFeedbackEmptyContent(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	w.WriteField("title", "标题")
	w.WriteField("content", "")
	w.Close()

	payload := &authPayload{}
	payload.User.ID = 1

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	(&DashboardHandler{DB: db}).SubmitFeedback(rec, req.WithContext(ctx))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("空 content 应 400, got %d", rec.Code)
	}
}

func TestSubmitFeedbackHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`INSERT INTO feedback`).
		WillReturnResult(sqlmock.NewResult(99, 1))

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	w.WriteField("title", "标题")
	w.WriteField("content", "反馈内容")
	w.WriteField("pageUrl", "/dashboard")
	w.Close()

	payload := &authPayload{}
	payload.User.ID = 1
	payload.User.Username = "alice"
	payload.User.RealName = "Alice"

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	(&DashboardHandler{DB: db}).SubmitFeedback(rec, req.WithContext(ctx))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "提交成功") {
		t.Errorf("应含成功消息: %s", rec.Body.String())
	}
}
