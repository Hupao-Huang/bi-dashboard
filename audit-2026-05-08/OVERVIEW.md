# BI 看板全代码审查总报告 (2026-05-08)

基线: cdb60b1 (v0.83 默认排序改库存降序)
范围: 后端 88 个 .go (36180 行) + 前端 102 个 .tsx/.ts (18776 行) + 数据库 28 个 .sql + 实际 DB schema
方法: 6 路并行 agent, 共耗时 ~15 分钟
误报清单: 已挂 8 条, 6 路都没再报

---

## 总成绩单

| 路 | 范围 | P0 | P1 | P2 | 报告 |
|---|---|---|---|---|---|
| A | 后端 handler 安全 | 0 | 8 | 7 | A-handler-security.md |
| B | 后端数据正确性 | **4** | 5 | 5 | B-data-correctness.md |
| C | 后端 sync/import 工具 | **6** | 15 | 14 | C-sync-import.md |
| D | 前端代码 | 3 | 7 | 28 | D-frontend.md |
| E | 数据库 schema | 3 | 5 | 8 | E-database.md |
| F | 大文件拆分 | 3 | 3 | 1 | F-large-files.md |
| **合计** | | **19** | **43** | **63** | (125 项) |

**整体评价**: 项目质量在线 (核心 SQL UK 完备, 中文注释执行到位, 前端 placeholder 修干净). **真正的痛是 19 个 P0**, 集中在 3 块: 数据正确性 (×N 翻倍), 工具脚本健壮性 (静默失败/无锁), schema/UK 漏配 (已有真实重复).

---

## P0 清单 (19 个 — 必须修, 按 ROI 排序)

### 🔥 第 1 优先 (改动小风险低 + 立即影响数字)

**P0-A1 supply_chain.go:470 计划看板品类库存 SUM ×N 翻倍** ⭐⭐⭐
- 改 1 行 SQL, JOIN goods 改子查询去重
- 影响: 计划看板品类库存金额/缺货率/高库存率 (采购决策依据)
- ROI: 极高, 工时 5 分钟

**P0-A2 dashboard.go GetSProducts 7 处 SUM ×N 翻倍** ⭐⭐⭐
- 跟 v0.62 修过的 7 倍 bug 同模式漏修, 7 个 SQL 都要改
- 影响: /api/s-products S 品渠道分析全部数字
- ROI: 极高, 工时 30 分钟

**P0-A3 ChannelManagement 保存/同步成功判定写错** ⭐⭐
- 文件: src/pages/system/ChannelManagement.tsx:72-78, 94-99
- 改 if (res.code === 200), 不再看 message 字段
- ROI: 高, 工时 5 分钟

**P0-A4 SpecialChannelAllot Alert 暴露 exe 文件名** ⭐⭐
- 文件: src/pages/ecommerce/SpecialChannelAllot.tsx:229
- 违反跑哥红线 feedback_tooltip_no_table_name
- 改业务话术
- ROI: 高, 工时 2 分钟

**P0-A5 BusinessReport Tooltip 写 "CLI 导入"** ⭐⭐
- 文件: src/pages/finance/BusinessReport.tsx:139
- 同上红线
- ROI: 高, 工时 2 分钟

### 🔥 第 2 优先 (数据正确性 / 重复累加)

**P0-B1 stock_in_log/stock_out_log 缺 UK, 已有 10 条真实重复** ⭐⭐⭐
- E 路验证: stock_in_log 2062 行已有重复键
- 修: dedupe + ALTER ADD UK + sync-stock-io 改 ON DUPLICATE KEY
- ROI: 极高, 工时 1 小时 (含 dedupe)

**P0-B2 hesi_flow_attachment 缺 UK, 已有 1 条重复** ⭐⭐
- ALTER ADD UK (flow_id, attachment_type, file_id)
- ROI: 高, 工时 30 分钟

**P0-B3 op_douyin_live_daily 无 UK, REPLACE INTO 重跑必累加** ⭐⭐
- B 路 + E 路双确认
- ALTER ADD UK (stat_date, shop_name, anchor_id, start_time)
- 加之前先 dedupe
- ROI: 高, 工时 30 分钟

**P0-B4 op_douyin_anchor_daily UK 缺 account, 同名主播覆盖** ⭐⭐
- B 路 + E 路双确认
- 取交集 UK 改 (stat_date, shop_name, account, anchor_id, anchor_name)
- ROI: 中, 工时 30 分钟

### 🔥 第 3 优先 (工具脚本静默失败)

**P0-C1 6 个 import 工具吞 config/sql.Open 错误** ⭐⭐
- import-vip / import-tmallcs / import-douyin-dist / import-jd / import-pdd / import-douyin
- 改 log.Fatalf
- ROI: 高, 工时 30 分钟

**P0-C2 sync-stock 翻页游标 lastQid 可能漏数据** ⭐⭐
- 文件: server/cmd/sync-stock/main.go:114-130
- lastQid 改 MAX(quantityId)
- ROI: 中, 工时 1 小时 (含验证)

**P0-C3 sync-stock-io 先 DELETE 再拉 API, 失败时空窗** ⭐⭐
- 改事务化: collect 先 → tx.Begin → DELETE → INSERT → Commit (照 sync-daily-summary)
- ROI: 中, 工时 1 小时

**P0-C4 sync-trades-v2/half-day/daily-trades 翻页缺 scrollId 直接 break** ⭐
- 加 fallback pageIndex 翻页 + 打印时间窗
- ROI: 中, 工时 2 小时

**P0-C5 sync-channels 部门映射写死, 10 个未映射** ⭐
- memory project_unmapped_channels.md 已记
- 等跑哥确认后搬到 channel_dept_mapping 表
- ROI: 低, 工时 半天 (要做管理界面)

**P0-C6 import-finance 无 file lock**
- 加 importutil.AcquireLock
- ROI: 中, 工时 5 分钟

### 🔥 第 4 优先 (架构拆分 — 长期 ROI)

**P0-F1 dashboard.go 3926 行拆 12 文件** ⭐
- ROI: 长期高, 工时 1 天

**P0-F2 11 个 cmd/import-* 复制 100+ 行 parseFloat/parseExcelDate** ⭐
- 抽 internal/importutil/ (excel/date/header/importer)
- 减 1100+ 行重复
- ROI: 高, 工时 半天

**P0-F3 StoreDashboard.tsx 1485 行拆 6 platform** ⭐
- 抽 KPICard / TimeSeriesChart / useApiData
- 减 600+ 行重复
- ROI: 高, 工时 1 天

---

## 跑哥批评 lens 命中清单 (5 条全部命中代码)

### 1. "不主动 grep 现有功能就动手" 命中 4 处
- 11 个 cmd/import-*/main.go 复制 parseFloat/parseExcelDate (internal/importutil 包已存在)
- auth.go + admin.go 各有一份 loadDataScopes
- dashboard.go 4 套平台映射 (platformToPlats / platTabDefs / platLabelMap / platPrefixMap)
- finance_report.go urlEscape 35 行重造 (url.PathEscape 1 行)

### 2. "打补丁思维 ≠ 重构思维" 命中 4 处
- supply_chain.go v0.71-v0.78 **7 重防御** (cooldown/诊断/进度/兜底), 注释自己写"防止前端 bug" — 没追前端根因
- GetInTransitDetail 双路径 `OR p.product_c_code = ?` 兜底是 goods.sku_code 没填全, 应补数据
- dashboard.go `strings.ReplaceAll(scopeCond, "shop_name", "s.shop_name")` 5 处 — 函数没接 alias 参数
- audit.go:157 Scan 错就 continue (打补丁式吞错)

### 3. "范围思维狭窄不看闭环" 命中 5 处
- dashboard.go GetDepartmentDetail **666 行 1 个函数干 8 件事**
- dashboard.go GetMarketingCost **4 个 case 复制 4 套查询**
- auth.go 里塞 op_douyin 抖音表 schema (270 行)
- 前端 res.error vs res.msg 协议不一致**散落 6 个文件** (D P1-6)
- B P0-2: GetSProducts 7 处 SUM ×N 翻倍, dashboard.go 别处早就用子查询去重, 唯独 GetSProducts 7 处漏改

### 4. "基础 UI 问题没自检" — ✅ 已修干净 (没有新发现)
- placeholder 处理 `value={x || undefined}` 已铺开, 跟 v0.80.1 同模式

### 5. "报告浮夸掩盖能力问题" — N/A 这是输出风格, 不是代码

---

## 上云前必修清单 (跑哥 2026-05 启动云迁移)

| 项 | 文件 | 风险 |
|---|---|---|
| time.Now() 加 Asia/Shanghai | dashboard.go 多处 | 上云后日期差 8 小时 |
| schema.sql 重新生成 (合并 ALTER) | server/sql/schema.sql | 云上从零建表立即撞 data too long |
| CORS Origin 白名单 | server/cmd/server/main.go:60-67 | 写死内网域名, 上云必改 |
| jd_tables.sql / pdd_tmallcs_vip_tables.sql 同步实际 UK | sql 目录 | 云上建表丢防重 UK |
| business_budget_report.sql UK 7 字段同步 | sql 目录 | 上传 xlsx 撞 duplicate |
| stock_in_log/out_log/snapshot 加 UK | DB | 同 P0-B1 |
| 11 张表中文注释补全 | DB | 强制规则 |

---

## 推荐 sprint 排期

### Sprint 1 (本周, 1-2 天)
- 5 个第 1 优先 P0 (改动小, ROI 极高): supply_chain ×N / GetSProducts ×N 7 处 / ChannelManagement 成功判定 / 2 处对外文案违规

### Sprint 2 (下周, 2-3 天)
- 4 个数据正确性 UK: stock_in_log / stock_out_log / hesi_flow_attachment / op_douyin_*
- 3 个工具静默失败: import 6 工具加 fatal / sync-stock 游标 / sync-stock-io 事务化

### Sprint 3 (上云前, 1 周)
- schema.sql 重新生成
- time.Now() 时区
- CORS 白名单
- 11 张表中文注释
- 客服 op_tmall_service_* 4 表确认 UK

### Sprint 4 (架构改善, 2-3 天)
- internal/importutil 抽公共
- StoreDashboard.tsx 拆 6 platform
- dashboard.go 拆 12 文件 (可选)

### 长期 (P1/P2 看 ROI 挑做)
- 43 个 P1 / 63 个 P2 按 sprint 节奏挑
- err.Error() 集中清理 (A P1-2)
- res.error vs res.msg 6 处统一 (D P1-6)
- v0.71-v0.78 7 重防御找根因清理 (跑哥批评 lens 命中)

---

## 合计估算工时

- Sprint 1 (P0 第 1 优先): ~1.5 天
- Sprint 2 (P0 第 2-3 优先): ~3 天
- Sprint 3 (上云准备): ~5 天
- Sprint 4 (架构): ~3 天
- **总计 P0 完整修复: ~12.5 工时日**

如果只挑 5 个 ⭐⭐⭐ 极高 ROI: **半天搞定 5 个**, 立即让看板数字准确.
