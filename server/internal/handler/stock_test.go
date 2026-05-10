package handler

// stock_test.go — stock.go 边界 + helper + state 切换
// 已 Read stock.go (line 1-380):
//   - SyncStockNow (45): 405 / scope / RunSyncStockOnce 409 锁 / exe 调用 (难测 happy)
//   - SyncStockStatus (102): 不调 SQL, 仅读全局状态
//   - tailLines (125): 纯函数
//   - setSyncStockStart / setSyncStockFinish: 全局状态切换

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ tailLines 纯函数 ============

func TestTailLines(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"a\nb\nc", 2, "b\nc"},
		{"a\nb\nc", 5, "a\nb\nc"},                  // n > 行数
		{"single line", 1, "single line"},
		{"a\nb\nc\nd\ne", 3, "c\nd\ne"},
		{"trailing\n", 2, "trailing"},               // TrimRight \n
		{"a\nb\nc\n\r\n", 2, "b\nc"},                // TrimRight \r\n
		{"", 1, ""},
	}
	for _, tc := range cases {
		if got := tailLines(tc.in, tc.n); got != tc.want {
			t.Errorf("tailLines(%q,%d)=%q want %q", tc.in, tc.n, got, tc.want)
		}
	}
}

// ============ setSyncStockStart / setSyncStockFinish ============

func TestSetSyncStockStateTransition(t *testing.T) {
	// 重置全局
	syncStockState.Lock()
	syncStockState.Running = false
	syncStockState.LastError = ""
	syncStockState.LastElapsed = 0
	syncStockState.Unlock()

	setSyncStockStart()
	syncStockState.Lock()
	if !syncStockState.Running {
		t.Error("setSyncStockStart 应将 Running=true")
	}
	if syncStockState.StartedAt.IsZero() {
		t.Error("StartedAt 应被设")
	}
	syncStockState.Unlock()

	setSyncStockFinish(time.Second*5, "test error")
	syncStockState.Lock()
	if syncStockState.Running {
		t.Error("setSyncStockFinish 应将 Running=false")
	}
	if syncStockState.LastError != "test error" {
		t.Errorf("LastError=%q want test error", syncStockState.LastError)
	}
	if syncStockState.LastElapsed != time.Second*5 {
		t.Errorf("LastElapsed=%v want 5s", syncStockState.LastElapsed)
	}
	syncStockState.Unlock()

	// cleanup
	setSyncStockFinish(0, "")
}

// ============ SyncStockNow ============

func TestSyncStockNowMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/stock/sync-now", nil)
	(&DashboardHandler{DB: db}).SyncStockNow(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec.Code)
	}
}

// ============ SyncStockStatus ============

func TestSyncStockStatusNotRunning(t *testing.T) {
	// reset 全局到 not running
	syncStockState.Lock()
	syncStockState.Running = false
	syncStockState.LastFinishedAt = time.Time{}
	syncStockState.Unlock()

	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/stock/sync-status", nil)
	(&DashboardHandler{DB: db}).SyncStockStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	if running, _ := resp["running"].(bool); running {
		t.Error("默认 running 应 false")
	}
}

func TestSyncStockStatusRunning(t *testing.T) {
	// 注入 running 状态
	syncStockState.Lock()
	syncStockState.Running = true
	syncStockState.StartedAt = time.Now().Add(-30 * time.Second)
	syncStockState.Unlock()
	defer func() {
		// cleanup
		syncStockState.Lock()
		syncStockState.Running = false
		syncStockState.Unlock()
	}()

	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/stock/sync-status", nil)
	(&DashboardHandler{DB: db}).SyncStockStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	if running, _ := resp["running"].(bool); !running {
		t.Error("应 running=true")
	}
	if elapsed, _ := resp["elapsedSec"].(float64); elapsed < 25 {
		t.Errorf("elapsedSec 应 ~30, got %v", elapsed)
	}
}

func TestSyncStockStatusWithLastFinish(t *testing.T) {
	// reset + 注入 LastFinishedAt
	syncStockState.Lock()
	syncStockState.Running = false
	syncStockState.LastFinishedAt = time.Now().Add(-5 * time.Minute)
	syncStockState.LastElapsed = 120 * time.Second
	syncStockState.LastError = "previous err"
	syncStockState.Unlock()
	defer func() {
		syncStockState.Lock()
		syncStockState.LastFinishedAt = time.Time{}
		syncStockState.LastElapsed = 0
		syncStockState.LastError = ""
		syncStockState.Unlock()
	}()

	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/stock/sync-status", nil)
	(&DashboardHandler{DB: db}).SyncStockStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	if resp["lastError"] != "previous err" {
		t.Errorf("lastError=%v want 'previous err'", resp["lastError"])
	}
	if resp["lastElapsedSec"] != float64(120) {
		t.Errorf("lastElapsedSec=%v want 120", resp["lastElapsedSec"])
	}
}
