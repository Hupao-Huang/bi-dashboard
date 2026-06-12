package importutil

// 2026-06-12 第三批: 数字解析收编各 import-* 本地实现后的行为锁定

import "testing"

func TestParseFloatCurrencyAndJunk(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"¥1,234.5", 1234.5}, // 原只有抖音处理 ¥, 其他平台静默返 0
		{"￥99", 99},          // 全角人民币符号
		{"1,234元", 1234},     // 原只有客服处理 元
		{" 28.80 %", 28.8},   // 空格+百分号
		{"--", 0},            // 拼多多式占位符
		{"-", 0},
		{"", 0},
		{"1,234.56", 1234.56}, // 原有行为不变
		{"12%", 12},           // 百分号只剥不除100 (既有口径)
	}
	for _, c := range cases {
		if got := ParseFloat(c.in); got != c.want {
			t.Errorf("ParseFloat(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseIntDecimalTolerant(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"3.0", 3},     // 原 Atoi 会失败返 0, 改走 ParseFloat 截断 (import-jd 本地实现语义)
		{"1,234", 1234},
		{"99.9", 99},   // 截断不四舍五入
		{"abc", 0},
		{"", 0},
	}
	for _, c := range cases {
		if got := ParseInt(c.in); got != c.want {
			t.Errorf("ParseInt(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
