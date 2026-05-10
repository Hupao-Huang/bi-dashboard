package handler

// purchase_plan_test.go — GetPurchasePlan 9 SQL 全空 happy path
// 已 Read supply_chain.go (line 954-1366):
//   - 4 KPI QueryRow (urgentSKU/inTransitOrders/inTransitSubcontract/recent30Amount)
//   - monthlyTrend Query
//   - topVendors Query
//   - 3 suggest queries (prodSQL '成品/半成品' / matSQL '原材料/包材' / otherSQL '其他')

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetPurchasePlanEmptyAllSQL(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.MatchExpectationsInOrder(false)

	// 1. urgentSKU (line 970, 子查询 GROUP BY goods_no HAVING)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM \(\s+SELECT goods_no, SUM\(current_qty - locked_qty\)`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0))

	// 2. inTransitOrders (line 981)
	mock.ExpectQuery(`SELECT COUNT\(DISTINCT id\) FROM ys_purchase_orders\s+WHERE purchase_orders_in_wh_status IN \(2,3\)`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0))

	// 3. inTransitSubcontract (line 985)
	mock.ExpectQuery(`SELECT COUNT\(DISTINCT id\) FROM ys_subcontract_orders\s+WHERE status NOT IN \(2\)`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0))

	// 4. recent30Amount (line 990)
	mock.ExpectQuery(`SELECT SUM\(ori_sum\) FROM ys_purchase_orders\s+WHERE vouchdate >= DATE_SUB`).
		WillReturnRows(sqlmock.NewRows([]string{"amt"}).AddRow(nil))

	// 5. monthlyTrend (line 1000)
	mock.ExpectQuery(`DATE_FORMAT\(vouchdate, '%Y-%m'\) AS month,\s+ROUND\(SUM\(ori_sum\), 0\)`).
		WillReturnRows(sqlmock.NewRows([]string{"month", "amt"}))

	// 6. topVendors (line 1022)
	mock.ExpectQuery(`SELECT vendor_name, ROUND\(SUM\(ori_sum\), 0\) AS amount,\s+COUNT\(DISTINCT id\)`).
		WillReturnRows(sqlmock.NewRows([]string{"v", "a", "c"}))

	// 7. prodSQL '成品/半成品' (line 1065)
	mock.ExpectQuery(`'成品/半成品' AS t,\s+sq\.goods_no AS jky_code`).
		WillReturnRows(sqlmock.NewRows([]string{"t", "jky", "ys", "name", "stock", "avg", "trans", "subc", "cls", "days", "nad", "ndays", "pos", "cate"}))

	// 8. matSQL '原材料/包材' (line 1149)
	mock.ExpectQuery(`'原材料/包材' AS t,\s+IFNULL\(MAX\(gm\.goods_no\)`).
		WillReturnRows(sqlmock.NewRows([]string{"t", "jky", "ys", "name", "stock", "avg", "trans", "subc", "cls", "days", "nad", "ndays", "pos", "cate"}))

	// 9. otherSQL '其他' (line 1221)
	mock.ExpectQuery(`'其他' AS t,\s+sq\.goods_no AS jky_code`).
		WillReturnRows(sqlmock.NewRows([]string{"t", "jky", "ys", "name", "stock", "avg", "trans", "subc", "cls", "days", "nad", "ndays", "pos", "cate"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/supply-chain/purchase-plan", nil)
	(&DashboardHandler{DB: db}).GetPurchasePlan(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// SyncYSStock 405
func TestSyncYSStockMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/supply-chain/sync-ys-stock", nil)
	(&DashboardHandler{DB: db}).SyncYSStock(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec.Code)
	}
}
