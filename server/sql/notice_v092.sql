-- v0.92 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.92 店铺看板前端代码结构优化(Sprint 4 阶段三)',
'本次完成内容:

【代码组织优化】
店铺看板组件 (StoreDashboard.tsx) 由 1485 行减小到 1049 行, 4 个平台运营数据模块拆成独立组件:
- 唯品会 / 拼多多 / S 品分析 / 天猫超市

【效果】
- 主组件减小约 30%, 各平台模块独立维护
- 改一个平台的展示不影响其他平台
- 行为零改动: 数据加载逻辑 / 图表样式 / KPI 数字 / 业务展示全部不变

剩余: 京东运营数据模块结构较复杂(含新老客/热词/促销 3 个子段), 留待下次重构

不影响日常使用, 不动定时任务',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
