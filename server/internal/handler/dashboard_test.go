package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetDateRangeUsesRequestValues(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/overview?start=2026-03-01&end=2026-03-07", nil)

	start, end := getDateRange(req, nil)

	if start != "2026-03-01" || end != "2026-03-07" {
		t.Fatalf("unexpected date range: got %s to %s", start, end)
	}
}

func TestGetDateRangeFallsBackWithoutDB(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/overview", nil)

	start, end := getDateRange(req, nil)

	now := time.Now()
	wantEnd := now.AddDate(0, 0, -1).Format("2006-01-02")
	wantStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	if now.Day() == 1 {
		wantStart = now.AddDate(0, -1, 0).Format("2006-01-02")
		wantStart = wantStart[:8] + "01"
	}

	if start != wantStart || end != wantEnd {
		t.Fatalf("unexpected fallback range: got %s to %s", start, end)
	}
}

func TestGetTrendDateRangeExpandsShortWindow(t *testing.T) {
	start, end := getTrendDateRange("2026-03-20", "2026-03-26")

	if start != "2026-03-13" || end != "2026-03-26" {
		t.Fatalf("unexpected trend range: got %s to %s", start, end)
	}
}

func TestGetTrendDateRangeKeepsLongWindow(t *testing.T) {
	start, end := getTrendDateRange("2026-03-01", "2026-03-20")

	if start != "2026-03-01" || end != "2026-03-20" {
		t.Fatalf("unexpected long trend range: got %s to %s", start, end)
	}
}

func TestBuildPlatformCond(t *testing.T) {
	tests := []struct {
		name     string
		dept     string
		platform string
		wantCond string
		wantArgs []interface{}
	}{
		{
			name:     "all",
			dept:     "ecommerce",
			platform: "all",
			wantCond: "",
			wantArgs: nil,
		},
		{
			name:     "instant",
			dept:     "ecommerce",
			platform: "instant",
			wantCond: " AND shop_name LIKE '%即时零售%'",
			wantArgs: nil,
		},
		{
			name:     "tmall",
			dept:     "ecommerce",
			platform: "tmall",
			wantCond: " AND shop_name IN (SELECT channel_name FROM sales_channel WHERE department = ? AND online_plat_name IN (?))",
			wantArgs: []interface{}{"ecommerce", "天猫商城"},
		},
		{
			name:     "social blocks taobao",
			dept:     "social",
			platform: "taobao",
			wantCond: " AND 1=0",
			wantArgs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCond, gotArgs := buildPlatformCond(tt.dept, tt.platform)
			if gotCond != tt.wantCond {
				t.Fatalf("unexpected cond: got %q want %q", gotCond, tt.wantCond)
			}
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Fatalf("unexpected args: got %#v want %#v", gotArgs, tt.wantArgs)
			}
		})
	}
}

func TestGetTmallOpsMissingShopReturns400(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tmall-ops", nil)

	(&DashboardHandler{}).GetTmallOps(rec, req)

	assertErrorResponse(t, rec, http.StatusBadRequest, "shop is required")
}

func TestGetChannelsMissingDeptReturns400(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/channels", nil)

	(&DashboardHandler{}).GetChannels(rec, req)

	assertErrorResponse(t, rec, http.StatusBadRequest, "dept is required")
}

func TestGetChannelsForbiddenByDeptScopeReturns403(t *testing.T) {
	rec := httptest.NewRecorder()
	req := withAuthPayload(
		httptest.NewRequest(http.MethodGet, "/api/channels?dept=ecommerce", nil),
		&authPayload{DataScopes: authDataScopes{Depts: []string{"social"}}},
	)

	(&DashboardHandler{}).GetChannels(rec, req)

	assertErrorResponse(t, rec, http.StatusForbidden, "forbidden by data scope")
}

func TestGetTmallOpsForbiddenByPlatformScopeReturns403(t *testing.T) {
	rec := httptest.NewRecorder()
	req := withAuthPayload(
		httptest.NewRequest(http.MethodGet, "/api/tmall-ops?shop=天猫旗舰店", nil),
		&authPayload{DataScopes: authDataScopes{Platforms: []string{"jd"}}},
	)

	(&DashboardHandler{}).GetTmallOps(rec, req)

	assertErrorResponse(t, rec, http.StatusForbidden, "forbidden by data scope")
}

func TestGetMarketingCostForbiddenByDomainScopeReturns403(t *testing.T) {
	rec := httptest.NewRecorder()
	req := withAuthPayload(
		httptest.NewRequest(http.MethodGet, "/api/marketing-cost", nil),
		&authPayload{DataScopes: authDataScopes{Domains: []string{"sales"}}},
	)

	(&DashboardHandler{}).GetMarketingCost(rec, req)

	assertErrorResponse(t, rec, http.StatusForbidden, "forbidden by data scope")
}

func TestGetMarketingCostForbiddenByShopScopeReturns403(t *testing.T) {
	rec := httptest.NewRecorder()
	req := withAuthPayload(
		httptest.NewRequest(http.MethodGet, "/api/marketing-cost?shop=店铺A", nil),
		&authPayload{DataScopes: authDataScopes{Domains: []string{"ops"}, Shops: []string{"店铺B"}}},
	)

	(&DashboardHandler{}).GetMarketingCost(rec, req)

	assertErrorResponse(t, rec, http.StatusForbidden, "forbidden by data scope")
}

func TestGetOverviewDatabaseErrorReturns500(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT department,").WillReturnError(errors.New("boom"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/overview?start=2026-03-01&end=2026-03-07", nil)

	(&DashboardHandler{DB: db}).GetOverview(rec, req)

	assertErrorResponse(t, rec, http.StatusInternalServerError, "database query failed")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestGetStockWarningDatabaseErrorReturns500(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("COUNT\\(\\*\\) AS total").WillReturnError(errors.New("boom"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/stock-warning", nil)

	(&DashboardHandler{DB: db}).GetStockWarning(rec, req)

	assertErrorResponse(t, rec, http.StatusInternalServerError, "database query failed")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func assertErrorResponse(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantMsg string) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("unexpected status: got %d want %d", rec.Code, wantStatus)
	}

	var body struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if body.Code != wantStatus {
		t.Fatalf("unexpected body code: got %d want %d", body.Code, wantStatus)
	}
	if body.Msg != wantMsg {
		t.Fatalf("unexpected body msg: got %q want %q", body.Msg, wantMsg)
	}
}

func withAuthPayload(req *http.Request, payload *authPayload) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), currentAuthPayloadKey, payload))
}
