package handler

// cache_business_test.go — WithCache middleware + GetBusinessReportFinanceLike 空快路径
// 已 Read dashboard_cache.go (line 91-131): WithCache + cacheResponseRecorder
// 已 Read business_report.go (line 356-435): GetBusinessReportFinanceLike snaps=空快返

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ WithCache middleware ============

func TestWithCacheMissCallsHandlerAndStores(t *testing.T) {
	ClearOverviewCache()

	called := 0
	handler := func(w http.ResponseWriter, r *http.Request) {
		called++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"msg": "fresh"})
	}

	h := &DashboardHandler{}
	wrapped := h.WithCache(time.Minute, handler)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test?x=1", nil)
	wrapped(rec, req)

	if called != 1 {
		t.Errorf("第一次应调 handler 1 次, got %d", called)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("应 200, got %d", rec.Code)
	}
}

func TestWithCacheHitReturnsCachedSkipsHandler(t *testing.T) {
	ClearOverviewCache()

	called := 0
	handler := func(w http.ResponseWriter, r *http.Request) {
		called++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"msg": "fresh"})
	}

	h := &DashboardHandler{}
	wrapped := h.WithCache(time.Minute, handler)

	// 第 1 次: cache miss, 调 handler
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/api/test?x=42", nil)
	wrapped(rec1, req1)

	// 第 2 次: cache hit, 不调 handler
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/test?x=42", nil)
	wrapped(rec2, req2)

	if called != 1 {
		t.Errorf("第二次应 cache hit, handler 不再调, got called=%d", called)
	}
	if rec2.Code != http.StatusOK {
		t.Errorf("cache hit 应 200, got %d", rec2.Code)
	}
}

func TestWithCacheNon200NotCached(t *testing.T) {
	// handler 返 500 → 不应 cache (源码 line 106 if statusCode == 200)
	ClearOverviewCache()

	called := 0
	handler := func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(500)
		w.Write([]byte(`{"err":"x"}`))
	}

	h := &DashboardHandler{}
	wrapped := h.WithCache(time.Minute, handler)

	// 第 1 次: 500
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/api/test?x=99", nil)
	wrapped(rec1, req1)

	// 第 2 次: 应再调 handler (上次 500 没 cache)
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/test?x=99", nil)
	wrapped(rec2, req2)

	if called != 2 {
		t.Errorf("非 200 不应 cache, 第 2 次应再调, got called=%d", called)
	}
}

func TestWithCacheBadJSONNotCached(t *testing.T) {
	// handler 返 200 + 非 JSON → 不应 cache (line 108 json.Unmarshal 失败)
	ClearOverviewCache()

	called := 0
	handler := func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(200)
		w.Write([]byte(`not json`))
	}

	h := &DashboardHandler{}
	wrapped := h.WithCache(time.Minute, handler)

	rec1 := httptest.NewRecorder()
	wrapped(rec1, httptest.NewRequest(http.MethodGet, "/api/test?x=badjson", nil))

	rec2 := httptest.NewRecorder()
	wrapped(rec2, httptest.NewRequest(http.MethodGet, "/api/test?x=badjson", nil))

	if called != 2 {
		t.Errorf("非 JSON body 不应 cache, got called=%d", called)
	}
}

// ============ cacheResponseRecorder.WriteHeader/Write ============

func TestCacheResponseRecorderTracksStatusAnd200Body(t *testing.T) {
	rec := httptest.NewRecorder()
	crr := &cacheResponseRecorder{ResponseWriter: rec, statusCode: 200}

	// 200 + body 应被记录
	crr.WriteHeader(200)
	crr.Write([]byte(`{"a":1}`))

	if crr.statusCode != 200 {
		t.Errorf("statusCode=%d want 200", crr.statusCode)
	}
	if string(crr.body) != `{"a":1}` {
		t.Errorf("body=%q want {a:1}", crr.body)
	}
}

func TestCacheResponseRecorderNon200NotRecorded(t *testing.T) {
	rec := httptest.NewRecorder()
	crr := &cacheResponseRecorder{ResponseWriter: rec, statusCode: 200}

	crr.WriteHeader(500)
	crr.Write([]byte(`{"err":"boom"}`))

	if crr.statusCode != 500 {
		t.Errorf("statusCode 应 500, got %d", crr.statusCode)
	}
	if len(crr.body) != 0 {
		t.Errorf("非 200 不应记 body, got %q", crr.body)
	}
}

// ============ GetBusinessReportFinanceLike 空快路径 ============

func TestGetBusinessReportFinanceLikeEmptySnapshots(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// snaps 查询返空 → 立即返 (line 429-435)
	mock.ExpectQuery(`SELECT snapshot_year, MAX\(snapshot_month\) AS sm\s+FROM business_budget_report`).
		WillReturnRows(sqlmock.NewRows([]string{"year", "month"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/finance/business-report?yearStart=2024&yearEnd=2026", nil)
	(&DashboardHandler{DB: db}).GetBusinessReportFinanceLike(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("空 snaps 应 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	rows, _ := resp["rows"].([]interface{})
	if len(rows) != 0 {
		t.Errorf("空 snaps 应 0 rows, got %d", len(rows))
	}
}

func TestGetBusinessReportFinanceLikeChannelDefaults(t *testing.T) {
	// 无 channels 参数 → 默认 "总|"
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM business_budget_report\s+WHERE snapshot_year BETWEEN`).
		WillReturnRows(sqlmock.NewRows([]string{"year", "month"})) // 空

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/finance/business-report", nil)
	(&DashboardHandler{DB: db}).GetBusinessReportFinanceLike(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("默认值应 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}
