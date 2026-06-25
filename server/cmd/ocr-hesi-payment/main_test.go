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
