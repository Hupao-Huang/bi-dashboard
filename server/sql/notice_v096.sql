-- v0.96 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.96 综合看板 SQL 拼接代码可读性优化',
'本次完成内容:

【代码质量】
原: 综合看板生成 SQL 时, 用字符串替换硬给列名加表别名(看着诡异)
现: 抽出 helper 函数 withAlias 封装, 调用方一行调用清晰

【影响】
- 行为零改动: 数据查询逻辑 / 数字 / 业务展示完全不变
- 综合看板 3 处 SQL 拼接的可维护性提升

不影响日常使用, 不动定时任务',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
