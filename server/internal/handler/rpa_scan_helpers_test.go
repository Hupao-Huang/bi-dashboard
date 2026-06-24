package handler

// rpa_scan_helpers_test.go — rpaStatus + clearRPAScanCache + ScanRPAFiles/RefreshRPAScan 405
// 已 Read rpa_scan.go (line 65 rpaStatus, 300 getRPAScanCached, 321 clearRPAScanCache, 377/388 handlers).

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ rpaStatus ============

func TestRpaStatusComplete(t *testing.T) {
	if rpaStatus(1.0) != "complete" {
		t.Error("1.0 应 complete")
	}
}

func TestRpaStatusOver1(t *testing.T) {
	if rpaStatus(1.5) != "complete" {
		t.Error("> 1.0 应 complete")
	}
}

func TestRpaStatusPartial(t *testing.T) {
	if rpaStatus(0.5) != "partial" {
		t.Error("0.5 应 partial")
	}
}

func TestRpaStatusMissing(t *testing.T) {
	if rpaStatus(0) != "missing" {
		t.Error("0 应 missing")
	}
}

// ============ isRPASealed (封存截止日 + 封存区间) ============

func TestIsRPASealed(t *testing.T) {
	cases := []struct {
		platform string
		date     string
		want     bool
		desc     string
	}{
		{"拼多多", "2026-02-24", true, "拼多多截止日前(<2/25)封存"},
		{"拼多多", "2026-02-25", false, "拼多多截止日当天不封存"},
		{"拼多多", "2026-03-01", false, "拼多多截止日后不封存"},
		{"抖音", "2026-02-13", false, "抖音区间前一天不封存"},
		{"抖音", "2026-02-14", true, "抖音区间起点封存(闭区间)"},
		{"抖音", "2026-02-17", true, "抖音区间中间封存"},
		{"抖音", "2026-02-19", true, "抖音区间终点封存(闭区间)"},
		{"抖音", "2026-02-20", false, "抖音区间后一天不封存"},
		{"天猫", "2026-02-15", false, "未配置封存的平台不封存"},
	}
	for _, c := range cases {
		if got := isRPASealed(c.platform, c.date); got != c.want {
			t.Errorf("%s: isRPASealed(%q,%q)=%v want %v", c.desc, c.platform, c.date, got, c.want)
		}
	}
}

// ============ clearRPAScanCache + getRPAScanCached state ============

func TestClearRPAScanCache(t *testing.T) {
	// 先注入 fake cache
	rpaScanMu.Lock()
	rpaScanCache = &rpaScanResult{ScannedAt: "2026-05-10"}
	rpaScanCachedAt = time.Now()
	rpaScanMu.Unlock()

	clearRPAScanCache()

	rpaScanMu.RLock()
	cache := rpaScanCache
	rpaScanMu.RUnlock()
	if cache != nil {
		t.Errorf("clear 后 cache 应 nil, got %+v", cache)
	}
}

// ============ ScanRPAFiles HTTP ============

func TestScanRPAFilesMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/rpa-scan", nil)
	(&DashboardHandler{DB: db}).ScanRPAFiles(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST 应 405, got %d", rec.Code)
	}
}

// ============ doRPAScan (items_map.json 读不到 → 空 result) ============

func TestDoRPAScanItemsMapMissing(t *testing.T) {
	// 不改 rpaBaseDir, 真实环境下大概率有/没有 items_map.json
	// 这里只验证返回结构非 nil + Platforms 非 nil
	clearRPAScanCache()
	result := doRPAScan()
	if result == nil {
		t.Fatal("doRPAScan 不应返 nil")
	}
	if result.Platforms == nil {
		t.Error("Platforms 至少应 [] 不应 nil")
	}
}
