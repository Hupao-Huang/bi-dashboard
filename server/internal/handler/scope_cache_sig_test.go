package handler

// scope_cache_sig_test.go — v1.75.20 缓存按权限范围共享的安全回归测试
//
// 核心不变式: WithCache 把响应缓存按「数据权限范围签名」分组(不再按用户ID)。
//   - 同权限范围的不同用户  → 共享同一份缓存 (高命中率, 秒开)
//   - 不同权限范围的用户    → 缓存隔离 (绝不串数据 / 越权)
// 这组 case 把上述性质固化, 防止以后有人改坏 scopeCacheSig 或 WithCache 导致越权。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func mkScopePayload(uid int64, superAdmin bool, depts, platforms, shops, warehouses, domains []string) *authPayload {
	p := &authPayload{IsSuperAdmin: superAdmin}
	p.User.ID = uid
	p.DataScopes = authDataScopes{
		Depts: depts, Platforms: platforms, Shops: shops,
		Warehouses: warehouses, Domains: domains,
	}
	return p
}

func TestScopeCacheSig_NilIsAnon(t *testing.T) {
	if got := scopeCacheSig(nil); got != "anon" {
		t.Fatalf("nil payload 应返 anon, got %q", got)
	}
}

func TestScopeCacheSig_SameScopeSameSig_UidIgnored(t *testing.T) {
	a := mkScopePayload(1, false, []string{"ecommerce"}, nil, nil, nil, nil)
	b := mkScopePayload(999, false, []string{"ecommerce"}, nil, nil, nil, nil)
	if scopeCacheSig(a) != scopeCacheSig(b) {
		t.Fatalf("同 scope 不同 uid 应得相同签名: %q vs %q", scopeCacheSig(a), scopeCacheSig(b))
	}
}

func TestScopeCacheSig_OrderIndependent(t *testing.T) {
	a := mkScopePayload(1, false, []string{"ecommerce", "social"}, nil, nil, nil, nil)
	b := mkScopePayload(2, false, []string{"social", "ecommerce"}, nil, nil, nil, nil)
	if scopeCacheSig(a) != scopeCacheSig(b) {
		t.Fatalf("scope 值相同但顺序不同应得相同签名(内部已排序): %q vs %q", scopeCacheSig(a), scopeCacheSig(b))
	}
}

func TestScopeCacheSig_EachDimensionDistinguishes(t *testing.T) {
	base := mkScopePayload(1, false, nil, nil, nil, nil, nil)
	baseSig := scopeCacheSig(base)
	variants := map[string]*authPayload{
		"superadmin": mkScopePayload(1, true, nil, nil, nil, nil, nil),
		"dept":       mkScopePayload(1, false, []string{"ecommerce"}, nil, nil, nil, nil),
		"platform":   mkScopePayload(1, false, nil, []string{"tmall"}, nil, nil, nil),
		"shop":       mkScopePayload(1, false, nil, nil, []string{"shopA"}, nil, nil),
		"warehouse":  mkScopePayload(1, false, nil, nil, nil, []string{"wh1"}, nil),
		"domain":     mkScopePayload(1, false, nil, nil, nil, nil, []string{"sales"}),
	}
	for dim, p := range variants {
		if scopeCacheSig(p) == baseSig {
			t.Errorf("维度 %q 不同却得到与 base 相同的签名 (%q) —— 该维度未纳入签名会导致越权", dim, baseSig)
		}
	}
}

// 集成: 同 scope 共享 / 不同 scope 隔离 —— 直接验证「不串数据」。
func TestWithCache_SharesBySameScope_IsolatesByDifferentScope(t *testing.T) {
	ClearOverviewCache()
	h := &DashboardHandler{}

	calls := 0
	cached := h.WithCache(1*time.Hour, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"caller":` + strconv.Itoa(calls) + `}}`))
	})

	reqFor := func(p *authPayload) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/overview?start=2026-05-01&end=2026-05-31", nil)
		ctx := context.WithValue(req.Context(), currentAuthPayloadKey, p)
		return req.WithContext(ctx)
	}

	userA1 := mkScopePayload(10, false, []string{"ecommerce"}, nil, nil, nil, nil)
	userA2 := mkScopePayload(20, false, []string{"ecommerce"}, nil, nil, nil, nil) // 同 scope, 不同 uid
	userB := mkScopePayload(30, false, []string{"social"}, nil, nil, nil, nil)     // 不同 scope

	cached(httptest.NewRecorder(), reqFor(userA1))
	if calls != 1 {
		t.Fatalf("userA1 首次(冷)应触发 handler, calls=%d", calls)
	}

	cached(httptest.NewRecorder(), reqFor(userA2))
	if calls != 1 {
		t.Fatalf("同 scope 的 userA2 应命中 userA1 的缓存(不调 handler), 但 calls=%d", calls)
	}

	cached(httptest.NewRecorder(), reqFor(userB))
	if calls != 2 {
		t.Fatalf("不同 scope 的 userB 应隔离(触发新 handler, 不串 A 的数据), 但 calls=%d", calls)
	}
}
