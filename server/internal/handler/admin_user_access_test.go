package handler

// admin_user_access_test.go — loadAdminUserAccess + saveUserAccessTx + saveRoleAccessTx + loadAdminRoleDetail
// 已 Read admin.go (line 949 loadAdminUserAccess 3 SQL chain, 1031 loadAdminRoleDetail 3 SQL chain,
//   1115 saveUserAccessTx, 1160 saveRoleAccessTx).

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ loadAdminUserAccess ============

func TestLoadAdminUserAccessHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 1. users SELECT
	mock.ExpectQuery(`SELECT id, username, IFNULL\(real_name,''\), status FROM users WHERE id = \?`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "u", "rn", "st"}).
			AddRow(int64(7), "alice", "Alice 老板", "active"))

	// 2. roles SELECT
	mock.ExpectQuery(`FROM roles r\s+INNER JOIN user_roles ur`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"code"}).
			AddRow("super_admin").AddRow("ops"))

	// 3. data_scopes SELECT (覆盖全部 5 种 scope_type)
	mock.ExpectQuery(`FROM data_scopes WHERE subject_type = 'user'`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"st", "sv"}).
			AddRow("dept", "电商").
			AddRow("platform", "tmall").
			AddRow("shop", "天猫旗舰店").
			AddRow("warehouse", "华东仓").
			AddRow("domain", "trade"))

	h := &DashboardHandler{DB: db}
	resp, err := h.loadAdminUserAccess(7)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.UserID != 7 || resp.Username != "alice" {
		t.Errorf("user fields wrong: %+v", resp)
	}
	if len(resp.RoleCodes) != 2 {
		t.Errorf("RoleCodes len=%d want 2", len(resp.RoleCodes))
	}
	if len(resp.DataScopes.Depts) != 1 || resp.DataScopes.Depts[0] != "电商" {
		t.Errorf("Depts wrong: %v", resp.DataScopes.Depts)
	}
	if len(resp.DataScopes.Platforms) != 1 || len(resp.DataScopes.Shops) != 1 ||
		len(resp.DataScopes.Warehouses) != 1 || len(resp.DataScopes.Domains) != 1 {
		t.Errorf("scopes 5 类拆分错: %+v", resp.DataScopes)
	}
}

func TestLoadAdminUserAccessUserNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT id, username.*FROM users WHERE id = \?`).
		WithArgs(int64(999)).
		WillReturnError(errBoom)

	h := &DashboardHandler{DB: db}
	_, err = h.loadAdminUserAccess(999)
	if err == nil {
		t.Error("user 不存在应返 err")
	}
}

func TestLoadAdminUserAccessRolesQueryError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT id, username.*FROM users WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "u", "rn", "st"}).
			AddRow(int64(1), "x", "", "active"))
	mock.ExpectQuery(`FROM roles r\s+INNER JOIN user_roles ur`).
		WillReturnError(errBoom)

	h := &DashboardHandler{DB: db}
	_, err = h.loadAdminUserAccess(1)
	if err == nil {
		t.Error("roles err 应返 err")
	}
}

func TestLoadAdminUserAccessScopesQueryError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT id, username.*FROM users WHERE id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "u", "rn", "st"}).
			AddRow(int64(1), "x", "", "active"))
	mock.ExpectQuery(`FROM roles r`).
		WillReturnRows(sqlmock.NewRows([]string{"code"}))
	mock.ExpectQuery(`FROM data_scopes WHERE subject_type = 'user'`).
		WillReturnError(errBoom)

	h := &DashboardHandler{DB: db}
	_, err = h.loadAdminUserAccess(1)
	if err == nil {
		t.Error("scopes err 应返 err")
	}
}

// ============ loadAdminRoleDetail ============

func TestLoadAdminRoleDetailHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// roles SELECT — code = "ops" (非 builtin)
	mock.ExpectQuery(`SELECT id, code, name, IFNULL\(description,''\) FROM roles WHERE id = \?`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name", "desc"}).
			AddRow(int64(5), "ops", "运营", "运营角色"))

	// permissions SELECT
	mock.ExpectQuery(`FROM permissions p\s+INNER JOIN role_permissions rp`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"code"}).
			AddRow("read:trade").AddRow("write:notice"))

	// data_scopes (subject_type=role) — 覆盖 3 种类型
	mock.ExpectQuery(`FROM data_scopes WHERE subject_type = 'role'`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"st", "sv"}).
			AddRow("dept", "电商").
			AddRow("dept", "社媒").
			AddRow("platform", "tmall"))

	h := &DashboardHandler{DB: db}
	resp, err := h.loadAdminRoleDetail(5)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.ID != 5 || resp.Code != "ops" {
		t.Errorf("role 字段错: %+v", resp)
	}
	if len(resp.Permissions) != 2 {
		t.Errorf("permissions len=%d want 2", len(resp.Permissions))
	}
	if len(resp.DataScopes.Depts) != 2 {
		t.Errorf("Depts len=%d want 2", len(resp.DataScopes.Depts))
	}
}

func TestLoadAdminRoleDetailRoleNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT id, code, name.*FROM roles WHERE id`).
		WillReturnError(errBoom)

	h := &DashboardHandler{DB: db}
	_, err = h.loadAdminRoleDetail(99)
	if err == nil {
		t.Error("role 不存在应返 err")
	}
}

// ============ saveUserAccessTx ============

func TestSaveUserAccessTxHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM user_roles WHERE user_id = \?`).
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM data_scopes WHERE subject_type = 'user' AND subject_id = \?`).
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// 角色 "ops" → 查 roleID + INSERT user_roles
	mock.ExpectQuery(`SELECT id FROM roles WHERE code = \?`).
		WithArgs("ops").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(3)))
	mock.ExpectExec(`INSERT INTO user_roles \(user_id, role_id\) VALUES \(\?, \?\)`).
		WithArgs(int64(7), int64(3)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// scope dept = ["电商", "社媒"] → 2 INSERT
	mock.ExpectExec(`INSERT INTO data_scopes \(subject_type, subject_id, scope_type, scope_value\) VALUES \('user', \?, \?, \?\)`).
		WithArgs(int64(7), "dept", "电商").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO data_scopes`).
		WithArgs(int64(7), "dept", "社媒").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// scope platform = ["tmall"] → 1 INSERT
	mock.ExpectExec(`INSERT INTO data_scopes`).
		WithArgs(int64(7), "platform", "tmall").
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	h := &DashboardHandler{DB: db}
	if err := h.saveUserAccessTx(tx, 7, []string{"ops"}, authDataScopes{
		Depts:     []string{"电商", "社媒"},
		Platforms: []string{"tmall"},
	}); err != nil {
		tx.Rollback()
		t.Fatalf("saveUserAccessTx err: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestSaveUserAccessTxInvalidRoleCode(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM user_roles`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DELETE FROM data_scopes`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// SELECT roleID → 找不到 (sql.ErrNoRows)
	mock.ExpectQuery(`SELECT id FROM roles WHERE code = \?`).
		WithArgs("not_exist_role").
		WillReturnError(errors.New("sql: no rows in result set"))

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	h := &DashboardHandler{DB: db}
	err = h.saveUserAccessTx(tx, 1, []string{"not_exist_role"}, authDataScopes{})
	tx.Rollback()
	if err == nil || err.Error() != "invalid role code" {
		t.Errorf("应 invalid role code, got %v", err)
	}
}

func TestSaveUserAccessTxDeleteUserRolesError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM user_roles`).WillReturnError(errBoom)

	tx, _ := db.Begin()
	h := &DashboardHandler{DB: db}
	err = h.saveUserAccessTx(tx, 1, nil, authDataScopes{})
	tx.Rollback()
	if err == nil {
		t.Error("DELETE user_roles 失败应返 err")
	}
}

func TestSaveUserAccessTxEmptyAll(t *testing.T) {
	// nil roleCodes + 空 scopes → 只 2 个 DELETE
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM user_roles`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM data_scopes WHERE subject_type = 'user'`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	h := &DashboardHandler{DB: db}
	if err := h.saveUserAccessTx(tx, 1, nil, authDataScopes{}); err != nil {
		t.Fatalf("err: %v", err)
	}
	tx.Commit()
}

// ============ saveRoleAccessTx ============

func TestSaveRoleAccessTxHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM role_permissions WHERE role_id = \?`).
		WithArgs(int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM data_scopes WHERE subject_type = 'role' AND subject_id = \?`).
		WithArgs(int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectQuery(`SELECT id FROM permissions WHERE code = \?`).
		WithArgs("read:trade").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(11)))
	mock.ExpectExec(`INSERT INTO role_permissions \(role_id, permission_id\) VALUES \(\?, \?\)`).
		WithArgs(int64(2), int64(11)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// scope warehouse = ["华东仓"]
	mock.ExpectExec(`INSERT INTO data_scopes \(subject_type, subject_id, scope_type, scope_value\) VALUES \('role', \?, \?, \?\)`).
		WithArgs(int64(2), "warehouse", "华东仓").
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit()

	tx, _ := db.Begin()
	h := &DashboardHandler{DB: db}
	err = h.saveRoleAccessTx(tx, 2, []string{"read:trade"}, authDataScopes{
		Warehouses: []string{"华东仓"},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	tx.Commit()
}

func TestSaveRoleAccessTxInvalidPermission(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM role_permissions`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DELETE FROM data_scopes`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT id FROM permissions WHERE code = \?`).
		WithArgs("not_exist:perm").
		WillReturnError(errors.New("sql: no rows in result set"))

	tx, _ := db.Begin()
	h := &DashboardHandler{DB: db}
	err = h.saveRoleAccessTx(tx, 1, []string{"not_exist:perm"}, authDataScopes{})
	tx.Rollback()
	if err == nil || err.Error() != "invalid permission code" {
		t.Errorf("应 invalid permission code, got %v", err)
	}
}
