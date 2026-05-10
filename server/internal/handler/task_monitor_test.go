package handler

// task_monitor_test.go — task_monitor.go 和 sync.go 的 pure 函数测试
// 已 Read parseStatusFromLog (line 385-394), formatDuration (499-512), summarizeToolOutput (sync.go 259-281)

import (
	"strings"
	"testing"
	"time"
)

// === parseStatusFromLog (line 385-394) ===
// 优先级: 含失败/error/fatal → failed; 含完成/成功 → success; 其他 → waiting

func TestParseStatusFromLog(t *testing.T) {
	cases := map[string]string{
		"任务 X 失败":              "failed",
		"connection error 后退出": "failed", // 小写 error
		"FATAL crash":           "failed", // 大写 fatal (lower 化后命中)
		"任务完成":                  "success",
		"操作成功":                  "success",
		"启动中, 暂未结束":             "waiting",
		"":                       "waiting",
		// 源码精确: 检查 中文"失败" 或 lower("error")/(lower("fatal")
		// "failed" 英文 单词不在检查列表, 不会命中
		"任务失败 但操作完成":      "failed", // 中文"失败"优先
		"操作完成 + error 都有":     "failed", // error 命中第一组
	}
	for input, want := range cases {
		if got := parseStatusFromLog(input); got != want {
			t.Errorf("parseStatusFromLog(%q)=%q want %q", input, got, want)
		}
	}
}

// === formatDuration (line 499-512) ===
// 时分秒分级: h>0 时分秒, m>0 分秒, 否则秒

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0秒"},
		{45 * time.Second, "45秒"},
		{2 * time.Minute, "2分0秒"},
		{2*time.Minute + 30*time.Second, "2分30秒"},
		{1 * time.Hour, "1时0分0秒"},
		{1*time.Hour + 5*time.Minute + 30*time.Second, "1时5分30秒"},
		{59 * time.Second, "59秒"},
		{500 * time.Millisecond, "1秒"}, // round
	}
	for _, c := range cases {
		if got := formatDuration(c.d); got != c.want {
			t.Errorf("formatDuration(%v)=%q want %q", c.d, got, c.want)
		}
	}
}

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
