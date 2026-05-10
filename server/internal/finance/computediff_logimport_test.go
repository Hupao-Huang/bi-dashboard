package finance

// computediff_logimport_test.go — ComputeDiff + LogImport 测试
// 已 Read parser.go:
//   - ComputeDiff (line 768): 1 SQL (查 finance_report 当年 dept×month REV_TOTAL),
//     根据 Mode (full/incremental) 决定对比 key 集合
//   - LogImport (line 935): 1 SQL (INSERT INTO finance_import_log)

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ---------- ComputeDiff ----------

// 增量模式: Excel 含 (ecommerce, 4月) REV_TOTAL=100W, DB 也有同 key REV_TOTAL=80W → action=update
func TestComputeDiffIncrementalUpdate(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// DB 旧数据: ecommerce 4 月 REV_TOTAL=80W, 共 30 行
	mock.ExpectQuery(`FROM finance_report\s+WHERE year = \?\s+GROUP BY department, month`).
		WithArgs(2026).
		WillReturnRows(sqlmock.NewRows([]string{"dept", "month", "rev", "rows"}).
			AddRow("ecommerce", 4, 800000.0, 30))

	result := &ParseResult{
		Year:        2026,
		Mode:        ImportModeIncremental,
		Departments: []string{"ecommerce"},
		Rows: []FinanceRow{
			{Year: 2026, Month: 4, Department: "ecommerce", SubjectCode: "REV_TOTAL", Amount: 1000000.0},
			{Year: 2026, Month: 4, Department: "ecommerce", SubjectCode: "COST_MAIN", Amount: 600000.0},
		},
	}

	diff, err := ComputeDiff(db, result)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(diff.Entries) != 1 {
		t.Fatalf("应 1 个 entry, got %d", len(diff.Entries))
	}
	e := diff.Entries[0]
	if e.Action != "update" {
		t.Errorf("action=%s want update", e.Action)
	}
	if e.OldAmount != 800000 || e.NewAmount != 1000000 {
		t.Errorf("Old/New=%v/%v", e.OldAmount, e.NewAmount)
	}
	if e.Delta != 200000 {
		t.Errorf("delta=%v want 200000", e.Delta)
	}
	if e.DeltaPct < 24.9 || e.DeltaPct > 25.1 {
		t.Errorf("deltaPct=%v want ~25", e.DeltaPct)
	}
	if e.OldRows != 30 || e.NewRows != 2 {
		t.Errorf("OldRows/NewRows=%d/%d want 30/2", e.OldRows, e.NewRows)
	}
}

// new action: DB 没有这个 (dept, month), Excel 有
func TestComputeDiffNewAction(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM finance_report`).
		WillReturnRows(sqlmock.NewRows([]string{"dept", "month", "rev", "rows"}))

	result := &ParseResult{
		Year:        2026,
		Mode:        ImportModeIncremental,
		Departments: []string{"social"},
		Rows: []FinanceRow{
			{Year: 2026, Month: 5, Department: "social", SubjectCode: "REV_TOTAL", Amount: 500000.0},
		},
	}

	diff, _ := ComputeDiff(db, result)
	if len(diff.Entries) != 1 {
		t.Fatalf("应 1 entry, got %d", len(diff.Entries))
	}
	if diff.Entries[0].Action != "new" {
		t.Errorf("action=%s want new", diff.Entries[0].Action)
	}
}

// delete action: full 模式下, DB 有 (dept, month) Excel 没有 → action=delete
func TestComputeDiffFullModeDelete(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// DB 有 ecommerce 1月 + 2月, 但 Excel 只有 1月 → 2月 应被 delete
	mock.ExpectQuery(`FROM finance_report`).
		WillReturnRows(sqlmock.NewRows([]string{"dept", "month", "rev", "rows"}).
			AddRow("ecommerce", 1, 1000000.0, 30).
			AddRow("ecommerce", 2, 1100000.0, 30))

	result := &ParseResult{
		Year:        2026,
		Mode:        ImportModeFull,
		Departments: []string{"ecommerce"},
		Rows: []FinanceRow{
			{Year: 2026, Month: 1, Department: "ecommerce", SubjectCode: "REV_TOTAL", Amount: 1100000.0},
		},
	}

	diff, _ := ComputeDiff(db, result)
	// 应有 2 entries: 1月 update, 2月 delete
	if len(diff.Entries) != 2 {
		t.Fatalf("应 2 entry, got %d", len(diff.Entries))
	}
	actions := map[string]int{}
	for _, e := range diff.Entries {
		actions[e.Action]++
	}
	if actions["delete"] != 1 {
		t.Errorf("应 1 个 delete entry, got %v", actions)
	}
	// DeleteRows 应反映 ecommerce 全部 dept (full 模式按 dept 删除)
	if diff.DeleteRows == 0 {
		t.Errorf("DeleteRows 应非零 (full mode 删除 ecommerce dept)")
	}
}

// unchanged action: 旧/新 REV_TOTAL+rows 完全一致
func TestComputeDiffUnchanged(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM finance_report`).
		WillReturnRows(sqlmock.NewRows([]string{"dept", "month", "rev", "rows"}).
			AddRow("ecommerce", 4, 1000000.0, 1)) // 1 行

	result := &ParseResult{
		Year:        2026,
		Mode:        ImportModeIncremental,
		Departments: []string{"ecommerce"},
		Rows: []FinanceRow{
			{Year: 2026, Month: 4, Department: "ecommerce", SubjectCode: "REV_TOTAL", Amount: 1000000.0},
		},
	}

	diff, _ := ComputeDiff(db, result)
	if len(diff.Entries) != 1 {
		t.Fatalf("应 1 entry, got %d", len(diff.Entries))
	}
	if diff.Entries[0].Action != "unchanged" {
		t.Errorf("action=%s want unchanged", diff.Entries[0].Action)
	}
}

// 警告: 营收变化超阈值 (>50% 或 <-30%)
func TestComputeDiffWarningOnLargeChange(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 旧 100W → 新 200W (delta 100%, > 50% 阈值)
	mock.ExpectQuery(`FROM finance_report`).
		WillReturnRows(sqlmock.NewRows([]string{"dept", "month", "rev", "rows"}).
			AddRow("ecommerce", 5, 1000000.0, 30))

	result := &ParseResult{
		Year:        2026,
		Mode:        ImportModeIncremental,
		Departments: []string{"ecommerce"},
		Rows: []FinanceRow{
			{Year: 2026, Month: 5, Department: "ecommerce", SubjectCode: "REV_TOTAL", Amount: 2000000.0},
		},
	}

	diff, _ := ComputeDiff(db, result)
	if len(diff.Warnings) == 0 {
		t.Error("营收变化 100% 应触发 warning")
	}
}

// UnmappedSubjects > 0 → warning
func TestComputeDiffUnmappedWarning(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM finance_report`).
		WillReturnRows(sqlmock.NewRows([]string{"dept", "month", "rev", "rows"}))

	result := &ParseResult{
		Year:        2026,
		Mode:        ImportModeIncremental,
		Departments: []string{"ecommerce"},
		Rows:        []FinanceRow{},
		UnmappedSubjects: []UnmappedEntry{
			{Sheet: "电商部门", Department: "ecommerce", Subject: "未知科目1"},
			{Sheet: "电商部门", Department: "ecommerce", Subject: "未知科目2"},
		},
	}

	diff, _ := ComputeDiff(db, result)
	if diff.UnmappedCount != 2 {
		t.Errorf("UnmappedCount=%d want 2", diff.UnmappedCount)
	}
	hasUnmappedWarning := false
	for _, w := range diff.Warnings {
		if strings.Contains(w, "未映射") {
			hasUnmappedWarning = true
			break
		}
	}
	if !hasUnmappedWarning {
		t.Error("应有'未映射'相关 warning")
	}
}

// DB error 应传出
func TestComputeDiffDBError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM finance_report`).WillReturnError(errors.New("connection lost"))

	_, err = ComputeDiff(db, &ParseResult{Year: 2026, Mode: ImportModeIncremental})
	if err == nil {
		t.Error("DB error 应传出")
	}
}

// ---------- LogImport ----------

func TestLogImportSuccess(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 创建临时文件供 FileMD5
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test_finance.xlsx")
	if err := os.WriteFile(path, []byte("xx"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	mock.ExpectExec(`INSERT INTO finance_import_log\s+\(year, filename, file_size, md5`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = LogImport(db, path, 2026, &ParseResult{
		SheetCount:       3,
		RowCount:         100,
		UnmappedSubjects: []UnmappedEntry{{Subject: "X"}},
	}, 1, "success", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestLogImportEmptyUnmappedDefaultsArrayLiteral(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.xlsx")
	os.WriteFile(path, []byte("x"), 0644)

	// 空 unmapped 应序列化为 "[]" (源码 line 938-940)
	mock.ExpectExec(`INSERT INTO finance_import_log`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), "[]", sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = LogImport(db, path, 2026, &ParseResult{}, 1, "success", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestLogImportDBError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.xlsx")
	os.WriteFile(path, []byte("x"), 0644)

	mock.ExpectExec(`INSERT INTO finance_import_log`).WillReturnError(errors.New("disk full"))

	err = LogImport(db, path, 2026, &ParseResult{}, 1, "failed", "msg")
	if err == nil {
		t.Error("DB error 应传出")
	}
}
