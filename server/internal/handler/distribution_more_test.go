package handler

// distribution_more_test.go — DistributionHVCustomerList / DistributionCustomerMonthly happy path
// 已 Read distribution_customer.go (line 296-477).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ DistributionHVCustomerList ============

func TestDistributionHVCustomerListEmptyList(t *testing.T) {
	// 高价值客户名单为空 → 走 line 340-348 快返
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT customer_code, customer_name, grade FROM distribution_high_value_customers WHERE grade IN`).
		WillReturnRows(sqlmock.NewRows([]string{"code", "name", "grade"})) // 空

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/distribution/hv-customers", nil)
	(&DashboardHandler{DB: db}).DistributionHVCustomerList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	if resp["total"] != float64(0) {
		t.Errorf("total 应 0, got %v", resp["total"])
	}
}

func TestDistributionHVCustomerListWithDateRange(t *testing.T) {
	// 单月范围 + 1 个客户 → 1 个 trade SQL (会失败 跳过统计)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT customer_code, customer_name, grade FROM distribution_high_value_customers`).
		WillReturnRows(sqlmock.NewRows([]string{"code", "name", "grade"}).
			AddRow("C001", "客户A", "S"))

	// 跨月 trade query: 1 月 = 1 次, table 不存在 → err continue
	// 不 mock 这一个就会让 Query 报"all expectations were already fulfilled" 但 source code line 361 if err != nil { continue } 会忽略.
	// 不过 sqlmock 不让未注册 Query 通过, 我们用 ExpectQuery WillReturnError
	mock.ExpectQuery(`FROM trade_\d+ t.*INNER JOIN sales_channel`).
		WillReturnError(errBoom)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/distribution/hv-customers?startDate=2026-05-01&endDate=2026-05-31", nil)
	(&DashboardHandler{DB: db}).DistributionHVCustomerList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	// 1 个客户在 list 中但 amount/orders 都 0 (trade SQL 失败 continue)
	if resp["total"] != float64(1) {
		t.Errorf("total 应 1, got %v", resp["total"])
	}
}

// ============ DistributionCustomerMonthly ============

func TestDistributionCustomerMonthlyHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.MatchExpectationsInOrder(false)

	// 单月 → 1 个 trade QueryRow (table 不存在 → silently 0 但还是会调用)
	// 注: sqlmock 不允许 unexpected query, 必须 mock 即使 sourced code 忽略 err
	mock.ExpectQuery(`SELECT IFNULL\(SUM\(payment\),0\), COUNT\(\*\) FROM trade_\d+ WHERE customer_code=\?`).
		WillReturnRows(sqlmock.NewRows([]string{"amt", "cnt"}).AddRow(0.0, 0))

	// 客户基础信息 (line 470)
	mock.ExpectQuery(`SELECT customer_name, IFNULL\(grade,''\) FROM distribution_high_value_customers WHERE customer_code=\?`).
		WillReturnRows(sqlmock.NewRows([]string{"name", "grade"}).AddRow("客户A", "S"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/distribution/customer-monthly?customerCode=C001&startMonth=2026-05&endMonth=2026-05", nil)
	(&DashboardHandler{DB: db}).DistributionCustomerMonthly(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	if resp["customerCode"] != "C001" {
		t.Errorf("customerCode=%v want C001", resp["customerCode"])
	}
	if resp["customerName"] != "客户A" {
		t.Errorf("customerName=%v want 客户A", resp["customerName"])
	}
	months, _ := resp["months"].([]interface{})
	if len(months) != 1 {
		t.Errorf("months 应 1 (单月), got %d", len(months))
	}
}
