package handler

// handler_helpers_70_test.go — 推 70% 一波: buildHealthAlertMessage / flowFilter.buildWhere / revokeUserSessions
//   GetRPAMapping / GetDBDictionary / requireWarehouseAccess / cleanupOldTasks
// 已 Read auth.go (line 1680 revokeUserSessions), warehouse_flow.go (line 166 buildWhere),
//   task_health.go (line 93 buildHealthAlertMessage), docs.go (line 19 GetRPAMapping, 128 GetDBDictionary),
//   scope.go (line 165 requireWarehouseAccess).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ revokeUserSessions ============

func TestRevokeUserSessionsHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`DELETE FROM user_sessions WHERE user_id = \?`).
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 3))

	h := &DashboardHandler{DB: db}
	if err := h.revokeUserSessions(7); err != nil {
		t.Errorf("err: %v", err)
	}
}

func TestRevokeUserSessionsNilDB(t *testing.T) {
	var h *DashboardHandler
	err := h.revokeUserSessions(1)
	if err == nil {
		t.Error("nil h 应返 err")
	}

	h2 := &DashboardHandler{}
	err = h2.revokeUserSessions(1)
	if err == nil {
		t.Error("nil DB 应返 err")
	}
}

func TestRevokeUserSessionsDBError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`DELETE FROM user_sessions`).WillReturnError(errBoom)

	h := &DashboardHandler{DB: db}
	if err := h.revokeUserSessions(1); err == nil {
		t.Error("DB err 应返 err")
	}
}

// ============ buildHealthAlertMessage ============

func TestBuildHealthAlertMessageSingleFail(t *testing.T) {
	fails := []TaskStatus{
		{Name: "BI-SyncDailySummary", Status: "failed", LastRun: "2026-05-10 03:00", LastOutput: "ERROR: 连接 MySQL 失败"},
	}
	got := buildHealthAlertMessage(fails)
	if !strings.Contains(got, "BI-SyncDailySummary") {
		t.Errorf("应含任务名: %s", got)
	}
	if !strings.Contains(got, "失败") {
		t.Errorf("单个失败应显示'失败': %s", got)
	}
	if !strings.Contains(got, "ERROR: 连接 MySQL 失败") {
		t.Errorf("应含日志尾部: %s", got)
	}
}

func TestBuildHealthAlertMessageStuck(t *testing.T) {
	fails := []TaskStatus{
		{Name: "BI-SyncStock", Status: "stuck", LastRun: "2026-05-10 23:00"},
	}
	got := buildHealthAlertMessage(fails)
	if !strings.Contains(got, "卡死") {
		t.Errorf("stuck 应显示'卡死': %s", got)
	}
}

func TestBuildHealthAlertMessageMultipleFails(t *testing.T) {
	fails := []TaskStatus{
		{Name: "Task1", Status: "failed", LastRun: "2026-05-10 03:00"},
		{Name: "Task2", Status: "stuck"},
		{Name: "Task3", Status: "failed"},
	}
	got := buildHealthAlertMessage(fails)
	if !strings.Contains(got, "3 个异常任务") {
		t.Errorf("多个应显示数量: %s", got)
	}
	if !strings.Contains(got, "Task1") || !strings.Contains(got, "Task2") || !strings.Contains(got, "Task3") {
		t.Errorf("应列举所有任务: %s", got)
	}
}

func TestBuildHealthAlertMessageLongOutput(t *testing.T) {
	longLog := strings.Repeat("X", 500) // > 200 截断
	fails := []TaskStatus{
		{Name: "T", Status: "failed", LastOutput: longLog},
	}
	got := buildHealthAlertMessage(fails)
	if !strings.Contains(got, "...") {
		t.Errorf("超长输出应截断 ...: %s", got)
	}
}

// ============ flowFilter.buildWhere ============

func TestFlowFilterBuildWhereDefault(t *testing.T) {
	f := flowFilter{ym: "202604"}
	where, args := f.buildWhere()
	if !strings.Contains(where, "trade_status_explain") {
		t.Errorf("应含取消过滤: %s", where)
	}
	if !strings.Contains(where, "trade_type NOT IN") {
		t.Errorf("应排除 trade_type 8/12: %s", where)
	}
	// planWarehouses 白名单 ? 占位
	if len(args) == 0 {
		t.Error("应有 planWarehouses 白名单 args")
	}
}

func TestFlowFilterBuildWhereWithShop(t *testing.T) {
	f := flowFilter{ym: "202604", shop: "天猫旗舰店"}
	where, args := f.buildWhere()
	if !strings.Contains(where, "shop_name = ?") {
		t.Errorf("shop 过滤丢失: %s", where)
	}
	hasShop := false
	for _, a := range args {
		if s, ok := a.(string); ok && s == "天猫旗舰店" {
			hasShop = true
		}
	}
	if !hasShop {
		t.Errorf("args 应含 shop: %v", args)
	}
}

func TestFlowFilterBuildWhereWithSkuKw(t *testing.T) {
	f := flowFilter{ym: "202604", skuKw: "酱油"}
	where, args := f.buildWhere()
	if !strings.Contains(where, "goods_name LIKE") || !strings.Contains(where, "goods_no LIKE") {
		t.Errorf("skuKw 应触发双 LIKE: %s", where)
	}
	hasKw := false
	for _, a := range args {
		if s, ok := a.(string); ok && strings.Contains(s, "酱油") {
			hasKw = true
		}
	}
	if !hasKw {
		t.Errorf("args 应含 %%酱油%%: %v", args)
	}
}

func TestFlowFilterBuildWhereWithSkuNo(t *testing.T) {
	// skuNo 优先于 skuKw
	f := flowFilter{ym: "202604", skuNo: "G001", skuKw: "ignored"}
	where, args := f.buildWhere()
	if !strings.Contains(where, "g.goods_no = ?") {
		t.Errorf("skuNo 应触发精确 =: %s", where)
	}
	if strings.Contains(where, "goods_name LIKE") {
		t.Errorf("skuNo 优先时不应有 skuKw LIKE: %s", where)
	}
	hasNo := false
	for _, a := range args {
		if s, ok := a.(string); ok && s == "G001" {
			hasNo = true
		}
	}
	if !hasNo {
		t.Errorf("args 应含 G001: %v", args)
	}
}

func TestFlowFilterBuildWhereWithProvince(t *testing.T) {
	f := flowFilter{ym: "202604", province: "浙江"}
	where, args := f.buildWhere()
	if !strings.Contains(where, "= ?") {
		t.Errorf("province 应有 = ?: %s", where)
	}
	hasProv := false
	for _, a := range args {
		if s, ok := a.(string); ok && s == "浙江" {
			hasProv = true
		}
	}
	if !hasProv {
		t.Errorf("args 应含 浙江: %v", args)
	}
}

// ============ GetRPAMapping ============

func TestGetRPAMappingHappyPath(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/rpa-mapping", nil)
	(&DashboardHandler{DB: db}).GetRPAMapping(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "天猫") || !strings.Contains(body, "import-tmall") {
		t.Errorf("应含平台名 + import 工具: %s", body[:100])
	}
}

func TestGetRPAMappingMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rpa-mapping", nil)
	(&DashboardHandler{DB: db}).GetRPAMapping(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST 应 405, got %d", rec.Code)
	}
}

// ============ GetDBDictionary ============

func TestGetDBDictionaryMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/db-dict", nil)
	(&DashboardHandler{DB: db}).GetDBDictionary(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST 应 405, got %d", rec.Code)
	}
}

func TestGetDBDictionaryDBError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM information_schema\.TABLES`).WillReturnError(errBoom)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/db-dict", nil)
	(&DashboardHandler{DB: db}).GetDBDictionary(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("DB err 应 500, got %d", rec.Code)
	}
}

// ============ requireWarehouseAccess ============

func TestRequireWarehouseAccessNoPayload(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	if err := requireWarehouseAccess(req, "华东仓"); err != nil {
		t.Errorf("无 payload 应放行 (无认证), got %v", err)
	}
}

func TestRequireWarehouseAccessSuperAdminAllowAll(t *testing.T) {
	payload := &authPayload{IsSuperAdmin: true}
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	if err := requireWarehouseAccess(req.WithContext(ctx), "华东仓"); err != nil {
		t.Errorf("super_admin 应放行, got %v", err)
	}
}

func TestRequireWarehouseAccessAllKeyword(t *testing.T) {
	payload := &authPayload{
		DataScopes: authDataScopes{Warehouses: []string{"华东仓"}},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	if err := requireWarehouseAccess(req.WithContext(ctx), "all"); err != nil {
		t.Errorf("'all' 关键字应放行, got %v", err)
	}
}

func TestRequireWarehouseAccessAllowed(t *testing.T) {
	payload := &authPayload{
		DataScopes: authDataScopes{Warehouses: []string{"华东仓", "华南仓"}},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	if err := requireWarehouseAccess(req.WithContext(ctx), "华东仓"); err != nil {
		t.Errorf("允许的仓应放行, got %v", err)
	}
}

func TestRequireWarehouseAccessForbidden(t *testing.T) {
	payload := &authPayload{
		DataScopes: authDataScopes{Warehouses: []string{"华东仓"}},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	if err := requireWarehouseAccess(req.WithContext(ctx), "华南仓"); err == nil {
		t.Error("禁止的仓应返 err")
	}
}

// ============ buildWarehouseScopeCond ============

func TestBuildWarehouseScopeCondNoFilter(t *testing.T) {
	// 无认证 payload + 无 requested → 不加条件
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	cond, args, err := buildWarehouseScopeCond(req, "", "wh.name")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if cond != "" || len(args) != 0 {
		t.Errorf("无 scope 应空, got %q %v", cond, args)
	}
}

func TestBuildWarehouseScopeCondRequestedAllowed(t *testing.T) {
	payload := &authPayload{
		DataScopes: authDataScopes{Warehouses: []string{"华东仓"}},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	cond, args, err := buildWarehouseScopeCond(req.WithContext(ctx), "华东仓", "wh.name")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(cond, "wh.name = ?") {
		t.Errorf("应有 = ?: %s", cond)
	}
	if len(args) != 1 || args[0] != "华东仓" {
		t.Errorf("args wrong: %v", args)
	}
}

func TestBuildWarehouseScopeCondRequestedForbidden(t *testing.T) {
	payload := &authPayload{
		DataScopes: authDataScopes{Warehouses: []string{"华东仓"}},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	_, _, err := buildWarehouseScopeCond(req.WithContext(ctx), "华南仓", "wh.name")
	if err == nil {
		t.Error("禁止仓应返 err")
	}
}

func TestBuildWarehouseScopeCondImplicitWhitelist(t *testing.T) {
	payload := &authPayload{
		DataScopes: authDataScopes{Warehouses: []string{"华东仓", "华南仓"}},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, payload)
	// 无 requested 但有 scope → 自动加 IN (?,?)
	cond, args, err := buildWarehouseScopeCond(req.WithContext(ctx), "", "wh.name")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(cond, "IN (") {
		t.Errorf("应有 IN: %s", cond)
	}
	if len(args) != 2 {
		t.Errorf("args len=%d want 2", len(args))
	}
}
