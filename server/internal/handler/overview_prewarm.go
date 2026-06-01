package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"
)

// 综合看板后台预热 (v1.75.20)
//
// 背景: /api/overview 冷查要跑十几条 SQL (177 万行的 sales_goods_summary 上聚合), 冷查 2~3s;
// 缓存命中后 ~1ms。但缓存会被每次同步清空, 用户撞上冷查就觉得"卡"。
//
// 方案: 后台 ticker 周期性把"默认日期范围 + 各去重权限范围"的 /api/overview 预先算好放进缓存,
// 让用户打开即热 (秒开)。配合 dashboard_cache.go 的 scopeCacheSig 共享缓存——只需为每个
// "去重权限范围"预热一份, 同权限的人共享。
//
// 关键: 预热构造的查询串(rawQuery)必须和前端 src/pages/overview/index.tsx 发的完全一致,
// 否则 WithCache 的 key 对不上, 预热白做。前端默认: ?start&end&trendStart&trendEnd (此顺序)。

const overviewPrewarmInterval = 90 * time.Second

// 安全上限: 正常公司去重权限范围 ≤10 个, 超过只预热前 N 个并告警 (防异常配置拖垮)
const overviewPrewarmMaxScopes = 30

// StartOverviewPrewarm 启动综合看板后台预热 goroutine。与 bi-server 进程同生命周期。
func (h *DashboardHandler) StartOverviewPrewarm() {
	go func() {
		// 启动后稍等, 让 DB / auth schema 就绪 (main.go 里 EnsureAuthSchemaAndSeed 之后才起 server)
		time.Sleep(8 * time.Second)
		h.safePrewarmOverview()
		ticker := time.NewTicker(overviewPrewarmInterval)
		defer ticker.Stop()
		for range ticker.C {
			h.safePrewarmOverview()
		}
	}()
}

// safePrewarmOverview 在 prewarmOverview 外再包一层 recover。
// 预热只是后台优化, 任何 panic (含 distinctActiveScopes 查 DB / loadAuthPayload 等) 都
// 绝不能沿这个顶层 goroutine 上抛拖垮 bi-server 主进程。每次 tick 都独立兜底。
func (h *DashboardHandler) safePrewarmOverview() {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[overview-prewarm] prewarmOverview panic 已捕获 (不影响主服务): %v", rec)
		}
	}()
	h.prewarmOverview()
}

// prewarmOverview 遍历去重权限范围, 仅对"当前缓存是冷的"那些预热 (热的跳过, 省资源)。
func (h *DashboardHandler) prewarmOverview() {
	start, end, trendStart, trendEnd := defaultOverviewParams()
	rawQuery := fmt.Sprintf("start=%s&end=%s&trendStart=%s&trendEnd=%s", start, end, trendStart, trendEnd)

	scopes := h.distinctActiveScopes()
	if len(scopes) > overviewPrewarmMaxScopes {
		log.Printf("[overview-prewarm] 去重权限范围 %d 个超过上限 %d, 只预热前 %d 个",
			len(scopes), overviewPrewarmMaxScopes, overviewPrewarmMaxScopes)
		scopes = scopes[:overviewPrewarmMaxScopes]
	}

	warmed, skipped := 0, 0
	for _, p := range scopes {
		// 与 WithCache 完全一致的 key, 命中即跳过
		key := fmt.Sprintf("api|/api/overview?%s|%s", rawQuery, scopeCacheSig(p))
		if _, ok := getOverviewCache(key); ok {
			skipped++
			continue
		}
		h.runOverviewPrewarm(p, rawQuery)
		warmed++
	}
	if warmed > 0 {
		log.Printf("[overview-prewarm] 焐热 %d 个权限范围 (已热跳过 %d), 区间 %s~%s", warmed, skipped, start, end)
	}
}

// distinctActiveScopes 枚举活跃用户, 按权限范围签名去重, 返回每种范围的一个代表 payload。
// 刻意【不】按"是否有 overview 权限"过滤: 多焐几个没人能看的范围只是少量浪费(每范围最多
// 24h 焐一次, 且有 overviewPrewarmMaxScopes 上限); 反过来若按权限名过滤, 权限名一旦漂移
// 就会漏焐真实可见范围 → 那些用户回到冷查, 失败方向更糟。故选"宁可多焐"。
func (h *DashboardHandler) distinctActiveScopes() []*authPayload {
	rows, err := h.DB.Query(`SELECT id FROM users WHERE status = 'active'`)
	if err != nil {
		log.Printf("[overview-prewarm] 查活跃用户失败: %v", err)
		return nil
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if scanErr := rows.Scan(&id); scanErr == nil {
			ids = append(ids, id)
		}
	}
	_ = rows.Close()

	seen := map[string]bool{}
	var out []*authPayload
	for _, id := range ids {
		p, perr := h.loadAuthPayload(id)
		if perr != nil {
			continue
		}
		sig := scopeCacheSig(p)
		if seen[sig] {
			continue
		}
		seen[sig] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		// 全部 loadAuthPayload 失败 / 没有活跃用户 → 预热本轮为空, 显式告警 (否则静默无声)
		log.Printf("[overview-prewarm] 警告: 未枚举到任何活跃用户权限范围, 本轮预热为空 (DB 异常或全部 loadAuthPayload 失败?)")
	}
	return out
}

// runOverviewPrewarm 以指定权限 payload 在后台跑一遍 /api/overview, 走与真实请求完全相同的
// WithCache→GetOverview 路径, 自动填充缓存 (保证预热数据与用户实际看到的字节一致)。
func (h *DashboardHandler) runOverviewPrewarm(p *authPayload, rawQuery string) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[overview-prewarm] panic 已捕获 (不影响主服务): %v", rec)
		}
	}()

	req, err := http.NewRequest(http.MethodGet, "/api/overview?"+rawQuery, nil)
	if err != nil {
		return
	}
	req.URL.RawQuery = rawQuery // 显式钉住顺序, 确保与前端请求的 key 一致
	ctx := context.WithValue(req.Context(), currentAuthPayloadKey, p)
	req = req.WithContext(ctx)

	// 直接调 WithCache(GetOverview): 跳过权限中间件 (预热只算数据, 不涉及访问授权——
	// 真实用户取数时仍走 RequirePermission 校验, 数据只按 scope 共享, 不会越权)。
	h.WithCache(24*time.Hour, h.GetOverview)(&discardWriter{}, req)
}

// defaultOverviewParams 复刻前端默认日期 (src/config.ts + src/pages/overview/index.tsx):
//   end       = 昨天
//   start     = 本月1号; 若本月1号晚于昨天(即今天是1号) → 取上月1号
//   trendStart= 若 (end-start) <= 3 天 → end 往前 13 天; 否则 = start
//   trendEnd  = end
func defaultOverviewParams() (start, end, trendStart, trendEnd string) {
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	startT := monthStart
	if monthStart.After(yesterday) {
		// 今天是1号: 昨天属于上月 → 用上月1号
		startT = time.Date(yesterday.Year(), yesterday.Month(), 1, 0, 0, 0, 0, yesterday.Location())
	}
	start = startT.Format("2006-01-02")
	end = yesterday.Format("2006-01-02")

	// 用解析后的纯日期算天数差, 与前端 dayjs(e).diff(dayjs(s),'day') 对齐
	sT, _ := time.Parse("2006-01-02", start)
	eT, _ := time.Parse("2006-01-02", end)
	if int(eT.Sub(sT).Hours()/24) <= 3 {
		trendStart = eT.AddDate(0, 0, -13).Format("2006-01-02")
	} else {
		trendStart = start
	}
	trendEnd = end
	return
}

// discardWriter 是预热用的丢弃式 ResponseWriter: WithCache 在内部记录响应体并写缓存,
// 真正写到这里的字节直接丢掉 (预热不需要返回给任何人)。
type discardWriter struct {
	header http.Header
	status int
}

func (d *discardWriter) Header() http.Header {
	if d.header == nil {
		d.header = http.Header{}
	}
	return d.header
}

func (d *discardWriter) Write(b []byte) (int, error) { return len(b), nil }

func (d *discardWriter) WriteHeader(status int) { d.status = status }
