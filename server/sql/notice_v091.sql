-- v0.91 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.91 数据导入工具公共代码抽取(Sprint 4 阶段二)',
'本次优化内容:

【代码组织优化】
6 个 RPA 数据导入工具(天猫 / 拼多多 / 京东 / 唯品会 / 飞瓜 / 推广)的共用工具函数(日期解析 / 数字解析 / 表头映射等)抽取到统一公共模块, 重复代码由 11 处合并到 1 处.

【效果】
- 净减约 340 行重复代码
- 后续修改工具函数只需改 1 处, 6 个工具自动同步
- 行为零改动: 所有数据导入逻辑、定时任务、导入数字全部不变

不影响日常使用, 不动定时任务',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
