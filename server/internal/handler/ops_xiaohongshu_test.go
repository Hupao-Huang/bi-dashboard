package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetXhsFiltersHappy(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT IFNULL\(DATE_FORMAT\(MAX\(stat_date\),'%Y-%m-%d'\),''\) FROM op_xhs_note_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"d"}).AddRow("2026-06-21"))
	mock.ExpectQuery(`SELECT DISTINCT shop_name FROM op_xhs_note_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"s"}).AddRow("糙能农场旗舰店").AddRow("松鲜鲜安心店铺旗舰店"))
	mock.ExpectQuery(`SELECT DISTINCT note_type FROM op_xhs_note_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"t"}).AddRow("图文").AddRow("视频"))
	mock.ExpectQuery(`SELECT DISTINCT category_l1 FROM op_xhs_goods_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow("粮油调味/速食/干货/烘焙"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/xiaohongshu/filters", nil)
	(&DashboardHandler{DB: db}).GetXhsFilters(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetXhsNoteHappy(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	// resolveXhsDate -> MAX
	mock.ExpectQuery(`SELECT IFNULL\(DATE_FORMAT\(MAX\(stat_date\),'%Y-%m-%d'\),''\) FROM op_xhs_note_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"d"}).AddRow("2026-06-21"))
	// KPI
	mock.ExpectQuery(`SELECT COUNT\(\*\), IFNULL\(SUM\(read_count\),0\)`).
		WillReturnRows(sqlmock.NewRows([]string{"notes", "reads", "interact", "gmv", "orders", "payuv", "clickuv"}).
			AddRow(1868, 7787423, 50000, 100000.0, 1500, 3000, 50000))
	// trend
	mock.ExpectQuery(`SELECT DATE_FORMAT\(stat_date,'%Y-%m-%d'\),\s+IFNULL\(SUM\(read_count\),0\), IFNULL\(SUM\(pay_amount\),0\)\s+FROM op_xhs_note_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"d", "reads", "gmv"}).
			AddRow("2026-06-20", 100000, 5000.0).AddRow("2026-06-21", 120000, 6000.0))
	// detail
	mock.ExpectQuery(`SELECT note_title, note_type, author_name`).
		WillReturnRows(sqlmock.NewRows([]string{"title", "type", "author", "read", "like", "collect", "comment", "share", "gmv", "conv", "prod", "url"}).
			AddRow("标题A", "图文", "糙能农场", 7760, 43, 15, 7, 3, 2213.9, 0.078, "山药面", "http://x"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/xiaohongshu/note", nil)
	(&DashboardHandler{DB: db}).GetXhsNote(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetXhsGoodsHappy(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT IFNULL\(DATE_FORMAT\(MAX\(stat_date\),'%Y-%m-%d'\),''\) FROM op_xhs_goods_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"d"}).AddRow("2026-06-21"))
	mock.ExpectQuery(`SELECT COUNT\(\*\), IFNULL\(SUM\(visitor_count\),0\)`).
		WillReturnRows(sqlmock.NewRows([]string{"g", "v", "pay", "ord", "qty", "ref"}).
			AddRow(131, 5000, 20000.0, 300, 350, 1000.0))
	mock.ExpectQuery(`SELECT DATE_FORMAT\(stat_date,'%Y-%m-%d'\),\s+IFNULL\(SUM\(pay_amount\),0\), IFNULL\(SUM\(visitor_count\),0\)\s+FROM op_xhs_goods_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"d", "pay", "v"}).AddRow("2026-06-21", 20000.0, 5000))
	mock.ExpectQuery(`SELECT product_name, category_l1, category_l2`).
		WillReturnRows(sqlmock.NewRows([]string{"name", "c1", "c2", "v", "view", "cart", "pay", "ord", "qty", "conv", "aov", "ref"}).
			AddRow("菌菇汤底", "粮油", "调味", 25, 43, 10, 126.4, 6, 6, 0.24, 21.07, 0.0))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/xiaohongshu/goods", nil)
	(&DashboardHandler{DB: db}).GetXhsGoods(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rec.Code, rec.Body.String())
	}
}
