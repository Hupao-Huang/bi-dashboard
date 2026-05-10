package handler

// dashboard_overview_test.go — GetOverview happy path sqlmock 测试
// 已 Read dashboard_overview.go 全文 (415 行), 列出 9 个 SQL 调用 + 1 个 QueryRow.
//
// SQL 调用顺序 (按源码 line):
//   1. line  25: 各部门汇总 (GROUP BY dept)
//   2. line  88: 每日趋势 (GROUP BY stat_date, dept)
//   3. line 123: 商品 TOP15 (GROUP BY goods_no, goods_name, brand_name, cate_name, grade)
//   4. line 178: 商品渠道分布 (条件: len(topGoods)>0)
//   5. line 207: 店铺 TOP15 (GROUP BY shop_name, department)
//   6. line 272: shopBreakdown 货品 (条件: len(topShops)>0, ROW_NUMBER PARTITION BY shop_name)
//   7. line 303: shopBreakdown 分类 (同条件)
//   8. line 339: 产品定位 (GROUP BY g.goods_field7)
//   9. line 372: 产品定位×部门 (GROUP BY g.goods_field7, s.department)
//   10. line 412: db.QueryRow MIN/MAX stat_date

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

var errBoom = errors.New("boom")

func keysOf(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// happy path: 让 topGoods 和 topShops 都返空, 跳过条件 SQL (4/6/7), 只 mock 7 个 SQL
func TestGetOverviewHappyPathReturnsAllSections(t *testing.T) {
	ClearOverviewCache() // 防其他 test 的 cache 干扰

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 1. dept 汇总 (line 25)
	mock.ExpectQuery(`GROUP BY dept`).WillReturnRows(
		sqlmock.NewRows([]string{"dept", "sales", "qty", "profit", "cost", "sku_count"}).
			AddRow("ecommerce", 1000.0, 100.0, 200.0, 800.0, 5).
			AddRow("social", 500.0, 50.0, 100.0, 400.0, 3),
	)

	// 2. trend (line 88)
	mock.ExpectQuery(`GROUP BY stat_date, dept`).WillReturnRows(
		sqlmock.NewRows([]string{"d", "dept", "sales", "qty"}).
			AddRow("2026-05-01", "ecommerce", 200.0, 20.0),
	)

	// 3. topGoods (line 123) — 返空跳过 line 178 渠道明细
	mock.ExpectQuery(`GROUP BY s\.goods_no, s\.goods_name, s\.brand_name, s\.cate_name, g\.goods_field7`).
		WillReturnRows(sqlmock.NewRows([]string{"goods_no", "goods_name", "brand_name", "cate_name", "grade", "sales", "qty", "profit"}))

	// 4. topShops (line 207) — 返空跳过 shopBreakdown 2 个 SQL
	mock.ExpectQuery(`GROUP BY shop_name, department`).
		WillReturnRows(sqlmock.NewRows([]string{"shop_name", "department", "sales", "qty"}))

	// 5. grades (line 339)
	mock.ExpectQuery(`GROUP BY g\.goods_field7\s+ORDER BY FIELD\(g\.goods_field7,'S','A','B','C','D'\), sales DESC`).
		WillReturnRows(sqlmock.NewRows([]string{"grade", "sales"}).
			AddRow("S", 800.0).AddRow("A", 500.0))

	// 6. gradeDeptSales (line 372)
	mock.ExpectQuery(`GROUP BY g\.goods_field7, s\.department`).
		WillReturnRows(sqlmock.NewRows([]string{"grade", "department", "sales", "profit"}).
			AddRow("S", "ecommerce", 800.0, 200.0))

	// 7. MIN/MAX stat_date (line 412)
	mock.ExpectQuery(`SELECT IFNULL\(MIN\(stat_date\),''\), IFNULL\(MAX\(stat_date\),''\) FROM sales_goods_summary`).
		WillReturnRows(sqlmock.NewRows([]string{"min", "max"}).AddRow("2025-01-01", "2026-05-09"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/overview?start=2026-05-01&end=2026-05-09", nil)

	(&DashboardHandler{DB: db}).GetOverview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	// BI 看板标准 envelope: {code, data}, 实际数据在 data 里
	resp, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("response 应为 {code, data} envelope, 实际 keys=%v", keysOf(envelope))
	}

	// 验证 data 结构: 必须含 10 个 key
	expectedKeys := []string{
		"departments", "trend", "topGoods", "goodsChannels",
		"topShops", "grades", "gradeDeptSales", "shopBreakdown",
		"dateRange", "trendRange",
	}
	for _, k := range expectedKeys {
		if _, ok := resp[k]; !ok {
			t.Errorf("response.data 缺字段 %s", k)
		}
	}

	// 验证: allDepts 补 0 后 5 部门都返
	depts, _ := resp["departments"].([]interface{})
	if len(depts) < 5 {
		t.Errorf("departments 应至少 5 主部门 (allDepts补0), got %d", len(depts))
	}

	// 验证 dateRange 含 min/max
	dr, _ := resp["dateRange"].(map[string]interface{})
	if dr["min"] != "2025-01-01" || dr["max"] != "2026-05-09" {
		t.Errorf("dateRange min/max 不一致: %v", dr)
	}

	// shopBreakdown 应是 map (空但存在)
	if _, ok := resp["shopBreakdown"].(map[string]interface{}); !ok {
		t.Error("shopBreakdown 应为 map 类型")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

// 边缘: dept 汇总返空时, allDepts 仍补 5 个 0 行
func TestGetOverviewWithNoDataStillReturns5Depts(t *testing.T) {
	ClearOverviewCache()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 全部 SQL 返空
	emptyDept := sqlmock.NewRows([]string{"dept", "sales", "qty", "profit", "cost", "sku_count"})
	emptyTrend := sqlmock.NewRows([]string{"d", "dept", "sales", "qty"})
	emptyGoods := sqlmock.NewRows([]string{"goods_no", "goods_name", "brand_name", "cate_name", "grade", "sales", "qty", "profit"})
	emptyShops := sqlmock.NewRows([]string{"shop_name", "department", "sales", "qty"})
	emptyGrades := sqlmock.NewRows([]string{"grade", "sales"})
	emptyGradeDept := sqlmock.NewRows([]string{"grade", "department", "sales", "profit"})
	emptyDateRange := sqlmock.NewRows([]string{"min", "max"}).AddRow("", "")

	mock.ExpectQuery(`GROUP BY dept`).WillReturnRows(emptyDept)
	mock.ExpectQuery(`GROUP BY stat_date, dept`).WillReturnRows(emptyTrend)
	mock.ExpectQuery(`GROUP BY s\.goods_no, s\.goods_name`).WillReturnRows(emptyGoods)
	mock.ExpectQuery(`GROUP BY shop_name, department`).WillReturnRows(emptyShops)
	mock.ExpectQuery(`GROUP BY g\.goods_field7\s+ORDER BY`).WillReturnRows(emptyGrades)
	mock.ExpectQuery(`GROUP BY g\.goods_field7, s\.department`).WillReturnRows(emptyGradeDept)
	mock.ExpectQuery(`SELECT IFNULL\(MIN\(stat_date\)`).WillReturnRows(emptyDateRange)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/overview?start=2026-05-01&end=2026-05-09", nil)
	(&DashboardHandler{DB: db}).GetOverview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("无数据应仍返 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var envelope map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &envelope)
	resp, _ := envelope["data"].(map[string]interface{})
	depts, _ := resp["departments"].([]interface{})
	if len(depts) != 5 {
		t.Errorf("无数据时 allDepts 应补 5 个 0 行 (ecommerce/social/offline/distribution/instant_retail), got %d", len(depts))
	}
}

// GetDepartmentDetail error path: 第一个 SQL boom 应返 500
func TestGetDepartmentDetailDatabaseErrorReturns500(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM sales_goods_summary`).WillReturnError(errBoom)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/department?dept=ecommerce&start=2026-05-01&end=2026-05-09", nil)
	(&DashboardHandler{DB: db}).GetDepartmentDetail(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("DB error 应返 500, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// 验证 cache hit: 第二次同样 query 应直接从 cache 返回, 不调 SQL
func TestGetOverviewCacheHitSkipsSQL(t *testing.T) {
	ClearOverviewCache()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 只 mock 一遍, 第二次调用应走 cache
	mock.ExpectQuery(`GROUP BY dept`).WillReturnRows(sqlmock.NewRows([]string{"dept", "sales", "qty", "profit", "cost", "sku_count"}))
	mock.ExpectQuery(`GROUP BY stat_date, dept`).WillReturnRows(sqlmock.NewRows([]string{"d", "dept", "sales", "qty"}))
	mock.ExpectQuery(`GROUP BY s\.goods_no`).WillReturnRows(sqlmock.NewRows([]string{"goods_no", "goods_name", "brand_name", "cate_name", "grade", "sales", "qty", "profit"}))
	mock.ExpectQuery(`GROUP BY shop_name, department`).WillReturnRows(sqlmock.NewRows([]string{"shop_name", "department", "sales", "qty"}))
	mock.ExpectQuery(`GROUP BY g\.goods_field7\s+ORDER BY`).WillReturnRows(sqlmock.NewRows([]string{"grade", "sales"}))
	mock.ExpectQuery(`GROUP BY g\.goods_field7, s\.department`).WillReturnRows(sqlmock.NewRows([]string{"grade", "department", "sales", "profit"}))
	mock.ExpectQuery(`MIN\(stat_date\)`).WillReturnRows(sqlmock.NewRows([]string{"min", "max"}).AddRow("", ""))

	// 第 1 次调用 — 实打实 SQL
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/api/overview?start=2026-05-01&end=2026-05-09", nil)
	(&DashboardHandler{DB: db}).GetOverview(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("第一次应 200, got %d", rec1.Code)
	}

	// 第 2 次调用 — 应该走 cache (没新 mock)
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/overview?start=2026-05-01&end=2026-05-09", nil)
	(&DashboardHandler{DB: db}).GetOverview(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("第二次应 cache hit 200, got %d", rec2.Code)
	}

	// mock 期望应全部满足 (只 mock 了 7 次, 第二次不调 SQL)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("第二次应走 cache 不调 SQL: %v", err)
	}
}
