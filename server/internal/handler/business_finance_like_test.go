package handler

// business_finance_like_test.go — GetBusinessReportFinanceLike 主路径 5 SQL
// 已 Read business_report.go (line 356-700+):
//   1. snaps Query (line 409): SELECT snapshot_year, MAX(snapshot_month) GROUP BY snapshot_year
//   2. tplYear QueryRow (line 442): LIMIT 1
//   3. periodSet loop (line 453): 每个 snap 一次
//   4. data Query (line 504): 主数据 OR 条件
//   5. tpl skeleton (line 529): SELECT DISTINCT subject

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetBusinessReportFinanceLikeHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 1. snaps (line 409, GROUP BY snapshot_year)
	mock.ExpectQuery(`SELECT snapshot_year, MAX\(snapshot_month\) AS sm\s+FROM business_budget_report`).
		WillReturnRows(sqlmock.NewRows([]string{"year", "sm"}).
			AddRow(2025, 12).
			AddRow(2026, 4))

	// 2. tplYear/Month QueryRow (line 442, ORDER BY DESC LIMIT 1)
	mock.ExpectQuery(`FROM business_budget_report\s+ORDER BY snapshot_year DESC, snapshot_month DESC LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"y", "m"}).AddRow(2026, 4))

	// 3. periodSetMap loop — 每个 snap 一次, 共 2 次
	for i := 0; i < 2; i++ {
		mock.ExpectQuery(`SELECT DISTINCT period_month FROM business_budget_report`).
			WillReturnRows(sqlmock.NewRows([]string{"pm"}).AddRow(1).AddRow(2).AddRow(3))
	}

	// 4. main data Query (line 496)
	mock.ExpectQuery(`SELECT snapshot_year, channel, sub_channel, parent_subject, subject,\s+subject_level, subject_category, sort_order, period_month`).
		WillReturnRows(sqlmock.NewRows([]string{"sy", "ch", "sc", "parent", "subject", "lvl", "cat", "sort", "pm", "budget", "actual", "ratio", "ar", "bys"}).
			AddRow(2026, "总", "", "", "GMV合计", 1, "GMV", 10, 0, 1000000.0, 950000.0, nil, 0.95, 12000000.0))

	// 5. tpl skeleton (line 524)
	mock.ExpectQuery(`SELECT DISTINCT subject, subject_level, parent_subject, subject_category, sort_order\s+FROM business_budget_report\s+WHERE snapshot_year=\? AND snapshot_month=\? AND channel='总'`).
		WillReturnRows(sqlmock.NewRows([]string{"subject", "level", "parent", "cat", "sort"}).
			AddRow("GMV合计", 1, "", "GMV", 10).
			AddRow("营业收入", 1, "", "财务", 20))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/finance/business-report?yearStart=2025&yearEnd=2026&monthStart=1&monthEnd=12&channels=总|", nil)
	(&DashboardHandler{DB: db}).GetBusinessReportFinanceLike(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// 多 channel 测试: 复用 happy path 逻辑, 验证 channels 解析
func TestGetBusinessReportFinanceLikeMultipleChannels(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// snap 返 1 条
	mock.ExpectQuery(`FROM business_budget_report\s+WHERE snapshot_year BETWEEN`).
		WillReturnRows(sqlmock.NewRows([]string{"y", "sm"}).AddRow(2026, 4))

	mock.ExpectQuery(`ORDER BY snapshot_year DESC, snapshot_month DESC LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"y", "m"}).AddRow(2026, 4))

	// 1 个 snap → 1 次 period
	mock.ExpectQuery(`SELECT DISTINCT period_month`).
		WillReturnRows(sqlmock.NewRows([]string{"pm"}))

	mock.ExpectQuery(`SELECT snapshot_year, channel, sub_channel, parent_subject, subject`).
		WillReturnRows(sqlmock.NewRows([]string{"sy", "ch", "sc", "parent", "subject", "lvl", "cat", "sort", "pm", "budget", "actual", "ratio", "ar", "bys"}))

	mock.ExpectQuery(`SELECT DISTINCT subject, subject_level, parent_subject, subject_category, sort_order`).
		WillReturnRows(sqlmock.NewRows([]string{"subject", "level", "parent", "cat", "sort"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/finance/business-report?yearStart=2026&yearEnd=2026&channels=电商|TOC,私域|", nil)
	(&DashboardHandler{DB: db}).GetBusinessReportFinanceLike(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}
