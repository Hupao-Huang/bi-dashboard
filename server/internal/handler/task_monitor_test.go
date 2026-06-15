package handler

// task_monitor_test.go — task_monitor.go 和 sync.go 的 pure 函数测试
// 已 Read summarizeToolOutput (sync.go 259-281), generateTaskID

import (
	"strings"
	"testing"
)

// === summarizeToolOutput (sync.go line 259-281) ===
// 多行 → "N行输出，最后一行: <last>"; 单行 → 直接返该行; 空 → ""

func TestSummarizeToolOutput(t *testing.T) {
	cases := map[string]string{
		"":           "",
		"   \n  ":     "",
		"single line": "single line",
		"abc\n\ndef": "2行输出，最后一行: def", // 空行被跳
	}
	for input, want := range cases {
		if got := summarizeToolOutput(input); got != want {
			t.Errorf("summarizeToolOutput(%q)=%q want %q", input, got, want)
		}
	}
}

func TestSummarizeToolOutputMultiline(t *testing.T) {
	out := summarizeToolOutput("line1\nline2\nline3")
	if !strings.Contains(out, "3行输出") {
		t.Errorf("应含 '3行输出', got %q", out)
	}
	if !strings.Contains(out, "line3") {
		t.Errorf("应含最后一行 'line3', got %q", out)
	}
}

// === generateTaskID (line 600+) — 跟 generateCaptchaID 类似, 16 byte hex ===
func TestGenerateTaskIDLengthAndUniqueness(t *testing.T) {
	id1 := generateTaskID()
	if len(id1) == 0 {
		t.Fatal("task ID 不应为空")
	}
	id2 := generateTaskID()
	if id1 == id2 {
		t.Error("两次 generateTaskID 应不同")
	}
}
