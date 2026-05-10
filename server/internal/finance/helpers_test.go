package finance

// helpers_test.go — finance/parser.go 内部 helper pure 函数
// 已 Read parser.go: isContainerLevel2 (83-89), cleanNumStr (92-96), parsePercentOrFloat (99-109),
// CollectDeptMonths (671-680)

import (
	"testing"
)

// === isContainerLevel2 (line 83-89) ===
// 业务: COST_MAIN / SALES_EXP / MGMT_EXP / GMV_TOTAL 这 4 个 Level 2 有子项

func TestIsContainerLevel2(t *testing.T) {
	containers := []string{"COST_MAIN", "SALES_EXP", "MGMT_EXP", "GMV_TOTAL"}
	for _, c := range containers {
		if !isContainerLevel2(c) {
			t.Errorf("%q 是 container, 应返 true", c)
		}
	}
	notContainers := []string{"REV_MAIN", "PROFIT_GROSS", "PROFIT_OP", "TAX_INCOME", "", "RAND_X"}
	for _, c := range notContainers {
		if isContainerLevel2(c) {
			t.Errorf("%q 不是 container, 应返 false", c)
		}
	}
}

// === cleanNumStr (line 92-96) — trim + 去千分位逗号 ===
// 注意: 源码只去 trim/逗号, 不去 % (那是 parsePercentOrFloat 的事)

func TestCleanNumStr(t *testing.T) {
	cases := map[string]string{
		"1,234.56":     "1234.56",
		"1,000,000":    "1000000",
		" 12 ":         "12",       // trim
		" 1,234 ":      "1234",     // trim + 逗号
		"":             "",
		"  ":           "",
		"abc":          "abc",      // 非数字原样 (源码不校验)
	}
	for input, want := range cases {
		if got := cleanNumStr(input); got != want {
			t.Errorf("cleanNumStr(%q)=%q want %q", input, got, want)
		}
	}
}

// === parsePercentOrFloat (line 99-109) — % 后缀转 0.288, 否则普通解析 ===

func TestParsePercentOrFloat(t *testing.T) {
	// 百分比
	v, err := parsePercentOrFloat("28.80%")
	if err != nil || v < 0.287 || v > 0.289 {
		t.Errorf("'28.80%%'=%v err=%v want ≈0.288", v, err)
	}
	// 整数百分比
	v, err = parsePercentOrFloat("100%")
	if err != nil || v != 1 {
		t.Errorf("'100%%'=%v err=%v want 1", v, err)
	}
	// 不含 % 普通解析
	v, err = parsePercentOrFloat("0.288")
	if err != nil || v != 0.288 {
		t.Errorf("'0.288'=%v err=%v", v, err)
	}
	// 千分位
	v, err = parsePercentOrFloat("1,234.56")
	if err != nil || v != 1234.56 {
		t.Errorf("'1,234.56'=%v err=%v", v, err)
	}
	// 错误格式应返 err
	if _, err := parsePercentOrFloat("abc"); err == nil {
		t.Error("非数字应返 err")
	}
}

// === CollectDeptMonths (line 671-680) ===
// 收集每个 dept 出现过的 month 集

func TestCollectDeptMonths(t *testing.T) {
	result := &ParseResult{
		Rows: []FinanceRow{
			{Department: "电商", Month: 1},
			{Department: "电商", Month: 2},
			{Department: "电商", Month: 1}, // 重复 month, set 去重
			{Department: "社媒", Month: 5},
			{Department: "社媒", Month: 6},
			{Department: "线下", Month: 12},
		},
	}
	got := CollectDeptMonths(result)

	// 电商 应有 1, 2 (不是 1, 2, 1)
	if len(got["电商"]) != 2 {
		t.Errorf("电商 month set 应有 2 个不同值, got %v", got["电商"])
	}
	if !got["电商"][1] || !got["电商"][2] {
		t.Errorf("电商 应含 1, 2: %v", got["电商"])
	}
	// 社媒 5, 6
	if !got["社媒"][5] || !got["社媒"][6] {
		t.Errorf("社媒 应含 5, 6: %v", got["社媒"])
	}
	// 线下 12
	if !got["线下"][12] {
		t.Error("线下 应含 12")
	}
}

func TestCollectDeptMonthsEmptyResult(t *testing.T) {
	got := CollectDeptMonths(&ParseResult{})
	if len(got) != 0 {
		t.Errorf("空 ParseResult 应返空 map, got %v", got)
	}
}
