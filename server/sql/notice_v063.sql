-- v0.63 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.63 采购计划拆成"成品 / 包材原料"双标签页',
'采购计划页面顶部新增标签切换:

📦 成品采购计划 — 销售业务用 (吉客云销售数据驱动, 7 个核心仓库存)
🏷 包材原料采购计划 — 生产业务用 (用友 BIP 领料消耗数据驱动, 全部 YS 仓库)

切换标签后, 建议清单 / 状态分布 / 公式说明 / 字段注释 全部跟着联动:

【成品】下显示
  · 列名: 日均销售 / 可售天数
  · 公式: 45 天目标 × 日均销售
  · 字段注释: 7 仓口径说明

【包材原料】下显示
  · 列名: 日均消耗 / 可用天数
  · 公式: 90 天目标 × 日均消耗
  · 字段注释: 用友 BIP 全仓口径说明

跑哥使用建议: 销售相关同事看"成品", 生产相关同事看"包材原料", 两边互不干扰',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
