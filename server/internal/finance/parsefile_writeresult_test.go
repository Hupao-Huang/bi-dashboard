package finance

// parsefile_writeresult_test.go — finance/parser.go ParseFile + parseSheet + WriteResult 主路径
// 已 Read parser.go:
//   - ParseFile (line 324): SheetDeptMap 匹配 sheet → parseSheet → RecomputeAggregateSubjects
//   - parseSheet (line 490): 找 A 列="项目" 的 header 行 + 月份列识别 + L1/L2/L3 科目
//   - WriteResult (line 685): tx.Begin → DELETE (按 mode) → Prepare INSERT → tx.Commit

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/xuri/excelize/v2"
)

// ---------- ParseFile error ----------

func TestParseFileFinanceNotFound(t *testing.T) {
	_, err := ParseFile("/path/that/does/not/exist.xlsx", 2026, nil)
	if err == nil {
		t.Error("不存在文件应返 err")
	}
}

// ---------- ParseFile happy path ----------

// 造 sheet "1、电商" + "项目"|"合计"|"1月" 表头 + 数据行
func TestParseFileFinanceMinimal(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.xlsx")

	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "1、电商")

	// Row 1-2 任意 (源码 line 498 在前 5 行找 "项目")
	f.SetCellStr("1、电商", "A1", "电商部门")
	f.SetCellStr("1、电商", "A2", "")

	// Row 3 (index 2): header "项目" + 合计 + 1月
	f.SetCellStr("1、电商", "A3", "项目")
	f.SetCellStr("1、电商", "B3", "合计")
	f.SetCellStr("1、电商", "C3", "占比销售")
	f.SetCellStr("1、电商", "D3", "1月")
	f.SetCellStr("1、电商", "E3", "占比销售")

	// Row 4: L2 科目 GMV合计 (Level2CodeForName 命中 → GMV_TOTAL)
	f.SetCellStr("1、电商", "A4", "GMV合计")
	f.SetCellStr("1、电商", "B4", "1000000")
	f.SetCellStr("1、电商", "D4", "100000")

	// Row 5: 营业额合计 (REV_TOTAL)
	f.SetCellStr("1、电商", "A5", "营业额合计")
	f.SetCellStr("1、电商", "B5", "800000")
	f.SetCellStr("1、电商", "D5", "80000")

	if err := f.SaveAs(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	f.Close()

	result, err := ParseFile(path, 2026, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if result.Year != 2026 {
		t.Errorf("Year=%d want 2026", result.Year)
	}
	if result.SheetCount == 0 {
		t.Error("应识别至少 1 sheet")
	}
	hasEcommerce := false
	for _, d := range result.Departments {
		if d == "电商" {
			hasEcommerce = true
		}
	}
	if !hasEcommerce {
		t.Errorf("Departments 应含'电商', got %v", result.Departments)
	}
	if len(result.Rows) == 0 {
		t.Error("应解析至少 1 行 (GMV_TOTAL/REV_TOTAL)")
	}
	// 验证 Rows 含 GMV_TOTAL
	hasGMV := false
	for _, r := range result.Rows {
		if r.SubjectCode == "GMV_TOTAL" {
			hasGMV = true
		}
	}
	if !hasGMV {
		t.Error("应有 GMV_TOTAL row")
	}
}

// sheet 名不在 SheetDeptMap → 跳过
func TestParseFileFinanceSkipsUnknownSheet(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.xlsx")

	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "随机sheet名")
	f.SetCellStr("随机sheet名", "A1", "x")

	f.SaveAs(path)
	f.Close()

	result, err := ParseFile(path, 2026, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.SheetCount != 0 {
		t.Errorf("不识别 sheet 不应计入, got %d", result.SheetCount)
	}
	if len(result.Rows) != 0 {
		t.Errorf("无识别 sheet 应 0 row, got %d", len(result.Rows))
	}
}

// 没 "项目" header → 跳过该 sheet
func TestParseFileFinanceNoHeaderRow(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.xlsx")

	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "1、电商")
	f.SetCellStr("1、电商", "A1", "随便")
	f.SetCellStr("1、电商", "A2", "无项目 header")
	f.SetCellStr("1、电商", "A3", "x")
	f.SetCellStr("1、电商", "A4", "y")

	f.SaveAs(path)
	f.Close()

	result, err := ParseFile(path, 2026, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(result.Rows) != 0 {
		t.Errorf("无 header sheet 应 0 row, got %d", len(result.Rows))
	}
}

// ---------- WriteResult ----------

func TestWriteResultIncrementalMode(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	// incremental: 按 (dept, month) DELETE
	mock.ExpectExec(`DELETE FROM finance_report WHERE year = \? AND department = \? AND month = \?`).
		WithArgs(2026, "电商", 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Prepare + Exec INSERT
	mock.ExpectPrepare(`INSERT INTO finance_report.*ON DUPLICATE KEY UPDATE`).
		ExpectExec().
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit()

	result := &ParseResult{
		Year:        2026,
		Mode:        ImportModeIncremental,
		Departments: []string{"电商"},
		Rows: []FinanceRow{
			{Year: 2026, Month: 1, Department: "电商", SubjectCode: "REV_MAIN", Amount: 100000.0, SortOrder: 1},
		},
	}

	if err := WriteResult(db, result); err != nil {
		t.Fatalf("err: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestWriteResultFullModeDefault(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	// Mode 为空 → 走 full (line 693)
	// full: 按 dept DELETE 一次
	mock.ExpectExec(`DELETE FROM finance_report WHERE year = \? AND department = \?`).
		WithArgs(2026, "电商").
		WillReturnResult(sqlmock.NewResult(0, 50))

	// Prepare + 2 row INSERT
	prep := mock.ExpectPrepare(`INSERT INTO finance_report`)
	prep.ExpectExec().WillReturnResult(sqlmock.NewResult(1, 1))
	prep.ExpectExec().WillReturnResult(sqlmock.NewResult(2, 1))

	mock.ExpectCommit()

	ratio := 0.5
	result := &ParseResult{
		Year:        2026,
		Mode:        "", // → full
		Departments: []string{"电商"},
		Rows: []FinanceRow{
			{Year: 2026, Month: 1, Department: "电商", SubjectCode: "REV_MAIN", Amount: 100000, SortOrder: 1},
			{Year: 2026, Month: 1, Department: "电商", SubjectCode: "COST_MAIN", Amount: 60000, SortOrder: 2, Ratio: &ratio},
		},
	}

	if err := WriteResult(db, result); err != nil {
		t.Fatalf("err: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestWriteResultBeginFails(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin().WillReturnError(errors.New("conn lost"))

	err = WriteResult(db, &ParseResult{Year: 2026, Departments: []string{"电商"}})
	if err == nil {
		t.Error("Begin 失败应返 err")
	}
}

func TestWriteResultDeleteFails(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM finance_report`).WillReturnError(errors.New("table missing"))
	mock.ExpectRollback()

	err = WriteResult(db, &ParseResult{
		Year: 2026, Mode: ImportModeFull,
		Departments: []string{"电商"},
		Rows:        []FinanceRow{{Year: 2026, Month: 1, Department: "电商", SubjectCode: "X"}},
	})
	if err == nil {
		t.Error("DELETE 失败应返 err")
	}
}

func TestWriteResultEmptyRows(t *testing.T) {
	// 空 Rows 也应正常 commit (按 dept 删完就完事)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM finance_report`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectPrepare(`INSERT INTO finance_report`)
	mock.ExpectCommit()

	if err := WriteResult(db, &ParseResult{
		Year: 2026, Departments: []string{"电商"}, Rows: []FinanceRow{},
	}); err != nil {
		t.Fatalf("err: %v", err)
	}
}
