# BI 看板测试覆盖 Sprint Plan (业界 BI 工具标准)

> 起点: 2026-05-10 (跑哥拍板全量补 + 2026-05-10 升 60% 目标)
> 目标: 整体覆盖率从 < 1% 提升到 **60%** (业界 BI/数据看板合理标准)
> 节奏: 12 周冲 50% 里程碑 → 16 周触 60% 退出标准

---

## 0. 现状基线 (audit @ 2026-05-10)

### 代码量

| 模块 | 行数 | 现有测试 | 覆盖率 |
|------|------|----------|--------|
| **后端 (Go)** | | | |
| handler/ | 17,895 | dashboard_test.go (220 行) | 2.0% |
| cmd_import (14 工具) | 7,154 | 0 | 0% |
| cmd_sync (~10 工具) | 7,094 | 0 | 0% |
| cmd_other (probe/debug/check) | 1,952 | 0 | 0% |
| finance | 948 | 0 | 0% |
| business | 837 | 0 | 0% |
| jackyun | 759 | 0 | 0% |
| yonsuite | 467 | 0 | 0% |
| importutil | 253 | 0 | 0% |
| dingtalk | 234 | 0 | 0% |
| **后端合计** | **~38,000** | **220 行** | **< 1%** |
| **前端 (TSX/TS)** | | | |
| src/pages (70 页, 10 部门) | ~3500 | App.test.tsx (CRA 占位) | 0% |
| src/components (27 文件) | 5,936 | shopBarTooltip.test.ts (新加) | < 1% |
| src/auth/utils/chartTheme | ~1500 | 0 | 0% |
| **前端合计** | **~10,000** | **1 个业务测试** | **< 1%** |

### 整体覆盖率: **< 1% (实际 0.6%)**

---

## 1. Sprint 1 — Dashboard 核心 SQL + handler 主路径 (Week 1-2)

### 目标
将 `dashboard_*.go` 测试覆盖率提到 **50%** (业界 70% 减 20pp 起步), 守住 dashboard API 不被改坏。

### Scope (P0)
- `handler/dashboard_overview.go` (411 行): `GetOverview` 全 7 处 SQL (dept/趋势/商品/店铺/grade/gradeDept/shopBreakdown)
- `handler/dashboard_department.go` (~700 行): `GetDepartment` 各部门 SQL + crossDept gradeDeptSalesAll
- `handler/dashboard_helpers.go` (~150 行): platform 映射 / cache key 构造 / 时间范围解析
- `handler/dashboard_cache.go`: cache invalidation 逻辑
- `handler/dashboard_warehouseflow.go`: warehouse_flow_summary 双轨路由 `canUseSummary`

### Test Cases (P0 必有)
- 各 SQL `IFNULL(department,'') NOT IN ('excluded','other','')` 排除规则验证 (sqlmock)
- shopBreakdown ROW_NUMBER per shop 聚合正确 (Top 5 不多不少)
- Top 15 LIMIT 截断 vs shopTotalCount 真实数 (今天的 bug 防御)
- platform 映射 5 主部门 + other/excluded 边缘
- cache key 不同时间段不冲突
- 401/500 error path

### Deliverable
- `handler/dashboard_test.go` 扩到 800-1000 行
- 跑 `go test ./internal/handler/... -cover` 显示 dashboard 包 ≥ 50%
- CI 加 go test gate

---

## 2. Sprint 2 — 财务/业务规则 + 高风险算法 (Week 3-4)

### 目标
财务报表/业务预决算/客服 KPI 等"业务老板看的数字"算法层面 100% 守护。这一段历史踩过最多坑 (memory `feedback_finance_aggregate_parent` / `feedback_customer_metrics_locked`).

### Scope (P0)
- `internal/finance/parser.go` (财务科目映射 1、电商→电商, 父项 SUM 子项)
- `internal/business/parser.go` (业务预决算 channel/sub_channel 解析, 大区 sheet → 线下)
- `handler/finance_report.go` (1177 行): 利润总览 / 部门利润 / 月度利润 / 报表导入两步流
- `handler/business_report.go` (794 行): 4 layout 数据组织 + snapshot 切月查询
- `handler/customer_*.go` (客服 KPI 算法 — 不主动改, 但守住现状)

### 目标提至 ≥ 85% (业界共识业务算法必须最严)

### Test Cases
- 财务父项 = SUM(子项) 自动补 row (仓储物流费用 v0.57 case)
- 业务报表"汇总" channel 解析正确
- 财务报表"预览 → 确认"两步流防误覆盖
- subject_code IN (...) 多科目筛选
- 客服指标算法快照 (输入固定数据 → 输出固定 KPI, 不允许业务规则被无心改坏)

### Deliverable
- `internal/finance/parser_test.go`
- `internal/business/parser_test.go`
- `handler/finance_report_test.go`
- `handler/business_report_test.go`
- 目标: finance + business 包 ≥ **85%** (业务规则不能错)

---

## 3. Sprint 3 — 吉客云 / YS 用友 SDK + 同步工具 (Week 5-6)

### 目标
对接外部 API 的层稳住. 这层挂了导致定时任务失败已经多次发生.

### Scope (P0)
- `internal/jackyun/`: API 签名 / scrollId 翻页 / 19+ 位 long id (`json.UseNumber()`) / archive isTableSwitch=2
- `internal/yonsuite/`: access_token urlencode / simpleVOs 日期过滤 / 1.1s 限流 / data 直接 array
- `cmd/sync-daily-trades/main.go`: 5min 窗口 + scrollId break / 当月汇总
- `cmd/sync-stock/main.go`: 库存快照 / 批次库存
- `cmd/sync-yonsuite-*` (4 个): purchase / subcontract / materialout / stock

### Test Cases
- API 签名生成 (历史已知 fixture)
- scrollId 200 截断检测 + 自动缩窗口
- json.Number 解析 19+ 位 id 不丢精度
- YS access_token 拼 URL 正确 urlencode
- 长 SQL 必加 `max_statement_time` (memory `feedback_no_long_sql_no_timeout`)

### Deliverable
- `internal/jackyun/*_test.go`
- `internal/yonsuite/*_test.go`
- 主要 cmd/sync-*/main_test.go
- 目标: jackyun + yonsuite ≥ **40%**

---

## 4. Sprint 4 — RPA 导入器 + Excel 解析 (Week 7-8)

### 目标
14 个 RPA 导入器全覆盖. 这一段坑特别多 (memory `feedback_rpa_date_excel` 提到全部 RPA 文件 stat_date 必须读 Excel 日期列, 不能用文件名).

### Scope (P0)
- `cmd/import-tmall` (双店一盘货+寄售)
- `cmd/import-tmallcs` (天猫超市 6 张推广表)
- `cmd/import-jd / -pdd / -vip / -douyin / -kuaishou / -xhs / -feigua` 等 14 个
- `cmd/import-customer` (5 个客服 import 函数)
- `internal/importutil`: Excel 日期解析 / PID lock 检测 (orphan STILL_ACTIVE 259)

### Test Cases
- stat_date 读 Excel 日期列, 不读文件名
- PDD MM-DD-YY 日期格式正确解析
- 多日 Excel 拆分 stat_date
- ON DUPLICATE KEY UPDATE 配 UNIQUE KEY (memory `feedback_upsert_requires_uk`)
- 字段长度溢出 → 立即停 (memory `feedback_data_too_long`)
- .xls 跳过不解析 (memory `feedback_no_xls_parse`)

### Deliverable
- 14 个 import_test.go
- `internal/importutil/*_test.go`
- 目标: cmd_import + importutil ≥ **90%** (parser 类业界严标 90)

---

## 5. Sprint 5 — 前端核心组件 jest 单测 (Week 9-10)

### 目标
前端业务组件 (ProductDashboard / StoreDashboard / FinanceProfitOverview / GoodsChannelExpand) 的逻辑层抽 pure function 加测试.

### Scope (P0)
- `components/ProductDashboard.tsx`: getPlatform / dept 路由
- `components/StoreDashboard.tsx`: shopTotalCount / 平台 tabs / 运营数据 trigger
- `components/FinanceProfitOverview.tsx`: 矩阵聚合 / 横向条 sort
- `components/FinanceProductProfit.tsx`: GRADE_COLORS 应用 / pieDim 切换
- `components/GoodsChannelExpand.tsx`: getPlatform / getDepartment 映射 (本次新加 视频号)
- `components/Chart.tsx` 配置 wrapper
- `chartTheme.ts`: GRADE_COLORS / DEPT_COLORS / formatWanHint 工具函数
- 各页面 inline formatter 抽出来 (跟本次 shopBarTooltip 一样)

### Test Cases
- `getPlatform('社媒-视频小店-X') === '视频号'` (本次 bug 防御)
- `formatWanHint(1234567)` 输出格式
- GRADE_COLORS 完整性 (S/A/B/C/D/未设置)
- 各 tooltip formatter pure function (配色对比 / 占比 / 边缘)

### Deliverable
- 25+ 个 *.test.ts/.test.tsx
- 目标: components 业务逻辑 ≥ **60%** (业界前端 50-70 上沿)

---

## 6. Sprint 6 — 前端页面 e2e + 视觉回归 + CI (Week 11-12)

> ⚠️ Week 13-16 加 Sprint 7-8 (新增) 冲 60% 退出标准: 补缺漏 / 提升业务规则到 85% / branch coverage 拉满

### 目标
playwright e2e 覆盖 P0 路径 100%, CI 卡门让以后改动必跑测试.

### Scope (P0 e2e 路径)
- 登录 → 综合看板 (含 hover 店铺榜 tooltip 验证显示完整 — 本次 bug 防御)
- 综合看板 → 各部门看板 (5 个) 数据加载
- 财务利润总览 → 产品利润 → SABCD 颜色一致 (本次 bug 防御)
- 渠道管理: dept Select 含 other/excluded
- 客户分析 → 钻取趋势/SKU Modal
- 库存预警 → 触发计算
- 审计日志查询

### Test Cases
- 关键页面截图比对 (visual regression)
- tooltip 显示完整 (上下都不被截断 — 本次 bug 防御)
- KPI 装饰色规则 (memory `feedback_kpi_card_no_decoration`): 不允许新增 valueStyle 装饰色
- 表格 grade 列染色 (本次 bug 防御)

### Deliverable
- `e2e/*.spec.ts` 20+ 个 playwright 用例
- `.github/workflows/test.yml` (npm test + go test + e2e 必通)
- README 加测试运行指引

### 目标 (16 周后整体, 业界 BI 工具标准)
- **整体覆盖率: < 1% → ≥ 60%** (业界 BI/数据看板共识)
- 业务规则/算法/财务计算: → **≥ 85%**
- handler / API 业务层: → **≥ 70%**
- jackyun / yonsuite SDK: → **≥ 50%**
- importutil / parser: → **≥ 90%**
- 前端业务组件: 0% → **≥ 50%**
- 关键业务 e2e 路径: 0% → **100%**
- CI 强制测试通过 → 阻止破坏性 PR merge

---

## 7. 工具/框架/标准

### 测试栈
- **后端**: `go test` + `sqlmock` (DB mock) + `httptest` (HTTP mock) — 跟现有 dashboard_test.go 一致
- **前端 unit**: jest + @testing-library/react (CRA 自带)
- **前端 e2e**: playwright MCP (跟本次自测同一套)
- **CI**: GitHub Actions

### 命名规范
- `*_test.go` 跟被测文件同目录
- `*.test.ts/.test.tsx` 跟被测文件同目录
- pure function 抽出来命名 `<feature>.ts` + `<feature>.test.ts` 配对

### 守底线规则 (新加 memory `feedback_test_and_verify` 已沉淀)
- ❌ build 通过就报"完成"
- ❌ 让跑哥当人肉测试员
- ✅ 改完代码必须 ≥ 1 个 test case 覆盖核心路径
- ✅ 视觉/tooltip/弹窗 → playwright 截图自测

---

## 8. 风险 + 缓解

| 风险 | 概率 | 影响 | 缓解 |
|------|------|------|------|
| 边补测试边发现历史 bug | 高 | 中 | 单独开 fix issue, 不阻塞 sprint; 严重 bug 优先修 |
| 业务迭代持续, 测试跟不上 | 高 | 高 | 新功能 PR 必带 test (CI 卡门); 老代码每周固定 1 天补 |
| 跑哥单人开发, 时间紧 | 中 | 中 | 每 PR 我主动写 test, 不再让跑哥催; 简单功能用 sqlmock 加速 |
| sqlmock regex 维护成本 | 中 | 低 | 用 SQL fragment 而非完整 SQL (本次 dashboard_test.go 改 `FROM sales_goods_summary` 通用匹配) |
| 滑块验证码挡 e2e 自测 | 高 | 高 | 测试环境关闭滑块, 或 e2e 用 admin token 注入跳过 |

---

## 9. 退出标准 (16 周后通过条件, 业界 BI 标准)

### 12 周里程碑 (中间检查点 ≥ 50%)
- [ ] 整体覆盖率 ≥ 50%
- [ ] handler/finance/business ≥ 65%
- [ ] importutil/parser ≥ 90%
- [ ] 前端业务组件 ≥ 35%
- [ ] 关键 e2e 路径覆盖 80%

### 16 周退出标准 (整体 ≥ 60%, 业界 BI 工具标准)
- [ ] `go test ./... -cover` 整体覆盖率 **≥ 60%**
- [ ] `npm test -- --coverage` 前端覆盖率 **≥ 50%**
- [ ] **业务规则/算法/财务计算: ≥ 85%** (核心战略防线)
- [ ] handler/finance/business 包 ≥ 70%
- [ ] jackyun/yonsuite 包 ≥ 50%
- [ ] cmd_import + importutil ≥ 90%
- [ ] 20+ playwright e2e 全 pass + 关键路径 100%
- [ ] CI 卡门: PR 必跑测试 + 必过
- [ ] memory feedback_test_and_verify 铁律 100% 遵循 (本人执行)
- [ ] 跑 branch coverage (不只是 statement) — 分支覆盖 ≥ 50%

---

## 10. Sprint 验收清单

每个 Sprint 结束跑哥验收:
1. 跑 coverage 命令贴数据
2. 列本 sprint 写的所有 test 文件
3. 列本 sprint 发现并修的历史 bug
4. 下个 sprint scope 是否调整

---

**Plan 起草: 2026-05-10 by Claude (跑哥拍板后)**
**Sprint 1 启动条件: 跑哥批准本 plan + 圈 P0 优先级**
