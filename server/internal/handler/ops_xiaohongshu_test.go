package handler

import (
	"database/sql/driver"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// xhsMockDetailRow 按 xhsCol 定义生成 sqlmock 明细行(列名+一行值), 列数随指标增减自适应
func xhsMockDetailRow(fixed []string, cols []xhsCol) (*sqlmock.Rows, []driver.Value) {
	names := append([]string{}, fixed...)
	vals := make([]driver.Value, 0, len(fixed)+len(cols))
	for range fixed {
		vals = append(vals, "x")
	}
	for _, c := range cols {
		names = append(names, c.Key)
		if c.Fmt == "text" {
			vals = append(vals, "x")
		} else {
			vals = append(vals, 0.0)
		}
	}
	return sqlmock.NewRows(names), vals
}

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
	// detail（数据驱动全字段：固定 note_id/标题/url + xhsNoteCols 各列）
	nRows, nVals := xhsMockDetailRow([]string{"noteid", "title", "url"}, xhsNoteCols)
	mock.ExpectQuery(`SELECT note_id,\s+ANY_VALUE\(note_title\)`).
		WillReturnRows(nRows.AddRow(nVals...))
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
	gRows, gVals := xhsMockDetailRow([]string{"name"}, xhsGoodsCols)
	mock.ExpectQuery(`SELECT ANY_VALUE\(product_name\)`).
		WillReturnRows(gRows.AddRow(gVals...))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/xiaohongshu/goods", nil)
	(&DashboardHandler{DB: db}).GetXhsGoods(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rec.Code, rec.Body.String())
	}
}
