package handler

// misc_handlers_test.go — audit.go + channel.go + special_channel.go handler sqlmock
// 已 Read 各 handler 函数源码.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// ========== audit.go ==========

// AuditLogPageView happy path (含 logAudit goroutine)
func TestAuditLogPageViewHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// goroutine INSERT, 用 MatchExpectationsInOrder(false) + 短 sleep 保证 goroutine 跑完
	mock.MatchExpectationsInOrder(false)
	mock.ExpectExec(`INSERT INTO audit_logs`).WillReturnResult(sqlmock.NewResult(1, 1))

	body := []byte(`{"path":"/dashboard/overview"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/audit/page-view", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).AuditLogPageView(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	// 等 goroutine 跑完
	time.Sleep(50 * time.Millisecond)
}

func TestAuditLogPageViewMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit/page-view", nil)
	(&DashboardHandler{DB: db}).AuditLogPageView(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec.Code)
	}
}

func TestAuditLogPageViewMissingPath(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/audit/page-view", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).AuditLogPageView(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("缺 path 应 400, got %d", rec.Code)
	}
}

func TestAdminAuditLogsHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 1. count
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM audit_logs`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(125))

	// 2. list
	mock.ExpectQuery(`SELECT id, IFNULL\(user_id,0\), username, real_name, action, resource`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "uid", "username", "real_name", "action", "resource", "detail", "ip", "ua", "created_at"}).
			AddRow(100, 1, "admin", "管理员", "page_view", "/dashboard", "", "127.0.0.1", "Mozilla", "2026-05-10 10:00:00").
			AddRow(99, 1, "admin", "管理员", "login", "user", "", "127.0.0.1", "Mozilla", "2026-05-10 09:55:00"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit-logs?page=1&pageSize=50", nil)
	(&DashboardHandler{DB: db}).AdminAuditLogs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	if resp["total"] != float64(125) {
		t.Errorf("total 应 125, got %v", resp["total"])
	}
	logs, _ := resp["list"].([]interface{})
	if len(logs) != 2 {
		t.Errorf("list 应 2 条, got %d", len(logs))
	}
}

func TestAdminAuditLogsWithFilters(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// action + username + startDate filters → SQL 应含 WHERE conditions
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM audit_logs WHERE action = \? AND username LIKE`).
		WithArgs("login", "%admin%", "2026-05-01 00:00:00").
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(5))

	mock.ExpectQuery(`FROM audit_logs WHERE action = \? AND username LIKE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "uid", "username", "real_name", "action", "resource", "detail", "ip", "ua", "created_at"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit-logs?action=login&username=admin&startDate=2026-05-01", nil)
	(&DashboardHandler{DB: db}).AdminAuditLogs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminAuditLogsMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/audit-logs", nil)
	(&DashboardHandler{DB: db}).AdminAuditLogs(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST 应 405, got %d", rec.Code)
	}
}

// ========== channel.go ==========

func TestAdminChannelsHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 1. 主查
	mock.ExpectQuery(`FROM sales_channel WHERE 1=1`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "channel_id", "channel_name", "channel_code", "online_plat_name", "cate_name", "channel_type_name", "depart_name", "company_name", "responsible", "department"}).
			AddRow(1, "CH001", "天猫旗舰店", "001", "天猫商城", "电商", "直营网店", "电商部", "总部", "张三", "ecommerce"))

	// 2. total
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM sales_channel$`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(120))

	// 3. unmappedCount
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM sales_channel WHERE department IS NULL OR department = ''`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(10))

	// 4. plat list
	mock.ExpectQuery(`SELECT DISTINCT online_plat_name FROM sales_channel`).
		WillReturnRows(sqlmock.NewRows([]string{"plat"}).AddRow("天猫商城").AddRow("京东"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/channels", nil)
	(&DashboardHandler{DB: db}).AdminChannels(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	if resp["total"] != float64(120) {
		t.Errorf("total 应 120, got %v", resp["total"])
	}
	if resp["unmappedCount"] != float64(10) {
		t.Errorf("unmappedCount 应 10, got %v", resp["unmappedCount"])
	}
}

func TestAdminChannelsKeywordFilter(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// keyword=天猫 应转 LIKE %天猫%
	mock.ExpectQuery(`channel_name LIKE \? OR channel_code LIKE \? OR responsible_user LIKE \?`).
		WithArgs("%天猫%", "%天猫%", "%天猫%").
		WillReturnRows(sqlmock.NewRows([]string{"id", "channel_id", "channel_name", "channel_code", "online_plat_name", "cate_name", "channel_type_name", "depart_name", "company_name", "responsible", "department"}))

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM sales_channel`).WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0))
	mock.ExpectQuery(`SELECT COUNT\(\*\)`).WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0))
	mock.ExpectQuery(`SELECT DISTINCT`).WillReturnRows(sqlmock.NewRows([]string{"plat"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/channels?keyword=天猫", nil)
	(&DashboardHandler{DB: db}).AdminChannels(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestChannelByPathBadID(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/channels/abc", nil)
	(&DashboardHandler{DB: db}).ChannelByPath(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("非数字 id 应 400, got %d", rec.Code)
	}
}

func TestChannelByPathMissingID(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/channels/", nil)
	(&DashboardHandler{DB: db}).ChannelByPath(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("缺 id 应 400, got %d", rec.Code)
	}
}

func TestChannelByPathMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/channels/1", nil)
	(&DashboardHandler{DB: db}).ChannelByPath(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec.Code)
	}
}

// 4 个 dept 边界 - v1.46.2 加 other/excluded
func TestUpdateChannelDepartmentInvalidDept(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"department":"invalid_dept"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/channels/1", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).UpdateChannelDepartment(rec, req, 1)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("无效 dept 应 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateChannelDepartmentBadJSON(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/channels/1", bytes.NewReader([]byte(`not json`)))
	(&DashboardHandler{DB: db}).UpdateChannelDepartment(rec, req, 1)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("无效 JSON 应 400, got %d", rec.Code)
	}
}

func TestUpdateChannelDepartmentNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sales_channel SET department = \? WHERE id = \?`).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 affected → not found
	mock.ExpectRollback()

	body := []byte(`{"department":"ecommerce"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/channels/999", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).UpdateChannelDepartment(rec, req, 999)

	if rec.Code != http.StatusNotFound {
		t.Errorf("affected=0 应 404, got %d", rec.Code)
	}
}

// SyncChannels: 不能真跑 .exe, 只测 405
func TestSyncChannelsMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/channels/sync", nil)
	(&DashboardHandler{DB: db}).SyncChannels(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec.Code)
	}
}

// ========== special_channel.go ==========

func TestGetSpecialChannelAllotSummaryHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM allocate_orders o\s+LEFT JOIN allocate_details d`).
		WillReturnRows(sqlmock.NewRows([]string{"channel_key", "in_status", "orders", "sales"}).
			AddRow("京东", 3, 50, 100000.0).      // 完成
			AddRow("京东", 1, 20, 40000.0).        // 待入库
			AddRow("猫超", 3, 30, 60000.0).
			AddRow("朴朴", 3, 10, 20000.0))

	// 2) 调拨单 list (源码 line 128)
	mock.ExpectQuery(`FROM allocate_orders o\s+WHERE DATE\(o\.gmt_modified\) BETWEEN`).
		WillReturnRows(sqlmock.NewRows([]string{"allocate_no", "channel_key", "in_wh", "in_status", "status", "gc", "gm", "stat_date", "sku_count", "excel", "api"}).
			AddRow("ALL001", "京东", "京东仓", 3, 1, "2026-04-01 10:00", "2026-04-02 10:00", "2026-04-01", 5, 1000.0, 1100.0))

	// 3) missing SKU (源码 line 172)
	mock.ExpectQuery(`FROM allocate_details d\s+JOIN allocate_orders o`).
		WillReturnRows(sqlmock.NewRows([]string{"channel_key", "goods_no", "barcode", "goods_name", "cnt", "qty"}).
			AddRow("京东", "G001", "B001", "样品 A", 2, 10.0))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/special-channel/allot/summary?start=2026-04-01&end=2026-04-30", nil)
	(&DashboardHandler{DB: db}).GetSpecialChannelAllotSummary(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetSpecialChannelAllotSummaryDeptFilter(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM allocate_orders o\s+LEFT JOIN allocate_details`).
		WillReturnRows(sqlmock.NewRows([]string{"channel_key", "in_status", "orders", "sales"}))
	mock.ExpectQuery(`FROM allocate_orders o\s+WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"allocate_no", "channel_key", "in_wh", "in_status", "status", "gc", "gm", "stat_date", "sku_count", "excel", "api"}))
	mock.ExpectQuery(`FROM allocate_details d`).
		WillReturnRows(sqlmock.NewRows([]string{"channel_key", "goods_no", "barcode", "goods_name", "cnt", "qty"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/special-channel/allot/summary?dept=ecommerce", nil)
	(&DashboardHandler{DB: db}).GetSpecialChannelAllotSummary(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetSpecialChannelAllotSummaryInvalidDept(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/special-channel/allot/summary?dept=invalid", nil)
	(&DashboardHandler{DB: db}).GetSpecialChannelAllotSummary(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid dept 应 400, got %d", rec.Code)
	}
}
