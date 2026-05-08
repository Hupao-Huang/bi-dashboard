-- v0.97 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.97 客服指标平均算法回退到原版',
'本次完成内容:

【回退说明】
v0.94 把客服总览/趋势/平台/店铺的"平均指标"算法从算数平均改成加权.
经客服部门确认: 这套指标口径是客服业务定的, BI 看板不应擅自调整.
本次回退到 v0.93 的原始算法, 客服指标计算方式保持业务规范.

【影响】
- 客服总览/趋势/平台分布/店铺分布 4 个维度的"平均"指标数字回到 v0.93 原状
- v0.94 临时变化的数字本次撤销
- 不影响日常使用, 不动定时任务',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
