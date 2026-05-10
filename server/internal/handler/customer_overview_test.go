package handler

// customer_overview_test.go — GetCustomerOverview happy path (跨平台 UNION ALL)
// 已 Read ops_customer.go (line 158 GetCustomerOverview): 1 个超长 UNION ALL SQL.

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetCustomerOverviewEmptyResult(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	cols := []string{
		"platform", "stat_date", "shop_name", "consult_users", "inquiry_users",
		"pay_users", "sales_amount", "first_response_seconds", "response_seconds",
		"satisfaction_rate", "conv_rate",
	}
	mock.ExpectQuery(`UNION ALL`).
		WillReturnRows(sqlmock.NewRows(cols))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/customer/overview?start=2026-04-01&end=2026-04-30", nil)
	(&DashboardHandler{DB: db}).GetCustomerOverview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("空结果应 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetCustomerOverviewWithData(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	cols := []string{
		"platform", "stat_date", "shop_name", "consult_users", "inquiry_users",
		"pay_users", "sales_amount", "first_response_seconds", "response_seconds",
		"satisfaction_rate", "conv_rate",
	}
	mock.ExpectQuery(`UNION ALL`).
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow("天猫", "2026-04-15", "天猫旗舰店", 100.0, 80.0, 50.0, 5000.0, 30.0, 60.0, 95.0, 60.0).
			AddRow("拼多多", "2026-04-15", "拼多多店", 200.0, 150.0, 80.0, 8000.0, 0.0, 0.0, 90.0, 50.0).
			AddRow("京东", "2026-04-15", "京东店", 80.0, 60.0, 40.0, 3000.0, 25.0, 50.0, 88.0, 70.0))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/customer/overview?start=2026-04-01&end=2026-04-30", nil)
	(&DashboardHandler{DB: db}).GetCustomerOverview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("有数据应 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetCustomerOverviewSkipTmallEmptyRow(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	cols := []string{
		"platform", "stat_date", "shop_name", "consult_users", "inquiry_users",
		"pay_users", "sales_amount", "first_response_seconds", "response_seconds",
		"satisfaction_rate", "conv_rate",
	}
	// 天猫全 0 → 应被 skip (line 368 source 跳过 tmall 全空行)
	mock.ExpectQuery(`UNION ALL`).
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow("天猫", "2026-04-15", "天猫零数据店", 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0).
			AddRow("拼多多", "2026-04-15", "拼多多店", 100.0, 80.0, 50.0, 5000.0, 0.0, 0.0, 90.0, 60.0))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/customer/overview?start=2026-04-01&end=2026-04-30", nil)
	(&DashboardHandler{DB: db}).GetCustomerOverview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("应 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetCustomerOverviewWithPlatformFilter(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	cols := []string{
		"platform", "stat_date", "shop_name", "consult_users", "inquiry_users",
		"pay_users", "sales_amount", "first_response_seconds", "response_seconds",
		"satisfaction_rate", "conv_rate",
	}
	mock.ExpectQuery(`UNION ALL`).
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow("天猫", "2026-04-15", "天猫店", 100.0, 80.0, 50.0, 5000.0, 30.0, 60.0, 95.0, 60.0).
			AddRow("拼多多", "2026-04-15", "拼多多店", 200.0, 150.0, 80.0, 8000.0, 0.0, 0.0, 90.0, 50.0))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/customer/overview?start=2026-04-01&end=2026-04-30&platform=天猫", nil)
	(&DashboardHandler{DB: db}).GetCustomerOverview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetCustomerOverviewDBError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`UNION ALL`).WillReturnError(errBoom)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/customer/overview?start=2026-04-01&end=2026-04-30", nil)
	(&DashboardHandler{DB: db}).GetCustomerOverview(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("DB err 应 500, got %d", rec.Code)
	}
}

// ============ RequirePermission/RequireAllPermissions happy ============

func TestRequirePermissionSuperAdminHappy(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.MatchExpectationsInOrder(false)

	// session + super_admin payload
	mock.ExpectQuery(`FROM user_sessions WHERE token_hash`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "lat"}).
			AddRow(int64(1), nowVal()))
	mock.ExpectExec(`UPDATE user_sessions SET last_active_at`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT id, username, real_name, must_change_password FROM users`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "u", "rn", "mcp"}).
			AddRow(int64(1), "admin", "Admin", false))
	mock.ExpectQuery(`SELECT r\.id, r\.code\s+FROM roles r`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code"}).AddRow(int64(1), "super_admin"))

	called := false
	next := func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}

	handler := (&DashboardHandler{DB: db}).RequirePermission("any:permission", next)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "valid"})
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("super_admin 应 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !called {
		t.Error("next 应被调用")
	}
}

func TestRequireAllPermissionsSuperAdminHappy(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.MatchExpectationsInOrder(false)

	mock.ExpectQuery(`FROM user_sessions WHERE token_hash`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "lat"}).
			AddRow(int64(1), nowVal()))
	mock.ExpectExec(`UPDATE user_sessions SET last_active_at`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT id, username, real_name, must_change_password FROM users`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "u", "rn", "mcp"}).
			AddRow(int64(1), "admin", "Admin", false))
	mock.ExpectQuery(`SELECT r\.id, r\.code\s+FROM roles r`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code"}).AddRow(int64(1), "super_admin"))

	called := false
	next := func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}

	handler := (&DashboardHandler{DB: db}).RequireAllPermissions(next, "p1", "p2", "p3")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "valid"})
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("super_admin 应 200, got %d", rec.Code)
	}
	if !called {
		t.Error("next 应被调用")
	}
}

func nowVal() interface{} {
	return time.Now()
}

// ============ GetDBDictionary happy path (info_schema 2 SQL chain) ============

func TestGetDBDictionaryEmptyTables(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM information_schema\.TABLES`).
		WillReturnRows(sqlmock.NewRows([]string{"name", "comment"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/db-dict", nil)
	(&DashboardHandler{DB: db}).GetDBDictionary(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("空表清单应 200, got %d", rec.Code)
	}
}

func TestGetDBDictionaryWithTables(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// tables (含 _template 排除 + 分表合并)
	mock.ExpectQuery(`FROM information_schema\.TABLES`).
		WillReturnRows(sqlmock.NewRows([]string{"name", "comment"}).
			AddRow("users", "用户表").
			AddRow("trade_template", "模板表"). // 排除
			AddRow("trade_202604", "订单分表 2026-04").
			AddRow("trade_202605", "订单分表 2026-05")) // 同 base, 跳过

	// columns: 每张过滤后的表查一次
	mock.ExpectQuery(`FROM information_schema\.COLUMNS`).
		WillReturnRows(sqlmock.NewRows([]string{"cn", "ct", "nul", "key", "def", "cmt"}).
			AddRow("id", "BIGINT", "NO", "PRI", "", "主键"))
	mock.ExpectQuery(`FROM information_schema\.COLUMNS`).
		WillReturnRows(sqlmock.NewRows([]string{"cn", "ct", "nul", "key", "def", "cmt"}).
			AddRow("id", "BIGINT", "NO", "PRI", "", "主键"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/db-dict", nil)
	(&DashboardHandler{DB: db}).GetDBDictionary(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ============ GetTmallcsOps happy path (4 Query chain) ============

func TestGetTmallcsOpsHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.MatchExpectationsInOrder(false)

	// 1. business: op_tmall_cs_shop_daily
	mock.ExpectQuery(`FROM op_tmall_cs_shop_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"d", "pa", "soap", "ap", "iuv", "pso", "pq", "cr", "pu"}))
	// 2. campaign: op_tmall_cs_campaign_daily GROUP BY stat_date, promo_type
	mock.ExpectQuery(`FROM op_tmall_cs_campaign_daily.*GROUP BY stat_date, promo_type`).
		WillReturnRows(sqlmock.NewRows([]string{"d", "pt", "c", "pa", "roi", "clk", "imp"}))
	// 3. keywords: op_tmall_cs_industry_keyword
	mock.ExpectQuery(`FROM op_tmall_cs_industry_keyword`).
		WillReturnRows(sqlmock.NewRows([]string{"kw", "si", "th", "ts", "ci", "vh"}))
	// 4. ranks: op_tmall_cs_market_rank
	mock.ExpectQuery(`FROM op_tmall_cs_market_rank`).
		WillReturnRows(sqlmock.NewRows([]string{"cat", "bn", "th", "tp", "vh", "ci", "ti"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ops/tmall_cs?start=2026-04-01&end=2026-04-30", nil)
	(&DashboardHandler{DB: db}).GetTmallcsOps(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}
