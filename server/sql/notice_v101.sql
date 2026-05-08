-- v1.01 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v1.01 采购计划在途明细数据完整性增强',
'本次完成内容:

【数据完整性】
新品在用友已下委外/采购订单后, 当天即可在"采购计划 → 在途明细"看到对应订单:
- 此前问题: 用友已建档下单 → 但吉客云货品档案当天未刷新 → 在途明细查不到该新品
- 现在改进: 货品档案改为每天凌晨 04:00 自动全量同步, 兜底窗口缩短至 24 小时内

【后端代码清理】
仅内部规范, 不影响日常使用:
- 同步按钮多重防御机制 — 文档化每一重防御的设计意图
- 历史遗留的"同步日志表" — 自项目首版以来一年未投入使用, 已清理

不影响定时任务运行',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS '时间'
FROM notices WHERE is_pinned=1;
