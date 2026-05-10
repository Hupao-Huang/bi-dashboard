package handler

// simple_handlers_test.go — notice.go + offline_target.go 简单 handler sqlmock
// 已 Read notice.go 全文 (217 行) + offline_target.go 全文 (141 行).

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ========== notice.go ==========

func TestGetNoticesHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM notices WHERE is_active = 1\s+ORDER BY is_pinned DESC`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "content", "type", "is_pinned", "is_active", "created_by", "created_at", "updated_at"}).
			AddRow(1, "v1.47.0 上线", "渠道管理新增2部门", "feature", true, true, "admin", "2026-05-09 10:00", "2026-05-09 10:00").
			AddRow(2, "v1.46.0 修复", "店铺数据真实化", "fix", false, true, "admin", "2026-05-08 14:00", "2026-05-08 14:00"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/notices", nil)
	(&DashboardHandler{DB: db}).GetNotices(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	notices, _ := resp["notices"].([]interface{})
	if len(notices) != 2 {
		t.Errorf("应 2 条公告, got %d", len(notices))
	}
}

func TestGetNoticesEmptyReturnsEmptyArray(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM notices WHERE is_active = 1`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "content", "type", "is_pinned", "is_active", "created_by", "created_at", "updated_at"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/notices", nil)
	(&DashboardHandler{DB: db}).GetNotices(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200")
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	notices, _ := resp["notices"].([]interface{})
	if notices == nil {
		t.Error("notices 应为空 array (line 49 防 nil)")
	}
	if len(notices) != 0 {
		t.Errorf("空查询应返空 array, got %d", len(notices))
	}
}

func TestAdminListNoticesHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 没有 WHERE is_active 限制 (admin 看全部)
	mock.ExpectQuery(`FROM notices\s+ORDER BY is_pinned DESC`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "content", "type", "is_pinned", "is_active", "created_by", "created_at", "updated_at"}).
			AddRow(1, "active", "...", "feature", true, true, "admin", "2026-05-09 10:00", "2026-05-09 10:00").
			AddRow(2, "disabled", "...", "feature", false, false, "admin", "2026-05-01 10:00", "2026-05-01 10:00"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/notices", nil)
	(&DashboardHandler{DB: db}).AdminListNotices(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200")
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	notices, _ := resp["notices"].([]interface{})
	if len(notices) != 2 {
		t.Errorf("admin 应看到 2 条 (含 disabled), got %d", len(notices))
	}
}

func TestCreateNoticeHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`INSERT INTO notices`).
		WillReturnResult(sqlmock.NewResult(42, 1))

	body := []byte(`{"title":"v1.48 测试覆盖","content":"测试覆盖率从 13% 涨到 22%","type":"feature","isPinned":true}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/notices", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).CreateNotice(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	if resp["id"] != float64(42) {
		t.Errorf("id 应 42, got %v", resp["id"])
	}
}

func TestCreateNoticeMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/notices", nil)
	(&DashboardHandler{DB: db}).CreateNotice(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec.Code)
	}
}

func TestCreateNoticeEmptyTitle(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"title":"","content":"x"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/notices", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).CreateNotice(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("空 title 应 400, got %d", rec.Code)
	}
}

func TestCreateNoticeDefaultsTypeUpdate(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// type 缺省应 fallback 为 "update" (line 103-105)
	mock.ExpectExec(`INSERT INTO notices`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "update", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	body := []byte(`{"title":"x","content":"y"}`) // type 缺省
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/notices", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).CreateNotice(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestUpdateNoticeHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`UPDATE notices SET title=\?,is_pinned=\?,updated_at=\?`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	pinned := true
	title := "新标题"
	bodyMap := map[string]interface{}{"title": &title, "isPinned": &pinned}
	body, _ := json.Marshal(bodyMap)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/notices/42", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).UpdateNotice(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateNoticeNoFields(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/notices/1", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).UpdateNotice(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("无字段应 400, got %d", rec.Code)
	}
}

func TestUpdateNoticeBadID(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/notices/abc", bytes.NewReader([]byte(`{}`)))
	(&DashboardHandler{DB: db}).UpdateNotice(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("非数字 id 应 400, got %d", rec.Code)
	}
}

func TestDeleteNoticeHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`DELETE FROM notices WHERE id=\?`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/notices/42", nil)
	(&DashboardHandler{DB: db}).DeleteNotice(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestNoticeByPathDispatch(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// PUT 路由到 UpdateNotice
	mock.ExpectExec(`UPDATE notices`).WillReturnResult(sqlmock.NewResult(0, 1))
	rec1 := httptest.NewRecorder()
	body := []byte(`{"title":"t"}`)
	req1 := httptest.NewRequest(http.MethodPut, "/api/admin/notices/1", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).NoticeByPath(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Errorf("PUT 应路由到 Update, got %d", rec1.Code)
	}

	// DELETE 路由到 DeleteNotice
	mock.ExpectExec(`DELETE FROM notices`).WillReturnResult(sqlmock.NewResult(0, 1))
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodDelete, "/api/admin/notices/1", nil)
	(&DashboardHandler{DB: db}).NoticeByPath(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("DELETE 应路由到 Delete, got %d", rec2.Code)
	}

	// GET 应 405
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/api/admin/notices/1", nil)
	(&DashboardHandler{DB: db}).NoticeByPath(rec3, req3)
	if rec3.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec3.Code)
	}
}

// ========== offline_target.go ==========

func TestGetOfflineTargetsHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM offline_region_target\s+WHERE year = \?`).
		WithArgs(2026).
		WillReturnRows(sqlmock.NewRows([]string{"month", "region", "target"}).
			AddRow(4, "华东大区", 6000000.0).
			AddRow(4, "华北大区", 5000000.0).
			AddRow(5, "华东大区", 6500000.0))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/offline/targets?year=2026", nil)
	(&DashboardHandler{DB: db}).GetOfflineTargets(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	items, _ := resp["items"].([]interface{})
	if len(items) != 3 {
		t.Errorf("items 应 3 条, got %d", len(items))
	}
}

func TestGetOfflineTargetsBadYearFallsBackToCurrent(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 非数字 year → fallback time.Now().Year() (line 18-20)
	mock.ExpectQuery(`FROM offline_region_target`).
		WillReturnRows(sqlmock.NewRows([]string{"month", "region", "target"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/offline/targets?year=abc", nil)
	(&DashboardHandler{DB: db}).GetOfflineTargets(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("year=abc 应 fallback 到当前年, got %d", rec.Code)
	}
}

func TestGetOfflineTargetsMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/offline/targets", nil)
	(&DashboardHandler{DB: db}).GetOfflineTargets(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST 应 405, got %d", rec.Code)
	}
}

func TestSaveOfflineTargetsHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO offline_region_target.*ON DUPLICATE KEY UPDATE`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO offline_region_target`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body := []byte(`{"year":2026,"items":[
		{"month":4,"region":"华东大区","target":6000000},
		{"month":4,"region":"华北大区","target":5000000}
	]}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/offline/targets", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).SaveOfflineTargets(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestSaveOfflineTargetsBadYear(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"year":1990,"items":[]}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/offline/targets", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).SaveOfflineTargets(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("year=1990 (越界) 应 400, got %d", rec.Code)
	}
}

func TestSaveOfflineTargetsSkipsInvalidItem(t *testing.T) {
	// month=0 / month=13 / region="" / target<0 应被跳过
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	// 只有 1 个 valid item 触发 INSERT (month=4, region 非空)
	mock.ExpectExec(`INSERT INTO offline_region_target`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body := []byte(`{"year":2026,"items":[
		{"month":0,"region":"X","target":100},
		{"month":13,"region":"X","target":100},
		{"month":4,"region":"","target":100},
		{"month":4,"region":"华东大区","target":-100}
	]}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/offline/targets", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).SaveOfflineTargets(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestGetOfflineTargetsByMonthHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT region, target FROM offline_region_target\s+WHERE year = \? AND month = \?`).
		WithArgs(2026, 4).
		WillReturnRows(sqlmock.NewRows([]string{"region", "target"}).
			AddRow("华东大区", 6000000.0).
			AddRow("华北大区", 5000000.0))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/offline/targets/month?year=2026&month=4", nil)
	(&DashboardHandler{DB: db}).GetOfflineTargetsByMonth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	data, _ := env["data"].(map[string]interface{})
	if data["华东大区"] != float64(6000000) {
		t.Errorf("华东大区 应 6000000, got %v", data["华东大区"])
	}
}

// ========== profile.go (no auth → 401) ==========

func TestGetProfileNoAuth(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/profile", nil)
	(&DashboardHandler{DB: db}).GetProfile(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无 auth 应 401, got %d", rec.Code)
	}
}

func TestUpdateProfileNoAuth(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/profile", bytes.NewReader([]byte(`{}`)))
	(&DashboardHandler{DB: db}).UpdateProfile(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无 auth 应 401, got %d", rec.Code)
	}
}

func TestUpdateProfileMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/profile", bytes.NewReader([]byte(`{}`)))
	(&DashboardHandler{DB: db}).UpdateProfile(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec.Code)
	}
}

func TestUploadAvatarNoAuth(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/profile/avatar", nil)
	(&DashboardHandler{DB: db}).UploadAvatar(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无 auth 应 401, got %d", rec.Code)
	}
}
