package handler

// admin_handlers_test.go — admin.go AdminMeta / AdminUsers GET / AdminRoles GET +
// 路由分发边界 + adminDeptLabel
// 已 Read admin.go:
//   - AdminMeta (126): 5 SQL (roles/permissions/depts/shops/warehouses) + hardcoded platforms/domains
//   - AdminUsers (280): GET → adminUsersList (1 SQL) / POST → adminUsersCreate
//   - AdminRoles (291): GET → 1 SQL (roles + LEFT JOIN role_permissions+user_roles) / POST → adminRoleCreate
//   - AdminUserByPath (442) / AdminRoleByPath (482): path 解析 + method 路由
//   - adminDeptLabel (114) / isBuiltInRole (121): 纯函数 (isBuiltInRole 已在 auth_random_test 测)

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ AdminMeta ============

func TestAdminMetaMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/meta", nil)
	(&DashboardHandler{DB: db}).AdminMeta(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST 应 405, got %d", rec.Code)
	}
}

func TestAdminMetaHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 1. roles
	mock.ExpectQuery(`SELECT code, name FROM roles ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"code", "name"}).
			AddRow("super_admin", "超级管理员").
			AddRow("operator", "运营"))

	// 2. permissions
	mock.ExpectQuery(`SELECT code, name, type FROM permissions`).
		WillReturnRows(sqlmock.NewRows([]string{"code", "name", "type"}).
			AddRow("user.list", "用户查看", "page"))

	// 3. depts (sales_channel DISTINCT)
	mock.ExpectQuery(`SELECT DISTINCT department\s+FROM sales_channel`).
		WillReturnRows(sqlmock.NewRows([]string{"department"}).
			AddRow("ecommerce").
			AddRow("social"))

	// 4. shops
	mock.ExpectQuery(`SELECT DISTINCT channel_name, IFNULL\(department, ''\)\s+FROM sales_channel`).
		WillReturnRows(sqlmock.NewRows([]string{"channel_name", "dept"}).
			AddRow("天猫旗舰店", "ecommerce").
			AddRow("无主店", ""))

	// 5. warehouses (UNION query)
	mock.ExpectQuery(`SELECT warehouse_name\s+FROM \(\s+SELECT DISTINCT TRIM\(warehouse_name\)`).
		WillReturnRows(sqlmock.NewRows([]string{"warehouse_name"}).
			AddRow("华东仓").
			AddRow("华南仓"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/meta", nil)
	(&DashboardHandler{DB: db}).AdminMeta(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})

	for _, k := range []string{"roles", "permissions", "depts", "platforms", "shops", "warehouses", "domains"} {
		if _, ok := resp[k]; !ok {
			t.Errorf("response 缺字段 %s", k)
		}
	}
	// platforms 是 hardcoded 13 个
	plats, _ := resp["platforms"].([]interface{})
	if len(plats) != 13 {
		t.Errorf("platforms 应 13 个 (hardcoded), got %d", len(plats))
	}
	// domains 是 hardcoded 4 个
	domains, _ := resp["domains"].([]interface{})
	if len(domains) != 4 {
		t.Errorf("domains 应 4 个, got %d", len(domains))
	}
}

// ============ AdminUsers ============

func TestAdminUsersGetHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// adminUsersList SQL (1 个)
	mock.ExpectQuery(`SELECT u\.id, u\.username, IFNULL\(u\.real_name,''\)`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "real_name", "phone", "status", "last_login", "roles", "remark"}).
			AddRow(1, "admin", "管理员", "", "active", "2026-05-10 10:00:00", "super_admin", "").
			AddRow(2, "user1", "测试用户", "13800000000", "active", "", "operator,viewer", ""))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	(&DashboardHandler{DB: db}).AdminUsers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	users, _ := resp["list"].([]interface{})
	if len(users) != 2 {
		t.Errorf("应 2 个用户, got %d", len(users))
	}
	// user1 roles 应 split 成 ["operator", "viewer"]
	u1, _ := users[1].(map[string]interface{})
	roles, _ := u1["roles"].([]interface{})
	if len(roles) != 2 {
		t.Errorf("user1 roles 应 split 为 2, got %d", len(roles))
	}
}

func TestAdminUsersMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/users", nil)
	(&DashboardHandler{DB: db}).AdminUsers(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT 应 405, got %d", rec.Code)
	}
}

func TestAdminUsersCreateBadJSON(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users", bytes.NewReader([]byte(`not json`)))
	(&DashboardHandler{DB: db}).AdminUsers(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json 应 400, got %d", rec.Code)
	}
}

func TestAdminUsersCreateMissingUsername(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"username":"","password":"Test1234","realName":"测试"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).AdminUsers(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("空 username 应 400, got %d", rec.Code)
	}
}

func TestAdminUsersCreateWeakPassword(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"username":"newuser","password":"short","realName":"测试"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).AdminUsers(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("弱密码应 400 (validatePassword), got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminUsersCreateNonAdminAssignsSuperAdmin(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"username":"x","password":"Test1234","realName":"X","roleCodes":["super_admin"]}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users", bytes.NewReader(body))
	// 没 auth context = 非 super admin → 403
	(&DashboardHandler{DB: db}).AdminUsers(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("非 super 分配 super_admin 应 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ============ AdminRoles ============

func TestAdminRolesGetHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM roles r\s+LEFT JOIN role_permissions rp`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name", "desc", "perm_count", "user_count"}).
			AddRow(1, "super_admin", "超级管理员", "", 50, 1).
			AddRow(2, "operator", "运营", "", 10, 5))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/roles", nil)
	(&DashboardHandler{DB: db}).AdminRoles(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	list, _ := resp["list"].([]interface{})
	if len(list) != 2 {
		t.Errorf("应 2 个 role, got %d", len(list))
	}
	// 第一个应是 builtin (super_admin 是内置)
	r1, _ := list[0].(map[string]interface{})
	if !r1["builtin"].(bool) {
		t.Error("super_admin 应 builtin=true")
	}
}

func TestAdminRolesMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/roles", nil)
	(&DashboardHandler{DB: db}).AdminRoles(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE 应 405, got %d", rec.Code)
	}
}

// ============ AdminUserByPath / AdminRoleByPath 路由 ============

func TestAdminUserByPathNotFound(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	// 路径不匹配 prefix → 404
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/wrong/path", nil)
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("不匹配 path 应 404, got %d", rec.Code)
	}
}

func TestAdminUserByPathBadID(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/abc", nil)
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("非数字 id 应 404, got %d", rec.Code)
	}
}

func TestAdminUserByPathDeleteRouteMissingMethod(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	// /users/1 (no action) + GET → 405 (期望 DELETE)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/1", nil)
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("/users/1 GET 应 405 (期望 DELETE), got %d", rec.Code)
	}
}

func TestAdminUserByPathUnknownAction(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/1/unknown", nil)
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("未知 action 应 404, got %d", rec.Code)
	}
}

func TestAdminUserByPathStatusGet(t *testing.T) {
	// /users/1/status + GET → 405 (期望 PUT)
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/1/status", nil)
	(&DashboardHandler{DB: db}).AdminUserByPath(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /status 应 405 (期望 PUT), got %d", rec.Code)
	}
}

func TestAdminRoleByPathNotFound(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/wrong/prefix", nil)
	(&DashboardHandler{DB: db}).AdminRoleByPath(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("不匹配 path 应 404, got %d", rec.Code)
	}
}

func TestAdminRoleByPathBadID(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/roles/xyz", nil)
	(&DashboardHandler{DB: db}).AdminRoleByPath(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("非数字 id 应 404, got %d", rec.Code)
	}
}

func TestAdminRoleByPathMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/roles/1", nil)
	(&DashboardHandler{DB: db}).AdminRoleByPath(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /roles/1 应 405, got %d", rec.Code)
	}
}

// ============ adminDeptLabel ============

func TestAdminDeptLabelKnownAndUnknown(t *testing.T) {
	cases := map[string]string{
		"ecommerce":      "电商部门",
		"social":         "社媒部门",
		"offline":        "线下部门",
		"distribution":   "分销部门",
		"instant_retail": "即时零售部",
		"finance":        "财务部门",
		"supply_chain":   "供应链管理",
		"supply-chain":   "供应链管理", // alias 也覆盖
		"other":          "其他",
		"excluded":       "不计算销售",
		// unknown 直接返原值
		"unknown":   "unknown",
		"":          "",
		"random123": "random123",
	}
	for in, want := range cases {
		if got := adminDeptLabel(in); got != want {
			t.Errorf("adminDeptLabel(%q)=%q want %q", in, got, want)
		}
	}
}
