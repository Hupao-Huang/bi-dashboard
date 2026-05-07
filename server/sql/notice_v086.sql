-- v0.86 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.86 部门详情数字翻倍修正 + 系统页/前端文案细节',
'本次修复内容:

【数据准确性 — 部门详情页数字会变小, 是修正不是丢数据】
1. 部门详情页"产品定位×平台"卡片之前金额翻倍 (停用+启用同名渠道时撞了多行), 修完显示真实值
2. 部门详情页"平台销售额分布"同上, 修完显示真实值

【系统/前端细节】
3. 审计日志页之前列表读不到后端数据 (协议字段错), 修后正常显示
4. 客服总览页 Tooltip 文案调整 (去掉技术词)
5. AI 工具箱外链加安全防护 (不影响使用)
6. 采购计划页同步按钮删调试日志 (不影响功能)

不影响日常使用, 不动定时任务',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
