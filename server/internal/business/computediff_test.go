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
		t.Errorf("应有 1 行变更, got TotalChanged=%d", diff.TotalChanged)
	}
	if len(diff.Groups) != 1 || diff.Groups[0].Channel != "电商" || diff.Groups[0].SubChannel != "TOC" {
		t.Fatalf("应有 1 组 电商|TOC, got %+v", diff.Groups)
	}
	// fix #2: ChangedCells 应为真实变更格数 (budget + actual 各 1 = 2)
	if diff.Groups[0].ChangedCells != 2 {
		t.Errorf("ChangedCells 应为 2 (budget+actual 各 1 格), got %d", diff.Groups[0].ChangedCells)
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
	// fix #3: 新行的 ChangedCells 应按有值字段计（只 budget=1）
	if len(diff.Groups) != 1 || diff.Groups[0].ChangedCells != 1 {
		t.Errorf("新行只有 budget，ChangedCells 应为 1, got %d", diff.Groups[0].ChangedCells)
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
	// incremental 模式下，分销|礼品 组不应出现在 Groups 里（本次未涉及）
	for _, g := range diff.Groups {
		if g.Channel == "分销" && g.SubChannel == "礼品" {
			t.Errorf("incremental 模式下 分销|礼品 不应出现在 Groups 里, got %+v", g)
		}
	}
}

// TestComputeDiffFullShowsDeletedGroup 验证 full 模式下，库里有但本次文件没有的组必须出现在 Groups（action=delete）
// 场景：库里有 (电商,TOC) 和 (国际零售,"")，本次文件只含 (电商,TOC)
// 断言：Groups 里出现 国际零售|"" 组，Action==delete，NewRows==0，OldRows==该组旧行数
func TestComputeDiffFullShowsDeletedGroup(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// 库里有两组：电商|TOC 一行，国际零售|"" 两行
	rows := sqlmock.NewRows([]string{"channel", "sub_channel", "parent_subject", "subject", "period_month", "budget", "actual", "budget_year_start"}).
		AddRow("电商", "TOC", "收入", "营业收入", 1, 100.0, 90.0, nil).
		AddRow("国际零售", "", "收入", "营业收入", 1, 50.0, 45.0, nil).
		AddRow("国际零售", "", "收入", "营业收入", 2, 60.0, 55.0, nil)
	mock.ExpectQuery(`SELECT channel, sub_channel, parent_subject, subject, period_month, budget, actual, budget_year_start FROM business_budget_report WHERE snapshot_year=\? AND snapshot_month=\?`).
		WithArgs(2026, 4).
		WillReturnRows(rows)

	// 本次文件只含 (电商,TOC) 组
	f100 := 100.0
	f90 := 90.0
	res := &ParseResult{
		SnapshotYear: 2026, SnapshotMonth: 4, Year: 2026, Mode: ImportModeFull,
		Rows: []BudgetRow{
			{Channel: "电商", SubChannel: "TOC", ParentSubject: "收入", Subject: "营业收入", PeriodMonth: 1, Budget: &f100, Actual: &f90},
		},
	}
	diff, err := ComputeDiff(db, res)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// 必须找到 国际零售|"" 的 delete 组
	var foundDelete *DiffGroup
	for i := range diff.Groups {
		g := &diff.Groups[i]
		if g.Channel == "国际零售" && g.SubChannel == "" {
			foundDelete = g
			break
		}
	}
	if foundDelete == nil {
		t.Fatalf("full 模式下 国际零售 组应出现在 Groups（被删）, Groups=%+v", diff.Groups)
	}
	if foundDelete.Action != "delete" {
		t.Errorf("国际零售 组 Action 应为 delete, got %q", foundDelete.Action)
	}
	if foundDelete.NewRows != 0 {
		t.Errorf("国际零售 组 NewRows 应为 0, got %d", foundDelete.NewRows)
	}
	if foundDelete.OldRows != 2 {
		t.Errorf("国际零售 组 OldRows 应为 2, got %d", foundDelete.OldRows)
	}
}

// TestComputeDiffIncrementalNoDeletedGroup 验证 incremental 模式下，库里有但本次没有的组不出现在 Groups
func TestComputeDiffIncrementalNoDeletedGroup(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// 库里有两组：电商|TOC 一行，国际零售|"" 两行
	rows := sqlmock.NewRows([]string{"channel", "sub_channel", "parent_subject", "subject", "period_month", "budget", "actual", "budget_year_start"}).
		AddRow("电商", "TOC", "收入", "营业收入", 1, 100.0, 90.0, nil).
		AddRow("国际零售", "", "收入", "营业收入", 1, 50.0, 45.0, nil).
		AddRow("国际零售", "", "收入", "营业收入", 2, 60.0, 55.0, nil)
	mock.ExpectQuery(`SELECT channel, sub_channel, parent_subject, subject, period_month, budget, actual, budget_year_start FROM business_budget_report WHERE snapshot_year=\? AND snapshot_month=\?`).
		WithArgs(2026, 4).
		WillReturnRows(rows)

	// incremental 模式，本次只含 (电商,TOC)
	f100 := 100.0
	f90 := 90.0
	res := &ParseResult{
		SnapshotYear: 2026, SnapshotMonth: 4, Year: 2026, Mode: ImportModeIncremental,
		Rows: []BudgetRow{
			{Channel: "电商", SubChannel: "TOC", ParentSubject: "收入", Subject: "营业收入", PeriodMonth: 1, Budget: &f100, Actual: &f90},
		},
	}
	diff, err := ComputeDiff(db, res)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// incremental 模式下，国际零售 不应出现在 Groups
	for _, g := range diff.Groups {
		if g.Channel == "国际零售" {
			t.Errorf("incremental 模式下 国际零售 组不应出现在 Groups, got %+v", g)
		}
	}
}
