package importutil

import "testing"

func TestParseFloat(t *testing.T) {
	cases := map[string]float64{
		"":            0,
		"-":           0,
		" - ":         0,
		"123":         123,
		"123.45":      123.45,
		"1,234.56":    1234.56,
		"1,234,567":   1234567,
		"50%":         50, // 跑哥业务: 百分号去掉直接当数字, 不除 100
		" 99.9 ":      99.9,
		"abc":         0,
		"-12.5":       -12.5,
		"1.23e3":      1230, // 科学计数也支持
	}
	for input, want := range cases {
		if got := ParseFloat(input); got != want {
			t.Errorf("ParseFloat(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestParseInt(t *testing.T) {
	cases := map[string]int{
		"":         0,
		"-":        0,
		"123":      123,
		"1,234":    1234,
		" 99 ":     99,
		"abc":      0,
		"-7":       -7,
		"0":        0,
		"1,000,000": 1000000,
	}
	for input, want := range cases {
		if got := ParseInt(input); got != want {
			t.Errorf("ParseInt(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestCellStr(t *testing.T) {
	row := []string{"a ", " b", "  c  ", ""}
	cases := []struct {
		idx  int
		want string
	}{
		{0, "a"},
		{1, "b"},
		{2, "c"},
		{3, ""},
		{-1, ""},  // 负索引保护
		{99, ""},  // 越界保护
		{4, ""},
	}
	for _, c := range cases {
		if got := CellStr(row, c.idx); got != c.want {
			t.Errorf("CellStr(row, %d) = %q, want %q", c.idx, got, c.want)
		}
	}
}

func TestFormatDate(t *testing.T) {
	cases := map[string]string{
		"20260510":   "2026-05-10",
		"20251231":   "2025-12-31",
		"":           "",                // 空保留
		"2026-05-10": "2026-05-10",      // 已格式化保留
		"abc":        "abc",             // 非 8 位原样
		"2026":       "2026",            // 短串原样
		"202605101":  "202605101",       // 9 位原样 (不是 YYYYMMDD)
	}
	for input, want := range cases {
		if got := FormatDate(input); got != want {
			t.Errorf("FormatDate(%q) = %q, want %q", input, got, want)
		}
	}
}
