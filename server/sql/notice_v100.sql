-- v1.00 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v1.00 内部 AI 协作工作流升级',
'本次完成内容:

【内部工作流】
启用 gstack skill 自动路由, 后续 AI 改动质量更可控:
- 业务规则/算法/口径改动前自动走第二意见复核(防越界)
- 代码改完自动走浏览器实测, 不再只做接口连通性测试
- 多 commit 攒到 4 个以上自动复盘是否分批合理

【缘由】
v0.94 把客服指标平均算法擅自改成加权 → 越过业务边界 → v0.97 回退.
此次配置后, 类似业务规则改动会先经"第二个 AI"二审, 跑哥再决定是否上线.

不影响日常使用, 不动定时任务',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
