package finance

// dict_test.go — Level2CodeByDict / MatchLevel3 字典查询单元测试
// 已 Read parser.go line 111-253 实现 + DictEntry struct (line 173-181).

import "testing"

// 共用 dict fixture: 模拟 finance_subject_dict 表的 Level 2 + Level 3 条目
func newTestDict() map[string]*DictEntry {
	return map[string]*DictEntry{
		// Level 2 科目
		"COST_MAIN":     {Code: "COST_MAIN", Name: "营业成本", Level: 2, Parent: ""},
		"SALES_EXP":     {Code: "SALES_EXP", Name: "销售费用", Level: 2, Parent: "", Aliases: []string{"销售"}},
		"MGMT_EXP":      {Code: "MGMT_EXP", Name: "管理费用", Level: 2, Parent: ""},
		"PROFIT_GROSS":  {Code: "PROFIT_GROSS", Name: "营业毛利", Level: 2, Parent: ""},

		// Level 3 子项 (parent=COST_MAIN)
		"COST_MAIN.物流费用":     {Code: "COST_MAIN.物流费用", Name: "物流费用", Level: 3, Parent: "COST_MAIN"},
		"COST_MAIN.临时工费用":   {Code: "COST_MAIN.临时工费用", Name: "临时工费用", Level: 3, Parent: "COST_MAIN"},
		"COST_MAIN.发货耗材成本": {Code: "COST_MAIN.发货耗材成本", Name: "发货耗材成本", Level: 3, Parent: "COST_MAIN", Aliases: []string{"包材"}},
		"COST_MAIN.仓储物流费用": {Code: "COST_MAIN.仓储物流费用", Name: "仓储物流费用", Level: 3, Parent: "COST_MAIN"},

		// Level 3 子项 (parent=SALES_EXP) — 跨 parent 测试用
		"SALES_EXP.广告费":      {Code: "SALES_EXP.广告费", Name: "广告费", Level: 3, Parent: "SALES_EXP"},
		"SALES_EXP.推广服务费":   {Code: "SALES_EXP.推广服务费", Name: "推广服务费", Level: 3, Parent: "SALES_EXP", Aliases: []string{"推广费"}},
	}
}

// === Level2CodeByDict (line 111-128) ===

func TestLevel2CodeByDict(t *testing.T) {
	dict := newTestDict()

	cases := []struct {
		name   string
		want   string
		wantOk bool
	}{
		// 精确 Name 匹配
		{"营业成本", "COST_MAIN", true},
		{"销售费用", "SALES_EXP", true},
		// alias 匹配
		{"销售", "SALES_EXP", true},
		// trim
		{" 营业成本 ", "COST_MAIN", true},
		// Level 3 不匹配 (函数明确 if d.Level != 2 continue)
		{"物流费用", "", false},
		// 未匹配
		{"不存在", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := Level2CodeByDict(dict, c.name)
		if got != c.want || ok != c.wantOk {
			t.Errorf("Level2CodeByDict(%q) = (%q,%v) want (%q,%v)",
				c.name, got, ok, c.want, c.wantOk)
		}
	}
}

// === MatchLevel3 (line 210-253) ===
// 源码 4 个分支按顺序:
//   1. dict[parentCode+"."+name] 直接查 (line 215-218)
//   2. parent==parentCode + Level==3 + Name==name 或 alias 匹配 (line 219-231)
//   3. 跨 parent 全局 fallback, 但只接受唯一候选 (line 233-251)
//   4. 都不匹配返 "","" (line 252)

func TestMatchLevel3_DirectKeyMatch(t *testing.T) {
	dict := newTestDict()
	// "COST_MAIN.物流费用" 直接是 dict key
	code, parent := MatchLevel3(dict, "COST_MAIN", "物流费用")
	if code != "COST_MAIN.物流费用" || parent != "COST_MAIN" {
		t.Errorf("直接 key 匹配 fail: got (%q,%q)", code, parent)
	}
}

func TestMatchLevel3_AliasUnderParent(t *testing.T) {
	dict := newTestDict()
	// COST_MAIN.发货耗材成本 alias=["包材"], 直接 key 不命中, 走 line 226-230 alias loop
	code, parent := MatchLevel3(dict, "COST_MAIN", "包材")
	if code != "COST_MAIN.发货耗材成本" || parent != "COST_MAIN" {
		t.Errorf("alias 匹配 fail: got (%q,%q)", code, parent)
	}
}

func TestMatchLevel3_CrossParentFallbackUnique(t *testing.T) {
	dict := newTestDict()
	// 找 "广告费", parentCode="错误parent", parent loop 不命中
	// 走 fallback: 只 SALES_EXP.广告费 唯一候选 → 命中
	code, parent := MatchLevel3(dict, "WRONG_PARENT", "广告费")
	if code != "SALES_EXP.广告费" || parent != "SALES_EXP" {
		t.Errorf("跨 parent 唯一候选 fail: got (%q,%q)", code, parent)
	}
}

func TestMatchLevel3_AmbiguousFallbackReturnsEmpty(t *testing.T) {
	dict := map[string]*DictEntry{
		"P1.X": {Code: "P1.X", Name: "重名", Level: 3, Parent: "P1"},
		"P2.Y": {Code: "P2.Y", Name: "重名", Level: 3, Parent: "P2"},
	}
	// "重名" 在 P1.X 和 P2.Y 都出现, 跨 parent fallback 候选 = 2 → 不取 (避免歧义)
	code, parent := MatchLevel3(dict, "WRONG", "重名")
	if code != "" || parent != "" {
		t.Errorf("多个跨 parent 候选应返空避免歧义, got (%q,%q)", code, parent)
	}
}

func TestMatchLevel3_EmptyName(t *testing.T) {
	dict := newTestDict()
	if code, parent := MatchLevel3(dict, "COST_MAIN", ""); code != "" || parent != "" {
		t.Errorf("空 name 应返空: got (%q,%q)", code, parent)
	}
	if code, parent := MatchLevel3(dict, "COST_MAIN", "   "); code != "" || parent != "" {
		t.Errorf("trim 后空也返空, got (%q,%q)", code, parent)
	}
}

func TestMatchLevel3_NameNotFound(t *testing.T) {
	dict := newTestDict()
	code, parent := MatchLevel3(dict, "COST_MAIN", "不存在科目")
	if code != "" || parent != "" {
		t.Errorf("未匹配返空, got (%q,%q)", code, parent)
	}
}

func TestMatchLevel3_LevelMustBe3(t *testing.T) {
	dict := newTestDict()
	// "营业成本" 是 Level 2 (COST_MAIN), MatchLevel3 不应命中 Level 2 entry
	code, parent := MatchLevel3(dict, "COST_MAIN", "营业成本")
	// 直接 key "COST_MAIN.营业成本" 不存在, parent loop d.Level != 3 也跳过, fallback Level 2 也排除
	if code != "" || parent != "" {
		t.Errorf("MatchLevel3 不应命中 Level 2, got (%q,%q)", code, parent)
	}
}
