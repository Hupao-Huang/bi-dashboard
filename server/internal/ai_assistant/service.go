package ai_assistant

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
	"time"
)

// Service AI 智能助手主服务
// W1 demo: 仅 department 模块走 SQL
// W3a (v1.73.0): 扩 8 modules (department/overview/shop_rank/product_rank/trend/stock_warning/warehouse_flow/rpa_status)
// W3a fix: classify 用 ClientPrimary (准), format 用 ClientFast (快)
// W3b 待办: Text-to-SQL fallback (LLM 生成 SQL + 白名单 + 注入防护)
type Service struct {
	DB         *sql.DB
	Client     *LLMClient // 兼容旧字段 = ClientPrimary
	ClientFast *LLMClient // 用于 formatAnswer; nil 时回退 Client

	// v1.74.0 P0: 完整问答结果缓存 (40s → < 50ms, 重复问题)
	CacheEnabled bool          // 总开关, false = 全部跳过 cache 路径
	CacheTTL     time.Duration // 单条 TTL, 0 = 同 disable
	cache        sync.Map      // string (cacheKey) -> *cacheEntry
	cacheHits    int64         // atomic 命中计数
	cacheMisses  int64         // atomic 未命中计数

	// v1.74.0 P2: 预计算 warm cache 调度
	WarmCacheEnabled bool // 总开关, true = bi-server 内部 goroutine 每天定时灌
	WarmCacheHour    int  // 0-23, 默认 0
	WarmCacheMinute  int  // 0-59, 默认 30
}

// FastClient 返回 fast client, 没配置时回退 primary
func (s *Service) FastClient() *LLMClient {
	if s.ClientFast != nil {
		return s.ClientFast
	}
	return s.Client
}

// Intent 意图识别结果 (LLM 输出)
type Intent struct {
	Type       string            `json:"type"`       // "see" 看数 / "rank" 排行 / "trend" 趋势 / "unknown"
	Module     string            `json:"module"`     // 见 routeModules 枚举
	Params     map[string]string `json:"params"`     // dept/start/end/shop/platform/sku/limit/...
	Confidence float64           `json:"confidence"` // 0.0-1.0
	Reasoning  string            `json:"reasoning"`  // LLM 给的判断理由
}

// UnmarshalJSON 容错: LLM 偶尔会把 params 里的 limit/order 返成 number/bool
// 自动强转 string, 避免端到端 fail
func (i *Intent) UnmarshalJSON(data []byte) error {
	type rawIntent struct {
		Type       string                     `json:"type"`
		Module     string                     `json:"module"`
		Params     map[string]json.RawMessage `json:"params"`
		Confidence float64                    `json:"confidence"`
		Reasoning  string                     `json:"reasoning"`
	}
	var r rawIntent
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	i.Type = r.Type
	i.Module = r.Module
	i.Confidence = r.Confidence
	i.Reasoning = r.Reasoning
	i.Params = make(map[string]string, len(r.Params))
	for k, v := range r.Params {
		raw := strings.TrimSpace(string(v))
		if raw == "null" || raw == "" {
			i.Params[k] = ""
			continue
		}
		// 字符串原样剥引号
		if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
			var s string
			if err := json.Unmarshal(v, &s); err == nil {
				i.Params[k] = s
				continue
			}
		}
		// number / bool / 其他 → 直接 fmt.Sprint
		i.Params[k] = strings.Trim(raw, `"`)
	}
	return nil
}

// AskResult 给 handler 的最终回答
type AskResult struct {
	SessionID    int64             `json:"sessionId"`
	MessageID    int64             `json:"messageId"`
	Answer       string            `json:"answer"`
	SourceType   string            `json:"sourceType"` // "api" / "sql" / "unknown"
	SourceAPI    string            `json:"sourceAPI,omitempty"`
	SourceParams map[string]string `json:"sourceParams,omitempty"`
	RawData      interface{}       `json:"rawData,omitempty"`
	Confidence   float64           `json:"confidence"`
	DurationMs   int64             `json:"durationMs"`
	Warning      string            `json:"warning,omitempty"`
	Intent       *Intent           `json:"intent,omitempty"`
	LLMTokens    int               `json:"llmTokens"`
	LLMModel     string            `json:"llmModel"`
	FromCache    bool              `json:"fromCache,omitempty"` // v1.74.0: true = cache 命中, 跳过 LLM
}

// Ask 主入口: 用户问题 → 答案 (含持久化 + v1.74.0 P0 cache)
// userID: 提问用户 (必填, 入库用)
// sessionID: nil = 新会话自动建; 非 nil = 续会话
func (s *Service) Ask(ctx context.Context, userID int64, sessionID *int64, question string) (*AskResult, error) {
	start := time.Now()

	// v1.74.0 P0: cache check (重复问题秒回, 跳过 2 次 LLM 调用)
	key := s.cacheKey(question)
	if cached := s.cacheGet(key); cached != nil {
		result := cloneResult(cached)
		result.DurationMs = time.Since(start).Milliseconds()
		result.FromCache = true
		// v1.74.2 修 Bug2: persist 用独立 ctx, 不绑 LLM 用过的 60s ctx
		s.persistAsync(userID, sessionID, question, result)
		return result, nil
	}

	result, err := s.askInternal(ctx, question, start)
	if err != nil {
		return nil, err
	}
	result.LLMModel = s.Client.Model

	// 灌 cache (cacheSet 内部会拒绝 unknown/warning 答案)
	s.cacheSet(key, result)

	// v1.74.2 修 Bug2: persist 用独立 ctx, 避免 LLM 撑满 60s 后 DB INSERT 失败
	s.persistAsync(userID, sessionID, question, result)
	return result, nil
}

// persistAsync 用独立 ctx 持久化 (5s timeout)
// 关键: 不绑外层 LLM 用过的 ctx, 否则 LLM 跑满 60s 后 INSERT ctx deadline exceeded → DB 漏行
func (s *Service) persistAsync(userID int64, sessionID *int64, question string, result *AskResult) {
	persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.persistAsk(persistCtx, userID, sessionID, question, result); err != nil {
		log.Printf("[ai-assistant] persist failed: %v", err)
	}
}

// askInternal 主流程
func (s *Service) askInternal(ctx context.Context, question string, start time.Time) (*AskResult, error) {
	// Step 1: 意图识别 (LLM Call 1)
	intent, classifyTokens, err := s.classifyIntent(ctx, question)
	if err != nil {
		return nil, fmt.Errorf("意图识别失败: %w", err)
	}
	log.Printf("[ai-assistant] question=%q intent=%+v", question, intent)

	// 置信度低 → 直接反问
	if intent.Confidence < 0.7 || intent.Type == "unknown" {
		return &AskResult{
			Answer:     fmt.Sprintf("抱歉, 我没完全听懂你的问题 (%s). 你可以试试: '上月电商部销售多少' / '本月哪个店卖最差' / '这周对比上周' / '哪些 SKU 缺货' / '今日发了多少单' / '天猫数据到 5/22 了吗'", intent.Reasoning),
			SourceType: "unknown",
			Confidence: intent.Confidence,
			DurationMs: time.Since(start).Milliseconds(),
			Intent:     intent,
			LLMTokens:  classifyTokens,
		}, nil
	}

	// Step 2: 路由到内部接口
	rawData, apiPath, routeErr := s.route(ctx, intent)
	if routeErr != nil {
		return &AskResult{
			Answer:     fmt.Sprintf("我理解了你的问题 (%s), 但调内部接口失败: %v. 你可以问问别的, 或者联系跑哥看看接口状态.", intent.Reasoning, routeErr),
			SourceType: "api",
			SourceAPI:  apiPath,
			Confidence: intent.Confidence,
			DurationMs: time.Since(start).Milliseconds(),
			Intent:     intent,
			LLMTokens:  classifyTokens,
		}, nil
	}

	// Step 3: LLM 二次组织成人话 (LLM Call 2, 三级 fallback)
	// rawData 大时截断喂 LLM, 防超时 (stock_warning TOP 20 / rpa_status 11 张表)
	truncated := truncateForLLM(rawData)
	answer, formatTokens, formatErr := s.formatAnswer(ctx, question, truncated)
	warning := ""
	if formatErr != nil {
		log.Printf("[ai-assistant] formatAnswer (fast) 失败, fallback primary: %v", formatErr)
		// fallback 到 primary 再试
		answer, formatTokens, formatErr = s.formatAnswerPrimary(ctx, question, truncated)
		if formatErr != nil {
			log.Printf("[ai-assistant] formatAnswer (primary) 也失败, 用模板兜底: %v", formatErr)
			// 模板兜底 — LLM 全挂时不让用户看到错误, 给原始数字
			answer = renderTemplateAnswer(intent.Module, truncated)
			warning = "AI 包装失败, 给的原始数据"
		}
	}

	return &AskResult{
		Answer:       answer,
		SourceType:   "api",
		SourceAPI:    apiPath,
		SourceParams: intent.Params,
		RawData:      rawData,
		Confidence:   intent.Confidence,
		DurationMs:   time.Since(start).Milliseconds(),
		Warning:      warning,
		Intent:       intent,
		LLMTokens:    classifyTokens + formatTokens,
	}, nil
}

// persistAsk 把一次问答入库
func (s *Service) persistAsk(ctx context.Context, userID int64, sessionID *int64, question string, result *AskResult) error {
	var sid int64
	if sessionID != nil && *sessionID > 0 {
		var ownerID int64
		if err := s.DB.QueryRowContext(ctx, "SELECT user_id FROM ai_chat_session WHERE id=?", *sessionID).Scan(&ownerID); err != nil {
			return fmt.Errorf("查 session %d 失败: %w", *sessionID, err)
		}
		if ownerID != userID {
			return fmt.Errorf("session %d 不属于 user %d (越权)", *sessionID, userID)
		}
		sid = *sessionID
		if _, err := s.DB.ExecContext(ctx, "UPDATE ai_chat_session SET updated_at=NOW() WHERE id=?", sid); err != nil {
			return fmt.Errorf("touch session 失败: %w", err)
		}
	} else {
		title := question
		if len([]rune(title)) > 100 {
			title = string([]rune(title)[:100])
		}
		res, err := s.DB.ExecContext(ctx, "INSERT INTO ai_chat_session (user_id, title) VALUES (?, ?)", userID, title)
		if err != nil {
			return fmt.Errorf("建 session 失败: %w", err)
		}
		sid, _ = res.LastInsertId()
	}
	result.SessionID = sid

	if _, err := s.DB.ExecContext(ctx,
		"INSERT INTO ai_chat_message (session_id, role, question) VALUES (?, 'user', ?)",
		sid, question); err != nil {
		return fmt.Errorf("插 user message 失败: %w", err)
	}

	intentJSON := ""
	if result.Intent != nil {
		if b, err := json.Marshal(result.Intent); err == nil {
			intentJSON = string(b)
		}
	}
	rawJSON := ""
	if result.RawData != nil {
		if b, err := json.Marshal(result.RawData); err == nil {
			rawJSON = string(b)
		}
	}
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO ai_chat_message
		(session_id, role, answer, intent_json, source_type, source_api, raw_data, confidence, llm_model, llm_tokens, duration_ms, warning)
		VALUES (?, 'assistant', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sid, result.Answer, intentJSON, result.SourceType, result.SourceAPI, rawJSON,
		result.Confidence, result.LLMModel, result.LLMTokens, result.DurationMs, result.Warning)
	if err != nil {
		return fmt.Errorf("插 assistant message 失败: %w", err)
	}
	mid, _ := res.LastInsertId()
	result.MessageID = mid
	return nil
}

// classifyIntent 意图识别 (LLM Call 1, 强制 JSON 输出)
func (s *Service) classifyIntent(ctx context.Context, question string) (*Intent, int, error) {
	now := time.Now()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	// v1.74.2 修 Bug1: 本周必须算到周日 (完整 7 天), 而不是 weekStart=today
	// 周日 Weekday()=0 在中国习惯下应视为本周最后一天 → 偏移 6 天回到周一
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	weekStartT := now.AddDate(0, 0, -(weekday - 1))
	weekStart := weekStartT.Format("2006-01-02")
	weekEnd := weekStartT.AddDate(0, 0, 6).Format("2006-01-02")
	lastWeekStart := weekStartT.AddDate(0, 0, -7).Format("2006-01-02")
	lastWeekEnd := weekStartT.AddDate(0, 0, -1).Format("2006-01-02")
	last7Start := now.AddDate(0, 0, -6).Format("2006-01-02")
	monthStart := now.Format("2006-01") + "-01"
	lastMonthStart := now.AddDate(0, -1, 0).Format("2006-01") + "-01"
	lastMonthEnd := now.AddDate(0, 0, -now.Day()).Format("2006-01-02")

	systemPrompt := fmt.Sprintf(`你是松鲜鲜 BI 看板的意图分类器. 用户问数据相关问题, 你输出 JSON 描述意图.

【时间口径 (今天%s)】
- 今天=%s, 昨天=%s
- 本周=%s 至 %s (本周一到本周日, 完整 7 天; 今天若在本周内, 未来日期数据为 0 属正常)
- 上周=%s 至 %s (上周一到上周日, 完整 7 天)
- 本月=%s 至 %s
- 上月=%s 至 %s
- 近 7 天=%s 至 %s
- 近 30 天=今日-29 到今日

【意图类型 type】
- see (看数): 单点数字查询, 如"上月电商部销售多少"
- rank (排行): TOP N 类, 如"哪个店最差/最好"
- trend (趋势/对比): 时间段对比, 如"这周对比上周"
- unknown: 听不懂 / 涉及敏感 (工资/密码/删除/用户列表) / 跨域复杂问题

【可路由模块 module (8 个)】

1. module="department" — 单部门销售详情
   params 必填: dept
   params 可选: start (YYYY-MM-DD), end (YYYY-MM-DD)
   答: "XX部上月销售多少" "电商部本月销售"

2. module="overview" — 全公司总销售/订单/SKU/部门汇总
   params 可选: start, end
   答: "公司上月销售多少" "本月总销售" "全公司今天卖了多少"

3. module="shop_rank" — 店铺排行 TOP N
   params 可选: start, end, dept (限定部门), order (desc=最好/asc=最差), limit (默认 10)
   答: "本月哪个店卖最好" "上周电商部哪个店最差" "TOP 5 店铺"

4. module="product_rank" — 商品/品类/SKU 排行
   params 可选: start, end, dim (goods=商品/cate=品类/sku=SKU/brand=品牌, 默认 goods), order, limit
   答: "卖最好的商品 TOP 10" "哪个品类最大" "本月畅销 SKU"

5. module="trend" — 时间段对比 (两段日期销售对比)
   params 必填: this_start, this_end, prev_start, prev_end
   params 可选: dept, shop
   答: "本周对比上周" "本月对比上月" "电商部 5 月对比 4 月"

6. module="stock_warning" — 库存预警/缺货 SKU
   params 可选: warehouse (仓库名关键字), limit (默认 20)
   答: "哪些 SKU 缺货" "X 仓有多少缺货 SKU" "库存预警 TOP"

7. module="warehouse_flow" — 仓储发货/包裹数
   params 可选: ym (年月 YYYY-MM, 默认本月), shop, warehouse
   答: "本月发了多少单/包裹" "X 仓发货量" "X 店本月发货"

8. module="rpa_status" — RPA 平台数据到没到 (各平台最近导入日期)
   params 可选: platform (tmall/jd/pdd/douyin/xhs/vip/kuaishou, 默认所有)
   答: "天猫数据到 5/22 了吗" "京东数据最新到哪天" "RPA 状态"

【业务术语字典】

部门映射 (dept 严格枚举):
- 电商部 → ecommerce  (天猫/京东/拼多多/唯品会等公域电商)
- 社媒部 → social  (抖音/快手/小红书/视频小店)
- 线下 / 线下大区 / 线下渠道 → offline
- 分销 → distribution  (分销一组~分销八组/蜂享家)
- 即时零售 → instant_retail  (美团/饿了么/朴朴/盒马)
- 注意: 没有"中台"部门, 也没有"全公司"部门 (全公司请用 overview)

平台/店铺识别:
- 店名命名规律: "ds-平台-店名" (公域电商) / "社媒-平台-店名" / "sy-平台-店名" (有赞) / "分销X组" / "线下渠道销售中心-X大区"
- 平台关键字 (用于 RPA 状态等): tmall=天猫, jd=京东, pdd=拼多多, douyin=抖音, xhs=小红书, vip=唯品会, kuaishou=快手
- 用户说"抖音店"可能指多个店, 不要硬 mapping 单店, 用 platform 参数

业务术语:
- 销售/营业额/销售额 → 字段 goods_amt (元)
- 销量/件数 → goods_qty
- 毛利 → gross_profit; 毛利率 → gross_profit_rate (按字面值, 如 25.30 表示 25.30%%)
- 客单价/单价 → tax_unit_price 或 avg_price
- 订单数/包裹数 → warehouse_flow 模块
- 缺货 → 库存 = 0 或 可用库存 <= 0
- 库存 → 用 stock_warning 模块

【输出格式 (严格 JSON)】
{
  "type": "see|rank|trend|unknown",
  "module": "department|overview|shop_rank|product_rank|trend|stock_warning|warehouse_flow|rpa_status|null",
  "params": {...},
  "confidence": 0.95,
  "reasoning": "用户问 XX, 路由到 XX 模块"
}

【硬规则】
- 不懂 → type=unknown, module=null, confidence<0.7, reasoning 说明为什么
- 涉及敏感 (工资/密码/删除/admin/SQL) → 一律 unknown
- 跨模块/复杂复合问题 → unknown (W3a 只支持单模块单查询)
- 时间没说默认 (see/rank): "本月", (trend): 必须用户明说两段
- limit 没说默认: rank=10, stock_warning=20`,
		today, today, yesterday,
		weekStart, weekEnd,
		lastWeekStart, lastWeekEnd,
		monthStart, today,
		lastMonthStart, lastMonthEnd,
		last7Start, today)

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: question},
	}

	content, tokens, err := s.Client.Chat(ctx, messages, true)
	if err != nil {
		return nil, 0, err
	}

	var intent Intent
	if err := json.Unmarshal([]byte(content), &intent); err != nil {
		return nil, tokens, fmt.Errorf("LLM 输出 JSON 解析失败: %w content=%.300s", err, content)
	}
	return &intent, tokens, nil
}

// truncateForLLM 大列表截断到前 5 条, 避免 LLM 处理百行明细超时
// 用反射, 兼容 []item / []map[string]any / []string 等任意 slice
func truncateForLLM(raw interface{}) interface{} {
	m, ok := raw.(map[string]interface{})
	if !ok {
		return raw
	}
	const maxRows = 5
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Slice && rv.Len() > maxRows {
			out[k] = rv.Slice(0, maxRows).Interface()
			out[k+"_truncatedTotal"] = rv.Len()
		}
	}
	return out
}

// formatAnswer 把数据包装成人话 (LLM Call 2)
func (s *Service) formatAnswer(ctx context.Context, question string, rawData interface{}) (string, int, error) {
	dataJSON, _ := json.Marshal(rawData)

	systemPrompt := `你是松鲜鲜 BI 看板的回答生成器. 用户问了一个数据问题, 我已经查到数据, 你用 1-2 句中文回答.

格式要求:
- 必含关键数字 (带单位: 元/万/单/% 等; 大数字用"万"或"亿"简化, 如 12345678 → "1234 万")
- 时间范围放括号里说清楚
- 不写"以下是查询结果"这种废话, 直接给数字
- 不用 markdown (不要 ** / # / 表格), 自然语言一行
- 排行类列前 3-5 名即可, 不全列
- 如果数字看起来异常 (如 totalAmount=0, day_count<3, 缺货数=0 等), 给个轻量提示`

	userMsg := fmt.Sprintf("用户原问题: %s\n\n查到的数据:\n%s\n\n请用 1-2 句话回答用户.", question, string(dataJSON))

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMsg},
	}

	// formatAnswer 用 fast client (glm-4.7-flash), 不需复杂推理, 主求快
	content, tokens, err := s.FastClient().Chat(ctx, messages, false)
	if err != nil {
		return "", 0, err
	}
	return strings.TrimSpace(content), tokens, nil
}

// formatAnswerPrimary 强制走 primary 模型 (用于 fast 限流/timeout 后 fallback)
func (s *Service) formatAnswerPrimary(ctx context.Context, question string, rawData interface{}) (string, int, error) {
	dataJSON, _ := json.Marshal(rawData)
	systemPrompt := `你是松鲜鲜 BI 看板的回答生成器. 用 1-2 句中文回答数据问题, 必含关键数字 (元/万单位), 时间放括号. 不写"以下是查询结果", 不用 markdown.`
	userMsg := fmt.Sprintf("用户原问题: %s\n\n查到的数据:\n%s\n\n请用 1-2 句话回答用户.", question, string(dataJSON))
	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMsg},
	}
	content, tokens, err := s.Client.Chat(ctx, messages, false)
	if err != nil {
		return "", 0, err
	}
	return strings.TrimSpace(content), tokens, nil
}

// renderTemplateAnswer LLM 全挂时, 用代码模板生成兜底回答 (硬编码各 module 关键字段)
// v1.74.2 修 Bug3: wrapper 模式让所有 case return 自动带告警前缀, 明确告知用户"这不是 AI 完整解读"
const fallbackPrefix = "⚠️ AI 解读暂繁忙, 这是机器原始数字 (请稍后重试看完整解读): "

func renderTemplateAnswer(module string, raw interface{}) string {
	return fallbackPrefix + renderTemplateInner(module, raw)
}

func renderTemplateInner(module string, raw interface{}) string {
	m, ok := raw.(map[string]interface{})
	if !ok {
		return "查到了数据, 但 AI 包装失败. 请刷新重试."
	}
	getF := func(key string) float64 {
		if v, ok := m[key].(float64); ok {
			return v
		}
		return 0
	}
	getS := func(key string) string {
		if v, ok := m[key].(string); ok {
			return v
		}
		return ""
	}
	fmtW := func(amt float64) string {
		if amt > 10000 {
			return fmt.Sprintf("%.1f 万元", amt/10000)
		}
		return fmt.Sprintf("%.0f 元", amt)
	}

	switch module {
	case "department":
		return fmt.Sprintf("%s 部门 (%s 至 %s) 销售额 %s, 涉及 %d 家店.",
			getS("dept"), getS("start"), getS("end"),
			fmtW(getF("totalAmount")), int(getF("shopCount")))
	case "overview":
		return fmt.Sprintf("全公司 (%s 至 %s) 销售额 %s, 涉及 %d 家店.",
			getS("start"), getS("end"),
			fmtW(getF("totalAmount")), int(getF("shopCount")))
	case "shop_rank":
		return fmt.Sprintf("店铺排行 (%s 至 %s, 排序 %s) 已查到, 详见数据明细.",
			getS("start"), getS("end"), getS("order"))
	case "product_rank":
		return fmt.Sprintf("商品排行 (维度 %s, %s 至 %s) 已查到, 详见数据明细.",
			getS("dim"), getS("start"), getS("end"))
	case "trend":
		return fmt.Sprintf("本期销售 %s, 上期 %s, 变化 %.1f%%.",
			fmtW(getF("thisAmount")), fmtW(getF("prevAmount")), getF("deltaAmountPct"))
	case "stock_warning":
		return fmt.Sprintf("当前缺货 SKU 共 %d 个, TOP 已列出.", int(getF("stockoutCount")))
	case "warehouse_flow":
		return fmt.Sprintf("%s 共发 %.0f 单 / %.0f 个包裹.",
			getS("ym"), getF("orders"), getF("packages"))
	case "rpa_status":
		return "RPA 各平台数据状态已查到, 详见明细."
	default:
		return "查到了数据, 但 AI 包装暂时不可用, 请刷新重试."
	}
}

// =================== Route 分发 ===================

// route 按 intent.Module 路由到对应处理函数
func (s *Service) route(ctx context.Context, intent *Intent) (interface{}, string, error) {
	switch intent.Module {
	case "department":
		return s.routeDepartment(ctx, intent)
	case "overview":
		return s.routeOverview(ctx, intent)
	case "shop_rank":
		return s.routeShopRank(ctx, intent)
	case "product_rank":
		return s.routeProductRank(ctx, intent)
	case "trend":
		return s.routeTrend(ctx, intent)
	case "stock_warning":
		return s.routeStockWarning(ctx, intent)
	case "warehouse_flow":
		return s.routeWarehouseFlow(ctx, intent)
	case "rpa_status":
		return s.routeRPAStatus(ctx, intent)
	default:
		return nil, "", fmt.Errorf("未支持的模块: %s", intent.Module)
	}
}

// 合法 dept 白名单 (W3a: 严格枚举, 防 LLM 输出非法值导致 SQL 异常)
var validDepts = map[string]bool{
	"ecommerce":      true,
	"social":         true,
	"offline":        true,
	"distribution":   true,
	"instant_retail": true,
}

// 合法 RPA 平台白名单
var validPlatforms = map[string]bool{
	"tmall":    true,
	"jd":       true,
	"pdd":      true,
	"douyin":   true,
	"xhs":      true,
	"vip":      true,
	"kuaishou": true,
}

// 默认时间区间 (本月初到今天)
func defaultRange(intent *Intent) (string, string) {
	start := intent.Params["start"]
	end := intent.Params["end"]
	if start == "" {
		start = time.Now().Format("2006-01") + "-01"
	}
	if end == "" {
		end = time.Now().Format("2006-01-02")
	}
	return start, end
}

// 限制 limit 在 [1, 50] 区间
func clampLimit(raw string, def, max int) int {
	if raw == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(raw, "%d", &n); err != nil {
		return def
	}
	if n < 1 {
		return 1
	}
	if n > max {
		return max
	}
	return n
}

// routeDepartment 单部门销售
func (s *Service) routeDepartment(ctx context.Context, intent *Intent) (interface{}, string, error) {
	dept := intent.Params["dept"]
	if !validDepts[dept] {
		return nil, "/api/department", fmt.Errorf("意图 dept=%q 不在白名单 (合法: ecommerce/social/offline/distribution/instant_retail)", dept)
	}
	start, end := defaultRange(intent)

	row := s.DB.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(goods_amt), 0) AS total_amount,
		COALESCE(SUM(goods_qty), 0) AS total_qty,
		COALESCE(SUM(gross_profit), 0) AS total_profit,
		COUNT(DISTINCT shop_name) AS shop_count,
		COUNT(DISTINCT stat_date) AS day_count,
		MIN(stat_date) AS first_date,
		MAX(stat_date) AS last_date
	FROM sales_goods_summary
	WHERE department = ? AND stat_date BETWEEN ? AND ?`, dept, start, end)

	var totalAmount, totalQty, totalProfit float64
	var shopCount, dayCount int
	var firstDate, lastDate sql.NullString
	if err := row.Scan(&totalAmount, &totalQty, &totalProfit, &shopCount, &dayCount, &firstDate, &lastDate); err != nil {
		return nil, "/api/department", fmt.Errorf("查询失败: %w", err)
	}

	return map[string]interface{}{
		"dept":        dept,
		"start":       start,
		"end":         end,
		"totalAmount": totalAmount,
		"totalQty":    totalQty,
		"totalProfit": totalProfit,
		"shopCount":   shopCount,
		"dayCount":    dayCount,
		"firstDate":   firstDate.String,
		"lastDate":    lastDate.String,
	}, fmt.Sprintf("/api/department?dept=%s&start=%s&end=%s", dept, start, end), nil
}

// routeOverview 全公司总览
func (s *Service) routeOverview(ctx context.Context, intent *Intent) (interface{}, string, error) {
	start, end := defaultRange(intent)

	// 总数 + 各部门拆分
	row := s.DB.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(goods_amt), 0) AS total_amount,
		COALESCE(SUM(goods_qty), 0) AS total_qty,
		COALESCE(SUM(gross_profit), 0) AS total_profit,
		COUNT(DISTINCT shop_name) AS shop_count
	FROM sales_goods_summary
	WHERE stat_date BETWEEN ? AND ?`, start, end)
	var totalAmount, totalQty, totalProfit float64
	var shopCount int
	if err := row.Scan(&totalAmount, &totalQty, &totalProfit, &shopCount); err != nil {
		return nil, "/api/overview", fmt.Errorf("查询失败: %w", err)
	}

	rows, err := s.DB.QueryContext(ctx, `SELECT
		IFNULL(department,'(未分类)') AS dept,
		COALESCE(SUM(goods_amt),0) AS amt
	FROM sales_goods_summary
	WHERE stat_date BETWEEN ? AND ?
	GROUP BY department
	ORDER BY amt DESC`, start, end)
	if err != nil {
		return nil, "/api/overview", fmt.Errorf("部门汇总查询失败: %w", err)
	}
	defer rows.Close()
	type deptItem struct {
		Dept   string  `json:"dept"`
		Amount float64 `json:"amount"`
	}
	var depts []deptItem
	for rows.Next() {
		var it deptItem
		if err := rows.Scan(&it.Dept, &it.Amount); err != nil {
			return nil, "/api/overview", err
		}
		depts = append(depts, it)
	}

	return map[string]interface{}{
		"start":       start,
		"end":         end,
		"totalAmount": totalAmount,
		"totalQty":    totalQty,
		"totalProfit": totalProfit,
		"shopCount":   shopCount,
		"byDept":      depts,
	}, fmt.Sprintf("/api/overview?start=%s&end=%s", start, end), nil
}

// routeShopRank 店铺排行
func (s *Service) routeShopRank(ctx context.Context, intent *Intent) (interface{}, string, error) {
	start, end := defaultRange(intent)
	limit := clampLimit(intent.Params["limit"], 10, 50)
	order := strings.ToUpper(intent.Params["order"])
	if order != "ASC" {
		order = "DESC"
	}
	dept := intent.Params["dept"]
	args := []interface{}{start, end}
	cond := "stat_date BETWEEN ? AND ?"
	if dept != "" {
		if !validDepts[dept] {
			return nil, "/api/shop-rank", fmt.Errorf("dept=%q 不在白名单", dept)
		}
		cond += " AND department = ?"
		args = append(args, dept)
	}

	q := fmt.Sprintf(`SELECT shop_name,
		COALESCE(SUM(goods_amt),0) AS amt,
		COALESCE(SUM(goods_qty),0) AS qty,
		COALESCE(SUM(gross_profit),0) AS profit
	FROM sales_goods_summary
	WHERE %s AND shop_name IS NOT NULL AND shop_name <> ''
	GROUP BY shop_name
	ORDER BY amt %s
	LIMIT %d`, cond, order, limit)

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "/api/shop-rank", fmt.Errorf("查询失败: %w", err)
	}
	defer rows.Close()
	type item struct {
		Shop   string  `json:"shop"`
		Amount float64 `json:"amount"`
		Qty    float64 `json:"qty"`
		Profit float64 `json:"profit"`
	}
	var list []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.Shop, &it.Amount, &it.Qty, &it.Profit); err != nil {
			return nil, "/api/shop-rank", err
		}
		list = append(list, it)
	}
	return map[string]interface{}{
		"start": start, "end": end, "dept": dept, "order": order, "limit": limit,
		"rank": list,
	}, fmt.Sprintf("/api/shop-rank?start=%s&end=%s&dept=%s&order=%s&limit=%d", start, end, dept, order, limit), nil
}

// routeProductRank 商品/品类/SKU 排行
func (s *Service) routeProductRank(ctx context.Context, intent *Intent) (interface{}, string, error) {
	start, end := defaultRange(intent)
	limit := clampLimit(intent.Params["limit"], 10, 50)
	order := strings.ToUpper(intent.Params["order"])
	if order != "ASC" {
		order = "DESC"
	}
	dim := intent.Params["dim"]
	groupCol := "goods_name"
	switch dim {
	case "cate":
		groupCol = "cate_name"
	case "sku":
		groupCol = "sku_name"
	case "brand":
		groupCol = "brand_name"
	case "goods", "":
		groupCol = "goods_name"
	default:
		return nil, "/api/product-rank", fmt.Errorf("dim=%q 非法 (合法: goods/cate/sku/brand)", dim)
	}

	q := fmt.Sprintf(`SELECT IFNULL(%s,'(无)') AS name,
		COALESCE(SUM(goods_amt),0) AS amt,
		COALESCE(SUM(goods_qty),0) AS qty
	FROM sales_goods_summary
	WHERE stat_date BETWEEN ? AND ? AND %s IS NOT NULL AND %s <> ''
	GROUP BY %s
	ORDER BY amt %s
	LIMIT %d`, groupCol, groupCol, groupCol, groupCol, order, limit)

	rows, err := s.DB.QueryContext(ctx, q, start, end)
	if err != nil {
		return nil, "/api/product-rank", fmt.Errorf("查询失败: %w", err)
	}
	defer rows.Close()
	type item struct {
		Name   string  `json:"name"`
		Amount float64 `json:"amount"`
		Qty    float64 `json:"qty"`
	}
	var list []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.Name, &it.Amount, &it.Qty); err != nil {
			return nil, "/api/product-rank", err
		}
		list = append(list, it)
	}
	return map[string]interface{}{
		"start": start, "end": end, "dim": dim, "order": order, "limit": limit,
		"rank": list,
	}, fmt.Sprintf("/api/product-rank?start=%s&end=%s&dim=%s&order=%s&limit=%d", start, end, dim, order, limit), nil
}

// routeTrend 时间段对比
func (s *Service) routeTrend(ctx context.Context, intent *Intent) (interface{}, string, error) {
	thisStart := intent.Params["this_start"]
	thisEnd := intent.Params["this_end"]
	prevStart := intent.Params["prev_start"]
	prevEnd := intent.Params["prev_end"]
	if thisStart == "" || thisEnd == "" || prevStart == "" || prevEnd == "" {
		return nil, "/api/trend", fmt.Errorf("对比类问题必须给两段时间 (this_start/this_end/prev_start/prev_end)")
	}
	dept := intent.Params["dept"]
	deptCond := ""
	args := func(s, e string) []interface{} {
		base := []interface{}{s, e}
		if dept != "" {
			base = append(base, dept)
		}
		return base
	}
	if dept != "" {
		if !validDepts[dept] {
			return nil, "/api/trend", fmt.Errorf("dept=%q 不在白名单", dept)
		}
		deptCond = " AND department = ?"
	}

	q := fmt.Sprintf(`SELECT
		COALESCE(SUM(goods_amt),0) AS amt,
		COALESCE(SUM(goods_qty),0) AS qty,
		COALESCE(SUM(gross_profit),0) AS profit
	FROM sales_goods_summary
	WHERE stat_date BETWEEN ? AND ?%s`, deptCond)

	var thisAmt, thisQty, thisProfit float64
	if err := s.DB.QueryRowContext(ctx, q, args(thisStart, thisEnd)...).Scan(&thisAmt, &thisQty, &thisProfit); err != nil {
		return nil, "/api/trend", fmt.Errorf("本期查询失败: %w", err)
	}
	var prevAmt, prevQty, prevProfit float64
	if err := s.DB.QueryRowContext(ctx, q, args(prevStart, prevEnd)...).Scan(&prevAmt, &prevQty, &prevProfit); err != nil {
		return nil, "/api/trend", fmt.Errorf("上期查询失败: %w", err)
	}

	pct := func(now, prev float64) float64 {
		if prev == 0 {
			return 0
		}
		return (now - prev) / prev * 100
	}

	return map[string]interface{}{
		"dept":           dept,
		"thisStart":      thisStart,
		"thisEnd":        thisEnd,
		"prevStart":      prevStart,
		"prevEnd":        prevEnd,
		"thisAmount":     thisAmt,
		"prevAmount":     prevAmt,
		"deltaAmountPct": pct(thisAmt, prevAmt),
		"thisQty":        thisQty,
		"prevQty":        prevQty,
		"deltaQtyPct":    pct(thisQty, prevQty),
		"thisProfit":     thisProfit,
		"prevProfit":     prevProfit,
		"deltaProfitPct": pct(thisProfit, prevProfit),
	}, fmt.Sprintf("/api/trend?dept=%s&this=%s~%s&prev=%s~%s", dept, thisStart, thisEnd, prevStart, prevEnd), nil
}

// routeStockWarning 库存预警 (缺货 SKU)
// 复用 GetStockWarning 同款公式: current_qty - locked_qty <= 0 AND month_qty > 0
func (s *Service) routeStockWarning(ctx context.Context, intent *Intent) (interface{}, string, error) {
	limit := clampLimit(intent.Params["limit"], 20, 50)
	warehouseKw := strings.TrimSpace(intent.Params["warehouse"])

	cond := "(current_qty - locked_qty) <= 0 AND month_qty > 0"
	args := []interface{}{}
	if warehouseKw != "" {
		cond += " AND warehouse_name LIKE ?"
		args = append(args, "%"+warehouseKw+"%")
	}

	// 1. 统计: 缺货总数
	var stockoutCount int
	if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM stock_quantity WHERE "+cond, args...).Scan(&stockoutCount); err != nil {
		return nil, "/api/stock/warning", fmt.Errorf("缺货统计失败: %w", err)
	}

	// 2. TOP N 缺货明细
	q := fmt.Sprintf(`SELECT goods_name, sku_name, warehouse_name, current_qty, locked_qty, month_qty
	FROM stock_quantity
	WHERE %s
	ORDER BY month_qty DESC
	LIMIT %d`, cond, limit)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "/api/stock/warning", fmt.Errorf("查询失败: %w", err)
	}
	defer rows.Close()
	type item struct {
		GoodsName   string  `json:"goodsName"`
		SkuName     string  `json:"skuName"`
		Warehouse   string  `json:"warehouse"`
		CurrentQty  float64 `json:"currentQty"`
		LockedQty   float64 `json:"lockedQty"`
		MonthQty    float64 `json:"monthQty"`
	}
	var list []item
	for rows.Next() {
		var it item
		var gn, sn, wn sql.NullString
		if err := rows.Scan(&gn, &sn, &wn, &it.CurrentQty, &it.LockedQty, &it.MonthQty); err != nil {
			return nil, "/api/stock/warning", err
		}
		it.GoodsName = gn.String
		it.SkuName = sn.String
		it.Warehouse = wn.String
		list = append(list, it)
	}
	return map[string]interface{}{
		"warehouseKw":   warehouseKw,
		"stockoutCount": stockoutCount,
		"limit":         limit,
		"top":           list,
	}, fmt.Sprintf("/api/stock/warning?warehouse=%s&limit=%d", warehouseKw, limit), nil
}

// routeWarehouseFlow 仓储发货量
func (s *Service) routeWarehouseFlow(ctx context.Context, intent *Intent) (interface{}, string, error) {
	ym := intent.Params["ym"]
	if ym == "" {
		ym = time.Now().Format("2006-01")
	}
	// 验 YYYY-MM 格式
	if _, err := time.Parse("2006-01", ym); err != nil {
		return nil, "/api/warehouse-flow", fmt.Errorf("ym=%q 非法 (格式 YYYY-MM)", ym)
	}
	shopKw := strings.TrimSpace(intent.Params["shop"])
	warehouseKw := strings.TrimSpace(intent.Params["warehouse"])

	cond := "ym = ?"
	args := []interface{}{ym}
	if shopKw != "" {
		cond += " AND shop_name LIKE ?"
		args = append(args, "%"+shopKw+"%")
	}
	if warehouseKw != "" {
		cond += " AND warehouse_name LIKE ?"
		args = append(args, "%"+warehouseKw+"%")
	}

	row := s.DB.QueryRowContext(ctx, fmt.Sprintf(`SELECT
		COALESCE(SUM(orders),0) AS orders,
		COALESCE(SUM(packages),0) AS packages,
		COUNT(DISTINCT shop_name) AS shop_count,
		COUNT(DISTINCT warehouse_name) AS warehouse_count
	FROM warehouse_flow_summary WHERE %s`, cond), args...)
	var orders, packages, shopCount, warehouseCount int64
	if err := row.Scan(&orders, &packages, &shopCount, &warehouseCount); err != nil {
		return nil, "/api/warehouse-flow", fmt.Errorf("查询失败: %w", err)
	}
	return map[string]interface{}{
		"ym":             ym,
		"shopKw":         shopKw,
		"warehouseKw":    warehouseKw,
		"orders":         orders,
		"packages":       packages,
		"shopCount":      shopCount,
		"warehouseCount": warehouseCount,
	}, fmt.Sprintf("/api/warehouse-flow?ym=%s&shop=%s&warehouse=%s", ym, shopKw, warehouseKw), nil
}

// routeRPAStatus 各 RPA 平台数据最新日期
func (s *Service) routeRPAStatus(ctx context.Context, intent *Intent) (interface{}, string, error) {
	platform := intent.Params["platform"]
	if platform != "" && !validPlatforms[platform] {
		return nil, "/api/rpa-status", fmt.Errorf("platform=%q 不在白名单", platform)
	}

	// 各平台代表表 (取每平台 1-2 张主表查 MAX(stat_date))
	tables := map[string][]string{
		"tmall":    {"op_tmall_shop_daily", "op_tmall_goods_daily"},
		"jd":       {"op_jd_shop_daily", "op_jd_customer_daily"},
		"pdd":      {"op_pdd_shop_daily", "op_pdd_goods_daily"},
		"douyin":   {"op_douyin_channel_daily", "op_douyin_goods_daily"},
		"xhs":      {"op_xhs_cs_trend_daily"},
		"vip":      {"op_vip_shop_daily"},
		"kuaishou": {"op_kuaishou_cs_assessment_daily"},
	}

	platforms := []string{}
	if platform != "" {
		platforms = []string{platform}
	} else {
		for p := range tables {
			platforms = append(platforms, p)
		}
	}

	type item struct {
		Platform string `json:"platform"`
		Table    string `json:"table"`
		LastDate string `json:"lastDate"`
	}
	var list []item
	for _, p := range platforms {
		for _, tb := range tables[p] {
			var last sql.NullString
			// 表名是 hardcoded 白名单, 拼接安全
			if err := s.DB.QueryRowContext(ctx, "SELECT IFNULL(MAX(stat_date),'') FROM "+tb).Scan(&last); err != nil {
				log.Printf("[ai-assistant] RPA 状态查 %s 失败: %v", tb, err)
				continue
			}
			list = append(list, item{Platform: p, Table: tb, LastDate: last.String})
		}
	}
	return map[string]interface{}{
		"platform": platform,
		"items":    list,
	}, fmt.Sprintf("/api/rpa-status?platform=%s", platform), nil
}
