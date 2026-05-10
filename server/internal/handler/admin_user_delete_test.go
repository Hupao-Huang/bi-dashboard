package handler

// admin_user_delete_test.go — adminUserDelete + isUserSuperAdmin + countOtherActiveSuperAdmins + AdminUsersBatchImport
// 已 Read admin.go (line 708-785 adminUserDelete, 1212-1245 helpers, 1260-1320 BatchImport).

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ isUserSuperAdmin ============

func TestIsUserSuperAdminTrue(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 用户存在
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE id = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(1))
	// 是 super_admin
	mock.ExpectQuery(`FROM user_roles ur\s+INNER JOIN roles r.*r\.code = 'super_admin'`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(1))

	h := &DashboardHandler{DB: db}
	got, err := h.isUserSuperAdmin(1)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !got {
		t.Error("应 super admin = true")
	}
}

func TestIsUserSuperAdminUserNotExists(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// COUNT = 0 → 用户不存在
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE id = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0))

	h := &DashboardHandler{DB: db}
	_, err = h.isUserSuperAdmin(999)
	if err == nil {
		t.Error("用户不存在应返 ErrNoRows")
	}
}

func TestIsUserSuperAdminNotSuperAdmin(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE id = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(1))
	mock.ExpectQuery(`FROM user_roles ur.*super_admin`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0))

	h := &DashboardHandler{DB: db}
	got, _ := h.isUserSuperAdmin(2)
	if got {
		t.Error("非 super admin 应返 false")
	}
}

// ============ countOtherActiveSuperAdmins ============

func TestCountOtherActiveSuperAdmins(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(DISTINCT u\.id\)\s+FROM users u`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(3))

	h := &DashboardHandler{DB: db}
	cnt, err := h.countOtherActiveSuperAdmins(1)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if cnt != 3 {
		t.Errorf("cnt=%d want 3", cnt)
	}
}

// ============ adminUserDelete ============

func TestAdminUserDeleteUserNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE id=\?`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/users/999", nil)
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("应 404, got %d", rec.Code)
	}
}

func TestAdminUserDeleteLastSuperAdmin(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 用户存在
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE id=\?`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(1))
	// 是 super_admin
	mock.ExpectQuery(`FROM user_roles ur.*r\.code = 'super_admin'`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(1))
	// 没其他 super_admin
	mock.ExpectQuery(`FROM user_roles ur.*r\.code = 'super_admin' AND ur\.user_id <> \?`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/users/1", nil)
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("最后一个 super_admin 应 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminUserDeleteHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE id=\?`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(1))
	mock.ExpectQuery(`FROM user_roles ur.*super_admin`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0)) // 不是 super_admin

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM user_sessions WHERE user_id = \?`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DELETE FROM user_roles WHERE user_id = \?`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM data_scopes WHERE subject_type = 'user'`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DELETE FROM users WHERE id = \?`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/users/2", nil)
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminUserDeleteDBError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE id=\?`).
		WillReturnError(errors.New("connection lost"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/users/1", nil)
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("DB err 应 500, got %d", rec.Code)
	}
}

// ============ AdminUsersBatchImport ============

func TestAdminUsersBatchImportMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/batch-import", nil)
	(&DashboardHandler{DB: db}).AdminUsersBatchImport(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec.Code)
	}
}

func TestAdminUsersBatchImportBadFormData(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	// POST 但没 multipart → ParseMultipartForm 失败
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/batch-import", bytes.NewReader([]byte(`raw`)))
	(&DashboardHandler{DB: db}).AdminUsersBatchImport(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("非 multipart 应 400, got %d", rec.Code)
	}
}
