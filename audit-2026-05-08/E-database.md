# E 路: 数据库 schema 审查 (2026-05-08)

agent: aa836205ca87b54ed
基线: cdb60b1 (v0.83)
范围: server/sql/*.sql + 实际 DB information_schema 对照

## 总评
**质量整体在线**, 核心业务表 UK + 索引 + 中文注释执行到位. 但有 **2 块痛**:
1. 流水/快照表 (stock_in_log/out_log/snapshot) 完全没 UK, **stock_in_log 已有真实重复**
2. schema.sql 跟实际 DB 严重脱节, 上云重建必踩坑

---

## P0 (必须立刻修)

### P0-1 ⭐⭐⭐ stock_in_log / stock_out_log 缺 UK, 已有真实重复
- 表: stock_in_log (2062 行) / stock_out_log
- 证据: stock_in_log 按 (goodsdoc_no, goods_no, sku_barcode, batch_no) 已发现至少 10 条重复键
  - 例: CRK202604202791 / 03010148 / 6975853790276 / 20260415SS c=2
- 修复:
```sql
-- stock_in_log: 先 dedupe 再加 UK
CREATE TABLE stock_in_log_dedup AS SELECT MIN(id) keep_id FROM stock_in_log
  GROUP BY goodsdoc_no, goods_no, sku_barcode, batch_no;
DELETE FROM stock_in_log WHERE id NOT IN (SELECT keep_id FROM stock_in_log_dedup);
DROP TABLE stock_in_log_dedup;
ALTER TABLE stock_in_log ADD UNIQUE KEY uk_doc_sku_batch (goodsdoc_no, goods_no, sku_barcode, batch_no);

-- stock_out_log: rec_id 自然唯一
ALTER TABLE stock_out_log ADD UNIQUE KEY uk_rec_id (rec_id);
```
- 配套: sync-stock-io INSERT 改 ON DUPLICATE KEY UPDATE

### P0-2 ⭐⭐ hesi_flow_attachment 缺 UK, 已有重复
- 表: hesi_flow_attachment (74268 行, 已发现 1 条重复)
- 重复: flow_id=ID01LFLXwNOV4P, attachment_type=flow.free, file_id=ID01LnAKVFKGD5 出现 2 次
- 修复: ALTER ADD UK (flow_id, attachment_type, file_id)

### P0-3 ⭐ stock_snapshot_template 与月分表 (130 万行) 缺 UK
- memory 已记 saveDetailSnapshot 停用, 但表还在
- 选 A: DROP 已停用月表 (确认无 Excel/报表使用后)
- 选 B: 保留但加 UK 防意外 ALTER ADD UK (snap_time, goods_id, sku_id, warehouse_id)

---

## P1 (强烈建议修)

### P1-1 ⭐⭐⭐ schema.sql 跟实际 DB 严重脱节 — 上云重建必踩坑
- 文件: server/sql/schema.sql
- 问题:
  - trade_template 第 11-42 行只声明 ~28 字段, 实际 100+ 字段 (alter_add_missing_fields.sql 56 个字段没合并)
  - 第 80-142 行 agg_daily_shop / agg_daily_goods / agg_monthly_department 三张预聚合表实际 DB 没建 (已被 sales_goods_summary_monthly 替换)
  - seller_memo VARCHAR(500), 实际 TEXT (最长 2300 字节)
  - pay_no VARCHAR(400), 实际 VARCHAR(2000)
- 修复: 用 mysqldump 或 INFORMATION_SCHEMA 重新生成 schema.sql, 合并 ALTER, 删死表

### P1-2 jd_tables.sql / pdd_tmallcs_vip_tables.sql UK 不一致
- op_jd_industry_keyword SQL 文件无 UK, 实际有 UK(stat_date, shop_name, keyword)
- op_jd_promo_sku_daily SQL 无 UK, 实际有 UK(stat_date, shop_name, ...)
- op_pdd_campaign_daily SQL impressions INT, 实际 BIGINT (防溢出)
- 修复: 用实际表覆盖 SQL 文件

### P1-3 business_budget_report UK 变更未同步 SQL
- 文件: server/sql/business_budget_report.sql:27
- SQL 文件 UK 6 字段, 实际 DB 已加 parent_subject 变 7 字段
- 修复: SQL 文件 UK 改 (snapshot_year, snapshot_month, channel, sub_channel, parent_subject, subject, period_month)

### P1-4 ⚠️ 11 张表大量字段无中文注释 (违反跑哥强制规则)
- 涉及: op_jd_cs_workload_daily / op_jd_cs_sales_perf_daily / op_kuaishou_cs_assessment_daily / op_xhs_cs_analysis_daily / op_douyin_cs_feige_daily / op_douyin_dist_material_daily / op_douyin_live_daily / op_douyin_ad_material_daily / op_douyin_dist_product_daily / op_douyin_goods_daily / op_douyin_dist_account_daily
- 字段总数 200+ 无注释
- 修复: ALTER MODIFY COLUMN ... COMMENT '中文' 一张一张补

### P1-5 sync_log 缺 UK 和索引
- 文件: server/sql/schema.sql:185-195
- 修复: ADD INDEX idx_type_status (sync_type, status, created_at) + idx_created_at

---

## P2 (锦上添花)

P2-1 trade_YYYYMM idx_trade_time 可能冗余 (按 consign_time 分月) — EXPLAIN 验证后删
P2-2 op_pdd_video/shop/goods_daily 缺 idx_shop_date (店铺趋势查询用不到 UK)
P2-3 fg_creator_daily UK 含 NULL 字段 (creator_id 允许 NULL → MySQL UK 不强制唯一)
P2-4 finance_subject_dict subject_code 当 PK 占空间 (改自增 BIGINT id + UK)
P2-5 hesi_flow.create_time 是 BIGINT 毫秒戳, 不是 DATETIME (SQL 可读性差) — 加生成列
P2-6 op_douyin_anchor_daily UK 用 anchor_name 改名风险 — 改 anchor_id
P2-7 ys_*_orders UK 设计正确 (说明)
P2-8 trade_template 字段顺序差异 (P1-1 子项)

---

## 与 B 路 (数据正确性) 重合的发现 (重要)

### B 路 P0-3 op_douyin_live_daily 缺 UK = E 路 P2-6 同根源
两路独立确认: 抖音直播表 UK 设计有问题. **B 路已修方案是 ALTER ADD UK (stat_date, shop_name, anchor_id, start_time)** — E 路同意.

### B 路 P0-4 op_douyin_anchor_daily UK 过窄 = E 路 P2-6
两路独立确认. B 路修复 UK 加 account, E 路建议 anchor_id. **取交集: UK 改 (stat_date, shop_name, account, anchor_id, anchor_name)**

---

## 提示性观察 (非问题)

- 销售单 trade_no/seller_memo 已按跑哥要求改对
- 按月分表 + consign_time 分月规则对
- finance_report idx_dept_year 已覆盖, 误报无误
- offline_region_target / op_douyin_ad_material_daily / trade_package_YYYYMM UK 都对, 误报无误
- warehouse_flow_summary PK 设计省 id, 赞
- sales_goods_summary 10 个索引都支撑 dashboard, 没冗余

---

## agent 推荐优先级

**本周必修 (P0)**:
1. P0-1 stock_in_log/out_log 加 UK (有真实重复)
2. P0-2 hesi_flow_attachment 加 UK
3. P0-3 stock_snapshot 处置 (确认是否删)

**下周排期 (P1)**:
4. P1-1 schema.sql 重新导出 (上云前必修)
5. P1-4 客服表中文注释补全

**P2 看心情**.
