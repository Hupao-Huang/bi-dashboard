package handler

// business_report_test.go — business_report.go GetBusinessReportChannelsList
//   - GetBusinessReportChannelsList: 2 SQL (snapshot 选最新 QueryRow + channel/sub_channel Query)

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

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

// splitCsv 已在 handler_pure_test.go 测过
