package finance

import (
	"testing"
)

// 已 Read parser.go RecomputeAggregateSubjects 完整实现 (line 400-490).
// 业务规则 (跑哥 2026-04-30 决策): 父项 amount = SUM(子项 amount), 字典里有 ParentCode 就补/覆盖.
//
// 当前 aggregateRules: { COST_MAIN.仓储物流费用 = 物流费用 + 临时工费用 + 发货耗材成本 }

const (
	parentCode = "COST_MAIN.仓储物流费用"
	childA     = "COST_MAIN.物流费用"
	childB     = "COST_MAIN.临时工费用"
	childC     = "COST_MAIN.发货耗材成本"
)

func newDict() map[string]*DictEntry {
	return map[string]*DictEntry{
		parentCode: {
			Code: parentCode, Name: "仓储物流费用", Category: "成本", Level: 3, Parent: "COST_MAIN",
		},
	}
}

func findRow(rows []FinanceRow, dept string, month int, subChannel, code string) *FinanceRow {
	for i, r := range rows {
		if r.Department == dept && r.Month == month && r.SubChannel == subChannel && r.SubjectCode == code {
			return &rows[i]
		}
	}
	return nil
}

// case 1: 已存在父项 row, SUM 子项覆盖 amount, REV_TOTAL 在则计算 ratio
func TestRecomputeAggregate_OverwriteExistingParent(t *testing.T) {
	rows := []FinanceRow{
		{Year: 2026, Month: 5, Department: "电商", SubjectCode: parentCode, Amount: 999.0}, // 错误手填值
		{Year: 2026, Month: 5, Department: "电商", SubjectCode: childA, Amount: 100.0},
		{Year: 2026, Month: 5, Department: "电商", SubjectCode: childB, Amount: 50.0},
		{Year: 2026, Month: 5, Department: "电商", SubjectCode: childC, Amount: 30.0},
		{Year: 2026, Month: 5, Department: "电商", SubjectCode: "REV_TOTAL", Amount: 1800.0},
	}

	out := RecomputeAggregateSubjects(rows, newDict(), 2026)

	parent := findRow(out, "电商", 5, "", parentCode)
	if parent == nil {
		t.Fatal("应保留父项 row")
	}
	if parent.Amount != 180.0 {
		t.Errorf("父项 amount 应被 SUM 覆盖 = 180, 实际 %v (业务规则: 父项=子项SUM, 不信手填)", parent.Amount)
	}
	if parent.Ratio == nil {
		t.Fatal("REV_TOTAL=1800 非 0, 应计算 ratio")
	}
	if got := *parent.Ratio; got < 0.099 || got > 0.101 {
		t.Errorf("ratio 应 ≈ 180/1800 = 0.1, got %v", got)
	}
}

// case 2: 不存在父项 row, 但有子项 → 自动补建 row, SortOrder 用 sentinel 9999
func TestRecomputeAggregate_AutoInsertWhenMissing(t *testing.T) {
	rows := []FinanceRow{
		{Year: 2026, Month: 5, Department: "电商", SubjectCode: childA, Amount: 100.0},
		{Year: 2026, Month: 5, Department: "电商", SubjectCode: childB, Amount: 50.0},
	}

	out := RecomputeAggregateSubjects(rows, newDict(), 2026)

	parent := findRow(out, "电商", 5, "", parentCode)
	if parent == nil {
		t.Fatal("子项存在但无父项时, 应自动补建父项 row")
	}
	if parent.Amount != 150.0 {
		t.Errorf("自动补建父项 amount = 100+50 = 150, got %v", parent.Amount)
	}
	if parent.SortOrder != 9999 {
		t.Errorf("自动补建 row SortOrder 必须 9999 sentinel, got %d", parent.SortOrder)
	}
	if parent.SubjectName != "仓储物流费用" {
		t.Errorf("应从 dict 取中文名 '仓储物流费用', got %q", parent.SubjectName)
	}
	if parent.SubjectCategory != "成本" {
		t.Errorf("应从 dict 取 category '成本', got %q", parent.SubjectCategory)
	}
	if parent.SubjectLevel != 3 {
		t.Errorf("自动补建 row Level 必须 3, got %d", parent.SubjectLevel)
	}
}

// case 3: 完全没子项 → 不应补建父项 row
func TestRecomputeAggregate_NoChildrenNoInsert(t *testing.T) {
	rows := []FinanceRow{
		{Year: 2026, Month: 5, Department: "电商", SubjectCode: "REV_TOTAL", Amount: 1000.0},
	}

	out := RecomputeAggregateSubjects(rows, newDict(), 2026)

	if findRow(out, "电商", 5, "", parentCode) != nil {
		t.Fatal("没有任何子项时, 不应补建父项 row")
	}
}

// case 4: REV_TOTAL=0 时 ratio 应为 nil (防除零)
func TestRecomputeAggregate_NoDivisionByZero(t *testing.T) {
	rows := []FinanceRow{
		{Year: 2026, Month: 5, Department: "电商", SubjectCode: childA, Amount: 100.0},
		{Year: 2026, Month: 5, Department: "电商", SubjectCode: "REV_TOTAL", Amount: 0.0},
	}

	out := RecomputeAggregateSubjects(rows, newDict(), 2026)
	parent := findRow(out, "电商", 5, "", parentCode)
	if parent == nil {
		t.Fatal("应补建父项")
	}
	if parent.Ratio != nil {
		t.Errorf("REV_TOTAL=0 时 ratio 必须 nil 防除零, got %v", *parent.Ratio)
	}
}

// case 5: 多个 (dept, month, subChannel) 独立聚合, 不串
func TestRecomputeAggregate_IsolatesByKeyTuple(t *testing.T) {
	rows := []FinanceRow{
		// 电商 5月
		{Year: 2026, Month: 5, Department: "电商", SubjectCode: childA, Amount: 100.0},
		{Year: 2026, Month: 5, Department: "电商", SubjectCode: childB, Amount: 50.0},
		// 社媒 5月 (不同 dept)
		{Year: 2026, Month: 5, Department: "社媒", SubjectCode: childA, Amount: 200.0},
		// 电商 6月 (不同 month)
		{Year: 2026, Month: 6, Department: "电商", SubjectCode: childA, Amount: 300.0},
		// 电商 5月 sub_channel='促销'
		{Year: 2026, Month: 5, Department: "电商", SubChannel: "促销", SubjectCode: childA, Amount: 50.0},
	}

	out := RecomputeAggregateSubjects(rows, newDict(), 2026)

	// 电商 5月 sub_channel='' = 100+50 = 150
	if p := findRow(out, "电商", 5, "", parentCode); p == nil || p.Amount != 150.0 {
		t.Errorf("电商 5月 sub='' 应聚合 150, got %v", p)
	}
	// 社媒 5月 sub='' = 200
	if p := findRow(out, "社媒", 5, "", parentCode); p == nil || p.Amount != 200.0 {
		t.Errorf("社媒 5月 应聚合 200, got %v", p)
	}
	// 电商 6月 sub='' = 300 (不跟 5月混)
	if p := findRow(out, "电商", 6, "", parentCode); p == nil || p.Amount != 300.0 {
		t.Errorf("电商 6月 应聚合 300, got %v", p)
	}
	// 电商 5月 sub='促销' = 50 (不跟 sub='' 混)
	if p := findRow(out, "电商", 5, "促销", parentCode); p == nil || p.Amount != 50.0 {
		t.Errorf("电商 5月 sub='促销' 应聚合 50, got %v", p)
	}
}

// case 6: dict 里无 ParentCode 时, fallback name = ParentCode 自身
func TestRecomputeAggregate_FallbackWhenDictMissing(t *testing.T) {
	rows := []FinanceRow{
		{Year: 2026, Month: 5, Department: "电商", SubjectCode: childA, Amount: 100.0},
	}

	emptyDict := map[string]*DictEntry{} // 没有 parentCode entry
	out := RecomputeAggregateSubjects(rows, emptyDict, 2026)

	parent := findRow(out, "电商", 5, "", parentCode)
	if parent == nil {
		t.Fatal("应仍补建父项 (即使 dict 缺)")
	}
	if parent.SubjectName != parentCode {
		t.Errorf("dict 缺时 SubjectName fallback 到 code 自身, got %q", parent.SubjectName)
	}
	if parent.Amount != 100.0 {
		t.Errorf("amount 仍正确 SUM, got %v", parent.Amount)
	}
}

// case 7: 空 rows 输入不 panic
func TestRecomputeAggregate_EmptyInput(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("空输入不应 panic: %v", r)
		}
	}()
	out := RecomputeAggregateSubjects([]FinanceRow{}, newDict(), 2026)
	if len(out) != 0 {
		t.Errorf("空输入应返空, got %d rows", len(out))
	}
}
