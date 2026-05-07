-- v0.84 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.84 数据准确性修正 + 文案优化',
'本次修复内容:

【数据准确性 — 看板数字会变小, 不是丢数据是修正】
1. 计划看板"品类库存健康度"卡片之前金额 / 缺货率 / 高库存率算翻倍了 (多规格商品被重复计算), 修完显示真实值
2. 综合看板"S 品渠道分析" 7 处数字 (店铺排行 / 单品排行 / 趋势 / 明细) 之前同样翻倍, 修完显示真实值

请放心: 翻倍是历史遗留 bug, 不影响数据库实际数据, 修完是修正不是丢失

【系统页面优化】
3. 渠道管理保存/同步状态判断修正 (之前会误显示"失败"实际成功)
4. 特殊渠道对账页 / 业务预决算页内部技术词汇改成业务话术

不影响日常使用, 不动定时任务',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
