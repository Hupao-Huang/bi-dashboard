package ai_assistant

// AI 助手响应缓存 (v1.74.0)
// 跑哥背景: GA 后 3 天 0 真实用户 → 怀疑 40s 等待是劝退头号嫌疑 → 加 cache 把重复问题压到 < 50ms
// 设计: question normalize + today bind 作 key, TTL 1h (远小于按天 sync 频率, 符合 feedback_cache_ttl_match_sync)
// 失效: 自动到期 + 跨日 today 变 → 昨天 cache 自动不再命中

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync/atomic"
	"time"
)

// cacheEntry 单条缓存
type cacheEntry struct {
	result    *AskResult
	expiresAt time.Time
}

// NormalizeQuestion 把问题归一化作 cache key 用
// 规则: 去首尾空白 + 内部多空格压成一个 + 不动中文 (保留语义)
// 不做小写化 (中文无大小写, 英文部分如 SKU/TOP 5 也希望保留原样)
func NormalizeQuestion(q string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(q)), " ")
}

// cacheKey 生成 cache key: SHA256(normalize(question) + "|" + today)
// today 嵌进去保证: "本月销售" 5/25 问的答案不会被 5/26 命中 (时间口径已经变了)
func (s *Service) cacheKey(question string) string {
	normalized := NormalizeQuestion(question)
	today := time.Now().Format("2006-01-02")
	sum := sha256.Sum256([]byte(normalized + "|" + today))
	return hex.EncodeToString(sum[:16]) // 取前 16 字节 = 32 字符 hex, 够用
}

// cacheGet 取缓存. 未配 / 过期 / 未命中都返 nil
func (s *Service) cacheGet(key string) *AskResult {
	if !s.CacheEnabled || s.CacheTTL <= 0 {
		return nil
	}
	v, ok := s.cache.Load(key)
	if !ok {
		atomic.AddInt64(&s.cacheMisses, 1)
		return nil
	}
	entry := v.(*cacheEntry)
	if time.Now().After(entry.expiresAt) {
		s.cache.Delete(key)
		atomic.AddInt64(&s.cacheMisses, 1)
		return nil
	}
	atomic.AddInt64(&s.cacheHits, 1)
	return entry.result
}

// cacheSet 存缓存. CacheEnabled=false / TTL<=0 / result nil 都 skip
// 只缓存"成功 + 高置信度"答案, 避免把 unknown / 错误兜底答案污染 cache
func (s *Service) cacheSet(key string, result *AskResult) {
	if !s.CacheEnabled || s.CacheTTL <= 0 || result == nil {
		return
	}
	// 不缓存 unknown / 兜底 / 失败答案
	if result.SourceType == "unknown" || result.Warning != "" {
		return
	}
	s.cache.Store(key, &cacheEntry{
		result:    result,
		expiresAt: time.Now().Add(s.CacheTTL),
	})
}

// CacheStats 返回 (hits, misses, size) 用于监控接口
func (s *Service) CacheStats() (hits, misses, size int64) {
	hits = atomic.LoadInt64(&s.cacheHits)
	misses = atomic.LoadInt64(&s.cacheMisses)
	s.cache.Range(func(_, _ interface{}) bool {
		size++
		return true
	})
	return
}

// cloneResult 浅拷贝 AskResult (cache hit 时返副本, 每次请求不同 DurationMs/SessionID/MessageID/FromCache)
// RawData/Intent 是指针/interface, 浅拷贝即可 (cache 后业务代码不应再 mutate, 当 read-only 看)
func cloneResult(src *AskResult) *AskResult {
	cp := *src
	return &cp
}
