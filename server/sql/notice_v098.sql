-- v0.98 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.98 店铺看板京东模块拆分(完成 Sprint 4 P3 残留)',
'本次完成内容:

【代码组织优化】
店铺看板组件 (StoreDashboard.tsx) 由 1049 行减小到 847 行,
京东运营数据模块 (店铺经营 / 客户分析 / 新老客 / 行业热词 / 促销活动) 拆成独立组件.

【效果】
- 主组件再减小约 19% (相对 v0.92 累计减小 43%)
- 京东模块的 5 块内容(店铺经营+客户分析+新老客+热词+促销)归到一个独立文件维护
- 行为零改动: 数据加载 / 图表 / KPI 数字 / 业务展示完全不变

至此, 店铺看板的 5 个平台运营数据模块全部拆出独立组件.

不影响日常使用, 不动定时任务',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
