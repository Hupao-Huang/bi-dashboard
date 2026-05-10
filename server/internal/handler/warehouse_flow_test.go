package handler

// warehouse_flow_test.go — GetWarehouseFlowOverview/Matrix sqlmock
// 已 Read warehouse_flow.go line 119-130 (canUseSummary), line 203-371 (Overview), line 376-432 (Matrix).
//
// GetWarehouseFlowOverview 主路径 (无 SKU 过滤, 物化 cnt > 0):
//   1. canUseSummary line 127: SELECT COUNT(*) FROM warehouse_flow_summary
//   2. KPI line 289 (物化): SUM(orders) ... FROM warehouse_flow_summary
//   3. prov line 294 (物化): GROUP BY province
//   4. wh line 309 (物化): GROUP BY warehouse_name
//   5. shopSQL line 324: SELECT DISTINCT t.shop_name FROM trade_YYYYMM
//   6. ymList line 339: information_schema.TABLES
//
// 注意: 2-6 是 goroutine 并发 → 必须 MatchExpectationsInOrder(false)
//
// GetWarehouseFlowMatrix 主路径 (无 SKU 过滤, 物化 cnt > 0):
//   1. canUseSummary
//   2. matrix raw line 406: warehouse_flow_summary GROUP BY warehouse,province

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetWarehouseFlowOverviewSummaryPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.MatchExpectationsInOrder(false) // 5 SQL 并发, 顺序无关

	// 1. canUseSummary: 物化已建 (cnt > 0)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM warehouse_flow_summary WHERE ym = \? LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(100))

	// 2. KPI (物化路径)
	mock.ExpectQuery(`SELECT SUM\(orders\), SUM\(packages\),\s+COUNT\(DISTINCT province\), COUNT\(DISTINCT warehouse_name\)\s+FROM warehouse_flow_summary`).
		WillReturnRows(sqlmock.NewRows([]string{"orders", "packages", "prov_cnt", "wh_cnt"}).
			AddRow(int64(5000), int64(5500), 30, 7))

	// 3. provinces (物化)
	mock.ExpectQuery(`SELECT province, SUM\(orders\), SUM\(packages\)\s+FROM warehouse_flow_summary`).
		WillReturnRows(sqlmock.NewRows([]string{"province", "orders", "packages"}).
			AddRow("广东省", int64(2000), int64(2200)).
			AddRow("浙江省", int64(1500), int64(1600)))

	// 4. warehouses (物化)
	mock.ExpectQuery(`SELECT warehouse_name, SUM\(orders\), SUM\(packages\)\s+FROM warehouse_flow_summary`).
		WillReturnRows(sqlmock.NewRows([]string{"warehouse_name", "orders", "packages"}).
			AddRow("华东仓", int64(3000), int64(3300)).
			AddRow("华南仓", int64(2000), int64(2200)))

	// 5. shopSQL (trade_YYYYMM 7 仓白名单内)
	mock.ExpectQuery(`SELECT DISTINCT t\.shop_name FROM trade_202604 t`).
		WillReturnRows(sqlmock.NewRows([]string{"shop_name"}).
			AddRow("天猫旗舰店").AddRow("京东自营"))

	// 6. ymList (information_schema)
	mock.ExpectQuery(`FROM information_schema\.TABLES\s+WHERE TABLE_SCHEMA=DATABASE\(\) AND TABLE_NAME REGEXP '\^trade_\[0-9\]\{6\}\$'`).
		WillReturnRows(sqlmock.NewRows([]string{"name"}).
			AddRow("trade_202604").AddRow("trade_202603").AddRow("trade_202602"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/warehouse-flow/overview?ym=2026-04", nil)
	(&DashboardHandler{DB: db}).GetWarehouseFlowOverview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	if resp["ym"] != "2026-04" {
		t.Errorf("ym 应 2026-04, got %v", resp["ym"])
	}
	kpi, _ := resp["kpi"].(map[string]interface{})
	if kpi["orders"] != float64(5000) {
		t.Errorf("kpi.orders 应 5000, got %v", kpi["orders"])
	}
	provs, _ := resp["provinces"].([]interface{})
	if len(provs) != 2 {
		t.Errorf("provinces 应 2 行, got %d", len(provs))
	}
	whs, _ := resp["warehouses"].([]interface{})
	if len(whs) != 2 {
		t.Errorf("warehouses 应 2 行, got %d", len(whs))
	}
	ymList, _ := resp["ymList"].([]interface{})
	if len(ymList) != 3 {
		t.Errorf("ymList 应 3 个 (mock 给 3 张表), got %d", len(ymList))
	}
}

// 错 ym 格式 → 400
func TestGetWarehouseFlowOverviewBadYM(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/warehouse-flow/overview?ym=invalid", nil)
	(&DashboardHandler{DB: db}).GetWarehouseFlowOverview(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid ym 应 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// Matrix happy path
func TestGetWarehouseFlowMatrixSummaryPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM warehouse_flow_summary WHERE ym = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(100))

	mock.ExpectQuery(`SELECT warehouse_name, province, SUM\(orders\), SUM\(packages\)\s+FROM warehouse_flow_summary`).
		WillReturnRows(sqlmock.NewRows([]string{"wh", "prov", "orders", "packages"}).
			AddRow("华东仓", "广东省", int64(800), int64(880)).
			AddRow("华东仓", "浙江省", int64(500), int64(550)).
			AddRow("华南仓", "广东省", int64(1000), int64(1100)))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/warehouse-flow/matrix?ym=2026-04", nil)
	(&DashboardHandler{DB: db}).GetWarehouseFlowMatrix(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	cells, _ := resp["cells"].([]interface{})
	if len(cells) != 3 {
		t.Errorf("cells 应 3 个 (wh,prov 组合), got %d", len(cells))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

// Matrix bad ym
func TestGetWarehouseFlowMatrixBadYM(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/warehouse-flow/matrix?ym=bad", nil)
	(&DashboardHandler{DB: db}).GetWarehouseFlowMatrix(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid ym 应 400, got %d", rec.Code)
	}
}

// resolveYM ym 默认值 (空 ym → information_schema 取最新) — 单元测试 helper
func TestResolveYMEmptyUsesLatestTable(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT TABLE_NAME FROM information_schema\.TABLES`).
		WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("trade_202604"))

	got, err := resolveYM(db, "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "202604" {
		t.Errorf("resolveYM 空 ym 应取最新表后缀, got %q", got)
	}
}

func TestResolveYMValidFormat(t *testing.T) {
	got, err := resolveYM(nil, "2026-04")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "202604" {
		t.Errorf("resolveYM('2026-04')=%q want '202604'", got)
	}
}

func TestResolveYMInvalidFormat(t *testing.T) {
	cases := []string{"2026-13", "abcd-04", "26-04"}
	for _, s := range cases {
		_, err := resolveYM(nil, s)
		if err == nil {
			t.Errorf("resolveYM(%q) 应返 err", s)
		}
	}
}
