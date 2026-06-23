package business

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestWriteResultIncrementalDeletesPerChannelSub(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	// 增量：逐 (channel, sub_channel) 精确删，本例文件只含 电商|TOC
	mock.ExpectExec(`DELETE FROM business_budget_report WHERE snapshot_year=\? AND snapshot_month=\? AND channel=\? AND sub_channel=\?`).
		WithArgs(2026, 4, "电商", "TOC").
		WillReturnResult(sqlmock.NewResult(0, 10))
	mock.ExpectExec(`INSERT INTO business_budget_report`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO business_budget_import_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err = WriteResult(db, &ParseResult{
		SnapshotYear: 2026, SnapshotMonth: 4, Year: 2026,
		Mode:       ImportModeIncremental,
		ImportedBy: "tester",
		Rows: []BudgetRow{
			{SnapshotYear: 2026, SnapshotMonth: 4, Year: 2026, Channel: "电商", SubChannel: "TOC", Subject: "营业收入", PeriodMonth: 1},
		},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestWriteResultFullDeletesWholeSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM business_budget_report WHERE snapshot_year=\? AND snapshot_month=\?`).
		WithArgs(2026, 4).
		WillReturnResult(sqlmock.NewResult(0, 100))
	mock.ExpectExec(`INSERT INTO business_budget_report`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO business_budget_import_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err = WriteResult(db, &ParseResult{
		SnapshotYear: 2026, SnapshotMonth: 4, Year: 2026,
		Mode: ImportModeFull,
		Rows: []BudgetRow{
			{SnapshotYear: 2026, SnapshotMonth: 4, Year: 2026, Channel: "电商", SubChannel: "TOC", Subject: "营业收入", PeriodMonth: 1},
		},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
