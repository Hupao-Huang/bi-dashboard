-- v0.88 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.88 同步工具防双进程冲突 + 销售单翻页防漏数据',
'本次修复内容:

【同步工具防双进程冲突】
1. 28 个数据同步/导入工具加文件锁: 同一个工具同时只允许一个实例运行 (避免重复触发导致脏写或 API 限频)
2. 涵盖: 13 个 RPA 数据导入 (天猫/京东/拼多多/抖音/唯品会/天猫超市/客服/飞瓜/促销等) + 15 个吉客云/用友/合思同步工具

【销售单翻页防漏数据】
3. 修复每日销售单同步在异常情况下可能跳过部分数据的潜在问题 (吉客云接口偶尔不返回翻页游标)
4. 异常时现在会打印警告日志, 标明需要补拉的时间段, 方便人工核查

不影响日常使用, 不动定时任务, 28 个 exe 已重新 build 部署',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
