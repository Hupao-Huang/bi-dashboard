package business

// writeresult_test.go — WriteResult 全分支
// 已 Read parser.go (line 563 WriteResult): 1 DELETE + N batch INSERT + 1 INSERT log + commit.

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestWriteResultEmptyRows(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	result := &ParseResult{
		SnapshotYear:  2026,
		SnapshotMonth: 5,
		Year:          2026,
		Rows:          []BudgetRow{},
	}
	if err := WriteResult(db, result); err == nil {
		t.Error("空 rows 应返 err")
	}
}

func TestWriteResultBeginError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin().WillReturnError(errors.New("begin fail"))

	result := &ParseResult{
		SnapshotYear: 2026, SnapshotMonth: 5, Year: 2026,
		Rows: []BudgetRow{{Year: 2026, Subject: "test"}},
	}
	if err := WriteResult(db, result); err == nil {
		t.Error("Begin err 应返 err")
	}
}

func TestWriteResultDeleteError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM business_budget_report`).
		WillReturnError(errors.New("delete fail"))

	result := &ParseResult{
		SnapshotYear: 2026, SnapshotMonth: 5, Year: 2026,
		Rows: []BudgetRow{{Year: 2026, Subject: "test"}},
	}
	if err := WriteResult(db, result); err == nil {
		t.Error("DELETE err 应返 err")
	}
}

func TestWriteResultInsertError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM business_budget_report`).
		WillReturnResult(sqlmock.NewResult(0, 5))
	mock.ExpectExec(`INSERT INTO business_budget_report`).
		WillReturnError(errors.New("insert fail"))

	result := &ParseResult{
		SnapshotYear: 2026, SnapshotMonth: 5, Year: 2026,
		Rows: []BudgetRow{{Year: 2026, Subject: "test", Channel: "电商"}},
	}
	if err := WriteResult(db, result); err == nil {
		t.Error("INSERT err 应返 err")
	}
}

func TestWriteResultLogError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM business_budget_report`).
		WillReturnResult(sqlmock.NewResult(0, 5))
	mock.ExpectExec(`INSERT INTO business_budget_report`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO business_budget_import_log`).
		WillReturnError(errors.New("log fail"))

	result := &ParseResult{
		SnapshotYear: 2026, SnapshotMonth: 5, Year: 2026,
		Rows: []BudgetRow{{Year: 2026, Subject: "test"}},
	}
	if err := WriteResult(db, result); err == nil {
		t.Error("log INSERT err 应返 err")
	}
}

func TestWriteResultHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM business_budget_report WHERE snapshot_year=\? AND snapshot_month=\?`).
		WithArgs(2026, 5).
		WillReturnResult(sqlmock.NewResult(0, 100))
	mock.ExpectExec(`INSERT INTO business_budget_report`).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(`INSERT INTO business_budget_import_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	val := 100.0
	result := &ParseResult{
		SnapshotYear: 2026, SnapshotMonth: 5, Year: 2026,
		SourceFile: "test.xlsx",
		Rows: []BudgetRow{
			{Year: 2026, Subject: "GMV合计", Channel: "电商", PeriodMonth: 0, BudgetYearStart: &val},
			{Year: 2026, Subject: "GMV合计", Channel: "电商", PeriodMonth: 1, Budget: &val},
			{Year: 2026, Subject: "GMV合计", Channel: "电商", PeriodMonth: 2, Actual: &val},
		},
	}
	if err := WriteResult(db, result); err != nil {
		t.Errorf("happy path err: %v", err)
	}
}

func TestWriteResultBatchSplit(t *testing.T) {
	// > 500 行 → 多个 batch
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM business_budget_report`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// 600 行 = 2 batch (500 + 100)
	mock.ExpectExec(`INSERT INTO business_budget_report`).WillReturnResult(sqlmock.NewResult(0, 500))
	mock.ExpectExec(`INSERT INTO business_budget_report`).WillReturnResult(sqlmock.NewResult(0, 100))
	mock.ExpectExec(`INSERT INTO business_budget_import_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	rows := make([]BudgetRow, 600)
	for i := range rows {
		rows[i] = BudgetRow{Year: 2026, Subject: "Test"}
	}

	result := &ParseResult{
		SnapshotYear: 2026, SnapshotMonth: 5, Year: 2026,
		Rows: rows,
	}
	if err := WriteResult(db, result); err != nil {
		t.Errorf("batch split err: %v", err)
	}
}

// ============ nullIfNil ============

func TestNullIfNilPointer(t *testing.T) {
	got := nullIfNil(nil)
	if got != nil {
		t.Errorf("nil 应返 nil, got %v", got)
	}

	v := 3.14
	got = nullIfNil(&v)
	f, ok := got.(float64)
	if !ok || f != 3.14 {
		t.Errorf("非空指针应返 float64 值, got %v", got)
	}
}
