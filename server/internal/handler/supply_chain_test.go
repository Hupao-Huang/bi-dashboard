package handler

// supply_chain_test.go — supply_chain.go 简单 handler sqlmock
// 已 Read supply_chain.go:
//   - GetSupplyChainMonthlyTrend (line 129): 1 SQL (line 166)
//   - GetInTransitDetail (line 1651): 2 SQL (line 1712 purchase + line 1751 subcontract)
//   - GetSyncYSProgress (line 1615): 0 SQL (读全局 syncYSProgress var)
//   - syncYSProgress (line 1412): 全局 var, zero value 即未运行状态

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ---------- GetSupplyChainMonthlyTrend ----------

func TestGetSupplyChainMonthlyTrendHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT stat_month AS m, ROUND\(SUM\(local_goods_amt\),2\)\s+FROM sales_goods_summary_monthly`).
		WillReturnRows(sqlmock.NewRows([]string{"month", "value"}).
			AddRow("2026-01", 1500000.0).
			AddRow("2026-02", 1700000.0).
			AddRow("2026-03", 2000000.0))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/supply-chain/monthly-trend?start_month=2026-01&end_month=2026-03", nil)
	(&DashboardHandler{DB: db}).GetSupplyChainMonthlyTrend(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	data, _ := env["data"].([]interface{})
	if len(data) != 3 {
		t.Errorf("data 应 3 个月, got %d", len(data))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestGetSupplyChainMonthlyTrendInvalidMonth(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/supply-chain/monthly-trend?start_month=invalid&end_month=2026-03", nil)
	(&DashboardHandler{DB: db}).GetSupplyChainMonthlyTrend(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid month 应 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetSupplyChainMonthlyTrendDefaultRange(t *testing.T) {
	// 缺 start_month/end_month → 走默认 (2020-01 ~ 9999-12)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM sales_goods_summary_monthly`).
		WillReturnRows(sqlmock.NewRows([]string{"m", "v"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/supply-chain/monthly-trend", nil)
	(&DashboardHandler{DB: db}).GetSupplyChainMonthlyTrend(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("default range 应 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

// ---------- GetInTransitDetail ----------

func TestGetInTransitDetailHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 1. 在途采购订单
	mock.ExpectQuery(`FROM ys_purchase_orders p\s+WHERE p\.purchase_orders_in_wh_status IN \(2,3\)`).
		WillReturnRows(sqlmock.NewRows([]string{"code", "vendor", "org", "vouch", "arrive", "total", "incoming", "in_transit", "status"}).
			AddRow("PO-001", "供应商A", "总部", "2026-04-01", "2026-05-01", 1000.0, 300.0, 700.0, "部分入库").
			AddRow("PO-002", "供应商B", "总部", "2026-04-15", "", 500.0, 0.0, 500.0, "已审核未入库"))

	// 2. 在途委外订单
	mock.ExpectQuery(`FROM ys_subcontract_orders s\s+WHERE s\.status NOT IN \(2\)`).
		WillReturnRows(sqlmock.NewRows([]string{"code", "vendor", "org", "vouch", "arrive", "total", "incoming", "in_transit", "status"}).
			AddRow("SC-001", "委外厂A", "总部", "2026-04-10", "2026-05-10", 800.0, 200.0, 600.0, "部分入库"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/in-transit-detail?goodsNo=03030236", nil)
	(&DashboardHandler{DB: db}).GetInTransitDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	if resp["goodsNo"] != "03030236" {
		t.Errorf("goodsNo 应 03030236, got %v", resp["goodsNo"])
	}
	po, _ := resp["purchaseOrders"].([]interface{})
	if len(po) != 2 {
		t.Errorf("purchaseOrders 应 2 条, got %d", len(po))
	}
	sc, _ := resp["subcontractOrders"].([]interface{})
	if len(sc) != 1 {
		t.Errorf("subcontractOrders 应 1 条, got %d", len(sc))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestGetInTransitDetailMissingGoodsNo(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/in-transit-detail", nil)
	(&DashboardHandler{DB: db}).GetInTransitDetail(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("缺 goodsNo 应 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ---------- GetSyncYSProgress ----------

func TestGetSyncYSProgressNotRunning(t *testing.T) {
	// 默认全局 syncYSProgress 是 zero value (Running=false, Done=false)
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sync/ys/progress", nil)
	(&DashboardHandler{DB: db}).GetSyncYSProgress(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	if resp == nil {
		t.Fatal("response.data 不应为 nil")
	}
	// 默认 Running=false
	if running, _ := resp["running"].(bool); running {
		t.Errorf("默认 running 应 false (全局 zero value)")
	}
}
