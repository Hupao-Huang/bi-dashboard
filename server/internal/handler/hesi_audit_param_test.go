package handler

// 2026-06-12 第三批: 审批业务参数 DB 配置化 (hesi_audit_param) 的三分支测试
// ① DB 自定义生效 ② 部分配置按档合并(缺档用默认补齐) ③ JSON 坏/查询失败回默认

import (
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// resetHesiAuditParamCache 清参数缓存让下一次 load 重新查库, 返回恢复函数
func resetHesiAuditParamCache() func() {
	hesiAuditParamMu.Lock()
	oldA, oldS, oldAt := accomStdCache, subsidyStdCache, hesiAuditParamAt
	accomStdCache, subsidyStdCache = nil, nil
	hesiAuditParamAt = time.Time{}
	hesiAuditParamMu.Unlock()
	return func() {
		hesiAuditParamMu.Lock()
		accomStdCache, subsidyStdCache, hesiAuditParamAt = oldA, oldS, oldAt
		hesiAuditParamMu.Unlock()
	}
}

func paramRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"param_key", "param_json"})
}

// mkParamHandler 造 handler + sqlmock; ensure 的 CREATE TABLE/INSERT 是否触发取决于全局 Once
// 是否已被同包其他测试消费, 故用无序匹配且不强制校验期望 — 只挂 SELECT 期望
func mkParamHandler(t *testing.T, rows *sqlmock.Rows, queryErr error) (*DashboardHandler, func()) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	mock.MatchExpectationsInOrder(false)
	// Once 未消费时 ensure 会发 CREATE TABLE + 2 INSERT, 宽容挂上 (已消费则这些期望闲置, 不校验)
	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hesi_audit_param`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT IGNORE INTO hesi_audit_param`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT IGNORE INTO hesi_audit_param`).WillReturnResult(sqlmock.NewResult(0, 0))
	q := mock.ExpectQuery(`SELECT param_key, param_json FROM hesi_audit_param`)
	if queryErr != nil {
		q.WillReturnError(queryErr)
	} else {
		q.WillReturnRows(rows)
	}
	return &DashboardHandler{DB: db}, func() { db.Close() }
}

func TestAuditParamCustomOverride(t *testing.T) {
	restore := resetHesiAuditParamCache()
	defer restore()
	h, closeDB := mkParamHandler(t, paramRows().
		AddRow("subsidy_standard", `{"总裁":300,"副总裁":150,"集团总监":100,"集团经理":80,"主管和其他":60}`), nil)
	defer closeDB()
	_, subsidy := h.loadHesiAuditParams()
	if subsidy["总裁"] != 300 {
		t.Errorf("DB 自定义总裁=300 应生效, got %v", subsidy["总裁"])
	}
	if subsidy["集团经理"] != 80 {
		t.Errorf("未改动档应保持, got %v", subsidy["集团经理"])
	}
}

func TestAuditParamPartialJSONMergesDefaults(t *testing.T) {
	restore := resetHesiAuditParamCache()
	defer restore()
	// 财务手填只给了 1 档 — 其余 4 档必须回落代码默认值, 不能变"未配置"
	h, closeDB := mkParamHandler(t, paramRows().
		AddRow("subsidy_standard", `{"总裁":250}`), nil)
	defer closeDB()
	_, subsidy := h.loadHesiAuditParams()
	if subsidy["总裁"] != 250 {
		t.Errorf("覆盖档应生效, got %v", subsidy["总裁"])
	}
	if subsidy["主管和其他"] != 60 {
		t.Errorf("缺档应用默认值补齐, got %v", subsidy["主管和其他"])
	}
	if len(subsidy) != 5 {
		t.Errorf("合并后应仍是 5 档, got %d", len(subsidy))
	}
}

func TestAuditParamBadJSONFallsBack(t *testing.T) {
	restore := resetHesiAuditParamCache()
	defer restore()
	h, closeDB := mkParamHandler(t, paramRows().
		AddRow("subsidy_standard", `{broken json`), nil)
	defer closeDB()
	_, subsidy := h.loadHesiAuditParams()
	if subsidy["总裁"] != 200 {
		t.Errorf("JSON 坏应回代码默认值 200, got %v", subsidy["总裁"])
	}
}

func TestAuditParamQueryErrorFallsBack(t *testing.T) {
	restore := resetHesiAuditParamCache()
	defer restore()
	h, closeDB := mkParamHandler(t, nil, errors.New("connection refused"))
	defer closeDB()
	accom, subsidy := h.loadHesiAuditParams()
	if subsidy["总裁"] != 200 || accom["总裁"]["一线"] != 1200 {
		t.Errorf("查询失败应回代码默认值, got subsidy=%v accom=%v", subsidy["总裁"], accom["总裁"]["一线"])
	}
}
