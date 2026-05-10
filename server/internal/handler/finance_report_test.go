package handler

// finance_report_test.go — finance_report.go 5 个 GetFinance* 主路径 sqlmock 测试
// 已 Read finance_report.go 全文 (1133 行):
//   - GetFinanceReport (113-388): 3 SQL
//       1. line 169: skeleton (子查询 SELECT MAX(year) FROM finance_report)
//       2. line 221: Level 1 分组行 (finance_subject_dict)
//       3. line 254: 实际数据 (finance_report year/month range)
//   - GetFinanceReportTrend (389-439): 1 SQL
//   - GetFinanceReportCompare (441-488): 1 SQL (month=0 走年累计 / month>0 走月度)
//   - GetFinanceReportStructure (490-580): 4 SQL (cost/salesExp/mgmtExp/waterfall)
//   - GetFinanceSubjects (582-605): 1 SQL
// 已 Read line 50-73: FinCell{Amount, Ratio*}, FinSeries{RangeTotal, Cells}, FinReportRow{...Total FinSeries}

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ---------- GetFinanceSubjects ----------

func TestGetFinanceSubjectsHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT subject_code, subject_name, subject_category, subject_level, parent_code, display_order FROM finance_subject_dict ORDER BY display_order`).
		WillReturnRows(sqlmock.NewRows([]string{"code", "name", "category", "level", "parent", "order"}).
			AddRow("GMV_TOTAL", "GMV 合计", "GMV", 2, "", 10).
			AddRow("REV_MAIN", "营业收入", "财务", 2, "", 20))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/finance/subjects", nil)
	(&DashboardHandler{DB: db}).GetFinanceSubjects(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	subs, _ := resp["subjects"].([]interface{})
	if len(subs) != 2 {
		t.Errorf("subjects 应 2 条, got %d", len(subs))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

// ---------- GetFinanceReportTrend ----------

func TestGetFinanceReportTrendHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT year, month, department, subject_code, subject_name, SUM\(amount\) amount FROM finance_report`).
		WillReturnRows(sqlmock.NewRows([]string{"year", "month", "dept", "code", "name", "amount"}).
			AddRow(2026, 1, "ecommerce", "GMV_TOTAL", "GMV 合计", 100000.0).
			AddRow(2026, 2, "ecommerce", "GMV_TOTAL", "GMV 合计", 120000.0))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/finance/report/trend?yearStart=2026&yearEnd=2026&subjects=GMV_TOTAL&channels=ecommerce", nil)
	(&DashboardHandler{DB: db}).GetFinanceReportTrend(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	pts, _ := resp["points"].([]interface{})
	if len(pts) != 2 {
		t.Errorf("points 应 2 条, got %d", len(pts))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestGetFinanceReportTrendMissingArgs(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/finance/report/trend?yearStart=2026", nil) // 缺 subjects/channels
	(&DashboardHandler{DB: db}).GetFinanceReportTrend(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("缺 subjects/channels 应 400, got %d", rec.Code)
	}
}

// ---------- GetFinanceReportCompare ----------

func TestGetFinanceReportCompareYearTotal(t *testing.T) {
	// month=0 → 走年累计分支 (line 457)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT department, subject_code, subject_name, SUM\(amount\) FROM finance_report WHERE year=\? AND month BETWEEN 1 AND 12`).
		WillReturnRows(sqlmock.NewRows([]string{"dept", "code", "name", "amount"}).
			AddRow("ecommerce", "GMV_TOTAL", "GMV 合计", 1000000.0).
			AddRow("social", "GMV_TOTAL", "GMV 合计", 500000.0))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/finance/report/compare?year=2026", nil)
	(&DashboardHandler{DB: db}).GetFinanceReportCompare(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	data, _ := resp["data"].(map[string]interface{})
	if data["ecommerce"] == nil || data["social"] == nil {
		t.Errorf("data 应有 ecommerce + social, got keys=%v", keysOf(data))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestGetFinanceReportCompareSpecificMonth(t *testing.T) {
	// month=5 → 走月度分支 (line 463)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT department, subject_code, subject_name, amount FROM finance_report WHERE year=\? AND month=\?`).
		WillReturnRows(sqlmock.NewRows([]string{"dept", "code", "name", "amount"}).
			AddRow("ecommerce", "REV_MAIN", "营业收入", 800000.0))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/finance/report/compare?year=2026&month=5", nil)
	(&DashboardHandler{DB: db}).GetFinanceReportCompare(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestGetFinanceReportCompareMissingYear(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/finance/report/compare", nil)
	(&DashboardHandler{DB: db}).GetFinanceReportCompare(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("缺 year 应 400, got %d", rec.Code)
	}
}

// ---------- GetFinanceReportStructure ----------

func TestGetFinanceReportStructureHappyPath(t *testing.T) {
	// 4 SQL: cost / salesExp / mgmtExp / waterfall
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 同一份 SQL 模板, 用 parent_code 区分 — sqlmock 按顺序消费
	costRows := sqlmock.NewRows([]string{"code", "name", "category", "amount"}).
		AddRow("COST_MAIN.成本", "主营成本", "成本", 600000.0)
	salesRows := sqlmock.NewRows([]string{"code", "name", "category", "amount"}).
		AddRow("SALES_EXP.广告", "广告费", "销售费用", 80000.0)
	mgmtRows := sqlmock.NewRows([]string{"code", "name", "category", "amount"}).
		AddRow("MGMT_EXP.工资", "管理工资", "管理费用", 50000.0)
	waterRows := sqlmock.NewRows([]string{"code", "name", "amount"}).
		AddRow("GMV_TOTAL", "GMV 合计", 1500000.0).
		AddRow("REV_MAIN", "营业收入", 1200000.0)

	// 4 个查询用相同 SQL 主体, 仅 parent_code 不同 → 用 subject_level=3 通用 fragment
	mock.ExpectQuery(`AND parent_code=\? AND subject_level=3`).WillReturnRows(costRows)
	mock.ExpectQuery(`AND parent_code=\? AND subject_level=3`).WillReturnRows(salesRows)
	mock.ExpectQuery(`AND parent_code=\? AND subject_level=3`).WillReturnRows(mgmtRows)
	mock.ExpectQuery(`AND subject_code IN`).WillReturnRows(waterRows)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/finance/report/structure?year=2026&month=5&department=ecommerce", nil)
	(&DashboardHandler{DB: db}).GetFinanceReportStructure(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	for _, k := range []string{"cost", "salesExp", "mgmtExp", "waterfall"} {
		if _, ok := resp[k]; !ok {
			t.Errorf("response.data 缺字段 %s", k)
		}
	}
	wf, _ := resp["waterfall"].([]interface{})
	if len(wf) != 2 {
		t.Errorf("waterfall 应 2 步 (mock 只给 GMV_TOTAL/REV_MAIN), got %d", len(wf))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestGetFinanceReportStructureMissingArgs(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/finance/report/structure?year=2026", nil) // 缺 department
	(&DashboardHandler{DB: db}).GetFinanceReportStructure(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("缺 department 应 400, got %d", rec.Code)
	}
}

// ---------- GetFinanceReport ----------

func TestGetFinanceReportHappyPath(t *testing.T) {
	// 3 SQL: skeleton + level1 + 实际数据
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 1. skeleton (line 164-168, 含子查询 SELECT MAX(year))
	mock.ExpectQuery(`SELECT DISTINCT subject_code, sub_channel FROM finance_report\s+WHERE year = \(SELECT MAX\(year\) FROM finance_report\)`).
		WillReturnRows(sqlmock.NewRows([]string{"code", "name", "category", "level", "parent", "sub_channel", "display_order"}).
			AddRow("REV_MAIN", "营业收入", "财务", 2, "", "", 20).
			AddRow("GMV_TOTAL", "GMV 合计", "GMV", 2, "", "", 10))

	// 2. Level 1 分组行 (line 221)
	mock.ExpectQuery(`FROM finance_subject_dict WHERE subject_level = 1 ORDER BY display_order`).
		WillReturnRows(sqlmock.NewRows([]string{"code", "name", "category", "level", "parent", "display_order"}).
			AddRow("GMV_GROUP", "GMV 数据", "GMV", 1, "", 5).
			AddRow("FIN_GROUP", "财务数据", "财务", 1, "", 15))

	// 3. 实际数据 (line 252-253)
	mock.ExpectQuery(`SELECT year, month, department, sub_channel, subject_code, amount\s+FROM finance_report WHERE year BETWEEN`).
		WillReturnRows(sqlmock.NewRows([]string{"year", "month", "dept", "sub_channel", "code", "amount"}).
			AddRow(2026, 1, "ecommerce", "", "REV_MAIN", 800000.0).
			AddRow(2026, 1, "ecommerce", "", "GMV_TOTAL", 1000000.0).
			AddRow(2026, 2, "ecommerce", "", "REV_MAIN", 850000.0))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/finance/report?yearStart=2026&yearEnd=2026&monthStart=1&monthEnd=2&channels=ecommerce", nil)
	(&DashboardHandler{DB: db}).GetFinanceReport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("json: %v", err)
	}
	resp, ok := env["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("应 {code,data} envelope, got keys=%v", keysOf(env))
	}
	rows, _ := resp["rows"].([]interface{})
	if len(rows) == 0 {
		t.Errorf("rows 应非空 (有 skeleton + level1)")
	}
	ymList, _ := resp["yearMonths"].([]interface{})
	if len(ymList) != 2 {
		t.Errorf("yearMonths 应 2 个 (2026-1, 2026-2), got %d", len(ymList))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestGetFinanceReportMissingYearStart(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/finance/report", nil) // 缺 yearStart
	(&DashboardHandler{DB: db}).GetFinanceReport(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("缺 yearStart 应 400, got %d", rec.Code)
	}
}

func TestGetFinanceReportMonthStartGreaterThanEnd(t *testing.T) {
	// monthStart=10 monthEnd=3 → 400 (line 152)
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/finance/report?yearStart=2026&monthStart=10&monthEnd=3", nil)
	(&DashboardHandler{DB: db}).GetFinanceReport(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("monthStart>monthEnd 应 400, got %d", rec.Code)
	}
}
