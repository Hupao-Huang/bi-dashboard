package handler

// sync_health_test.go — sync.go runSync/runSyncTool/summarizeToolOutput +
//   task_health.go runHealthCheck/StartTaskHealthMonitor + notifyFeedbackReply
// 已 Read sync.go (line 152 runSync, 211 runSyncTool, 259 summarizeToolOutput).
// 已 Read task_health.go (line 30 StartTaskHealthMonitor, 48 runHealthCheck).
// 已 Read feedback.go (line 325 notifyFeedbackReply).

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ summarizeToolOutput ============

func TestSummarizeToolOutputEmpty(t *testing.T) {
	if got := summarizeToolOutput(""); got != "" {
		t.Errorf("空 output 应空, got %q", got)
	}
}

func TestSummarizeToolOutputBlankLines(t *testing.T) {
	if got := summarizeToolOutput("\n\n\n   \n"); got != "" {
		t.Errorf("全空白行应空, got %q", got)
	}
}

func TestSummarizeToolOutputSingleLine(t *testing.T) {
	got := summarizeToolOutput("import 完成")
	if got != "import 完成" {
		t.Errorf("单行应原样返回, got %q", got)
	}
}

func TestSummarizeToolOutputMultiLine(t *testing.T) {
	output := "开始\n处理...\n完成 100 条"
	got := summarizeToolOutput(output)
	if !strings.Contains(got, "3行输出") || !strings.Contains(got, "完成 100 条") {
		t.Errorf("多行应汇总, got %q", got)
	}
}

func TestSummarizeToolOutputCRLF(t *testing.T) {
	got := summarizeToolOutput("a\r\nb\r\n")
	if !strings.Contains(got, "b") {
		t.Errorf("CRLF 应正常处理, got %q", got)
	}
}

// ============ runSyncTool ============

func TestRunSyncToolNonExistentExe(t *testing.T) {
	// 调一个不存在的 exe → err 分支
	res := runSyncTool("/no/such/dir", "definitely-not-exist.exe", "20260510")
	if res.Status == "成功" {
		t.Errorf("不存在 exe 不应成功, got %+v", res)
	}
	if res.Name != "definitely-not-exist.exe" {
		t.Errorf("Name 应保留, got %q", res.Name)
	}
}

// ============ runSync ============

func TestRunSyncFinishesWithoutPanic(t *testing.T) {
	// runSync 跑完所有 tools (会失败但应清理 syncRunning)
	syncMu.Lock()
	syncRunning = true // simulate running before runSync
	syncMu.Unlock()

	dh := &DashboardHandler{} // 没 DingToken/Secret → sendDingTalk 不发
	dh.runSync("20260510")

	// 跑完 syncRunning 应被清成 false
	syncMu.Lock()
	running := syncRunning
	syncMu.Unlock()
	if running {
		t.Error("runSync defer 应清 syncRunning")
	}
}

// ============ runHealthCheck ============

func TestRunHealthCheckNoNewFails(t *testing.T) {
	// loadSchtasksStatus 在测试环境下大概率会失败/返回少量任务
	// 关键是不 panic + 优雅返回
	db, _, _ := sqlmock.New()
	defer db.Close()

	dh := &DashboardHandler{DB: db}
	dh.runHealthCheck() // 不应 panic
}

// ============ StartTaskHealthMonitor 早返 ============

func TestStartTaskHealthMonitorNoNotifier(t *testing.T) {
	// Notifier=nil → 立即 return
	dh := &DashboardHandler{}
	dh.StartTaskHealthMonitor() // 不应 panic + 立即返回
}

// ============ notifyFeedbackReply ============

func TestNotifyFeedbackReplyFeedbackNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM feedback f\s+LEFT JOIN users u`).
		WillReturnError(errBoom)

	dh := &DashboardHandler{DB: db}
	dh.notifyFeedbackReply(99, "回复内容", "管理员")
	// 不应 panic
}

func TestNotifyFeedbackReplyNoDingtalkBinding(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// feedback 存在但 submitter 没绑钉钉
	mock.ExpectQuery(`FROM feedback f\s+LEFT JOIN users u`).
		WillReturnRows(sqlmock.NewRows([]string{"title", "did", "rn"}).
			AddRow("反馈标题", nil, "用户A"))

	dh := &DashboardHandler{DB: db}
	dh.notifyFeedbackReply(1, "回复", "管理员") // 不应 panic, 静默 return
}
