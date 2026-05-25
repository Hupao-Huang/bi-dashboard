package ai_assistant

// AI 助手预计算缓存 (v1.74.0 P2)
// 跑哥背景: P0 cache 只能让"重复问题"秒回, 第一次问还是 40s
// 思路: 半夜 00:30 跑一遍 16 道高频标准题, 灌满 cache, 早上 08:00 第一个用户来问就秒回
// 实现: bi-server 进程内 goroutine, 不用独立 schtasks (cache 在内存里, 跨进程不共享)

import (
	"context"
	"log"
	"time"
)

// StandardQuestions v1.74.0 GA: 8 模块 × 2 题 = 16 道高频标准题
// 这些是管理层最常问的问法 (跑哥自测样本 + 业务直觉)
// 后续可根据 ai_chat_message 真实分布迭代
var StandardQuestions = []string{
	// department (单部门销售)
	"上月电商部销售多少",
	"本月分销部销售多少",
	// overview (全公司总览)
	"公司本月销售多少",
	"公司上月销售多少",
	// shop_rank (店铺排行)
	"本月哪个店卖最好",
	"本月哪个店卖最差",
	// product_rank (商品排行)
	"本月卖最好的商品 TOP 5",
	"本月卖最差的商品 TOP 10",
	// trend (两段对比)
	"本周对比上周销售情况",
	"本月对比上月销售情况",
	// stock_warning (缺货)
	"哪些 SKU 缺货",
	"本月缺货商品",
	// warehouse_flow (仓储发货)
	"本月发了多少单",
	"今日发了多少包裹",
	// rpa_status (RPA 数据日期)
	"天猫数据最新到哪天",
	"京东数据最新到哪天",
}

// WarmAsk 预计算一次 (不 persist 入库, 避免污染 user 统计数据)
// 失败不阻断, log 即可 (下一题继续)
func (s *Service) WarmAsk(ctx context.Context, question string) error {
	key := s.cacheKey(question)
	if cached := s.cacheGet(key); cached != nil {
		// 已有 cache 跳过, 省一次 LLM 调用
		return nil
	}
	result, err := s.askInternal(ctx, question, time.Now())
	if err != nil {
		return err
	}
	result.LLMModel = s.Client.Model
	s.cacheSet(key, result)
	return nil
}

// WarmCache 灌一遍所有标准题
// 错峰: 题间 sleep 5s, 避免 LLM 限流 (16 题约 12 分钟)
func (s *Service) WarmCache(ctx context.Context) {
	if !s.CacheEnabled {
		log.Printf("[ai-warmcache] cache disabled, skip")
		return
	}
	start := time.Now()
	successCnt, errCnt, skipCnt := 0, 0, 0
	log.Printf("[ai-warmcache] start warming %d standard questions", len(StandardQuestions))

	for i, q := range StandardQuestions {
		if ctx.Err() != nil {
			log.Printf("[ai-warmcache] ctx cancelled at %d/%d", i, len(StandardQuestions))
			return
		}
		// 单题 timeout 60s (跟用户提问保持一致)
		qctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		err := s.WarmAsk(qctx, q)
		cancel()
		if err != nil {
			errCnt++
			log.Printf("[ai-warmcache] [%d/%d] %q FAILED: %v", i+1, len(StandardQuestions), q, err)
		} else {
			// 不知道是 skip 还是 success, 简单当 success
			successCnt++
		}
		// 错峰间隔 5s 避 LLM 限流
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			log.Printf("[ai-warmcache] ctx cancelled during sleep at %d/%d", i+1, len(StandardQuestions))
			return
		}
	}

	hits, misses, size := s.CacheStats()
	log.Printf("[ai-warmcache] done in %v: success=%d err=%d skip=%d | cache_stats hits=%d misses=%d size=%d",
		time.Since(start).Round(time.Second), successCnt, errCnt, skipCnt, hits, misses, size)
}

// nextWarmCacheTime 计算下次触发时刻 (今天 HH:MM 已过则取明天)
func nextWarmCacheTime(hour, minute int) time.Time {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

// RunWarmCacheLoop 主调度循环, bi-server 启动时 go 一份
// ctx 关闭即退出 (bi-server graceful shutdown 用)
func (s *Service) RunWarmCacheLoop(ctx context.Context) {
	if !s.WarmCacheEnabled {
		log.Printf("[ai-warmcache] disabled (config.ai_assistant.warm_cache_enabled=false), loop skip")
		return
	}
	log.Printf("[ai-warmcache] loop started, daily trigger at %02d:%02d", s.WarmCacheHour, s.WarmCacheMinute)
	for {
		next := nextWarmCacheTime(s.WarmCacheHour, s.WarmCacheMinute)
		wait := time.Until(next)
		log.Printf("[ai-warmcache] next trigger: %s (in %v)", next.Format("2006-01-02 15:04:05"), wait.Round(time.Second))
		select {
		case <-time.After(wait):
			s.WarmCache(ctx)
		case <-ctx.Done():
			log.Printf("[ai-warmcache] loop stopped (ctx done)")
			return
		}
	}
}
