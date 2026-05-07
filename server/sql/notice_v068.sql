-- v0.68 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.68 同步按钮升级 — 一键拉取全部用友数据',
'采购计划页面右上角"立即同步"按钮升级:

【之前】点击只同步用友 BIP 现存量(库存)
跑哥反馈: 想看最新的在途采购订单, 但同步按钮不刷订单, 只能等明早 9 点自动定时

【现在】点击一键同步全部 4 类用友数据(串行执行约 1-2 分钟):
  1. 现存量 (库存)
  2. 采购订单 (在途采购)
  3. 委外订单 (在途委外)
  4. 材料出库 (日均消耗)

【按钮文案】"立即同步" → "立即同步全部 YS 数据"

【提示信息】同步完成后弹出消息显示每一步的细节:
  ✓ 现存量 (新增/更新/失败/耗时)
  ✓ 采购订单 (...)
  ✓ 委外订单 (...)
  ✓ 材料出库 (...)
  总耗时 X 秒

【自动定时不变】
每天 09:00-09:30 各类自动同步一次
现存量额外 14:00 / 18:00 再刷新

跑哥使用建议: 改完用友 BIP 那边的采购单 / 委外单后, 在 BI 看板点这个按钮, 1-2 分钟后就能在采购计划页看到最新数据 + 在途明细',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
