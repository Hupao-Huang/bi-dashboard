package handler

// final_push_test.go — 跨 60% 最后一波: ChangePassword + DistributionCustomerSkus + Me + GetDouyinDistOps account
// 已 Read auth.go (line 1230 ChangePassword, 1311 Me, 1320 RequireAuth).
// 已 Read distribution_customer.go (line 509 DistributionCustomerSkus).

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ ChangePassword ============

func TestChangePasswordUnauthorized(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	// 没有 authPayload context
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", bytes.NewReader([]byte(`{}`)))
	(&DashboardHandler{DB: db}).ChangePassword(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无 auth 应 401, got %d", rec.Code)
	}
}

func TestChangePasswordBadJSON(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	payload := &authPayload{}
	payload.User.ID = 1
	payload.User.Username = "alice"

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", bytes.NewReader([]byte(`bad json`)))
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	(&DashboardHandler{DB: db}).ChangePassword(rec, req.WithContext(ctx))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json 应 400, got %d", rec.Code)
	}
}

func TestChangePasswordInvalidNewPassword(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	payload := &authPayload{}
	payload.User.ID = 1
	payload.User.Username = "alice"

	body := []byte(`{"oldPassword":"old","newPassword":"abc"}`) // 太短 < 8
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	(&DashboardHandler{DB: db}).ChangePassword(rec, req.WithContext(ctx))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("弱密码应 400, got %d", rec.Code)
	}
}

func TestChangePasswordSelectUserError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	payload := &authPayload{}
	payload.User.ID = 1
	payload.User.Username = "alice"

	mock.ExpectQuery(`SELECT IFNULL\(password_hash,''\) FROM users WHERE id`).
		WillReturnError(errBoom)

	body := []byte(`{"oldPassword":"OldPass1","newPassword":"NewPass1"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	(&DashboardHandler{DB: db}).ChangePassword(rec, req.WithContext(ctx))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("DB err 应 500, got %d", rec.Code)
	}
}

func TestChangePasswordWrongOldPassword(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	payload := &authPayload{}
	payload.User.ID = 1
	payload.User.Username = "alice"

	// bcrypt hash 是某随机串, 跟用户提供的 oldPassword 不匹配 → 400
	mock.ExpectQuery(`SELECT IFNULL\(password_hash,''\) FROM users WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"hash"}).
			AddRow("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy")) // bcrypt hash for "hello" or other

	body := []byte(`{"oldPassword":"wrongpass","newPassword":"NewPass1"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	(&DashboardHandler{DB: db}).ChangePassword(rec, req.WithContext(ctx))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("旧密码错应 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ============ DistributionCustomerSkus ============

func TestDistributionCustomerSkusMissingCode(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/distribution/customer/skus", nil)
	(&DashboardHandler{DB: db}).DistributionCustomerSkus(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("缺 customerCode 应 400, got %d", rec.Code)
	}
}

func TestDistributionCustomerSkusHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 单月 (startDate = endDate 同月) → 1 个 trade_goods JOIN trade Query
	mock.ExpectQuery(`FROM trade_goods_\d{6} tg.*JOIN trade_\d{6}`).
		WillReturnRows(sqlmock.NewRows([]string{"gn", "gname", "qty", "amt", "ord", "ispkg"}).
			AddRow("G001", "商品A", 10.0, 1000.0, 5, 0).
			AddRow("G002", "商品B", 5.0, 500.0, 3, 1))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/distribution/customer/skus?customerCode=C001&startDate=2026-04-01&endDate=2026-04-30", nil)
	(&DashboardHandler{DB: db}).DistributionCustomerSkus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ============ Me 接口 ============

func TestMeUnauthorized(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	(&DashboardHandler{DB: db}).Me(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无 auth 应 401, got %d", rec.Code)
	}
}

func TestMeHappyPath(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	payload := &authPayload{}
	payload.User.ID = 1
	payload.User.Username = "alice"
	payload.Roles = []string{"ops"}
	payload.Permissions = []string{"read:trade"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	(&DashboardHandler{DB: db}).Me(rec, req.WithContext(ctx))

	if rec.Code != http.StatusOK {
		t.Errorf("有 auth 应 200, got %d", rec.Code)
	}
}
