# BI 看板 AI 智能助手 MVP 设计文档

> 版本: v0.1 草案 | 作者: Claude (跑哥 review 中) | 创建: 2026-05-22

---

## 1. 项目背景

### 1.1 需求来源
领导提出: 在 BI 看板加 AI 智能助手, 业务/管理层用人话提问, AI 自动查数据回答.

### 1.2 目标
- 覆盖 3 类高频场景: **看数 / 排行 / 趋势**
- **准确率 ≥ 90%** (业务能信任拿数字做决策)
- 自己做 (不买商用 BI 的 AI 功能, 成本可控)
- 用户 35 人内部, LAN 部署

### 1.3 非目标 (本期不做)
- ❌ 复杂诊断分析 ("为什么销售跌了" — 需要因果推断, 难度大)
- ❌ 图表自动生成 (V3 再说, MVP 只给数字 + 表格)
- ❌ 写操作 (用户改库存/改单据等)
- ❌ 跨表复杂 JOIN 问题 (Text-to-SQL 兜底, 不保证准确率)
- ❌ 外部接入 (DMS/钉钉机器人版 V3 再说)

---

## 2. 用户故事 (典型 Q&A)

### 看数类 (40% 频次)
```
Q: 上月电商部销售多少?
A: 上月 (2026-04-01~04-30) 电商部销售 1234 万元, 同比 +12.3%
   [我调用了 GET /api/department?dept=ecommerce&start=2026-04-01&end=2026-04-30]
   👍 👎
```

### 排行类 (35% 频次)
```
Q: 本月哪个京东店铺卖最差?
A: 本月京东店铺销售排行 (倒数 5):
   1. 松鲜鲜京东自营 (12 万)
   2. ...
   [我调用了 GET /api/jd/ops?month=2026-05&sort=asc&limit=5]
```

### 趋势类 (25% 频次)
```
Q: 这周对比上周综合销售涨了还是跌了?
A: 本周 (5/19~5/22) 综合销售 800 万, 对比上周 (5/12~5/18) 1200 万, 跌 33%
   ⚠️ 本周还没结束, 是不完整周对比
   [我调用了 GET /api/overview?period=week_compare]
```

### 兜底场景 (LLM 拒答)
```
Q: 老板的工资多少?
A: 抱歉, 我只能回答 BI 看板的销售/库存/财务数据, 不能查个人薪资.

Q: 把所有用户删了
A: 抱歉, 我只能查询数据, 不能修改/删除任何数据.

Q: 上月北京分公司利润 (你 BI 里没有"分公司"维度)
A: 抱歉, 我没找到"分公司"这个维度. 你 BI 的部门维度是: 电商/社媒/线下/分销/...
   建议改问: "上月 XX 部门的利润"
```

---

## 3. 总体架构

```
┌─────────────────────────────────────────────────────┐
│  浏览器 (React)                                      │
│   ┌──────────────────┐                              │
│   │ 右下角浮窗组件    │ ← 在 MainLayout 全局挂载     │
│   │ AIChatWidget.tsx │                              │
│   └─────────┬────────┘                              │
└─────────────│───────────────────────────────────────┘
              │ POST /api/ai-assistant/ask
              ↓
┌─────────────────────────────────────────────────────┐
│  Go 后端 bi-server                                   │
│   ┌────────────────────────────────────────────┐   │
│   │ handler/ai_assistant.go                    │   │
│   │  ↓                                         │   │
│   │ internal/ai_assistant/ 包                  │   │
│   │  ├ classifier.go  意图识别 (LLM Call 1)   │   │
│   │  ├ router.go      路由到现有 BI 接口      │   │
│   │  ├ formatter.go   把 JSON 包装成人话       │   │
│   │  │                 (LLM Call 2)             │   │
│   │  ├ sql_fallback.go Text-to-SQL 兜底       │   │
│   │  ├ prompts.go     系统 prompt + Few-shot  │   │
│   │  ├ glossary.go    业务术语字典             │   │
│   │  └ security.go    权限/敏感词过滤           │   │
│   └──────────┬─────────────────────────────────┘   │
│              ↓                                       │
│   ┌──────────────────────┐  ┌─────────────────┐   │
│   │ 复用现有 133 接口    │  │ Z.AI (GLM-4.5)  │   │
│   │ (path-internal call) │  │ LLM API          │   │
│   └──────────────────────┘  └─────────────────┘   │
└─────────────────────────────────────────────────────┘
```

---

## 4. 详细模块设计

### 4.1 后端 `internal/ai_assistant/` 包

#### 4.1.1 `classifier.go` — 意图识别

**职责**: 把用户问题 → 结构化意图 JSON (调哪个接口/传什么参数)

```go
type Intent struct {
    Type       string            // "see" / "rank" / "trend" / "unknown"
    Module     string            // "department" / "overview" / "store_rank" / ...
    Params     map[string]string // {dept:"ecommerce", start:"2026-04-01", end:"2026-04-30"}
    Confidence float64           // 0.0-1.0
    Reasoning  string            // LLM 给出的判断理由 (调试用)
}

func ClassifyIntent(ctx context.Context, question string, llm LLMClient) (*Intent, error)
```

**Prompt 设计** (系统层):
```
你是松鲜鲜 BI 看板的意图分类器. 用户问题分 3 类:
- see (看数): 单点数字查询, 如"上月电商部销售多少"
- rank (排行): TOP N 类, 如"哪个店最差"
- trend (趋势): 时间对比类, 如"这周对比上周"

当前可路由到的 30 个接口模块:
  - overview: 综合看板 (按部门聚合)
  - department: 部门详情 (electric/social/offline/distribution/...)
  - tmall/jd/pdd/vip/douyin/tmallcs/feigua/douyin-dist: 电商平台
  - hesi: 合思费控
  - ...

业务部门枚举: 电商/社媒/线下/分销/即时零售/中台
时间口径:
  - "今天"=今日, "昨天"=今日-1, "本周"=周一到今天
  - "上周"=上周一到上周日, "本月"=月初到今天, "上月"=上月完整月
  - "近 7 天"=今日-6 到今日, "近 30 天"=今日-29 到今日

请输出 JSON: {type, module, params, confidence, reasoning}
低置信度 (<0.7) 时 type 设为 "unknown", reasoning 写为什么.
```

**Few-shot** (举 10 个例子, 包括边界情况).

#### 4.1.2 `router.go` — 路由

**职责**: 拿到 Intent JSON, 调内部接口

```go
// 路由表 (代码定义, 易扩展)
var routes = map[string]RouteHandler{
    "department":  callDepartmentAPI,
    "overview":    callOverviewAPI,
    "tmall_ops":   callTmallOpsAPI,
    "store_rank":  callStoreRankAPI,
    "hesi_stats":  callHesiStatsAPI,
    // ... 30 个
}

func Route(ctx context.Context, intent *Intent, userScope DataScope) (RawData, error)
```

**关键**: 调内部接口时**注入当前用户的 data_scope**, 保证权限隔离.

#### 4.1.3 `formatter.go` — 包装回答

**职责**: 把接口返回的 JSON + 用户原问题, 调 LLM 二次组织成人话

**Prompt**:
```
用户原问题: {question}
我调用的接口: {api_path}
接口返回数据: {json}

请用 1-2 句话回答用户问题, 包含:
1. 关键数字 (带单位: 元/万/单/% 等)
2. 同比/环比 (如果数据里有)
3. 重要警告 (如本周不完整, 数据延迟)

格式: 自然语言, 不要 markdown 表格 (前端会单独展示原始数据).
```

#### 4.1.4 `sql_fallback.go` — Text-to-SQL 兜底

**职责**: Intent 是 "unknown" 时, 让 LLM 直接生成 SQL

**安全**:
- 只读账号 (单独 MySQL 账号, GRANT SELECT only)
- 强制加 `LIMIT 100`
- 30s timeout (memory `feedback_no_long_sql_no_timeout`)
- 敏感表黑名单: users / user_sessions / audit_logs / ...
- 返回 SQL 给前端展示 (业务能核对)

```go
func TextToSQL(ctx context.Context, question string, schema string, llm LLMClient) (sql, result string, err error)
```

#### 4.1.5 `prompts.go` + `glossary.go`

**业务术语字典** (静态 + DB 字典表):
```go
var Glossary = map[string]string{
    "GMV":      "成交总额, 字段 goods_amt",
    "净成交":   "去除退款后的成交, 字段 settle_amount",
    "客单价":   "总销售额/订单数",
    "电商部":   "department='ecommerce'",
    "线下大区": "9 个大区: 华南/华东/华北/华中/西南/西北/东北/重客/山东/母婴/新零售",
    // ...
}
```

#### 4.1.6 `security.go`

**3 道关**:
1. **敏感问题拦截** (LLM 第 0 步): "工资" "密码" "删除" 关键词直接拒
2. **权限过滤**: 调接口时注入 `data_scope` (复用现有框架)
3. **SQL 黑名单**: Fallback SQL 不能含 `INSERT/UPDATE/DELETE/DROP/TRUNCATE/CREATE/ALTER`

### 4.2 前端 `src/components/AIChatWidget.tsx`

**形态**: 右下角悬浮按钮, 点击展开对话框 (350×500px), 类客服 widget

**结构**:
```
┌──────────────────────────────┐
│ 🤖 BI 智能助手        [─][×] │
├──────────────────────────────┤
│ AI: 您好, 我能查 BI 数据...   │
│                              │
│ 我: 上月电商部销售多少        │
│                              │
│ AI: 上月电商部销售 1234 万,   │
│     同比 +12%                 │
│     📊 [查看原始数据]          │
│     🔍 我调用了 /api/dept    │
│     👍 👎                     │
├──────────────────────────────┤
│ [输入问题...]          [发送] │
└──────────────────────────────┘
```

**关键 UX**:
- 答案旁显示置信度 (低于 80% 标 "⚠️ AI 置信度低, 请核对")
- 答案附"我调用了什么接口/SQL", 业务能核对
- 👍/👎 反馈一键收集
- 历史对话本地 localStorage 持久化 (跨页面切换不丢)
- 输入历史 ↑/↓ 切换

### 4.3 数据库新增 3 张表

```sql
-- AI 会话表
CREATE TABLE ai_chat_session (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  user_id BIGINT NOT NULL COMMENT '提问用户',
  title VARCHAR(200) COMMENT '会话标题 (第一句话截取)',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY idx_user (user_id, created_at)
) COMMENT='AI 智能助手会话';

-- AI 消息表 (每条问答)
CREATE TABLE ai_chat_message (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  session_id BIGINT NOT NULL,
  role ENUM('user','assistant','system') NOT NULL,
  question TEXT COMMENT '用户问题 (role=user 时)',
  answer TEXT COMMENT 'AI 回答 (role=assistant 时)',
  intent_json TEXT COMMENT '识别出的意图 JSON',
  source_api VARCHAR(200) COMMENT '调用的内部接口 (路由命中时)',
  source_sql TEXT COMMENT '生成的 SQL (fallback 时)',
  confidence DECIMAL(3,2) COMMENT '置信度',
  llm_model VARCHAR(50) COMMENT '使用的 LLM',
  llm_tokens INT COMMENT 'token 数 (成本统计)',
  duration_ms INT COMMENT '耗时',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  KEY idx_session (session_id, created_at)
) COMMENT='AI 智能助手消息';

-- 反馈表
CREATE TABLE ai_chat_feedback (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  message_id BIGINT NOT NULL UNIQUE,
  user_id BIGINT NOT NULL,
  thumb TINYINT NOT NULL COMMENT '1=👍 -1=👎',
  comment TEXT COMMENT '可选文字反馈',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
) COMMENT='AI 回答反馈';
```

---

## 5. API 设计 (新增 4 个接口)

| 接口 | 方法 | 鉴权 | 说明 |
|------|------|------|------|
| `/api/ai-assistant/ask` | POST | 登录 | 提问, 返回回答 |
| `/api/ai-assistant/sessions` | GET | 登录 | 我的历史会话列表 |
| `/api/ai-assistant/sessions/:id/messages` | GET | 登录 | 某会话的消息列表 |
| `/api/ai-assistant/feedback` | POST | 登录 | 提交 👍/👎 反馈 |

### `/api/ai-assistant/ask` 请求/响应示例

**请求**:
```json
POST /api/ai-assistant/ask
{
  "question": "上月电商部销售多少",
  "sessionId": null  // null = 新会话, 非 null = 接续会话 (V2)
}
```

**响应**:
```json
{
  "code": 200,
  "data": {
    "sessionId": 123,
    "messageId": 456,
    "answer": "上月 (2026-04-01~04-30) 电商部销售 1234 万元, 同比 +12.3%",
    "sourceType": "api",
    "sourceAPI": "/api/department",
    "sourceParams": {"dept":"ecommerce","start":"2026-04-01","end":"2026-04-30"},
    "rawData": {...},  // 调用接口返回的原始 JSON (前端可展开)
    "confidence": 0.95,
    "durationMs": 1234,
    "warning": null  // 置信度低/数据延迟时填
  }
}
```

---

## 6. 安全设计

### 6.1 权限 (2026-05-22 跑哥拍板调整: 管理层视角, 无权限隔离)

**MVP 决策**: 种子用户全部是管理层 (大领导都能看全部数据), **不做 data_scope 过滤**.

- 调内部接口时不注入 user data_scope, 走"全数据"视角
- **未来如果开放给 35 人全员**, 必须重新加权限 (V2 改动)
- **风险**: 现阶段如果有非管理层用户拿到了 AI 助手访问权, 能看到 ta 平时看不到的数据 — 必须用功能开关严格限制访问名单 (config.json `ai_assistant.enabled_for_users`)

### 6.2 敏感问题拦截
LLM 第 0 步 prompt:
```
判断用户问题是否涉及:
- 个人隐私 (工资/密码/手机号/家庭)
- 写操作 (删除/修改/导出)
- 系统管理 (用户列表/角色/权限)
若涉及, 直接拒绝, 不要进入意图识别.
```

### 6.3 SQL Fallback 安全
- 用独立的只读 MySQL 账号 (`bi_ai_readonly`, `GRANT SELECT`)
- 强制 `LIMIT 100`
- 30s timeout
- 敏感表黑名单 (users / user_sessions / audit_logs / ai_chat_*)
- 返回 SQL 给前端展示

---

## 7. 性能

- LLM 调用 1 次 (意图识别) + 内部接口 (200ms) + LLM 调用 2 次 (包装) ≈ **3-5 秒/次**
- LLM timeout 10s (单次), 总 timeout 30s
- 高频问题缓存 (相同 question + user 24h 缓存, 同 user 短期重复问省钱)
- 35 人 × 100 次/天 = 3500 次/天 ≈ 1.5M token/天
- 2026-05-22 跑哥拍板用 **GLM-4.7 旗舰** (358B, Interleaved Thinking), 月费估算 **$200-400** (不设上限, 跑哥已知)
- 自动降级策略: 高频简单问题自动走 `glm-4.7-flash` (免费版), 复杂问题走 `glm-4.7` (旗舰) — 省钱不降准确率

---

## 8. 错误处理

| 错误 | 处理 |
|------|------|
| LLM 超时 | 重试 1 次, 仍超时返回"AI 响应慢, 请重试" |
| 意图识别置信度低 (<0.7) | AI 反问 "您是想问 A 还是 B?", 不瞎答 |
| 接口调用失败 | 返回 "数据接口暂时不可用, 请稍后" |
| Fallback SQL 执行失败 | 返回 "AI 没能生成正确的 SQL, 请换个说法" + 显示尝试的 SQL |
| LLM 调用 (Z.AI) 不可用 | 降级提示"AI 助手暂时离线, 请直接看看板" |

---

## 9. 测试方案

### 9.1 准确率测试集 (90% 目标)
人工标注 200 个典型问题 (3 类各 60+, 兜底 20), 跑过 AI 助手, 对答案打分:
- 完全正确 (数字 + 解读都对): +1
- 数字对解读错: +0.5
- 数字错: 0
- 目标: 总分 ≥ 180/200 = 90%

### 9.2 测试用例
- 每个意图模块 ≥ 5 个用例
- 边界: 空问题/超长问题/特殊字符/SQL 注入字符
- 时间口径: 今天/昨天/本周/上周/本月/上月/近 7 天/近 30 天/具体日期范围
- 业务术语: GMV/净成交/客单价/同比/环比

### 9.3 业务实测
W4 上线后, 内部 5 个种子用户先用, 收集反馈调优 1 周, 再放给 35 人.

---

## 10. 上线计划 / 灰度

| 阶段 | 范围 | 时长 |
|------|------|------|
| Alpha | 只跑哥自己用 | W3 末 |
| Beta | 5 个种子用户 (1 个业务 + 2 个管理 + 2 个客服) | W4 |
| GA | 35 人全员 | W5 |

通过功能开关控制 (config.json 加 `ai_assistant.enabled_for_users: []int64`).

---

## 11. 监控 / 反馈

- 每天看 ai_chat_message 表的 👍/👎 比例
- 跟踪 LLM 成本 (累计 token / 月费)
- 每周 review 错答 case, 加进 Few-shot prompt
- 监控 LLM API 不可用比例 (用 Z.AI 备 OpenAI/Claude 作 fallback)

---

## 12. 风险 / 已知限制

| 风险 | 影响 | 应对 |
|------|------|------|
| 90% 准确率达不到 | 业务不信任, 弃用 | Beta 测 1 周不到 90% 就延期 GA, 调到 90% 再上 |
| LLM 厂商封号 | 助手离线 | Z.AI / OpenAI / Claude 多 provider, 自动 failover |
| 跨表复杂问题答不出 | 用户失望 | 文案明确"我能答看数/排行/趋势, 不能答复杂诊断" |
| 业务术语持续演进 (新部门/新平台) | 准确率下降 | glossary.go 加 reload 接口, 不用重启 |
| LLM 成本超预算 | 月费暴涨 | 加 token 上限 + 单用户每日上限 (50 次/天) |

---

## 13. Sprint Plan (3 周 MVP + 1 周 Beta)

### W1 (5/26 - 5/30): 后端框架
- D1-2: `internal/ai_assistant/` 包结构 + Z.AI client 接通 + 1 个 intent 跑通端到端 demo
- D3-4: 30 个接口路由表 + `classifier.go` Few-shot prompt + 业务术语字典
- D5: 单测 + 200 个测试用例集准备

### W2 (6/2 - 6/6): 前端 + 数据持久化
- D6-7: AIChatWidget.tsx 组件 + Markdown 渲染 + 历史会话 localStorage
- D8: 3 张数据库表 + 4 个新接口 (ask/sessions/messages/feedback)
- D9-10: 联调 + UX 微调

### W3 (6/9 - 6/13): 兜底 + 安全 + Alpha
- D11-12: Text-to-SQL fallback + 只读账号 + LIMIT/timeout
- D13: 敏感问题拦截 + 权限注入
- D14-15: 跑哥 Alpha 测 + 调 prompt

### W4 (6/16 - 6/20): Beta + 调优
- 5 个种子用户用一周
- 每天看 👍/👎, 调 prompt, 补 Few-shot
- 达 90% 准确率 → GA

### 总: 3 周 MVP, 4 周 GA

---

## 14. 跑哥决策记录 (2026-05-22 拍板)

| # | 问题 | 跑哥决策 |
|---|------|---------|
| 1 | 种子用户选谁 | **管理层** (具体几人待定, 跑哥找领导问) |
| 2 | LLM 用什么 | **先用 Z.AI GLM-4.7** (旗舰 358B), 不加 Claude fallback |
| 3 | 数据权限 | **不设权限**, 管理层都是大领导, 全数据视角 |
| 4 | 预算上限 | **不设上限**, 月费 ~$200-400 跑哥已知 |
| 5 | 谁用 | **主要管理层使用**, 35 人全员后续看效果再放 |

### 还需跑哥追加确认 (不阻塞 W1 开干)

- 种子用户具体几人, 谁? (建议 3-5 人, 跑哥跟领导对齐拿名单)
- 第 2 章 3 类用户故事 (看数/排行/趋势) 你拿给领导对一下, 是否覆盖他的真实需求

---

## 附录 A: 业务红线

按 CLAUDE.md 部署红线:
- 新业务规则改造 → **必须 /codex 二审** (本期 W3 末提交)
- KPI 算法变更 → 不允许 AI 自己改, 只能调用现有接口
- 部署 → Alpha 在 Beta 前必须人工 review, 不走自动 build

## 附录 B: 不在本期范围 (V2+ Roadmap)

- 图表生成 (V2)
- 钉钉机器人接入 (V2)
- 跨会话上下文 (V2)
- 用户画像 / 推荐问题 (V3)
- 多模态 (上传截图问问题, V3)
- 外部接入 (DMS 等业务系统调 AI, V3)
- 主动推送 (周报/异常告警, V3)

---

**End of Document**
