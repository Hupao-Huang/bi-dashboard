package handler

// douyin_test.go — GetDouyinOps + GetDouyinDistOps happy path
// 已 Read douyin.go: GetDouyinOps 7 SQL (live_daily/goods_daily/funnel/channel/ad_live x2),
//                     GetDouyinDistOps 4 SQL (dist_account x3 + dist_product x1)

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ GetDouyinOps (7 SQL) ============

func TestGetDouyinOpsHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.MatchExpectationsInOrder(false)

	// 1. live trend (line 32-37)
	mock.ExpectQuery(`COUNT\(DISTINCT anchor_id, start_time\), SUM\(watch_uv\), ROUND\(SUM\(pay_amount\),2\),\s+ROUND\(AVG\(avg_online\),0\)`).
		WillReturnRows(sqlmock.NewRows([]string{"date", "sess", "uv", "amt", "online", "rr"}))

	// 2. goods top (line 64)
	mock.ExpectQuery(`FROM op_douyin_goods_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"name", "amt", "qty"}))

	// 3. anchor top (line 92)
	mock.ExpectQuery(`FROM op_douyin_live_daily.*GROUP BY anchor_name`).
		WillReturnRows(sqlmock.NewRows([]string{"name", "sess", "uv", "amt"}))

	// 4. channel daily (line 118)
	mock.ExpectQuery(`FROM op_douyin_channel_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"date", "ch", "amt"}))

	// 5. funnel (line 140)
	mock.ExpectQuery(`SELECT step_name, step_value FROM op_douyin_funnel_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"step", "value"}))

	// 6. ad live trend (line 171)
	mock.ExpectQuery(`FROM op_douyin_ad_live_daily.*GROUP BY stat_date`).
		WillReturnRows(sqlmock.NewRows([]string{"d", "cost", "amt", "uv", "roi"}))

	// 7. ad live douyin_name detail (line 199, GROUP BY douyin_name)
	mock.ExpectQuery(`SELECT douyin_name, ROUND\(SUM\(cost\),2\).*FROM op_douyin_ad_live_daily.*GROUP BY douyin_name ORDER BY SUM\(cost\) DESC`).
		WillReturnRows(sqlmock.NewRows([]string{"dn", "cost", "amt", "roi", "net", "nroi", "imp", "clicks", "rr"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ops/douyin?start=2026-05-01&end=2026-05-31", nil)
	(&DashboardHandler{DB: db}).GetDouyinOps(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ============ GetDouyinDistOps (4 SQL) ============

func TestGetDouyinDistOpsHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.MatchExpectationsInOrder(false)

	// 1. plans list (line 264)
	mock.ExpectQuery(`SELECT account_name, ROUND\(SUM\(cost\),2\).*FROM op_douyin_dist_account_daily.*GROUP BY account_name`).
		WillReturnRows(sqlmock.NewRows([]string{"name", "cost", "amt", "talents"}).
			AddRow("投放计划A", 5000.0, 25000.0, 10))

	// 2. dist trend (line 290)
	mock.ExpectQuery(`FROM op_douyin_dist_account_daily.*GROUP BY stat_date`).
		WillReturnRows(sqlmock.NewRows([]string{"date", "cost", "amt", "roi"}).
			AddRow("2026-05-01", 1000.0, 5000.0, 5.0))

	// 3. account rank (line 320)
	mock.ExpectQuery(`SELECT douyin_name, account_name, ROUND\(SUM\(cost\),2\).*GROUP BY douyin_name`).
		WillReturnRows(sqlmock.NewRows([]string{"dn", "an", "cost", "amt", "roi", "net"}).
			AddRow("达人1", "投放A", 1000.0, 5000.0, 5.0, 4000.0))

	// 4. product rank (line 350)
	mock.ExpectQuery(`FROM op_douyin_dist_product_daily.*GROUP BY product_name`).
		WillReturnRows(sqlmock.NewRows([]string{"pn", "an", "cost", "amt", "roi", "clicks"}).
			AddRow("商品A", "投放A", 500.0, 2500.0, 5.0, 1000))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ops/douyin-dist?start=2026-05-01&end=2026-05-31", nil)
	(&DashboardHandler{DB: db}).GetDouyinDistOps(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// 测 account 过滤
func TestGetDouyinDistOpsWithAccountFilter(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.MatchExpectationsInOrder(false)

	// plans 不受 account filter 影响
	mock.ExpectQuery(`GROUP BY account_name ORDER BY SUM\(pay_amount\) DESC`).
		WillReturnRows(sqlmock.NewRows([]string{"name", "cost", "amt", "talents"}))
	// 后 3 SQL 加 AND account_name = ?
	mock.ExpectQuery(`AND account_name = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"date", "cost", "amt", "roi"}))
	mock.ExpectQuery(`AND account_name = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"dn", "an", "cost", "amt", "roi", "net"}))
	mock.ExpectQuery(`AND account_name = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"pn", "an", "cost", "amt", "roi", "clicks"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ops/douyin-dist?account=投放A&start=2026-05-01&end=2026-05-31", nil)
	(&DashboardHandler{DB: db}).GetDouyinDistOps(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d body=%s", rec.Code, rec.Body.String())
	}
}

// GetDouyinDistOps method not allowed
func TestGetDouyinDistOpsMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ops/douyin-dist", nil)
	(&DashboardHandler{DB: db}).GetDouyinDistOps(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST 应 405, got %d", rec.Code)
	}
}
