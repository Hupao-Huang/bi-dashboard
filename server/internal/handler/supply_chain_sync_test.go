package handler

// supply_chain_sync_test.go — SyncYSStock running/cooldown 状态机
// 已 Read supply_chain.go (line 1417-1465 SyncYSStock).

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// running 状态 → 429
func TestSyncYSStockAlreadyRunning(t *testing.T) {
	syncYSStockMu.Lock()
	syncYSStockRunning = true
	syncYSStockMu.Unlock()
	defer func() {
		syncYSStockMu.Lock()
		syncYSStockRunning = false
		syncYSStockMu.Unlock()
	}()

	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/supply-chain/sync-ys-stock", nil)
	(&DashboardHandler{DB: db}).SyncYSStock(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("running 应 429, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "正在执行") {
		t.Errorf("错误信息应提示已在执行, got %s", rec.Body.String())
	}
}

// cooldown 状态 → 429 (上次结束 < 60s)
func TestSyncYSStockCooldown(t *testing.T) {
	// 重置 running, 设 LastEndTime = 10s 前 (在 60s cooldown 内)
	syncYSStockMu.Lock()
	syncYSStockRunning = false
	syncYSLastEndTime = time.Now().Add(-10 * time.Second)
	syncYSStockMu.Unlock()
	defer func() {
		syncYSStockMu.Lock()
		syncYSLastEndTime = time.Time{}
		syncYSStockMu.Unlock()
	}()

	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/supply-chain/sync-ys-stock", nil)
	(&DashboardHandler{DB: db}).SyncYSStock(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("cooldown 应 429, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "秒后再试") {
		t.Errorf("错误应含'秒后再试' cooldown 提示, got %s", rec.Body.String())
	}
}

// cooldown 已过 (61s 前结束) → 不被 cooldown 拦, 但会进入业务逻辑 → DB 查询
// 这里只验证 cooldown 不再拦 (会走到 DB 查 ys_purchase_orders 等), 通过 DB 错误返回快速终止
func TestSyncYSStockCooldownExpired(t *testing.T) {
	syncYSStockMu.Lock()
	syncYSStockRunning = false
	syncYSLastEndTime = time.Now().Add(-90 * time.Second) // 90s 前 > 60s cooldown
	syncYSStockMu.Unlock()
	defer func() {
		syncYSStockMu.Lock()
		syncYSLastEndTime = time.Time{}
		syncYSStockRunning = false
		syncYSStockMu.Unlock()
	}()

	// 不 mock DB → 进入 SyncYSStock 后会真跑 fork exe, 我们不测它
	// 所以这个 case 跳过, 只保留前两个核心状态机 case
	t.Skip("cooldown 过期会触发真 fork, 跳过")
}
