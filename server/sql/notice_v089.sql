-- v0.89 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.89 后端代码结构优化(Sprint 4 阶段一)',
'本次优化内容:

【代码组织优化】
后端 dashboard.go 文件由 3926 行拆分到 6 个独立文件管理:
- 唯品会 / 拼多多 / 京东 / 飞瓜 / 天猫(含天猫超市) / 客服总览

【效果】
- 主文件减小 43% (3926 → 2246 行), 维护更清晰
- 各运营平台的 handler 独立, 改一处不影响别的
- 行为零改动, 所有接口数字 / 路由地址 / 缓存策略全部不变

不影响日常使用, 不动定时任务',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
