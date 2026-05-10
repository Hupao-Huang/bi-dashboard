package handler

// marketing_task_test.go — marketing_cost helper / task_monitor / rpa_scan 边界
// 已 Read marketing_cost.go (mergeCpcDaily/mergeCpsDaily helper, GetMarketingCost),
//          task_monitor.go (GetTaskStatus 405 + powershell 调用),
//          rpa_scan.go (ScanRPAFiles/RefreshRPAScan handler 边界).

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ mergeCpcDaily 纯函数 ============

func TestMergeCpcDailyAccumulatesSameDate(t *testing.T) {
	arr := []CpcDaily{}

	// 第一次插 2026-05-01
	arr = mergeCpcDaily(arr, CpcDaily{Date: "2026-05-01", Cost: 100, PayAmount: 500, Clicks: 50, Impr: 10000})
	if len(arr) != 1 {
		t.Fatalf("应 1 条, got %d", len(arr))
	}

	// 同日再插 → 累加
	arr = mergeCpcDaily(arr, CpcDaily{Date: "2026-05-01", Cost: 50, PayAmount: 250, Clicks: 30, Impr: 5000})
	if len(arr) != 1 {
		t.Errorf("同日合并应仍 1 条, got %d", len(arr))
	}
	if arr[0].Cost != 150 {
		t.Errorf("Cost 应累加 = 150, got %v", arr[0].Cost)
	}
	if arr[0].PayAmount != 750 {
		t.Errorf("PayAmount 应 750, got %v", arr[0].PayAmount)
	}
	if arr[0].Clicks != 80 {
		t.Errorf("Clicks 应 80, got %d", arr[0].Clicks)
	}
	if arr[0].Impr != 15000 {
		t.Errorf("Impr 应 15000, got %d", arr[0].Impr)
	}
	// ROI = PayAmount / Cost = 750/150 = 5.0
	if arr[0].ROI < 4.99 || arr[0].ROI > 5.01 {
		t.Errorf("ROI 应 ~5.0, got %v", arr[0].ROI)
	}

	// 不同日期 → 新加一条
	arr = mergeCpcDaily(arr, CpcDaily{Date: "2026-05-02", Cost: 200, PayAmount: 800, Clicks: 100, Impr: 20000})
	if len(arr) != 2 {
		t.Errorf("不同日期应 2 条, got %d", len(arr))
	}
}

func TestMergeCpcDailyEmptyCostNoDivByZero(t *testing.T) {
	arr := []CpcDaily{}
	arr = mergeCpcDaily(arr, CpcDaily{Date: "2026-05-01", Cost: 0, PayAmount: 100, Clicks: 10})
	// Cost=0 时不应计算 ROI (源码 line 687 if Cost > 0)
	if arr[0].ROI != 0 {
		t.Errorf("Cost=0 应 ROI=0 不除零, got %v", arr[0].ROI)
	}
}

// ============ mergeCpsDaily 纯函数 ============

func TestMergeCpsDailyAccumulatesSameDate(t *testing.T) {
	arr := []CpsDaily{}

	arr = mergeCpsDaily(arr, CpsDaily{Date: "2026-05-01", PayAmount: 100, PayCommission: 5, PayUsers: 10})
	arr = mergeCpsDaily(arr, CpsDaily{Date: "2026-05-01", PayAmount: 50, PayCommission: 2.5, PayUsers: 5})

	if len(arr) != 1 {
		t.Errorf("同日合并应 1 条, got %d", len(arr))
	}
	if arr[0].PayAmount != 150 {
		t.Errorf("PayAmount=%v want 150", arr[0].PayAmount)
	}
	if arr[0].PayCommission != 7.5 {
		t.Errorf("PayCommission=%v want 7.5", arr[0].PayCommission)
	}
	if arr[0].PayUsers != 15 {
		t.Errorf("PayUsers=%d want 15", arr[0].PayUsers)
	}
}

func TestMergeCpsDailyDifferentDates(t *testing.T) {
	arr := []CpsDaily{}
	arr = mergeCpsDaily(arr, CpsDaily{Date: "2026-05-01", PayAmount: 100})
	arr = mergeCpsDaily(arr, CpsDaily{Date: "2026-05-02", PayAmount: 200})
	if len(arr) != 2 {
		t.Errorf("不同日期应 2 条, got %d", len(arr))
	}
}

// ============ GetTaskStatus 405 ============

func TestGetTaskStatusMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tasks/status", nil)
	(&DashboardHandler{DB: db}).GetTaskStatus(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST 应 405, got %d", rec.Code)
	}
}

// ============ ScanRPAFiles ============

func TestScanRPAFilesBasicCall(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/rpa/scan", nil)
	(&DashboardHandler{DB: db}).ScanRPAFiles(rec, req)

	// ScanRPAFiles 调 doRPAScan 走真实文件系统, 依赖 Z 盘.
	// 在 test 环境通常 Z 盘不存在 → cache 或 error path.
	// 主要验证 handler 不 panic + 返响应
	if rec.Code == 0 {
		t.Error("响应无效")
	}
}

func TestRefreshRPAScanMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/rpa/refresh", nil)
	(&DashboardHandler{DB: db}).RefreshRPAScan(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405 (期望 POST), got %d", rec.Code)
	}
}

// ============ GetRunningTasks ============

func TestGetRunningTasksBasicCall(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tasks/running", nil)
	(&DashboardHandler{DB: db}).GetRunningTasks(rec, req)

	// 该 handler 读全局 manualTasks map
	if rec.Code != http.StatusOK && rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("应 200/405, got %d", rec.Code)
	}
}

// ============ StopManualTask ============

func TestStopManualTaskMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tasks/stop", nil)
	(&DashboardHandler{DB: db}).StopManualTask(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec.Code)
	}
}

// ============ RunManualTask ============

func TestRunManualTaskMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tasks/run", nil)
	(&DashboardHandler{DB: db}).RunManualTask(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec.Code)
	}
}
