package business

// parser_helpers_test.go — parser.go 9+ 个纯 helper 函数测试
// 已 Read parser.go:
//   - isValidHeader (354) / isGroupHeader (361) / detectLevel (384)
//   - safeCol (477) / parseNum (485) / parsePct (508) / isExcelError (536)
//   - hasAnyValue (551) / nullIfNil (635) / FormatTimestamp (643)

import (
	"testing"
	"time"
)

// ---------- safeCol ----------

func TestSafeCol(t *testing.T) {
	row := []string{"a", "b", "c"}
	cases := map[int]string{0: "a", 1: "b", 2: "c", 3: "", 4: "", -1: "", -100: ""}
	for i, want := range cases {
		if got := safeCol(row, i); got != want {
			t.Errorf("safeCol(%d)=%q want %q", i, got, want)
		}
	}
	// nil row
	if got := safeCol(nil, 0); got != "" {
		t.Errorf("safeCol(nil, 0)=%q want empty", got)
	}
	// empty row
	if got := safeCol([]string{}, 0); got != "" {
		t.Errorf("safeCol([]) 应返 empty, got %q", got)
	}
}

// ---------- parseNum ----------

func TestParseNum(t *testing.T) {
	cases := []struct {
		in   string
		want *float64
	}{
		// 正常数字
		{"123.45", floatPtr(123.45)},
		{"100", floatPtr(100)},
		{"0", floatPtr(0)},
		// 千位逗号
		{"1,234,567.89", floatPtr(1234567.89)},
		{"1,000", floatPtr(1000)},
		// 空白
		{"  100  ", floatPtr(100)},
		{"1 000", floatPtr(1000)}, // 空格被 strip
		// 负数 — 括号 (123.45) → -123.45
		{"(123.45)", floatPtr(-123.45)},
		{"(100)", floatPtr(-100)},
		// 普通 - 前缀
		{"-50.5", floatPtr(-50.5)},
		// 空 / Excel error
		{"", nil},
		{"#REF!", nil},
		{"#DIV/0!", nil},
		{"#N/A", nil},
		// 非数字
		{"abc", nil},
		{"--", nil},
	}
	for _, tc := range cases {
		got := parseNum(tc.in)
		if !floatEq(got, tc.want) {
			t.Errorf("parseNum(%q)=%v want %v", tc.in, deref(got), deref(tc.want))
		}
	}
}

// ---------- parsePct ----------

func TestParsePct(t *testing.T) {
	cases := []struct {
		in   string
		want *float64
	}{
		// 百分比
		{"61.98%", floatPtr(0.6198)},
		{"100%", floatPtr(1.0)},
		{"0%", floatPtr(0)},
		{"-50%", floatPtr(-0.5)},
		// 裸数字 (无 %)
		{"0.5", floatPtr(0.5)},
		{"1.0", floatPtr(1.0)},
		// 千位 + 百分号
		{"1,234.56%", floatPtr(12.3456)},
		// 括号负 — 源码先 HasSuffix "%" 判断, "(20%)" 结尾是 ")" pctMode=false,
		// 然后剥括号成 "-20%", ParseFloat 失败 → nil (源码 line 519/523 顺序限制)
		// 空白
		{"  50%  ", floatPtr(0.5)},
		// 空 / Excel error
		{"", nil},
		{"#DIV/0!", nil},
		// 非数字
		{"abc%", nil},
	}
	for _, tc := range cases {
		got := parsePct(tc.in)
		if !floatEq(got, tc.want) {
			t.Errorf("parsePct(%q)=%v want %v", tc.in, deref(got), deref(tc.want))
		}
	}
}

// ---------- isExcelError ----------

func TestIsExcelError(t *testing.T) {
	excelErrors := []string{"#REF!", "#DIV/0!", "#VALUE!", "#N/A", "#NAME?", "#NUM!", "#NULL!"}
	for _, e := range excelErrors {
		if !isExcelError(e) {
			t.Errorf("%q 应识别为 Excel error", e)
		}
	}
	// '#' 开头都视为 error (源码 line 548)
	if !isExcelError("#unknown") {
		t.Error("'#' 开头都应视为 error")
	}
	// 不是 error
	notErrors := []string{"", "abc", "100", "#"}
	for _, n := range notErrors {
		if n == "#" {
			// '#' 单字符: 0 < len 但 t[0]=='#' → 进入第 2 个 if 不匹配, 走 strings.HasPrefix(t, "#") = true
			// 实际源码 line 548: return strings.HasPrefix(t, "#") — 所以 "#" 也是 error
			if !isExcelError(n) {
				t.Errorf("'#' 单字符按源码 line 548 应是 error")
			}
			continue
		}
		if isExcelError(n) {
			t.Errorf("%q 不应是 error", n)
		}
	}
}

// ---------- detectLevel ----------

func TestDetectLevel(t *testing.T) {
	// SKU 单字符 → level 3
	level3Single := []string{"S", "A", "B", "C", "其他", "新品"}
	for _, s := range level3Single {
		if got := detectLevel(s, s); got != 3 {
			t.Errorf("detectLevel(%q)=%d want 3 (SKU 分级)", s, got)
		}
	}

	// "A、xxx" / "B、xxx" → level 3
	level3Prefix := []string{"A、产品", "B、毛利", "C、净利", "A.毛利"}
	for _, s := range level3Prefix {
		if got := detectLevel(s, s); got != 3 {
			t.Errorf("detectLevel(%q)=%d want 3 (level3 prefix)", s, got)
		}
	}

	// "一、" / "二、" → level 1
	level1Num := []string{"一、营业收入", "二、营业成本", "三、营业税金", "十、其他"}
	for _, s := range level1Num {
		if got := detectLevel(s, s); got != 1 {
			t.Errorf("detectLevel(%q)=%d want 1 (level1 数字前缀)", s, got)
		}
	}

	// "减：" 前缀 → level 1
	deductCases := []string{"减：销售费用", "减:管理费用"}
	for _, s := range deductCases {
		if got := detectLevel(s, s); got != 1 {
			t.Errorf("detectLevel(%q)=%d want 1 (减 前缀)", s, got)
		}
	}

	// 计算项关键词 → level 1
	calcCases := []string{"营业毛利", "运营利润", "利润总额", "净利润", "扣税前营业毛利", "本期净利润"}
	for _, s := range calcCases {
		if got := detectLevel(s, s); got != 1 {
			t.Errorf("detectLevel(%q)=%d want 1 (计算项关键词)", s, got)
		}
	}

	// 默认 level 2
	level2Cases := []string{"主营产品销售", "广告费", "工资", "原材料"}
	for _, s := range level2Cases {
		if got := detectLevel(s, s); got != 2 {
			t.Errorf("detectLevel(%q)=%d want 2 (默认)", s, got)
		}
	}
}

// ---------- isGroupHeader ----------

func TestIsGroupHeader(t *testing.T) {
	// 白名单 = true
	groupHeaders := []string{"GMV数据", "财务数据", "品牌费用", "管理费用", "财务费用", "人数", "人均薪酬"}
	for _, h := range groupHeaders {
		if !isGroupHeader([]string{h}) {
			t.Errorf("%q 应识别为 group header", h)
		}
	}
	// 数据行 = false (跑哥 2026-04-30 反馈: 不能误判数据科目)
	dataRows := [][]string{
		{"样品费用"},
		{"主营业务收入"},
		{"广告费"},
		{""},
		nil,
		{},
	}
	for _, r := range dataRows {
		if isGroupHeader(r) {
			t.Errorf("%v 不应是 group header (数据行)", r)
		}
	}
}

// ---------- isValidHeader ----------

func TestIsValidHeader(t *testing.T) {
	// 全为空的 row → unknown layout → false
	if isValidHeader([]string{"", ""}) {
		t.Error("全空 row 不应识别为 valid header")
	}
}

// ---------- hasAnyValue ----------

func TestHasAnyValue(t *testing.T) {
	v := 100.0

	// 全 nil → false
	if hasAnyValue(BudgetRow{}) {
		t.Error("全 nil 应 hasAnyValue=false")
	}

	// 任一字段非 nil → true
	cases := []BudgetRow{
		{Budget: &v},
		{RatioBudget: &v},
		{Actual: &v},
		{RatioActual: &v},
		{BudgetYearStart: &v},
		{RatioYearStart: &v},
		{AchievementRate: &v},
	}
	for i, br := range cases {
		if !hasAnyValue(br) {
			t.Errorf("case %d: 含一个非 nil, 应返 true", i)
		}
	}
}

// ---------- nullIfNil ----------

func TestNullIfNil(t *testing.T) {
	if nullIfNil(nil) != nil {
		t.Error("nullIfNil(nil) 应返 nil")
	}
	v := 3.14
	got := nullIfNil(&v)
	if g, ok := got.(float64); !ok || g != 3.14 {
		t.Errorf("nullIfNil(&3.14) 应返 float64 3.14, got %v", got)
	}
}

// ---------- FormatTimestamp ----------

func TestFormatTimestamp(t *testing.T) {
	tm := time.Date(2026, 5, 10, 15, 30, 45, 0, time.UTC)
	got := FormatTimestamp(tm)
	want := "2026-05-10 15:30:45"
	if got != want {
		t.Errorf("FormatTimestamp=%q want %q", got, want)
	}
}

// helpers
func floatPtr(v float64) *float64 { return &v }

func floatEq(a, b *float64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	diff := *a - *b
	if diff < 0 {
		diff = -diff
	}
	return diff < 1e-9
}

func deref(p *float64) interface{} {
	if p == nil {
		return "<nil>"
	}
	return *p
}
