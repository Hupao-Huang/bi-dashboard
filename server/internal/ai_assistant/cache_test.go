package ai_assistant

// AI 助手 cache 单元测试 (v1.74.0)
// 不依赖 DB / LLM, 纯逻辑覆盖

import (
	"testing"
	"time"
)

func TestNormalizeQuestion(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"  公司本月销售多少 ", "公司本月销售多少"},
		{"公司  本月\t销售\n多少", "公司 本月 销售 多少"},
		{"", ""},
		{"   ", ""},
		{"本月 卖最好 TOP 5", "本月 卖最好 TOP 5"},
	}
	for _, c := range cases {
		got := NormalizeQuestion(c.in)
		if got != c.want {
			t.Errorf("NormalizeQuestion(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCacheKey_SameQuestionSameDay(t *testing.T) {
	s := &Service{CacheEnabled: true, CacheTTL: time.Hour}
	k1 := s.cacheKey("公司本月销售多少")
	k2 := s.cacheKey("  公司本月销售多少  ") // 空白不同
	k3 := s.cacheKey("公司本月销售多少")
	if k1 != k2 || k1 != k3 {
		t.Errorf("normalize 后应同 key: %s vs %s vs %s", k1, k2, k3)
	}
}

func TestCacheKey_DifferentQuestions(t *testing.T) {
	s := &Service{CacheEnabled: true, CacheTTL: time.Hour}
	k1 := s.cacheKey("公司本月销售多少")
	k2 := s.cacheKey("公司上月销售多少") // 不同问题
	if k1 == k2 {
		t.Errorf("不同问题不应同 key: %s == %s", k1, k2)
	}
}

func TestCacheGetSet_Disabled(t *testing.T) {
	s := &Service{CacheEnabled: false, CacheTTL: time.Hour}
	key := s.cacheKey("test")
	s.cacheSet(key, &AskResult{Answer: "hello", SourceType: "api"})
	if got := s.cacheGet(key); got != nil {
		t.Errorf("CacheEnabled=false 时 Get 应返 nil, 实际 %+v", got)
	}
}

func TestCacheGetSet_ZeroTTL(t *testing.T) {
	s := &Service{CacheEnabled: true, CacheTTL: 0}
	key := s.cacheKey("test")
	s.cacheSet(key, &AskResult{Answer: "hello", SourceType: "api"})
	if got := s.cacheGet(key); got != nil {
		t.Errorf("CacheTTL=0 时 Get 应返 nil, 实际 %+v", got)
	}
}

func TestCacheGetSet_Hit(t *testing.T) {
	s := &Service{CacheEnabled: true, CacheTTL: time.Hour}
	key := s.cacheKey("test")
	want := &AskResult{Answer: "hello", SourceType: "api", LLMTokens: 100}
	s.cacheSet(key, want)
	got := s.cacheGet(key)
	if got == nil {
		t.Fatal("expected hit, got nil")
	}
	if got.Answer != want.Answer || got.LLMTokens != want.LLMTokens {
		t.Errorf("cache get mismatch: %+v vs %+v", got, want)
	}
}

func TestCacheSet_RejectUnknown(t *testing.T) {
	s := &Service{CacheEnabled: true, CacheTTL: time.Hour}
	key := s.cacheKey("test")
	s.cacheSet(key, &AskResult{Answer: "听不懂", SourceType: "unknown"})
	if got := s.cacheGet(key); got != nil {
		t.Errorf("不应缓存 unknown 答案, 实际 %+v", got)
	}
}

func TestCacheSet_RejectWarning(t *testing.T) {
	s := &Service{CacheEnabled: true, CacheTTL: time.Hour}
	key := s.cacheKey("test")
	s.cacheSet(key, &AskResult{Answer: "fallback", SourceType: "api", Warning: "AI 包装失败"})
	if got := s.cacheGet(key); got != nil {
		t.Errorf("不应缓存 warning 答案, 实际 %+v", got)
	}
}

func TestCacheExpire(t *testing.T) {
	s := &Service{CacheEnabled: true, CacheTTL: 50 * time.Millisecond}
	key := s.cacheKey("test")
	s.cacheSet(key, &AskResult{Answer: "hello", SourceType: "api"})
	if got := s.cacheGet(key); got == nil {
		t.Fatal("expected hit before expire")
	}
	time.Sleep(80 * time.Millisecond)
	if got := s.cacheGet(key); got != nil {
		t.Errorf("expected expire after 80ms (TTL=50ms), got %+v", got)
	}
}

func TestCacheStats(t *testing.T) {
	s := &Service{CacheEnabled: true, CacheTTL: time.Hour}
	key := s.cacheKey("statkey")
	// 一次 miss
	s.cacheGet(key)
	// 一次 set + 三次 hit
	s.cacheSet(key, &AskResult{Answer: "ok", SourceType: "api"})
	for i := 0; i < 3; i++ {
		s.cacheGet(key)
	}
	hits, misses, size := s.CacheStats()
	if hits != 3 || misses != 1 || size != 1 {
		t.Errorf("stats hits=%d misses=%d size=%d, want 3/1/1", hits, misses, size)
	}
}

func TestCloneResult(t *testing.T) {
	src := &AskResult{Answer: "原", DurationMs: 100, FromCache: false}
	cp := cloneResult(src)
	cp.DurationMs = 50
	cp.FromCache = true
	if src.DurationMs == 50 || src.FromCache {
		t.Errorf("原 result 被改了: %+v", src)
	}
	if cp.Answer != "原" {
		t.Errorf("Answer 字段没复制: %+v", cp)
	}
}

func TestNextWarmCacheTime_FutureToday(t *testing.T) {
	now := time.Now()
	futureHour := (now.Hour() + 1) % 24
	next := nextWarmCacheTime(futureHour, 0)
	// 如果加 1 后跨日 (now=23 → futureHour=0), 那么 next 应该是明天的 0:00
	// 否则是今天 (futureHour):00
	expected := time.Date(now.Year(), now.Month(), now.Day(), futureHour, 0, 0, 0, now.Location())
	if !expected.After(now) {
		expected = expected.Add(24 * time.Hour)
	}
	if !next.Equal(expected) {
		t.Errorf("nextWarmCacheTime(future hour) = %s, want %s", next, expected)
	}
}

func TestNextWarmCacheTime_PastToday(t *testing.T) {
	now := time.Now()
	pastHour := (now.Hour() - 1 + 24) % 24
	next := nextWarmCacheTime(pastHour, 0)
	// past hour 必须跑明天
	if next.Before(now) || next.Sub(now) > 24*time.Hour {
		t.Errorf("nextWarmCacheTime(past hour) 应在 0~24h 内, 实际 %s (now=%s)", next, now)
	}
}
