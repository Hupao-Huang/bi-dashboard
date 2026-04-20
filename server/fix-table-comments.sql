-- H1: 月份表注释修正（跳过 trade_202501 系列，sync在写）
-- 模板表
ALTER TABLE trade_template COMMENT='销售单数据模板表（CREATE TABLE LIKE 用）';
ALTER TABLE trade_goods_template COMMENT='销售单商品明细模板表（CREATE TABLE LIKE 用）';
ALTER TABLE trade_package_template COMMENT='订单包裹详情模板表（CREATE TABLE LIKE 用）';

-- 2025年3月
ALTER TABLE trade_202503 COMMENT='2025年3月销售单数据';
ALTER TABLE trade_goods_202503 COMMENT='2025年3月销售单商品明细';
ALTER TABLE trade_package_202503 COMMENT='2025年3月订单包裹详情';

-- 2025年4月
ALTER TABLE trade_202504 COMMENT='2025年4月销售单数据';
ALTER TABLE trade_goods_202504 COMMENT='2025年4月销售单商品明细';
ALTER TABLE trade_package_202504 COMMENT='2025年4月订单包裹详情';

-- 2025年5月
ALTER TABLE trade_202505 COMMENT='2025年5月销售单数据';
ALTER TABLE trade_goods_202505 COMMENT='2025年5月销售单商品明细';
ALTER TABLE trade_package_202505 COMMENT='2025年5月订单包裹详情';

-- 2025年6月
ALTER TABLE trade_202506 COMMENT='2025年6月销售单数据';
ALTER TABLE trade_goods_202506 COMMENT='2025年6月销售单商品明细';
ALTER TABLE trade_package_202506 COMMENT='2025年6月订单包裹详情';

-- 2025年7月
ALTER TABLE trade_202507 COMMENT='2025年7月销售单数据';
ALTER TABLE trade_goods_202507 COMMENT='2025年7月销售单商品明细';
ALTER TABLE trade_package_202507 COMMENT='2025年7月订单包裹详情';

-- 2025年8月
ALTER TABLE trade_202508 COMMENT='2025年8月销售单数据';
ALTER TABLE trade_goods_202508 COMMENT='2025年8月销售单商品明细';
ALTER TABLE trade_package_202508 COMMENT='2025年8月订单包裹详情';

-- 2025年9月
ALTER TABLE trade_202509 COMMENT='2025年9月销售单数据';
ALTER TABLE trade_goods_202509 COMMENT='2025年9月销售单商品明细';
ALTER TABLE trade_package_202509 COMMENT='2025年9月订单包裹详情';

-- 2025年10月
ALTER TABLE trade_202510 COMMENT='2025年10月销售单数据';
ALTER TABLE trade_goods_202510 COMMENT='2025年10月销售单商品明细';
ALTER TABLE trade_package_202510 COMMENT='2025年10月订单包裹详情';

-- 2025年11月
ALTER TABLE trade_202511 COMMENT='2025年11月销售单数据';
ALTER TABLE trade_goods_202511 COMMENT='2025年11月销售单商品明细';
ALTER TABLE trade_package_202511 COMMENT='2025年11月订单包裹详情';

-- 2025年12月
ALTER TABLE trade_202512 COMMENT='2025年12月销售单数据';
ALTER TABLE trade_goods_202512 COMMENT='2025年12月销售单商品明细';
ALTER TABLE trade_package_202512 COMMENT='2025年12月订单包裹详情';

-- 2026年1月
ALTER TABLE trade_202601 COMMENT='2026年1月销售单数据';
ALTER TABLE trade_goods_202601 COMMENT='2026年1月销售单商品明细';
ALTER TABLE trade_package_202601 COMMENT='2026年1月订单包裹详情';

-- 2026年2月
ALTER TABLE trade_202602 COMMENT='2026年2月销售单数据';
ALTER TABLE trade_goods_202602 COMMENT='2026年2月销售单商品明细';
ALTER TABLE trade_package_202602 COMMENT='2026年2月订单包裹详情';

-- 2026年3月
ALTER TABLE trade_202603 COMMENT='2026年3月销售单数据';
ALTER TABLE trade_goods_202603 COMMENT='2026年3月销售单商品明细';
ALTER TABLE trade_package_202603 COMMENT='2026年3月订单包裹详情';

-- 2026年4月
ALTER TABLE trade_202604 COMMENT='2026年4月销售单数据';
ALTER TABLE trade_goods_202604 COMMENT='2026年4月销售单商品明细';
ALTER TABLE trade_package_202604 COMMENT='2026年4月订单包裹详情';

-- 注：trade_202501/trade_goods_202501/trade_package_202501 跳过（sync-trades-v2正在写入）
-- 等sync完成后单独执行：
-- ALTER TABLE trade_202501 COMMENT='2025年1月销售单数据';
-- ALTER TABLE trade_goods_202501 COMMENT='2025年1月销售单商品明细';
-- ALTER TABLE trade_package_202501 COMMENT='2025年1月订单包裹详情';
