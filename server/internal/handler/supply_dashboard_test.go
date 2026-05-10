package handler

// supply_dashboard_test.go — GetSupplyChainDashboard 17 并发 SQL 全空 happy path
// 已 Read supply_chain.go (line 201-945): 17 wg goroutines + 2 QueryRow (stockSnapDate min/max)
// 用 MatchExpectationsInOrder(false) 让 OOO 匹配可行.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetSupplyChainDashboardEmptyAllSQL(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.MatchExpectationsInOrder(false)

	// 1. stockSnapDate 主线程 QueryRow (line 229)
	mock.ExpectQuery(`SELECT IFNULL\(MAX\(snapshot_date\),''\) FROM stock_quantity_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"d"}).AddRow("2026-05-09"))

	// 2. salesGMV (line 284)
	mock.ExpectQuery(`SELECT IFNULL\(SUM\(local_goods_amt\),0\) FROM sales_goods_summary WHERE stat_date BETWEEN \? AND \?`).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))

	// 3. stockCost + dailyCost (line 296)
	mock.ExpectQuery(`SELECT IFNULL\(SUM\(current_qty \* cost_price\),0\), IFNULL\(SUM\(month_qty \* cost_price`).
		WillReturnRows(sqlmock.NewRows([]string{"sc", "dc"}).AddRow(0.0, 0.0))

	// 4. stockoutSKU + salesSKU (line 313)
	mock.ExpectQuery(`SUM\(CASE WHEN sum_avail<=0 AND sum_month>0 THEN 1`).
		WillReturnRows(sqlmock.NewRows([]string{"so", "s"}).AddRow(0, 0))

	// 5. highStockValue + totalStockValue (line 336)
	mock.ExpectQuery(`SUM\(CASE WHEN sum_month>0 AND sum_avail/\(sum_month/30\) > 50 THEN sku_stock_value`).
		WillReturnRows(sqlmock.NewRows([]string{"h", "t"}).AddRow(0.0, 0.0))

	// 6. agedStockValue (line 360)
	mock.ExpectQuery(`SELECT IFNULL\(SUM\(b\.current_qty \* IFNULL\(s\.cost_price,0\)\),0\)\s+FROM stock_batch_daily b`).
		WillReturnRows(sqlmock.NewRows([]string{"a"}).AddRow(0.0))

	// 7. monthly sales (line 382)
	mock.ExpectQuery(`SELECT DATE_FORMAT\(stat_date,'%Y-%m'\) AS m, ROUND\(SUM\(local_goods_amt\),2\)\s+FROM sales_goods_summary`).
		WillReturnRows(sqlmock.NewRows([]string{"m", "v"}))

	// 8. channel current period (line 427)
	mock.ExpectQuery(`SELECT CASE WHEN department IS NULL OR department='' THEN 'other' ELSE department END,\s+ROUND\(SUM\(local_goods_amt\)/GREATEST\(DATEDIFF`).
		WillReturnRows(sqlmock.NewRows([]string{"c", "avg", "total"}))

	// 9. last month (line 469)
	mock.ExpectQuery(`SELECT CASE WHEN department IS NULL OR department='' THEN 'other' ELSE department END,.*FROM sales_goods_summary\s+WHERE stat_date BETWEEN DATE_SUB\(\?, INTERVAL 1 MONTH\)`).
		WillReturnRows(sqlmock.NewRows([]string{"d", "v"}))

	// 10. last year (line 501)
	mock.ExpectQuery(`WHERE stat_date BETWEEN DATE_SUB\(\?, INTERVAL 1 YEAR\)`).
		WillReturnRows(sqlmock.NewRows([]string{"d", "v"}))

	// 11. category health (line 547, 复杂子查询)
	mock.ExpectQuery(`SELECT\s+category,.*FROM stock_quantity_daily s\s+LEFT JOIN \(SELECT goods_no, MAX\(cate_full_name\)`).
		WillReturnRows(sqlmock.NewRows([]string{"cat", "sv", "dc", "hv", "so", "s"}))

	// 12. high stock items (line 624)
	mock.ExpectQuery(`SELECT goods_no, MAX\(goods_name\),\s+ROUND\(SUM\(current_qty - locked_qty\),0\),.*HAVING SUM\(month_qty\) > 0`).
		WillReturnRows(sqlmock.NewRows([]string{"no", "name", "uq", "ds", "to", "sv"}))

	// 13. stockout items (line 671)
	mock.ExpectQuery(`SELECT goods_no, MAX\(goods_name\),\s+ROUND\(SUM\(month_qty\)/30,1\),\s+ROUND\(SUM\(month_qty \* cost_price\)/30,2\)`).
		WillReturnRows(sqlmock.NewRows([]string{"no", "name", "ds", "dv"}))

	// 14. top products by sales (line 714, FORCE INDEX (idx_date_goods_amt))
	mock.ExpectQuery(`FROM sales_goods_summary FORCE INDEX \(idx_date_goods_amt\) WHERE stat_date BETWEEN \? AND \?.*GROUP BY goods_no ORDER BY sales DESC LIMIT 20`).
		WillReturnRows(sqlmock.NewRows([]string{"no", "name", "cat", "grade", "sales", "qty"}))

	// 15. top products by qty (line 744)
	mock.ExpectQuery(`GROUP BY goods_no ORDER BY qty DESC LIMIT 20`).
		WillReturnRows(sqlmock.NewRows([]string{"no", "name", "cat", "grade", "qty", "sales"}))

	// 16. cate sales (line 782, FORCE INDEX (idx_date_amt))
	mock.ExpectQuery(`FROM sales_goods_summary FORCE INDEX \(idx_date_amt\) WHERE stat_date BETWEEN \? AND \?`).
		WillReturnRows(sqlmock.NewRows([]string{"cat", "sales", "profit"}))

	// 17. aged items (line 827)
	mock.ExpectQuery(`SELECT b\.goods_no, b\.goods_name, b\.warehouse_name,\s+ROUND\(b\.current_qty,0\)`).
		WillReturnRows(sqlmock.NewRows([]string{"no", "name", "wh", "qty", "sv", "bn", "pd", "age"}))

	// 18. warehouse list (line 861)
	mock.ExpectQuery(`SELECT DISTINCT warehouse_name FROM stock_quantity_daily WHERE snapshot_date=\? AND goods_attr=1 AND warehouse_name!=''`).
		WillReturnRows(sqlmock.NewRows([]string{"wh"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/supply-chain/dashboard?start=2026-04-01&end=2026-04-30", nil)
	(&DashboardHandler{DB: db}).GetSupplyChainDashboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}
