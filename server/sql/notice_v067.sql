-- v0.67 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.67 在途采购/委外鼠标 hover 看明细',
'采购计划页面"在途采购"和"在途委外"两列, 现在鼠标 hover 上去就直接展示该 SKU 的全部用友 BIP 在途订单明细。

【交互方式】
鼠标移到带蓝色或紫色虚下划线的数字上 → 自动弹出小窗
不需要点击, 移开鼠标小窗自动消失

【弹出小窗内容】
首行: 共 X 单, 在途合计 X
表格列:
  · 用友单号 (可复制粘贴去用友 BIP 系统验证)
  · 供应商
  · 开单日期 / 计划到货日期
  · 订单总量 / 已入库 / 在途量
  · 当前状态 (已审核未入库 / 部分入库 等业务大白话)

【设计细节】
- 第一次 hover 该 SKU 时异步加载明细数据(几百毫秒), 之后切回该 SKU 直接显示缓存
- 兼容 v0.65 双路径桥接: 即使吉客云档案不全的 SKU 也能正确关联用友 BIP 订单
- 排除安徽香松组织(跟主接口口径一致)

跑哥使用建议: 现在可以快速核对每个 SKU 的具体在途明细, 拿用友单号去 YS 系统对账验证',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
