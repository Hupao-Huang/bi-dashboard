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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
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

// =================== v1.74.3 电商部 KPI 调拨合并 单测 ===================

func TestLoadEcommerceAllotAdjustment_HappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	// query 1: 2 渠道销售单口径 (sales + qty)
	mock.ExpectQuery(regexp.QuoteMeta("FROM sales_goods_summary")).
		WithArgs("2026-04-01", "2026-04-30", "1819610592561398400", "1819610591915475584").
		WillReturnRows(sqlmock.NewRows([]string{"sales", "qty"}).AddRow(5130000.0, 800.0))

	// query 2: 2 渠道调拨口径 (amt + sku_count)
	mock.ExpectQuery(regexp.QuoteMeta("FROM allocate_orders o")).
		WithArgs("2026-04-01", "2026-04-30").
		WillReturnRows(sqlmock.NewRows([]string{"amt", "sku_count"}).AddRow(7320000.0, 1200.0))

	h := &DashboardHandler{DB: db}
	salesExcluded, allotAmt, qtyExcluded, allotQty, err := h.loadEcommerceAllotAdjustment(
		context.Background(), "2026-04-01", "2026-04-30", "", nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if salesExcluded != 5130000.0 {
		t.Errorf("salesExcluded=%v, want 5130000", salesExcluded)
	}
	if allotAmt != 7320000.0 {
		t.Errorf("allotAmt=%v, want 7320000", allotAmt)
	}
	if qtyExcluded != 800.0 {
		t.Errorf("qtyExcluded=%v, want 800", qtyExcluded)
	}
	if allotQty != 1200.0 {
		t.Errorf("allotQty=%v, want 1200", allotQty)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestLoadEcommerceAllotAdjustment_Query1Error(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("FROM sales_goods_summary")).
		WillReturnError(errors.New("connection lost"))

	h := &DashboardHandler{DB: db}
	_, _, _, _, err := h.loadEcommerceAllotAdjustment(
		context.Background(), "2026-04-01", "2026-04-30", "", nil)
	if err == nil {
		t.Fatal("expected err for query 1 failure, got nil")
	}
}

func TestLoadEcommerceAllotAdjustment_Query2Error(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	// query 1 成功
	mock.ExpectQuery(regexp.QuoteMeta("FROM sales_goods_summary")).
		WillReturnRows(sqlmock.NewRows([]string{"sales", "qty"}).AddRow(0.0, 0.0))
	// query 2 失败
	mock.ExpectQuery(regexp.QuoteMeta("FROM allocate_orders o")).
		WillReturnError(errors.New("table missing"))

	h := &DashboardHandler{DB: db}
	_, _, _, _, err := h.loadEcommerceAllotAdjustment(
		context.Background(), "2026-04-01", "2026-04-30", "", nil)
	if err == nil {
		t.Fatal("expected err for query 2 failure, got nil")
	}
}

func TestApplyEcommerceAllotAdjustment_HappyPath(t *testing.T) {
	deptList := []DeptSummary{
		{Department: "ecommerce", Sales: 7094000.0, Qty: 5000},
		{Department: "social", Sales: 500000.0, Qty: 100},
	}
	// salesExcluded=5130000, allotAmt=7320000, qtyExcluded=800, allotQty=1200
	applyEcommerceAllotAdjustment(deptList, 5130000.0, 7320000.0, 800.0, 1200.0)

	// ecommerce dept sales/amt 应该被修改
	wantSalesAmt := 7094000.0 - 5130000.0
	if deptList[0].SalesAmt != wantSalesAmt {
		t.Errorf("ecommerce.SalesAmt = %v, want %v", deptList[0].SalesAmt, wantSalesAmt)
	}
	if deptList[0].AllotAmt != 7320000.0 {
		t.Errorf("ecommerce.AllotAmt = %v, want 7320000", deptList[0].AllotAmt)
	}
	wantSales := wantSalesAmt + 7320000.0
	if deptList[0].Sales != wantSales {
		t.Errorf("ecommerce.Sales = %v, want %v (新总和)", deptList[0].Sales, wantSales)
	}

	// ecommerce dept qty 同步处理 = 5000 - 800 + 1200 = 5400
	wantQty := 5000.0 - 800.0 + 1200.0
	if deptList[0].Qty != wantQty {
		t.Errorf("ecommerce.Qty = %v, want %v", deptList[0].Qty, wantQty)
	}

	// social dept 不应该被修改
	if deptList[1].Sales != 500000.0 {
		t.Errorf("social.Sales 不应该变, 实际 %v", deptList[1].Sales)
	}
	if deptList[1].Qty != 100.0 {
		t.Errorf("social.Qty 不应该变, 实际 %v", deptList[1].Qty)
	}
	if deptList[1].SalesAmt != 0 || deptList[1].AllotAmt != 0 {
		t.Errorf("social 不应该有 SalesAmt/AllotAmt 设置, 实际 SalesAmt=%v AllotAmt=%v",
			deptList[1].SalesAmt, deptList[1].AllotAmt)
	}
}

func TestApplyEcommerceAllotAdjustment_NegativeSalesClampedToZero(t *testing.T) {
	// 极端边界: 销售单口径 > 实际 sales (理论不应发生, 防御性测试)
	deptList := []DeptSummary{
		{Department: "ecommerce", Sales: 1000000.0, Qty: 500},
	}
	// salesExcluded > Sales + qtyExcluded > Qty
	applyEcommerceAllotAdjustment(deptList, 5000000.0, 7000000.0, 2000.0, 1500.0)

	if deptList[0].SalesAmt != 0 {
		t.Errorf("SalesAmt 应该被钳到 0, 实际 %v", deptList[0].SalesAmt)
	}
	if deptList[0].Sales != 7000000.0 {
		t.Errorf("Sales = 0 + AllotAmt = %v, want 7000000", deptList[0].Sales)
	}
	// Qty = 500 - 2000 + 1500 = 0 (钳到 0 因为内部 newQty = -1500 + 1500 = 0, 实际计算 = 500-2000+1500 = 0)
	if deptList[0].Qty != 0 {
		t.Errorf("Qty 应该是 0 (500 - 2000 + 1500), 实际 %v", deptList[0].Qty)
	}
}

func TestApplyEcommerceAllotAdjustment_NoEcommerceDept(t *testing.T) {
	// 边界: deptList 不含 ecommerce
	deptList := []DeptSummary{
		{Department: "social", Sales: 500000.0, Qty: 100},
	}
	applyEcommerceAllotAdjustment(deptList, 100.0, 200.0, 10.0, 20.0)

	if deptList[0].Sales != 500000.0 {
		t.Errorf("无 ecommerce 时不应修改其它部门, 实际 social.Sales=%v", deptList[0].Sales)
	}
	if deptList[0].Qty != 100.0 {
		t.Errorf("无 ecommerce 时不应修改其它部门 qty, 实际 social.Qty=%v", deptList[0].Qty)
	}
}

func TestApplyEcommerceDailyAllot_HappyPath(t *testing.T) {
	// v1.74.3 UX 更新: 调拨单独 AllotSales 字段, 不 merge 到 Sales
	trend := []TrendPoint{
		{Date: "2026-05-01", Department: "ecommerce", Sales: 100000.0, Qty: 100},
		{Date: "2026-05-01", Department: "social", Sales: 50000.0, Qty: 50},
		{Date: "2026-05-02", Department: "ecommerce", Sales: 200000.0, Qty: 200},
		{Date: "2026-05-02", Department: "social", Sales: 80000.0, Qty: 80},
		{Date: "2026-05-03", Department: "ecommerce", Sales: 150000.0, Qty: 150},
		{Date: "2026-05-03", Department: "social", Sales: 60000.0, Qty: 60},
	}
	dailyAllot := map[string]ecomDailyAllot{
		"2026-05-01": {salesExcluded: 30000.0, allotAmt: 50000.0, qtyExcluded: 30, allotQty: 40},
		"2026-05-02": {salesExcluded: 0, allotAmt: 80000.0, qtyExcluded: 0, allotQty: 60},
	}
	trend = applyEcommerceDailyAllot(trend, dailyAllot)

	// 5/1 ecommerce: sales 排除 30000 = 70000; allotSales = 50000; 总高 = 120000
	if trend[0].Sales != 70000.0 {
		t.Errorf("5/1 ecommerce.Sales = %v, want 70000 (排除销售单)", trend[0].Sales)
	}
	if trend[0].AllotSales != 50000.0 {
		t.Errorf("5/1 ecommerce.AllotSales = %v, want 50000 (调拨)", trend[0].AllotSales)
	}
	if trend[0].Qty != 70.0 {
		t.Errorf("5/1 ecommerce.Qty = %v, want 70 (100-30)", trend[0].Qty)
	}
	if trend[0].AllotQty != 40.0 {
		t.Errorf("5/1 ecommerce.AllotQty = %v, want 40", trend[0].AllotQty)
	}
	// 5/1 social 不应该变
	if trend[1].Sales != 50000.0 || trend[1].AllotSales != 0 {
		t.Errorf("5/1 social.Sales=%v AllotSales=%v 不应变", trend[1].Sales, trend[1].AllotSales)
	}
	// 5/2 ecommerce: sales 200000-0=200000; allotSales=80000; 总高 280000
	if trend[2].Sales != 200000.0 || trend[2].AllotSales != 80000.0 {
		t.Errorf("5/2 ecommerce.Sales=%v AllotSales=%v, want 200000 / 80000", trend[2].Sales, trend[2].AllotSales)
	}
	// 5/3 ecommerce 无 dailyAllot, 保持原数字 (AllotSales=0)
	if trend[4].Sales != 150000.0 || trend[4].AllotSales != 0 {
		t.Errorf("5/3 ecommerce 无 dailyAllot 应保持, Sales=%v AllotSales=%v", trend[4].Sales, trend[4].AllotSales)
	}
}

func TestApplyEcommerceDailyAllot_NegativeClamped(t *testing.T) {
	// 边界: salesExcluded > sales (理论不应发生)
	trend := []TrendPoint{
		{Date: "2026-05-01", Department: "ecommerce", Sales: 10000.0, Qty: 10},
	}
	dailyAllot := map[string]ecomDailyAllot{
		"2026-05-01": {salesExcluded: 50000.0, allotAmt: 5000.0, qtyExcluded: 100, allotQty: 5},
	}
	trend = applyEcommerceDailyAllot(trend, dailyAllot)

	// sales = 10000 - 50000 = -40000 → 钳 0; allotSales = 5000 单独
	if trend[0].Sales != 0 {
		t.Errorf("Sales 应钳 0, 实际 %v", trend[0].Sales)
	}
	if trend[0].AllotSales != 5000.0 {
		t.Errorf("AllotSales 应是 5000, 实际 %v", trend[0].AllotSales)
	}
	// qty = 10 - 100 = -90 → 钳 0; allotQty = 5 单独
	if trend[0].Qty != 0 {
		t.Errorf("Qty 应钳 0, 实际 %v", trend[0].Qty)
	}
	if trend[0].AllotQty != 5.0 {
		t.Errorf("AllotQty 应是 5, 实际 %v", trend[0].AllotQty)
	}
}

func TestApplyEcommerceDailyAllot_AddsAllotOnlyDay(t *testing.T) {
	trend := []TrendPoint{
		{Date: "2026-05-02", Department: "social", Sales: 1000.0, Qty: 10},
		{Date: "2026-05-03", Department: "ecommerce", Sales: 2000.0, Qty: 20},
	}
	dailyAllot := map[string]ecomDailyAllot{
		"2026-05-01": {allotAmt: 500.0, allotQty: 5},
		"2026-05-03": {allotAmt: 800.0, allotQty: 8},
	}

	trend = applyEcommerceDailyAllot(trend, dailyAllot)

	var added *TrendPoint
	for i := range trend {
		if trend[i].Date == "2026-05-01" && trend[i].Department == "ecommerce" {
			added = &trend[i]
			break
		}
	}
	if added == nil {
		t.Fatalf("应补 2026-05-01 ecommerce 纯调拨点, trend=%v", trend)
	}
	if added.Sales != 0 || added.Qty != 0 || added.AllotSales != 500.0 || added.AllotQty != 5 {
		t.Errorf("补点口径不对: %+v", *added)
	}
	if trend[0].Date != "2026-05-01" {
		t.Errorf("补点后应按日期重排, 第一个日期=%s", trend[0].Date)
	}
}

func TestApplyDeptEcommerceDailyAllot_AddsAllotOnlyDay(t *testing.T) {
	daily := []deptDailyData{
		{Date: "2026-05-02", Sales: 1000.0, Qty: 10},
		{Date: "2026-05-03", Sales: 2000.0, Qty: 20},
	}
	dailyAllot := map[string]ecomDailyAllot{
		"2026-05-01": {allotAmt: 500.0, allotQty: 5},
		"2026-05-03": {allotAmt: 800.0, allotQty: 8},
	}

	daily = applyDeptEcommerceDailyAllot(daily, dailyAllot)

	if len(daily) != 3 {
		t.Fatalf("daily 应补到 3 行, got %d: %+v", len(daily), daily)
	}
	if daily[0].Date != "2026-05-01" || daily[0].Sales != 500.0 || daily[0].Qty != 5 {
		t.Errorf("补点不对: %+v", daily[0])
	}
	if daily[2].Date != "2026-05-03" || daily[2].Sales != 2800.0 || daily[2].Qty != 28 {
		t.Errorf("已有日期应加调拨: %+v", daily[2])
	}
}

func TestApplyEcommerceShopAllot_InTop(t *testing.T) {
	// 2 调拨 shop 都在 TOP 内
	topShops := []ShopRank{
		{ShopName: "ds-京东-清心湖自营", Department: "ecommerce", Sales: 2980000.0, Qty: 100},
		{ShopName: "ds-天猫超市-寄售", Department: "ecommerce", Sales: 2150000.0, Qty: 80},
		{ShopName: "ds-某其它店", Department: "ecommerce", Sales: 5000000.0, Qty: 200},
	}
	shopAllot := map[string]shopAllotData{
		"ds-京东-清心湖自营": {salesExcluded: 2980000.0, allotAmt: 5210000.0, qtyExcluded: 100, allotQty: 150},
		"ds-天猫超市-寄售":  {salesExcluded: 2150000.0, allotAmt: 2110000.0, qtyExcluded: 80, allotQty: 70},
	}
	result := applyEcommerceShopAllot(topShops, shopAllot, 15)

	// 排序后顺序: 某其它店 ¥500 万 > 京东 ¥521 万? No - 京东更大. 让我重算.
	// 京东: 2980000 - 2980000 + 5210000 = 5210000
	// 猫超: 2150000 - 2150000 + 2110000 = 2110000
	// 其它: 5000000 不变
	// 排序: 京东 (¥521) > 其它 (¥500) > 猫超 (¥211)
	if len(result) != 3 {
		t.Fatalf("len = %d, want 3", len(result))
	}
	if result[0].ShopName != "ds-京东-清心湖自营" || result[0].Sales != 5210000.0 {
		t.Errorf("排第 1 应为京东 ¥521, 实际 %s ¥%v", result[0].ShopName, result[0].Sales)
	}
	if result[1].ShopName != "ds-某其它店" {
		t.Errorf("排第 2 应为其它店, 实际 %s", result[1].ShopName)
	}
	if result[2].ShopName != "ds-天猫超市-寄售" || result[2].Sales != 2110000.0 {
		t.Errorf("排第 3 应为猫超 ¥211, 实际 %s ¥%v", result[2].ShopName, result[2].Sales)
	}
}

func TestApplyEcommerceShopAllot_NotInTop(t *testing.T) {
	// 2 调拨 shop 不在 TOP (销售单口径数据小排在外面), 但调拨数据让它进 TOP
	topShops := []ShopRank{
		{ShopName: "ds-某其它店", Department: "ecommerce", Sales: 1000000.0, Qty: 50},
	}
	shopAllot := map[string]shopAllotData{
		"ds-京东-清心湖自营": {salesExcluded: 0, allotAmt: 5210000.0, qtyExcluded: 0, allotQty: 150},
		"ds-天猫超市-寄售":  {salesExcluded: 0, allotAmt: 2110000.0, qtyExcluded: 0, allotQty: 70},
	}
	result := applyEcommerceShopAllot(topShops, shopAllot, 15)

	// 加进 2 个 entry + 排序: 京东 (¥521) > 猫超 (¥211) > 其它 (¥100)
	if len(result) != 3 {
		t.Fatalf("len = %d, want 3 (加 2 个调拨 shop)", len(result))
	}
	if result[0].ShopName != "ds-京东-清心湖自营" {
		t.Errorf("排第 1 应为京东, 实际 %s", result[0].ShopName)
	}
}

func TestApplyEcommerceShopAllot_NoData(t *testing.T) {
	// 边界: shopAllot 全是 0 (无销售单无调拨)
	topShops := []ShopRank{
		{ShopName: "ds-某其它店", Department: "ecommerce", Sales: 1000000.0, Qty: 50},
	}
	shopAllot := map[string]shopAllotData{
		"ds-京东-清心湖自营": {salesExcluded: 0, allotAmt: 0, qtyExcluded: 0, allotQty: 0},
		"ds-天猫超市-寄售":  {salesExcluded: 0, allotAmt: 0, qtyExcluded: 0, allotQty: 0},
	}
	result := applyEcommerceShopAllot(topShops, shopAllot, 15)

	// 不应该加 entry
	if len(result) != 1 {
		t.Errorf("len = %d, want 1 (无数据时不加 entry)", len(result))
	}
}

// 抹去 json package 未用 lint warning (依赖现有 happypath 测试使用)
var _ = json.Marshal
