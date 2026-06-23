package business

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestComputeDiffDetectsChangedCell(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// 库里现有该快照一行：电商|TOC|收入|营业收入|period 1，budget=100 actual=90
	rows := sqlmock.NewRows([]string{"channel", "sub_channel", "parent_subject", "subject", "period_month", "budget", "actual", "budget_year_start"}).
		AddRow("电商", "TOC", "收入", "营业收入", 1, 100.0, 90.0, nil)
	mock.ExpectQuery(`SELECT channel, sub_channel, parent_subject, subject, period_month, budget, actual, budget_year_start FROM business_budget_report WHERE snapshot_year=\? AND snapshot_month=\?`).
		WithArgs(2026, 4).
		WillReturnRows(rows)

	f110, f95 := 110.0, 95.0
	res := &ParseResult{
		SnapshotYear: 2026, SnapshotMonth: 4, Year: 2026, Mode: ImportModeFull,
		Rows: []BudgetRow{
			{Channel: "电商", SubChannel: "TOC", ParentSubject: "收入", Subject: "营业收入", PeriodMonth: 1, Budget: &f110, Actual: &f95},
		},
	}
	diff, err := ComputeDiff(db, res)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if diff.IsNewSnapshot {
		t.Error("快照已存在，IsNewSnapshot 应为 false")
	}
	if diff.TotalChanged != 1 {
		t.Errorf("应有 1 个变更格(budget 100→110, actual 90→95 算 1 行有变化 ChangedCells), got TotalChanged=%d", diff.TotalChanged)
	}
	if len(diff.Groups) != 1 || diff.Groups[0].Channel != "电商" || diff.Groups[0].SubChannel != "TOC" {
		t.Fatalf("应有 1 组 电商|TOC, got %+v", diff.Groups)
	}
}

func TestComputeDiffNewSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT channel, sub_channel, parent_subject, subject, period_month, budget, actual, budget_year_start FROM business_budget_report WHERE snapshot_year=\? AND snapshot_month=\?`).
		WithArgs(2026, 5).
		WillReturnRows(sqlmock.NewRows([]string{"channel", "sub_channel", "parent_subject", "subject", "period_month", "budget", "actual", "budget_year_start"}))

	f1 := 1.0
	res := &ParseResult{
		SnapshotYear: 2026, SnapshotMonth: 5, Year: 2026, Mode: ImportModeFull,
		Rows: []BudgetRow{{Channel: "电商", SubChannel: "TOC", ParentSubject: "收入", Subject: "营业收入", PeriodMonth: 1, Budget: &f1}},
	}
	diff, err := ComputeDiff(db, res)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !diff.IsNewSnapshot {
		t.Error("库里无此快照，IsNewSnapshot 应为 true")
	}
	if diff.TotalNew != 1 {
		t.Errorf("应有 1 个新增, got %d", diff.TotalNew)
	}
}

// TestComputeDiffIncrementalDeleteScope 验证 incremental 模式下删除判定只针对本次出现的 (channel,sub_channel) 组
// 场景：库里有两个组 (电商,TOC) 和 (分销,礼品)，本次文件只含 (电商,TOC) 且该组少一行
// 断言：TotalDeleted=1（只算电商|TOC 里的缺行），不算分销|礼品 组的行
func TestComputeDiffIncrementalDeleteScope(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// 库里有两组：电商|TOC 两行，分销|礼品 一行
	rows := sqlmock.NewRows([]string{"channel", "sub_channel", "parent_subject", "subject", "period_month", "budget", "actual", "budget_year_start"}).
		AddRow("电商", "TOC", "收入", "营业收入", 1, 100.0, 90.0, nil).
		AddRow("电商", "TOC", "收入", "营业收入", 2, 200.0, 180.0, nil).
		AddRow("分销", "礼品", "收入", "营业收入", 1, 50.0, 45.0, nil)
	mock.ExpectQuery(`SELECT channel, sub_channel, parent_subject, subject, period_month, budget, actual, budget_year_start FROM business_budget_report WHERE snapshot_year=\? AND snapshot_month=\?`).
		WithArgs(2026, 4).
		WillReturnRows(rows)

	// 本次文件只含 (电商,TOC) 组，且只有 period_month=1 这一行（少了 period_month=2）
	f110 := 110.0
	res := &ParseResult{
		SnapshotYear: 2026, SnapshotMonth: 4, Year: 2026, Mode: ImportModeIncremental,
		Rows: []BudgetRow{
			{Channel: "电商", SubChannel: "TOC", ParentSubject: "收入", Subject: "营业收入", PeriodMonth: 1, Budget: &f110},
		},
	}
	diff, err := ComputeDiff(db, res)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// 电商|TOC period_month=2 被删，分销|礼品 不在本次出现的组里，不算删
	if diff.TotalDeleted != 1 {
		t.Errorf("incremental 模式下应只删电商|TOC 组内缺失的 1 行，got TotalDeleted=%d", diff.TotalDeleted)
	}
}
