package handler

// business_report_test.go — business_report.go 5 个 GetBusiness* + parseSnapshotParam
// 已 Read business_report.go line 53-771:
//   - GetBusinessReportDetail (53): 2 SQL (line 65 主查 + line 141 queryDistinctSubChannels)
//   - GetBusinessReportOverview (169): 1 SQL (line 179)
//   - GetBusinessReportTrend (216): 1 SQL (line 228)
//   - GetBusinessReportChannelsList (700): 2 SQL (line 702 QueryRow + line 708 Query)
//   - parseSnapshotParam (267): 纯函数, 3 种错误分支

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ---------- GetBusinessReportDetail ----------

func TestGetBusinessReportDetailHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 1. 主查 (line 65): subject + level + ... + 13 columns, period_month=0/1..12
	mock.ExpectQuery(`FROM business_budget_report\s+WHERE snapshot_year=\? AND snapshot_month=\? AND channel=\? AND sub_channel=\?`).
		WillReturnRows(sqlmock.NewRows([]string{
			"subject", "subject_level", "subject_category", "parent_subject", "sort_order", "period_month",
			"budget_year_start", "ratio_year_start", "budget", "ratio_budget", "actual", "ratio_actual", "achievement_rate",
		}).
			// period_month=0 = 年度汇总
			AddRow("GMV合计", 1, "GMV", "", 10, 0, 12000000.0, 1.0, 1000000.0, nil, 950000.0, nil, 0.95).
			// period_month=1 = 1 月明细
			AddRow("GMV合计", 1, "GMV", "", 10, 1, nil, nil, 100000.0, nil, 95000.0, nil, nil))

	// 2. queryDistinctSubChannels (line 141)
	mock.ExpectQuery(`SELECT DISTINCT sub_channel FROM business_budget_report\s+WHERE snapshot_year=\? AND snapshot_month=\? AND channel=\?`).
		WillReturnRows(sqlmock.NewRows([]string{"sub_channel"}).
			AddRow("").AddRow("TOC"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/business/report/detail?snapshot=2026-04&channel=电商", nil)
	(&DashboardHandler{DB: db}).GetBusinessReportDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	if resp["channel"] != "电商" {
		t.Errorf("channel 应为电商, got %v", resp["channel"])
	}
	cells, _ := resp["cells"].([]interface{})
	if len(cells) != 1 {
		t.Errorf("cells 应 1 个 sortOrder=10, got %d", len(cells))
	}
	subs, _ := resp["subChannels"].([]interface{})
	if len(subs) != 2 {
		t.Errorf("subChannels 应 2 个, got %d", len(subs))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestGetBusinessReportDetailMissingChannel(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/business/report/detail?snapshot=2026-04", nil)
	(&DashboardHandler{DB: db}).GetBusinessReportDetail(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("缺 channel 应 400, got %d", rec.Code)
	}
}

// ---------- GetBusinessReportOverview ----------

func TestGetBusinessReportOverviewHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT channel, sub_channel, subject, budget, actual, achievement_rate\s+FROM business_budget_report\s+WHERE snapshot_year=\? AND snapshot_month=\? AND subject=\? AND period_month=0`).
		WillReturnRows(sqlmock.NewRows([]string{"channel", "sub_channel", "subject", "budget", "actual", "achievement_rate"}).
			AddRow("电商", "", "GMV合计", 1000000.0, 950000.0, 0.95).
			AddRow("私域", "", "GMV合计", 500000.0, 480000.0, 0.96))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/business/report/overview?snapshot=2026-04", nil)
	(&DashboardHandler{DB: db}).GetBusinessReportOverview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	// 默认 subject 是 GMV合计 (line 176)
	if resp["subject"] != "GMV合计" {
		t.Errorf("默认 subject 应 GMV合计, got %v", resp["subject"])
	}
	chs, _ := resp["channels"].([]interface{})
	if len(chs) != 2 {
		t.Errorf("channels 应 2 条, got %d", len(chs))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestGetBusinessReportOverviewExplicitSubject(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`AND subject=\? AND period_month=0`).
		WithArgs(2026, 4, "营业收入"). // 显式 subject
		WillReturnRows(sqlmock.NewRows([]string{"channel", "sub_channel", "subject", "budget", "actual", "achievement_rate"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/business/report/overview?snapshot=2026-04&subject=营业收入", nil)
	(&DashboardHandler{DB: db}).GetBusinessReportOverview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

// ---------- GetBusinessReportTrend ----------

func TestGetBusinessReportTrendHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT period_month, budget, actual\s+FROM business_budget_report\s+WHERE.*period_month BETWEEN 1 AND 12`).
		WillReturnRows(sqlmock.NewRows([]string{"period_month", "budget", "actual"}).
			AddRow(1, 100000.0, 95000.0).
			AddRow(2, 110000.0, 105000.0).
			AddRow(3, 120000.0, 118000.0))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/business/report/trend?snapshot=2026-04&channel=电商&subject=GMV合计", nil)
	(&DashboardHandler{DB: db}).GetBusinessReportTrend(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	pts, _ := resp["points"].([]interface{})
	if len(pts) != 3 {
		t.Errorf("points 应 3 条, got %d", len(pts))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestGetBusinessReportTrendMissingChannel(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/business/report/trend?snapshot=2026-04&subject=GMV合计", nil) // 缺 channel
	(&DashboardHandler{DB: db}).GetBusinessReportTrend(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("缺 channel 应 400, got %d", rec.Code)
	}
}

// ---------- GetBusinessReportChannelsList ----------

func TestGetBusinessReportChannelsListHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 1. snapshot 选最新 QueryRow (line 702)
	mock.ExpectQuery(`SELECT snapshot_year, snapshot_month FROM business_budget_report\s+ORDER BY snapshot_year DESC, snapshot_month DESC LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"y", "m"}).AddRow(2026, 4))

	// 2. channel/sub_channel/subj_count (line 708) — '经营指标' 排除
	mock.ExpectQuery(`SELECT channel, sub_channel, COUNT\(DISTINCT subject\) AS subj_count\s+FROM business_budget_report\s+WHERE snapshot_year=\? AND snapshot_month=\? AND channel != '经营指标'`).
		WillReturnRows(sqlmock.NewRows([]string{"channel", "sub_channel", "subj_count"}).
			AddRow("电商", "", 30).
			AddRow("电商", "TOC", 25).
			AddRow("私域", "", 30))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/business/report/channels", nil)
	(&DashboardHandler{DB: db}).GetBusinessReportChannelsList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	if resp["snapshotYear"] != float64(2026) {
		t.Errorf("snapshotYear 应 2026, got %v", resp["snapshotYear"])
	}
	groups, _ := resp["groups"].([]interface{})
	// 电商 group 含 2 items, 私域 group 含 1 item → 共 2 个 group
	if len(groups) != 2 {
		t.Errorf("groups 应 2 个 (电商/私域), got %d", len(groups))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

// ---------- parseSnapshotParam ----------

func TestParseSnapshotParamErrors(t *testing.T) {
	cases := []struct {
		name   string
		query  string
		wantOk bool
	}{
		{"empty", "", false},
		{"bad-format-no-dash", "202604", false},
		{"bad-year-too-low", "2010-04", false},
		{"bad-year-too-high", "2099-04", false},
		{"bad-month-zero", "2026-0", false},
		{"bad-month-13", "2026-13", false},
		{"non-numeric", "abc-04", false},
		{"valid-2020-1", "2020-1", true},
		{"valid-2026-4", "2026-04", true},
		{"valid-2050-12", "2050-12", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/?snapshot="+tc.query, nil)
			_, _, ok := parseSnapshotParam(rec, req)
			if ok != tc.wantOk {
				t.Errorf("snapshot=%q ok=%v want %v", tc.query, ok, tc.wantOk)
			}
		})
	}
}

// zeroPad / formatSnapshotLabel / splitCsv 已在 handler_pure_test.go 测过
