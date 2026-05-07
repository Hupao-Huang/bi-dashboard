# B 路: 后端数据正确性审查 (2026-05-08)

agent: a93c9e5ce3a01bcfa
基线: cdb60b1 (v0.83)

## 总评
**4 P0 都是真坑, 现在就在影响 BI 看板上的数字**. 跟 v0.62 修过的"7 倍 bug"同模式.

---

## P0 (现在就能影响看板数字 — 必须立刻修)

### P0-1 ⭐⭐⭐ 计划看板品类库存健康度 SUM ×N 翻倍
- 文件: supply_chain.go:470
- 问题: `LEFT JOIN goods g ON s.goods_no = g.goods_no` 没带 sku_id. goods 表多规格 SKU 行复制 stock 行, SUM 翻 N 倍
- 影响: /api/supply-chain/dashboard 品类库存健康度卡片 (库存金额/滞销/缺货/高库存) — 采购计划决策依据
- 修复: 子查询去重 `LEFT JOIN (SELECT DISTINCT goods_no, cate_full_name FROM goods WHERE is_delete=0) g` (照 stock.go:198 写法)

### P0-2 ⭐⭐⭐ S品渠道分析 7 处 SUM ×N 翻倍 (跟 v0.62 同模式漏修)
- 文件: dashboard.go:1855, 1864, 1907, 1916, 1945, 1984, 1994
- 问题: 7 段 SQL 都 `JOIN goods g ON g.goods_no = s.goods_no AND g.goods_field7 = 'S'` 没带 sku_id. dashboard.go 别处 (349, 467, 501, 764, 912, 948, 1006, 1014, 1219, 1245) 早就用子查询, **唯独 GetSProducts 这 7 处漏改**
- 影响: /api/s-products S品渠道销售分析全部数字 (店铺/单品/趋势/明细)
- 修复: 7 处统一 `INNER JOIN (SELECT DISTINCT goods_no FROM goods WHERE goods_field7 = 'S') g`

### P0-3 ⭐⭐ op_douyin_live_daily 无 UNIQUE KEY → REPLACE INTO 重跑必累加
- 文件: auth.go:671-707 建表 + cmd/import-douyin/main.go:180 用 REPLACE INTO
- 问题: 表只有 KEY (索引), 没有 UNIQUE KEY. 命中跑哥 feedback_upsert_requires_uk 规则
- 影响: /api/douyin/ops 直播趋势 COUNT(*) 虚高, 主播排行 SUM 虚高
- 修复: ALTER ADD UK (stat_date, shop_name, anchor_id, start_time), 加之前先 dedupe

### P0-4 ⭐⭐ op_douyin_anchor_daily UK 过窄, 同名主播互相覆盖
- 文件: auth.go:865 UK 定义
- 问题: UK = (stat_date, shop_name, anchor_name) 没带 account. 同店铺不同账号同名主播 REPLACE INTO 覆盖, 数据丢失
- 影响: /api/douyin/ops anchors 排行
- 修复: UK 改 (stat_date, shop_name, account, anchor_name)

---

## P1 (中等坑, 偶尔翻车)

### P1-1 sales_channel JOIN 不保证唯一可能 ×N
- 文件: dashboard.go:949, 1120
- 问题: JOIN 用 (channel_name, department), 但 sales_channel UK 是 channel_id. 吉客云"已停用+启用同名渠道"会 channel_name 重复
- 影响: GetDepartmentDetail 产品定位×平台 / 平台销售额分布
- 修复: 子查询去重 或 按 channel_id JOIN (sales_goods_summary 也存 shop_id)

### P1-2 douyin handler COUNT(*) 受 P0-3 影响虚高
- 文件: douyin.go:33-39, 89-94
- 修 P0-3 后这边自动好

### P1-3 customer overview 满意度/转化率 算数平均失真
- 文件: dashboard.go:3489-3496, 3513-3525
- 问题: 跨实体合并率用算数平均不正确, 应加权平均
- 影响: 客服总览 KPI 数字小幅偏差, 不是数据丢失
- 修复: 满意度按 inquiryUsers/consultUsers 加权

### P1-4 audit Scan 错误吞掉
- 文件: audit.go:157-159
- 问题: Scan 错就 continue, 不返回不打日志
- 影响: 审计日志查询页可能漏行

### P1-5 special_channel pending 状态 ELSE 兜底
- 文件: special_channel.go:71-77
- 问题: `if inStatus == 3 完成 else pending` — 若有"已取消"(in_status=0/-1)会被算 pending
- 修复: 跑哥确认 in_status 全部值后用 IN 显式列出

---

## P2 (小问题)

P2-1 stock.go:198 子查询 DISTINCT 多列可能多行 (改 GROUP BY + MAX)
P2-2 ⚠️ time.Now() 没指定 Asia/Shanghai — **上云前必修** (跑哥 2026-05 启动上云)
P2-3 admin.go:410 用户名重复 vs 其他错误返回相同 message
P2-4 dashboard.go:3334 mergeCpcDaily ROI 用 int 截断不是四舍五入 (改 round2)
P2-5 op_tmall_service_* 4 表 sql 目录和 auth.go 都没 CREATE TABLE — 跑哥手工建? 确认 UK 是否存在

---

## 已知误报核对

- op_douyin_ad_material_daily UK: auth.go 869-893 建表里只有 KEY 没看到 UK, 但跑哥 memory 说已加 → 信跑哥. **建议把 ALTER 补到 auth.go addCols 里同步**

---

## agent 推荐优先动手

1. **supply_chain.go:470** 改 1 行 SQL (P0-1) — 计划看板品类库存
2. **dashboard.go GetSProducts 7 处** (P0-2) — S 品分析全翻倍, 跟 v0.62 同模式漏修
3. **op_douyin_live_daily 加 UK** (P0-3) — 否则 import-douyin 重跑就累加
4. **op_douyin_anchor_daily UK 加 account** (P0-4) — 同名主播覆盖
5. 上云前: time.Now() 加 Asia/Shanghai (P2-2)
