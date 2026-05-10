package handler

// sync_handlers_test.go — sync.go SyncOps/ClearCache/SyncStatus/ManualImport/ImportProgress 边界
// 已 Read sync.go (line 66-442).

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ SyncOps ============

func TestSyncOpsForbiddenWithoutSecret(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/sync-ops", bytes.NewReader([]byte(`{}`)))
	(&DashboardHandler{DB: db, WebhookSecret: "expected-secret"}).SyncOps(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("无 X-Webhook-Secret 应 403, got %d", rec.Code)
	}
}

func TestSyncOpsForbiddenBadSecret(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/sync-ops", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-Webhook-Secret", "wrong-secret")
	(&DashboardHandler{DB: db, WebhookSecret: "expected-secret"}).SyncOps(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("错误 secret 应 403, got %d", rec.Code)
	}
}

func TestSyncOpsBadDateFormat(t *testing.T) {
	// reset syncRunning
	syncMu.Lock()
	syncRunning = false
	syncMu.Unlock()

	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"date":"2026-05-01"}`) // 长度 != 8
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/sync-ops", bytes.NewReader(body))
	req.Header.Set("X-Webhook-Secret", "x")
	(&DashboardHandler{DB: db, WebhookSecret: "x"}).SyncOps(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("非 8 位日期应 400, got %d", rec.Code)
	}
}

func TestSyncOpsAlreadyRunning(t *testing.T) {
	// 注入 running 状态
	syncMu.Lock()
	syncRunning = true
	syncMu.Unlock()
	defer func() {
		syncMu.Lock()
		syncRunning = false
		syncMu.Unlock()
	}()

	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/sync-ops", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-Webhook-Secret", "x")
	(&DashboardHandler{DB: db, WebhookSecret: "x"}).SyncOps(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("running 应 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ============ ClearCache ============

func TestClearCacheForbiddenWithoutSecret(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/clear-cache", nil)
	(&DashboardHandler{DB: db, WebhookSecret: "x"}).ClearCache(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("缺 secret 应 403, got %d", rec.Code)
	}
}

func TestClearCacheHappyPath(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/clear-cache", nil)
	req.Header.Set("X-Webhook-Secret", "secret")
	(&DashboardHandler{DB: db, WebhookSecret: "secret"}).ClearCache(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("happy path 应 200, got %d", rec.Code)
	}
}

// ============ SyncStatus ============

func TestSyncStatusForbiddenWithoutSecret(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/webhook/sync-status", nil)
	(&DashboardHandler{DB: db, WebhookSecret: "x"}).SyncStatus(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("缺 secret 应 403, got %d", rec.Code)
	}
}

func TestSyncStatusHappyPath(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/webhook/sync-status", nil)
	req.Header.Set("X-Webhook-Secret", "x")
	(&DashboardHandler{DB: db, WebhookSecret: "x"}).SyncStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("应 200, got %d", rec.Code)
	}
}

// ============ ManualImport ============

func TestManualImportBadJSON(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/manual-import", bytes.NewReader([]byte(`bad`)))
	(&DashboardHandler{DB: db}).ManualImport(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json 应 400, got %d", rec.Code)
	}
}

func TestManualImportBadDateLength(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"date":"2026","platform":"天猫"}`) // 长度 != 8
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/manual-import", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).ManualImport(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("非 8 位日期应 400, got %d", rec.Code)
	}
}

func TestManualImportUnknownPlatform(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"date":"20260501","platform":"未知平台"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/manual-import", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).ManualImport(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("未知平台应 400, got %d", rec.Code)
	}
}

func TestManualImportAlreadyRunning(t *testing.T) {
	syncMu.Lock()
	syncRunning = true
	syncMu.Unlock()
	defer func() {
		syncMu.Lock()
		syncRunning = false
		syncMu.Unlock()
	}()

	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"date":"20260501","platform":"天猫"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/manual-import", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).ManualImport(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("running 应 409, got %d", rec.Code)
	}
}

// ============ ImportProgress ============

func TestImportProgressNoActiveProgress(t *testing.T) {
	// 重置全局
	importProgressMu.Lock()
	currentImportProgress = nil
	importProgressMu.Unlock()

	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/rpa-scan/import-progress", nil)
	(&DashboardHandler{DB: db}).ImportProgress(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("应 200, got %d", rec.Code)
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	if running, _ := resp["running"].(bool); running {
		t.Error("应 running=false")
	}
}

// 锁住 syncMu 防 panic, 不实际 panic 验证
var _ = sync.Mutex{}
