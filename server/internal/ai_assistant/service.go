package ai_assistant

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// Service AI 智能助手主服务 (W1 demo)
// 当前只实现"看数"类问题 + 路由到 /api/department 一个接口
// W2 加更多 intent + 30 接口路由表; W3 加 Text-to-SQL fallback
type Service struct {
	DB     *sql.DB
	Client *LLMClient
}

// Intent 意图识别结果 (LLM 输出)
type Intent struct {
	Type       string            `json:"type"`       // "see" 看数 / "rank" 排行 / "trend" 趋势 / "unknown"
	Module     string            `json:"module"`     // "department" / "overview" / ... (W1 只支持 department)
	Params     map[string]string `json:"params"`     // dept/start/end/...
	Confidence float64           `json:"confidence"` // 0.0-1.0
	Reasoning  string            `json:"reasoning"`  // LLM 给的判断理由
}

// AskResult 给 handler 的最终回答
type AskResult struct {
	Answer       string                 `json:"answer"`
	SourceType   string                 `json:"sourceType"` // "api" / "sql" / "unknown"
	SourceAPI    string                 `json:"sourceAPI,omitempty"`
	SourceParams map[string]string      `json:"sourceParams,omitempty"`
	RawData      interface{}            `json:"rawData,omitempty"`
	Confidence   float64                `json:"confidence"`
	DurationMs   int64                  `json:"durationMs"`
	Warning      string                 `json:"warning,omitempty"`
	Intent       *Intent                `json:"intent,omitempty"`
	LLMTokens    int                    `json:"llmTokens"`
}

// Ask 主入口: 用户问题 → 答案
func (s *Service) Ask(ctx context.Context, question string) (*AskResult, error) {
	start := time.Now()

	// Step 1: 意图识别 (LLM Call 1)
	intent, classifyTokens, err := s.classifyIntent(ctx, question)
	if err != nil {
		return nil, fmt.Errorf("意图识别失败: %w", err)
	}
	log.Printf("[ai-assistant] question=%q intent=%+v", question, intent)

	// 置信度低 → 直接反问 (W1 demo 简化版: 不调内部接口)
	if intent.Confidence < 0.7 || intent.Type == "unknown" {
		return &AskResult{
			Answer:     fmt.Sprintf("抱歉, 我没完全听懂你的问题 (%s). 你可以试试: '上月电商部销售多少' / '本月哪个店卖最差' / '这周对比上周'", intent.Reasoning),
			SourceType: "unknown",
			Confidence: intent.Confidence,
			DurationMs: time.Since(start).Milliseconds(),
			Intent:     intent,
			LLMTokens:  classifyTokens,
		}, nil
	}

	// Step 2: 路由到内部接口 (W1 demo 只支持 department)
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

	// Step 3: LLM 二次组织成人话 (LLM Call 2)
	answer, formatTokens, formatErr := s.formatAnswer(ctx, question, rawData)
	if formatErr != nil {
		return nil, fmt.Errorf("生成回答失败: %w", formatErr)
	}

	return &AskResult{
		Answer:       answer,
		SourceType:   "api",
		SourceAPI:    apiPath,
		SourceParams: intent.Params,
		RawData:      rawData,
		Confidence:   intent.Confidence,
		DurationMs:   time.Since(start).Milliseconds(),
		Intent:       intent,
		LLMTokens:    classifyTokens + formatTokens,
	}, nil
}

// classifyIntent 意图识别 (LLM Call 1, 强制 JSON 输出)
func (s *Service) classifyIntent(ctx context.Context, question string) (*Intent, int, error) {
	today := time.Now().Format("2006-01-02")
	monthStart := time.Now().Format("2006-01") + "-01"
	lastMonthStart := time.Now().AddDate(0, -1, 0).Format("2006-01") + "-01"
	lastMonthEnd := time.Now().AddDate(0, 0, -time.Now().Day()).Format("2006-01-02")

	systemPrompt := fmt.Sprintf(`你是松鲜鲜 BI 看板的意图分类器. 用户问数据相关问题, 你输出 JSON 描述意图.

今天日期: %s
本月: %s 至今
上月: %s 至 %s

3 类意图:
- see (看数): 单点数字查询, 如"上月电商部销售多少"
- rank (排行): TOP N 类, 如"哪个店最差"
- trend (趋势): 时间对比类, 如"这周对比上周"
- unknown (不懂): 听不懂 / 问的不是 BI 数据 / 涉及敏感 (工资/删除/用户列表)

当前可路由接口 (W1 demo 只支持这 1 个, 其他全部走 unknown):
- module="department": 部门详情查询
  params 必填: dept (枚举 ecommerce/social/offline/distribution/instant_retail/zhongtai)
  params 可选: start (YYYY-MM-DD), end (YYYY-MM-DD), shop (店铺名), platform (平台名)
  能回答: "上月XX部销售多少" / "XX部本月各店排行" / "XX部周对比"

部门枚举映射:
- 电商部 → ecommerce
- 社媒部 → social
- 线下/线下大区 → offline
- 分销 → distribution
- 即时零售 → instant_retail
- 中台/中台部 → zhongtai

时间口径:
- "今天"=%s, "昨天"=今日-1
- "本周"=本周一到今天, "上周"=上周一到上周日
- "本月"=%s, "上月"=%s 至 %s
- "近 7 天"=今日-6 到今日

输出格式 (严格 JSON):
{
  "type": "see|rank|trend|unknown",
  "module": "department|null",
  "params": {"dept":"ecommerce", "start":"2026-04-01", "end":"2026-04-30"},
  "confidence": 0.95,
  "reasoning": "用户问上月电商部销售, 是看数类, 路由到 department 模块"
}

注意:
- 不懂的问题 type="unknown", module=null, confidence 给个低值 (<0.7), reasoning 写为什么不懂
- 跨部门/全公司汇总暂时也走 unknown (W1 只支持单部门)
- 涉及敏感词 (工资/密码/删除) 一律 unknown`,
		today, monthStart, lastMonthStart, lastMonthEnd, today, monthStart, lastMonthStart, lastMonthEnd)

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: question},
	}

	content, tokens, err := s.Client.Chat(ctx, messages, true)
	if err != nil {
		return nil, 0, err
	}

	// 解析 LLM 输出的 JSON
	var intent Intent
	if err := json.Unmarshal([]byte(content), &intent); err != nil {
		return nil, tokens, fmt.Errorf("LLM 输出 JSON 解析失败: %w content=%.300s", err, content)
	}
	return &intent, tokens, nil
}

// route 路由到内部接口 (W1 demo: 只支持 department)
func (s *Service) route(ctx context.Context, intent *Intent) (interface{}, string, error) {
	if intent.Module != "department" {
		return nil, "", fmt.Errorf("W1 demo 暂时只支持 department 模块, 当前 module=%s", intent.Module)
	}

	dept := intent.Params["dept"]
	if dept == "" {
		return nil, "/api/department", fmt.Errorf("意图缺少 dept 参数")
	}

	// W1 demo: 直接走 SQL 拉数据 (避免 HTTP 自调用复杂度)
	// W2 改成 internal HTTP call (复用现有 handler + 权限/缓存等中间件)
	start := intent.Params["start"]
	end := intent.Params["end"]
	if start == "" {
		start = time.Now().AddDate(0, -1, 0).Format("2006-01") + "-01"
	}
	if end == "" {
		end = time.Now().AddDate(0, 0, -time.Now().Day()).Format("2006-01-02")
	}

	// 注意 dept 已被 LLM 限定在枚举内 (ecommerce/social/...), 但还是用 ? 参数化防注入
	query := `SELECT
		COALESCE(SUM(goods_amt), 0) AS total_amount,
		COUNT(DISTINCT shop_name) AS shop_count,
		COUNT(DISTINCT stat_date) AS day_count,
		MIN(stat_date) AS first_date,
		MAX(stat_date) AS last_date
	FROM sales_goods_summary
	WHERE department = ? AND stat_date BETWEEN ? AND ?`

	row := s.DB.QueryRowContext(ctx, query, dept, start, end)
	var totalAmount float64
	var shopCount, dayCount int
	var firstDate, lastDate sql.NullString
	if err := row.Scan(&totalAmount, &shopCount, &dayCount, &firstDate, &lastDate); err != nil {
		return nil, "/api/department", fmt.Errorf("查询 sales_goods_summary 失败: %w", err)
	}

	return map[string]interface{}{
		"dept":        dept,
		"start":       start,
		"end":         end,
		"totalAmount": totalAmount,
		"shopCount":   shopCount,
		"dayCount":    dayCount,
		"firstDate":   firstDate.String,
		"lastDate":    lastDate.String,
	}, fmt.Sprintf("/api/department?dept=%s&start=%s&end=%s", dept, start, end), nil
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
- 如果数字看起来异常 (如 totalAmount=0, day_count<3 等), 给个轻量提示`

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
