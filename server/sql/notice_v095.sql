-- v0.95 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.95 错误信息安全收紧(防数据库结构泄露)',
'本次完成内容:

【修复内容】
原: 服务器内部错误时, 错误提示直接显示数据库报错原文给前端
   (可能露出数据库表名 / 字段名 / 服务器路径等敏感信息)
现: 27 处接口改为对外只显示业务说明
   (例: "查询费控数据失败" / "保存反馈失败" / "更新公告失败" 等)
   详细错误改为只记录到服务器内部日志, 运维侧排查不受影响

【影响接口】
- 公告 / 个人中心 / 反馈 / 费控 / 财务报表导入 / 仓库流向 / 任务监控
- 业务校验类提示(如"密码不符合规则")保持原样, 用户能看到具体哪条不对

不影响日常使用, 不动定时任务',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
