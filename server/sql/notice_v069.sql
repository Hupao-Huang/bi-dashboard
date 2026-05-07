-- v0.69 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.69 修复在途订单状态更新滞后问题',
'跑哥发现: 用友 BIP 里的旧采购单已经变成"部分入库"或"已关闭", 但 BI 看板上还显示"已审核未入库"。

【根因】
之前同步采购订单/委外订单的逻辑是只拉"今天和昨天新开"的单。一个 5 天前下的订单, 之后哪怕状态变化, 系统都拉不进来, 导致"在途量"被高估、订单状态滞后。

【修复】
"立即同步全部 YS 数据"按钮升级:
- 之前: 拉昨天+今天的采购单/委外单
- 现在: 拉最近 30 天的采购单/委外单 (覆盖大部分长尾未结单的状态变化)

【自动定时不变】
每天 09:00-09:30 的自动同步保持轻量(昨天+今天), 节省资源
跑哥需要看完整状态时, 手动点"立即同步全部 YS 数据"即可

【耗时变化】
之前: 1-2 分钟
现在: 2-3 分钟 (多拉 28 天数据)

跑哥使用建议: 改完用友 BIP 那边的单据后, 在 BI 看板点"立即同步全部 YS 数据", 2-3 分钟后所有近 30 天单的最新状态全部刷新, hover 在途数字看明细就能直接对账',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
