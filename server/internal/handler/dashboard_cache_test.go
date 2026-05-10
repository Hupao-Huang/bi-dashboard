package handler

// dashboard_cache_test.go — overview cache 模块单元测试
// 已 Read dashboard_cache.go 全文 (132 行), 按源码每个公开函数和分支写 case.
//
// 注意: overviewCache 是 package-level 变量, 每个 test 开头 ClearOverviewCache 重置防互相污染.

import (
	"testing"
	"time"
)

func TestClearOverviewCache(t *testing.T) {
	ClearOverviewCache() // 起手清

	setOverviewCache("k1", map[string]interface{}{"v": 1})
	setOverviewCache("k2", map[string]interface{}{"v": 2})
	setOverviewCache("k3", map[string]interface{}{"v": 3})

	cleared := ClearOverviewCache()
	if cleared != 3 {
		t.Fatalf("应清除 3 条, got %d", cleared)
	}
	if _, ok := getOverviewCache("k1"); ok {
		t.Fatal("清空后 k1 不应存在")
	}

	// 重复清空, 应返回 0
	if cleared := ClearOverviewCache(); cleared != 0 {
		t.Fatalf("清空后再清, 应返 0, got %d", cleared)
	}
}

func TestClearCacheByPrefix(t *testing.T) {
	ClearOverviewCache()

	setOverviewCache("api|/api/stock/list?x=1|u:1", map[string]interface{}{})
	setOverviewCache("api|/api/stock/detail?x=2|u:1", map[string]interface{}{})
	setOverviewCache("api|/api/finance/report|u:1", map[string]interface{}{})
	setOverviewCache("api|/api/dashboard|u:2", map[string]interface{}{})

	// 只清 stock 相关 (前缀匹配)
	cleared := ClearCacheByPrefix("api|/api/stock/")
	if cleared != 2 {
		t.Fatalf("应清 2 条 stock cache, got %d", cleared)
	}

	// finance 和 dashboard 应仍在
	if _, ok := getOverviewCache("api|/api/finance/report|u:1"); !ok {
		t.Error("finance cache 不应被 stock prefix 清掉")
	}
	if _, ok := getOverviewCache("api|/api/dashboard|u:2"); !ok {
		t.Error("dashboard cache 不应被 stock prefix 清掉")
	}

	// 不匹配的 prefix 返 0
	if cleared := ClearCacheByPrefix("api|/api/notexist/"); cleared != 0 {
		t.Fatalf("不匹配前缀应返 0, got %d", cleared)
	}
}

func TestGetOverviewCacheReturnsFreshEntries(t *testing.T) {
	ClearOverviewCache()

	data := map[string]interface{}{"foo": "bar", "n": 42}
	setOverviewCache("fresh-key", data)

	got, ok := getOverviewCache("fresh-key")
	if !ok {
		t.Fatal("刚 set 的应返回 ok=true")
	}
	if got["foo"] != "bar" || got["n"] != 42 {
		t.Errorf("数据不一致: %v", got)
	}
}

func TestGetOverviewCacheRespectsTTLExpiry(t *testing.T) {
	ClearOverviewCache()

	// 用 setCacheWithTTL 设极短 TTL (1ms) 模拟过期
	setCacheWithTTL("expire-key", map[string]interface{}{"x": 1}, 1*time.Millisecond)
	time.Sleep(10 * time.Millisecond) // 等过期

	if _, ok := getOverviewCache("expire-key"); ok {
		t.Fatal("TTL 过期后 getOverviewCache 应返 ok=false")
	}
}

func TestGetOverviewCacheMissReturnsFalse(t *testing.T) {
	ClearOverviewCache()
	if _, ok := getOverviewCache("never-set-key"); ok {
		t.Fatal("不存在 key 应返 ok=false")
	}
}

func TestSetCacheWithTTLEvictsExpiredAtCapacity(t *testing.T) {
	ClearOverviewCache()

	// 塞满到 capacity 边缘, 一半已过期, 一半未过期
	const cap = 1024
	expired := cap / 2
	for i := 0; i < expired; i++ {
		setCacheWithTTL(fmtKey("expired", i), map[string]interface{}{}, 1*time.Millisecond)
	}
	time.Sleep(5 * time.Millisecond)

	for i := 0; i < cap-expired; i++ {
		setCacheWithTTL(fmtKey("alive", i), map[string]interface{}{}, 1*time.Hour)
	}

	// 触发 eviction (再 set 1 条)
	setCacheWithTTL("trigger-evict", map[string]interface{}{}, 1*time.Hour)

	// 过期的 expired-* 应该被清, alive-* 应在
	overviewCacheMu.RLock()
	defer overviewCacheMu.RUnlock()
	for i := 0; i < expired; i++ {
		if _, exists := overviewCache[fmtKey("expired", i)]; exists {
			t.Errorf("已过期 key %s 应在 capacity 触发时被 evict", fmtKey("expired", i))
		}
	}
}

func fmtKey(prefix string, i int) string {
	return prefix + "-" + itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}
