package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncate(t *testing.T) {
	// (a) 600 个中文字符截断到 500 rune
	long := strings.Repeat("付款", 300) // 600 runes
	out := truncate(long, 500)
	if utf8.RuneCountInString(out) != 500 {
		t.Errorf("期望 500 rune, 实际 %d", utf8.RuneCountInString(out))
	}
	if !utf8.ValidString(out) {
		t.Error("截断后字符串不是合法 UTF-8")
	}

	// (b) 短 ASCII 字符串原样返回
	short := "hello"
	if truncate(short, 500) != short {
		t.Errorf("短字符串应原样返回, 实际 %q", truncate(short, 500))
	}
}

// TestIsPaymentScreenshotName 验证付款截图文件名判定: 含付款类关键词才OCR,
// 但"对账单"(含"账单"会被关键词误命中)必须排除 —— 它金额是对账总额非单次实付, 会误判超付。(#6, 跑哥 2026-06-25)
func TestIsPaymentScreenshotName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"微信付款截图.jpg", true},
		{"支付宝支付记录.png", true},
		{"转账回单.jpeg", true},
		{"微信账单.png", true},          // "账单" 是合法付款凭证 (微信/支付宝账单)
		{"潍坊中百订单对账单.png", false}, // "对账单" 必须排除 (#6 误报根因)
		{"供应商6月对账明细.jpg", false}, // 任何含"对账"的都排除
		{"增值税电子发票.jpg", false},    // 无付款关键词
		{"报销说明.png", false},        // 无付款关键词
	}
	for _, c := range cases {
		if got := isPaymentScreenshotName(c.name); got != c.want {
			t.Errorf("isPaymentScreenshotName(%q) = %v, 期望 %v", c.name, got, c.want)
		}
	}
}
