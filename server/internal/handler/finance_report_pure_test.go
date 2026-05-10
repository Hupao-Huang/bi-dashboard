package handler

// finance_report_pure_test.go — finance_report.go pure 函数测试
// 已 Read finance_report.go: sortIntsAsc(77-85) / placeholders(87-93) / nullStr(95-100) /
// trimStrings(102-110) / displayName(1070-1075) / isGmvCategory(1077) /
// findChannelSeries(1079-1086) / urlEscape(1088-...)

import (
	"database/sql"
	"testing"
)

// === sortIntsAsc (line 77-85) — 简单冒泡排序 ===
func TestSortIntsAsc(t *testing.T) {
	cases := []struct {
		input []int
		want  []int
	}{
		{[]int{}, []int{}},
		{[]int{1}, []int{1}},
		{[]int{3, 1, 2}, []int{1, 2, 3}},
		{[]int{5, 5, 5}, []int{5, 5, 5}},
		{[]int{-3, 0, -1}, []int{-3, -1, 0}},
		{[]int{10, 9, 8, 7, 6, 5, 4, 3, 2, 1}, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
	}
	for _, c := range cases {
		input := make([]int, len(c.input))
		copy(input, c.input)
		sortIntsAsc(input)
		for i := range input {
			if input[i] != c.want[i] {
				t.Errorf("sortIntsAsc(%v) → %v want %v", c.input, input, c.want)
				break
			}
		}
	}
}

// === placeholders (line 87-93) — 生成 SQL "?,?,?" ===
func TestPlaceholders(t *testing.T) {
	cases := map[int]string{
		0:  "",
		-1: "",
		1:  "?",
		2:  "?,?",
		3:  "?,?,?",
		5:  "?,?,?,?,?",
	}
	for n, want := range cases {
		if got := placeholders(n); got != want {
			t.Errorf("placeholders(%d)=%q want %q", n, got, want)
		}
	}
}

// === nullStr (line 95-100) ===
func TestNullStr(t *testing.T) {
	cases := []struct {
		ns   sql.NullString
		want string
	}{
		{sql.NullString{String: "abc", Valid: true}, "abc"},
		{sql.NullString{String: "abc", Valid: false}, ""}, // !Valid 返空 (即使 String 有值)
		{sql.NullString{String: "", Valid: true}, ""},
		{sql.NullString{}, ""},
	}
	for _, c := range cases {
		if got := nullStr(c.ns); got != c.want {
			t.Errorf("nullStr(%+v)=%q want %q", c.ns, got, c.want)
		}
	}
}

// === trimStrings (line 102-110) — trim 空白 + 跳过空 ===
func TestTrimStrings(t *testing.T) {
	cases := []struct {
		input []string
		want  []string
	}{
		{[]string{"a", " b ", "  ", "c"}, []string{"a", "b", "c"}}, // 空白行跳过
		{[]string{"", " "}, nil},                                     // 全空返 nil
		{[]string{}, nil},
		{[]string{"x"}, []string{"x"}},
	}
	for _, c := range cases {
		got := trimStrings(c.input)
		if len(got) != len(c.want) {
			t.Errorf("trimStrings(%v): len=%d want %d", c.input, len(got), len(c.want))
			continue
		}
		for i, v := range got {
			if v != c.want[i] {
				t.Errorf("trimStrings(%v)[%d]=%q want %q", c.input, i, v, c.want[i])
			}
		}
	}
}

// === displayName (line 1070-1075) ===
// SubChannel 非空 → "· " + SubChannel; 空 → row.Name
func TestDisplayName(t *testing.T) {
	cases := []struct {
		row  FinReportRow
		want string
	}{
		{FinReportRow{Name: "营业收入"}, "营业收入"},
		{FinReportRow{Name: "营业收入", SubChannel: "TOC"}, "· TOC"}, // 优先 SubChannel
		{FinReportRow{SubChannel: "礼品"}, "· 礼品"},
		{FinReportRow{}, ""},
	}
	for _, c := range cases {
		if got := displayName(c.row); got != c.want {
			t.Errorf("displayName(%+v)=%q want %q", c.row, got, c.want)
		}
	}
}

// === isGmvCategory (line 1077) ===
func TestIsGmvCategory(t *testing.T) {
	if !isGmvCategory("GMV") {
		t.Error("'GMV' 应为 GMV category")
	}
	if isGmvCategory("成本") {
		t.Error("'成本' 不是 GMV category")
	}
	if isGmvCategory("") {
		t.Error("空 string 不应是 GMV")
	}
	if isGmvCategory("gmv") {
		t.Error("严格大写敏感, 'gmv' 不算 (源码: cat == 'GMV' 严格相等)")
	}
}

// === findChannelSeries (line 1079-1086) ===
// 已 Read FinSeries (line 55-58): { RangeTotal FinCell; Cells map[string]FinCell }
// 已 Read FinChannelSeries (line 60-63): { Channel string; Series FinSeries }
func TestFindChannelSeries(t *testing.T) {
	list := []FinChannelSeries{
		{Channel: "电商"},
		{Channel: "社媒"},
	}
	// 命中
	if got := findChannelSeries(list, "电商"); got == nil {
		t.Error("'电商' 应命中, got nil")
	}
	if got := findChannelSeries(list, "社媒"); got == nil {
		t.Error("'社媒' 应命中, got nil")
	}
	// 未命中
	if got := findChannelSeries(list, "不存在"); got != nil {
		t.Error("不存在的渠道应返 nil")
	}
	// 空列表
	if got := findChannelSeries(nil, "电商"); got != nil {
		t.Error("空列表应返 nil")
	}
}

// === urlEscape (line 1088-...) ===
// 安全字符 -_.~ 0-9 a-z A-Z 直接保留, 其他 → %XX
func TestUrlEscape(t *testing.T) {
	cases := map[string]string{
		"abc":      "abc",      // 字母原样
		"abc123":   "abc123",   // 数字原样
		"a-b_c.d~": "a-b_c.d~", // 4 个安全标点
		" ":        "%20",      // 空格 → %20
		"中":        "%E4%B8%AD", // 中文 UTF-8 3 字节
		"":         "",
	}
	for input, want := range cases {
		if got := urlEscape(input); got != want {
			t.Errorf("urlEscape(%q)=%q want %q", input, got, want)
		}
	}
}
