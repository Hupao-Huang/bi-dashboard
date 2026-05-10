package handler

// task_monitor_helpers_test.go — readLastNLines / readLastNLinesScanner / formatDuration 纯函数
// 已 Read task_monitor.go (line 397 readLastNLines, 474 readLastNLinesScanner, 499 formatDuration).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ============ readLastNLines ============

func TestReadLastNLinesNonExistent(t *testing.T) {
	got := readLastNLines("/path/that/does/not/exist.log", 10)
	if got != nil {
		t.Errorf("不存在文件应 nil, got %v", got)
	}
}

func TestReadLastNLinesEmpty(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "empty.log")
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := readLastNLines(path, 5)
	if got != nil {
		t.Errorf("空文件应 nil, got %v", got)
	}
}

func TestReadLastNLinesFewerThanN(t *testing.T) {
	// 文件 3 行, 要 10 行 → 返 3 行
	tmp := t.TempDir()
	path := filepath.Join(tmp, "few.log")
	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := readLastNLines(path, 10)
	if len(got) != 3 {
		t.Errorf("len=%d want 3", len(got))
	}
	if got[0] != "line1" || got[2] != "line3" {
		t.Errorf("正序错: %v", got)
	}
}

func TestReadLastNLinesMoreThanN(t *testing.T) {
	// 文件 5 行, 要 3 行 → 返最后 3 行
	tmp := t.TempDir()
	path := filepath.Join(tmp, "more.log")
	content := "a\nb\nc\nd\ne\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := readLastNLines(path, 3)
	if len(got) != 3 {
		t.Errorf("len=%d want 3", len(got))
	}
	if got[0] != "c" || got[1] != "d" || got[2] != "e" {
		t.Errorf("末 3 行错: %v", got)
	}
}

func TestReadLastNLinesLargeFile(t *testing.T) {
	// 大文件 (> 4096 字节) 测块读取分支
	tmp := t.TempDir()
	path := filepath.Join(tmp, "big.log")

	var sb strings.Builder
	for i := 0; i < 1000; i++ {
		sb.WriteString("line ")
		sb.WriteString(string(rune('A' + i%26)))
		sb.WriteString(strings.Repeat("x", 20)) // 让每行 ~25 字节, 共 ~25KB
		sb.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := readLastNLines(path, 5)
	if len(got) != 5 {
		t.Errorf("大文件取末 5 行应 len=5, got %d", len(got))
	}
}

func TestReadLastNLinesWithCRLF(t *testing.T) {
	// Windows CRLF \r\n 应被去掉 \r
	tmp := t.TempDir()
	path := filepath.Join(tmp, "crlf.log")
	content := "win1\r\nwin2\r\nwin3\r\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := readLastNLines(path, 5)
	if len(got) != 3 {
		t.Errorf("len=%d want 3", len(got))
	}
	for _, line := range got {
		if strings.Contains(line, "\r") {
			t.Errorf("行内不应残留 \\r: %q", line)
		}
	}
}

func TestReadLastNLinesSkipBlankLines(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "blank.log")
	content := "x1\n\n\nx2\n   \nx3\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := readLastNLines(path, 10)
	if len(got) != 3 {
		t.Errorf("len=%d want 3, got %v", len(got), got)
	}
}

// ============ readLastNLinesScanner ============

func TestReadLastNLinesScannerHappyPath(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "scanner.log")
	content := "a\nb\nc\nd\ne\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := readLastNLinesScanner(path, 3)
	if len(got) != 3 {
		t.Errorf("len=%d want 3", len(got))
	}
	if got[2] != "e" {
		t.Errorf("末行应 'e', got %v", got)
	}
}

func TestReadLastNLinesScannerNonExistent(t *testing.T) {
	got := readLastNLinesScanner("/no/such/file", 3)
	if got != nil {
		t.Errorf("不存在文件应 nil, got %v", got)
	}
}

func TestReadLastNLinesScannerSkipBlank(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "scanner_blank.log")
	content := "x\n\n\ny\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := readLastNLinesScanner(path, 5)
	if len(got) != 2 {
		t.Errorf("len=%d want 2 (跳空行), got %v", len(got), got)
	}
}

// ============ formatDuration ============

func TestFormatDurationSeconds(t *testing.T) {
	got := formatDuration(45 * time.Second)
	if got != "45秒" {
		t.Errorf("got %q want 45秒", got)
	}
}

func TestFormatDurationMinutes(t *testing.T) {
	got := formatDuration(3*time.Minute + 7*time.Second)
	if got != "3分7秒" {
		t.Errorf("got %q want 3分7秒", got)
	}
}

func TestFormatDurationHours(t *testing.T) {
	got := formatDuration(2*time.Hour + 15*time.Minute + 30*time.Second)
	if got != "2时15分30秒" {
		t.Errorf("got %q want 2时15分30秒", got)
	}
}

func TestFormatDurationZero(t *testing.T) {
	got := formatDuration(0)
	if got != "0秒" {
		t.Errorf("got %q want 0秒", got)
	}
}
