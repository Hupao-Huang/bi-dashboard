package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type overviewCacheEntry struct {
	data      map[string]interface{}
	expiresAt time.Time
}

var (
	overviewCacheMu sync.RWMutex
	overviewCache   = map[string]overviewCacheEntry{}
)

const (
	overviewCacheTTL      = 30 * time.Second
	deptCacheTTL          = 5 * time.Minute
	overviewCacheMaxItems = 1024
)

// ClearOverviewCache 清空所有接口缓存（同步脚本完成后调用，立即反映最新数据）
func ClearOverviewCache() int {
	overviewCacheMu.Lock()
	defer overviewCacheMu.Unlock()
	n := len(overviewCache)
	overviewCache = map[string]overviewCacheEntry{}
	return n
}

// ClearCacheByPrefix 精准清理：只删 key 以 prefix 开头的缓存条目
// prefix 示例: "api|/api/stock/" 匹配所有 stock 相关接口（不影响别的模块）
// WithCache 的 key 格式: "api|<path>?<query>|u:<uid>"
func ClearCacheByPrefix(prefix string) int {
	overviewCacheMu.Lock()
	defer overviewCacheMu.Unlock()
	n := 0
	for k := range overviewCache {
		if strings.HasPrefix(k, prefix) {
			delete(overviewCache, k)
			n++
		}
	}
	return n
}

func getOverviewCache(key string) (map[string]interface{}, bool) {
	now := time.Now()
	overviewCacheMu.RLock()
	entry, ok := overviewCache[key]
	overviewCacheMu.RUnlock()
	if !ok || now.After(entry.expiresAt) {
		return nil, false
	}
	return entry.data, true
}

func setOverviewCache(key string, data map[string]interface{}) {
	setCacheWithTTL(key, data, overviewCacheTTL)
}

func setCacheWithTTL(key string, data map[string]interface{}, ttl time.Duration) {
	now := time.Now()

	overviewCacheMu.Lock()
	defer overviewCacheMu.Unlock()

	if len(overviewCache) >= overviewCacheMaxItems {
		for cacheKey, entry := range overviewCache {
			if now.After(entry.expiresAt) {
				delete(overviewCache, cacheKey)
			}
		}
		if len(overviewCache) >= overviewCacheMaxItems {
			overviewCache = map[string]overviewCacheEntry{}
		}
	}

	overviewCache[key] = overviewCacheEntry{
		data:      data,
		expiresAt: now.Add(ttl),
	}
}

func (h *DashboardHandler) WithCache(ttl time.Duration, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		payload, _ := authPayloadFromContext(r)
		uid := int64(0)
		if payload != nil {
			uid = payload.User.ID
		}
		key := fmt.Sprintf("api|%s?%s|u:%d", r.URL.Path, r.URL.RawQuery, uid)
		if cached, ok := getOverviewCache(key); ok {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cached)
			return
		}
		rec := &cacheResponseRecorder{ResponseWriter: w, statusCode: 200}
		handler(rec, r)
		if rec.statusCode == 200 && len(rec.body) > 0 {
			var parsed map[string]interface{}
			if json.Unmarshal(rec.body, &parsed) == nil {
				setCacheWithTTL(key, parsed, ttl)
			}
		}
	}
}

type cacheResponseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       []byte
}

func (r *cacheResponseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *cacheResponseRecorder) Write(b []byte) (int, error) {
	if r.statusCode == 200 {
		r.body = append(r.body, b...)
	}
	return r.ResponseWriter.Write(b)
}
