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
	// KPI（按发布日期 note_create_time 聚合，跨所有快照日加总）
	mock.ExpectQuery(`SELECT COUNT\(DISTINCT note_id\), IFNULL\(SUM\(read_count\),0\)`).
		WillReturnRows(sqlmock.NewRows([]string{"notes", "reads", "interact", "gmv", "orders", "payuv", "clickuv"}).
			AddRow(200, 126819, 50000, 28124.0, 1032, 3000, 50000))
	// detail（按笔记聚合 15 列：属性 + 金额/次数 SUM + 率类加权重算；第一列 note_id 供下钻）
	mock.ExpectQuery(`SELECT note_id,\s+ANY_VALUE\(note_title\)`).
		WillReturnRows(sqlmock.NewRows([]string{"noteid", "title", "url", "author", "ctime", "type", "prod", "pay", "clickpv", "clickrate", "payconv", "refund", "cart", "toshoppay", "finish"}).
			AddRow("6a2156de", "标题A", "https://www.xiaohongshu.com/explore/abc?xsec_token=t", "糙能农场", "2026-06-10 18:43:42", "图文", "山药面", 2213.9, 733, 0.092808, 0.110505, 12.5, 8, 100.0, 0.0))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/xiaohongshu/note", nil)
	(&DashboardHandler{DB: db}).GetXhsNote(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetXhsNoteTrendHappy(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	// 单条笔记按数据更新日的每天走势
	mock.ExpectQuery(`SELECT DATE_FORMAT\(stat_date,'%Y-%m-%d'\),\s+IFNULL\(SUM\(read_count\),0\), IFNULL\(SUM\(pay_amount\),0\), IFNULL\(SUM\(pay_order_count\),0\)\s+FROM op_xhs_note_daily WHERE note_id=`).
		WillReturnRows(sqlmock.NewRows([]string{"d", "reads", "gmv", "orders"}).
			AddRow("2026-06-18", 1200, 300.0, 5).AddRow("2026-06-19", 800, 150.0, 3))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/xiaohongshu/note-trend?note_id=abc&start=2026-06-01&end=2026-06-21", nil)
	(&DashboardHandler{DB: db}).GetXhsNoteTrend(rec, req)
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
	mock.ExpectQuery(`SELECT COUNT\(DISTINCT product_id\), IFNULL\(SUM\(visitor_count\),0\)`).
		WillReturnRows(sqlmock.NewRows([]string{"g", "v", "pay", "ord", "qty", "ref"}).
			AddRow(131, 5000, 20000.0, 300, 350, 1000.0))
	mock.ExpectQuery(`SELECT DATE_FORMAT\(stat_date,'%Y-%m-%d'\),\s+IFNULL\(SUM\(pay_amount\),0\), IFNULL\(SUM\(visitor_count\),0\)\s+FROM op_xhs_goods_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"d", "pay", "v"}).AddRow("2026-06-21", 20000.0, 5000))
	mock.ExpectQuery(`SELECT ANY_VALUE\(product_name\), ANY_VALUE\(category_l1\), ANY_VALUE\(category_l2\)`).
		WillReturnRows(sqlmock.NewRows([]string{"name", "c1", "c2", "v", "view", "cart", "pay", "ord", "qty", "conv", "aov", "ref"}).
			AddRow("菌菇汤底", "粮油", "调味", 25, 43, 10, 126.4, 6, 6, 0.24, 21.07, 0.0))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/xiaohongshu/goods", nil)
	(&DashboardHandler{DB: db}).GetXhsGoods(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rec.Code, rec.Body.String())
	}
}
