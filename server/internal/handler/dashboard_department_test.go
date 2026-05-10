package handler

// dashboard_department_test.go — GetDepartmentDetail happy path sqlmock
// 已 Read dashboard_department.go 全文 (722 行), GetDepartmentDetail line 28-721.
// 已 Read dashboard_helpers.go line 52-91 (getDateRange/getTrendDateRange) — 传 start+end 不调 DB.
// 已 Read scope.go line 59-118 (buildSalesDataScopeCond) — no auth payload = pass-through.
//
// dept=ecommerce + start/end + 主路径 = 10 个 SQL (按顺序):
//   1. line  91: trend daily (FROM sales_goods_summary GROUP BY stat_date)
//   2. line 149: shopList non-offline (GROUP BY shop_name ORDER BY sales DESC)
//   3. line 187: totalShopCount QueryRow (SELECT COUNT(DISTINCT shop_name))
//   4. line 194: goods TOP15 (LIMIT 15, 返空跳过 3.5 商品渠道分布)
//   5. line 311: brand TOP10 (LIMIT 10)
//   6. line 345: grades (GROUP BY g.goods_field7 ORDER BY FIELD)
//   7. line 376: gradePlat ecommerce/social/instant_retail (sales_channel JOIN, GROUP BY grade,plat)
//   8. line 476: plat list raw (SELECT DISTINCT sc.online_plat_name)
//   9. line 500: instant count QueryRow (WHERE shop_name LIKE '%即时零售%')
//  10. line 549: platSales LEFT JOIN sales_channel GROUP BY plat ORDER BY SUM
//
// crossDept=1 多 2 SQL (line 651/676): gradeDeptSalesAll + gradeShopSalesAll = 12 SQL

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ecommerce dept happy path: 10 SQL mock
func TestGetDepartmentDetailEcommerceHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 1. trend (line 91)
	mock.ExpectQuery(`FROM sales_goods_summary\s+WHERE department = \? AND stat_date BETWEEN`).
		WillReturnRows(sqlmock.NewRows([]string{"d", "sales", "qty", "profit", "cost"}).
			AddRow("2026-04-01", 1000.0, 50.0, 200.0, 800.0).
			AddRow("2026-04-02", 1500.0, 80.0, 300.0, 1200.0))

	// 2. shopList (line 149)
	mock.ExpectQuery(`GROUP BY shop_name ORDER BY sales DESC`).
		WillReturnRows(sqlmock.NewRows([]string{"shop_name", "sales", "qty", "profit"}).
			AddRow("天猫旗舰店", 2000.0, 100.0, 400.0).
			AddRow("京东自营", 1500.0, 80.0, 300.0))

	// 3. totalShopCount QueryRow (line 187)
	mock.ExpectQuery(`SELECT COUNT\(DISTINCT shop_name\) FROM sales_goods_summary`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(15))

	// 4. goods TOP15 (line 194) — 返空跳过 3.5
	mock.ExpectQuery(`ORDER BY sales DESC LIMIT 15`).
		WillReturnRows(sqlmock.NewRows([]string{"goods_no", "goods_name", "brand", "cate_name", "sales", "qty", "profit", "grade"}))

	// 5. brand TOP10 (line 311)
	mock.ExpectQuery(`IFNULL\(brand_name,'未知'\) as brand`).
		WillReturnRows(sqlmock.NewRows([]string{"brand", "sales"}).AddRow("品牌A", 1000.0))

	// 6. grades (line 345) — 跟 gradePlat 区分: \s+ROUND 紧贴 as grade
	mock.ExpectQuery(`IFNULL\(g\.goods_field7,'未设置'\) as grade,\s+ROUND\(SUM\(s\.local_goods_amt\)`).
		WillReturnRows(sqlmock.NewRows([]string{"grade", "sales"}).
			AddRow("S", 800.0).AddRow("A", 500.0))

	// 7. gradePlat ecommerce (line 376) — sales_channel JOIN, GROUP BY grade,plat
	mock.ExpectQuery(`GROUP BY grade, plat\s+ORDER BY FIELD\(grade,'S','A','B','C','D'\)`).
		WillReturnRows(sqlmock.NewRows([]string{"grade", "plat", "sales"}).
			AddRow("S", "天猫商城", 600.0).
			AddRow("A", "京东", 400.0))

	// 8. plat list (line 476)
	mock.ExpectQuery(`SELECT DISTINCT sc\.online_plat_name`).
		WillReturnRows(sqlmock.NewRows([]string{"online_plat_name"}).
			AddRow("天猫商城").AddRow("京东"))

	// 9. instant count QueryRow (line 500)
	mock.ExpectQuery(`WHERE shop_name LIKE '%即时零售%' AND stat_date BETWEEN`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0))

	// 10. platSales (line 549)
	mock.ExpectQuery(`GROUP BY plat\s+ORDER BY SUM\(s\.local_goods_amt\) DESC`).
		WillReturnRows(sqlmock.NewRows([]string{"plat", "sales", "qty"}).
			AddRow("天猫商城", 1500.0, 80.0).
			AddRow("京东", 1000.0, 50.0))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/department?dept=ecommerce&start=2026-04-01&end=2026-04-30", nil)
	(&DashboardHandler{DB: db}).GetDepartmentDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	// {code, data} envelope (CLAUDE.md)
	resp, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("response 应为 {code, data} envelope, 实际 keys=%v", keysOf(envelope))
	}

	// 必须含的 15 个 key
	expectedKeys := []string{
		"daily", "shops", "shopTotalCount", "goods", "goodsChannels",
		"brands", "grades", "platforms", "platformSales", "gradePlatSales",
		"gradeDeptSalesAll", "gradeShopSalesAll", "regionTargets",
		"dateRange", "trendRange",
	}
	for _, k := range expectedKeys {
		if _, ok := resp[k]; !ok {
			t.Errorf("response.data 缺字段 %s", k)
		}
	}

	// shopTotalCount = 15 (mock)
	if cnt, _ := resp["shopTotalCount"].(float64); cnt != 15 {
		t.Errorf("shopTotalCount 应为 15, got %v", resp["shopTotalCount"])
	}

	// daily 2 行
	daily, _ := resp["daily"].([]interface{})
	if len(daily) != 2 {
		t.Errorf("daily 应 2 行, got %d", len(daily))
	}

	// shops 2 行
	shops, _ := resp["shops"].([]interface{})
	if len(shops) != 2 {
		t.Errorf("shops 应 2 行, got %d", len(shops))
	}

	// platformSales 应按 sales 降序 (line 601 sort.Slice)
	platSales, _ := resp["platformSales"].([]interface{})
	if len(platSales) >= 2 {
		first, _ := platSales[0].(map[string]interface{})
		second, _ := platSales[1].(map[string]interface{})
		s1, _ := first["sales"].(float64)
		s2, _ := second["sales"].(float64)
		if s1 < s2 {
			t.Errorf("platformSales 应按 sales 降序, got %v < %v", s1, s2)
		}
	}

	// gradePlatSales 必返 (ecommerce 走 line 376, mock 给 2 行)
	gps, _ := resp["gradePlatSales"].([]interface{})
	if len(gps) == 0 {
		t.Error("ecommerce dept gradePlatSales 应非空")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

// 缺 dept 参数 → 400
func TestGetDepartmentDetailMissingDept(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/department?start=2026-04-01&end=2026-04-30", nil)
	(&DashboardHandler{DB: db}).GetDepartmentDetail(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("缺 dept 应返 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// crossDept=1: 复用 ecommerce 主路径 10 SQL + 多 2 SQL (gradeDeptSalesAll/gradeShopSalesAll) = 12 SQL
func TestGetDepartmentDetailCrossDeptAddsTwoSQL(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`GROUP BY stat_date ORDER BY stat_date`).
		WillReturnRows(sqlmock.NewRows([]string{"d", "sales", "qty", "profit", "cost"}))
	mock.ExpectQuery(`GROUP BY shop_name ORDER BY sales DESC`).
		WillReturnRows(sqlmock.NewRows([]string{"shop_name", "sales", "qty", "profit"}))
	mock.ExpectQuery(`SELECT COUNT\(DISTINCT shop_name\) FROM sales_goods_summary`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0))
	mock.ExpectQuery(`ORDER BY sales DESC LIMIT 15`).
		WillReturnRows(sqlmock.NewRows([]string{"goods_no", "goods_name", "brand", "cate_name", "sales", "qty", "profit", "grade"}))
	mock.ExpectQuery(`IFNULL\(brand_name,'未知'\) as brand`).
		WillReturnRows(sqlmock.NewRows([]string{"brand", "sales"}))
	mock.ExpectQuery(`IFNULL\(g\.goods_field7,'未设置'\) as grade,\s+ROUND\(SUM\(s\.local_goods_amt\)`).
		WillReturnRows(sqlmock.NewRows([]string{"grade", "sales"}))
	mock.ExpectQuery(`GROUP BY grade, plat\s+ORDER BY FIELD`).
		WillReturnRows(sqlmock.NewRows([]string{"grade", "plat", "sales"}))
	mock.ExpectQuery(`SELECT DISTINCT sc\.online_plat_name`).
		WillReturnRows(sqlmock.NewRows([]string{"online_plat_name"}))
	mock.ExpectQuery(`WHERE shop_name LIKE '%即时零售%'`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0))
	mock.ExpectQuery(`GROUP BY plat\s+ORDER BY SUM`).
		WillReturnRows(sqlmock.NewRows([]string{"plat", "sales", "qty"}))

	// crossDept=1 多 2 SQL
	mock.ExpectQuery(`GROUP BY grade, s\.department\s+ORDER BY FIELD`).
		WillReturnRows(sqlmock.NewRows([]string{"grade", "department", "sales", "profit"}).
			AddRow("S", "ecommerce", 1000.0, 200.0))

	mock.ExpectQuery(`GROUP BY grade, s\.department, s\.shop_name`).
		WillReturnRows(sqlmock.NewRows([]string{"grade", "department", "shop_name", "sales", "profit"}).
			AddRow("S", "ecommerce", "天猫", 800.0, 150.0))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/department?dept=ecommerce&start=2026-04-01&end=2026-04-30&crossDept=1", nil)
	(&DashboardHandler{DB: db}).GetDepartmentDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("crossDept=1 应返 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var envelope map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &envelope)
	resp, _ := envelope["data"].(map[string]interface{})
	gd, _ := resp["gradeDeptSalesAll"].([]interface{})
	if len(gd) != 1 {
		t.Errorf("gradeDeptSalesAll 应 1 行, got %d", len(gd))
	}
	gs, _ := resp["gradeShopSalesAll"].([]interface{})
	if len(gs) != 1 {
		t.Errorf("gradeShopSalesAll 应 1 行, got %d", len(gs))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql: %v", err)
	}
}

// distribution 走 line 435 else if 分支 (s.shop_name as channel)
func TestGetDepartmentDetailDistribution(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`GROUP BY stat_date ORDER BY stat_date`).
		WillReturnRows(sqlmock.NewRows([]string{"d", "sales", "qty", "profit", "cost"}))
	mock.ExpectQuery(`GROUP BY shop_name ORDER BY sales DESC`).
		WillReturnRows(sqlmock.NewRows([]string{"shop_name", "sales", "qty", "profit"}))
	mock.ExpectQuery(`SELECT COUNT\(DISTINCT shop_name\) FROM sales_goods_summary`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0))
	mock.ExpectQuery(`ORDER BY sales DESC LIMIT 15`).
		WillReturnRows(sqlmock.NewRows([]string{"goods_no", "goods_name", "brand", "cate_name", "sales", "qty", "profit", "grade"}))
	mock.ExpectQuery(`IFNULL\(brand_name,'未知'\) as brand`).
		WillReturnRows(sqlmock.NewRows([]string{"brand", "sales"}))
	mock.ExpectQuery(`IFNULL\(g\.goods_field7,'未设置'\) as grade,\s+ROUND\(SUM\(s\.local_goods_amt\)`).
		WillReturnRows(sqlmock.NewRows([]string{"grade", "sales"}))

	// distribution 走 line 448 (s.shop_name as channel) — 区别于 ecommerce 的 plat
	mock.ExpectQuery(`s\.shop_name as channel,\s+ROUND\(SUM\(s\.local_goods_amt\),2\) as sales`).
		WillReturnRows(sqlmock.NewRows([]string{"grade", "channel", "sales"}).
			AddRow("S", "经销商A", 500.0))

	mock.ExpectQuery(`SELECT DISTINCT sc\.online_plat_name`).
		WillReturnRows(sqlmock.NewRows([]string{"online_plat_name"}))
	mock.ExpectQuery(`WHERE shop_name LIKE '%即时零售%'`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0))
	mock.ExpectQuery(`GROUP BY plat\s+ORDER BY SUM`).
		WillReturnRows(sqlmock.NewRows([]string{"plat", "sales", "qty"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/department?dept=distribution&start=2026-04-01&end=2026-04-30", nil)
	(&DashboardHandler{DB: db}).GetDepartmentDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("distribution dept 应返 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var envelope map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &envelope)
	resp, _ := envelope["data"].(map[string]interface{})
	gps, _ := resp["gradePlatSales"].([]interface{})
	if len(gps) != 1 {
		t.Errorf("distribution gradePlatSales 应 1 行 (mock 给了), got %d", len(gps))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql: %v", err)
	}
}
