package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
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
// v1.75.20 起 key 格式:
//   - WithCache 外层:        "api|<path>?<query>|<scopeSig>"
//   - GetOverview 内层(ov|): "api|ov|<scopeSig>|<dates>"
// 两层都以 "api|" 开头, 所以 ClearCacheByPrefix("api|") 广义清缓存能同时清内外层。
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

// scopeCacheSig 生成用户「数据权限范围」的签名串。
// 同一权限范围(部门/平台/店铺/仓库/域 + 是否超管)的用户得到相同签名,
// 可安全共享同一份数据缓存——因为他们各自调 buildXxxDataScopeCond 得到的过滤条件完全相同,
// 返回数据必然一致。权限(permission)不同的人在到达缓存前已被 RequirePermission 拦下,
// 不会污染缓存。必须覆盖全部 5 个 scope 维度 + 超管标志, 漏一个就可能让不同权限的人撞 key 串数据。
func scopeCacheSig(payload *authPayload) string {
	if payload == nil {
		return "anon"
	}
	s := payload.DataScopes
	join := func(in []string) string {
		out := append([]string(nil), in...)
		sort.Strings(out)
		return strings.Join(out, ",")
	}
	return fmt.Sprintf("sa:%t|d:%s|p:%s|s:%s|w:%s|dom:%s",
		payload.IsSuperAdmin,
		join(s.Depts), join(s.Platforms), join(s.Shops), join(s.Warehouses), join(s.Domains))
}

func (h *DashboardHandler) WithCache(ttl time.Duration, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		payload, _ := authPayloadFromContext(r)
		// v1.75.20: 缓存按「权限范围签名」分组(不再按用户 ID), 同权限的人共享一份缓存,
		// 命中率从「每人每天冷一次」提升到「一组人里第一个开的人焐热, 其余全秒开」。
		key := fmt.Sprintf("api|%s?%s|%s", r.URL.Path, r.URL.RawQuery, scopeCacheSig(payload))
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
