package handler

// stock_warning_test.go — GetStockWarning 主路径 + writeStockResponse
// 已 Read stock.go (line 134-378): GetStockWarning 3 SQL chain.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetStockWarningHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 1. summary QueryRow (line 158)
	mock.ExpectQuery(`SELECT\s+COUNT\(\*\) AS total,\s+SUM\(CASE WHEN current_qty - locked_qty <= 0 AND month_qty > 0 THEN 1`).
		WillReturnRows(sqlmock.NewRows([]string{"total", "stockout", "urgent", "low", "overstock", "dead"}).
			AddRow(1000, 50, 30, 100, 20, 5))

	// 2. detail Query (line 228)
	mock.ExpectQuery(`SELECT sq\.goods_no, sq\.goods_name, sq\.unit_name`).
		WillReturnRows(sqlmock.NewRows([]string{"no", "name", "unit", "wh", "uq", "mq", "da", "sd", "cq", "cat", "pos"}).
			AddRow("G001", "商品A", "瓶", "华东仓", 50.0, 100.0, 3.3, 15.0, 60.0, "调味料", "S"))

	// 3. writeStockResponse warehouse list (line 391)
	mock.ExpectQuery(`SELECT DISTINCT warehouse_name FROM stock_quantity WHERE goods_attr = 1`).
		WillReturnRows(sqlmock.NewRows([]string{"wh"}).AddRow("华东仓").AddRow("华南仓"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/stock/warning", nil)
	(&DashboardHandler{DB: db}).GetStockWarning(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	summary, _ := resp["summary"].(map[string]interface{})
	if summary["total"] != float64(1000) {
		t.Errorf("total=1000 missing, got %v", summary["total"])
	}
	whs, _ := resp["warehouses"].([]interface{})
	if len(whs) != 2 {
		t.Errorf("warehouses 应 2, got %d", len(whs))
	}
}

func TestGetStockWarningWithFilter(t *testing.T) {
	// keyword + warning=stockout 过滤
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`COUNT\(\*\) AS total`).
		WillReturnRows(sqlmock.NewRows([]string{"total", "stockout", "urgent", "low", "overstock", "dead"}).
			AddRow(0, 0, 0, 0, 0, 0))

	// detail with keyword + warning=stockout
	mock.ExpectQuery(`AND \(sq\.goods_no LIKE \? OR sq\.goods_name LIKE \?\) AND \(sq\.current_qty - sq\.locked_qty\) <= 0 AND sq\.month_qty > 0`).
		WillReturnRows(sqlmock.NewRows([]string{"no", "name", "unit", "wh", "uq", "mq", "da", "sd", "cq", "cat", "pos"}))

	mock.ExpectQuery(`SELECT DISTINCT warehouse_name`).
		WillReturnRows(sqlmock.NewRows([]string{"wh"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/stock/warning?keyword=酱油&warning=stockout", nil)
	(&DashboardHandler{DB: db}).GetStockWarning(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetStockWarningSummaryDBError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`COUNT\(\*\) AS total`).WillReturnError(errBoom)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/stock/warning", nil)
	(&DashboardHandler{DB: db}).GetStockWarning(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("DB err 应 500, got %d", rec.Code)
	}
}
