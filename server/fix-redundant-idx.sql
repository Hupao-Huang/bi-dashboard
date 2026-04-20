-- H2: 前缀冗余索引清理（单列idx被多列uk的首列覆盖，完全冗余）
-- 跳过 trade_goods_202501（sync在写）

-- sales_goods_summary（166万行 / 966MB索引）
ALTER TABLE sales_goods_summary DROP INDEX idx_stat_date;
ALTER TABLE sales_goods_summary DROP INDEX idx_department;  -- 被 idx_dept_date_amt(department,stat_date,amt) 覆盖

-- trade_goods_* 月份表（idx_trade_id 被 uk_trade_goods 首列覆盖）
ALTER TABLE trade_goods_202503 DROP INDEX idx_trade_id;
ALTER TABLE trade_goods_202504 DROP INDEX idx_trade_id;
ALTER TABLE trade_goods_202505 DROP INDEX idx_trade_id;
ALTER TABLE trade_goods_202506 DROP INDEX idx_trade_id;
ALTER TABLE trade_goods_202507 DROP INDEX idx_trade_id;
ALTER TABLE trade_goods_202508 DROP INDEX idx_trade_id;
ALTER TABLE trade_goods_202509 DROP INDEX idx_trade_id;
ALTER TABLE trade_goods_202510 DROP INDEX idx_trade_id;
ALTER TABLE trade_goods_202511 DROP INDEX idx_trade_id;
ALTER TABLE trade_goods_202512 DROP INDEX idx_trade_id;
ALTER TABLE trade_goods_202601 DROP INDEX idx_trade_id;
ALTER TABLE trade_goods_202602 DROP INDEX idx_trade_id;
ALTER TABLE trade_goods_202603 DROP INDEX idx_trade_id;
ALTER TABLE trade_goods_202604 DROP INDEX idx_trade_id;
ALTER TABLE trade_goods_template DROP INDEX idx_trade_id;

-- agg_* 聚合表
ALTER TABLE agg_daily_shop DROP INDEX idx_stat_date;
ALTER TABLE agg_monthly_department DROP INDEX idx_stat_month;

-- 飞瓜
ALTER TABLE fg_creator_daily DROP INDEX idx_stat_date;
ALTER TABLE fg_creator_roster DROP INDEX idx_platform;

-- 合思
ALTER TABLE hesi_flow_detail DROP INDEX idx_flow_id;
ALTER TABLE hesi_flow_invoice DROP INDEX idx_flow_id;

-- 抖音运营数据
ALTER TABLE op_douyin_ad_live_daily DROP INDEX idx_douyin_ad_live_date;
ALTER TABLE op_douyin_ad_material_daily DROP INDEX idx_douyin_material_date;
ALTER TABLE op_douyin_anchor_daily DROP INDEX idx_douyin_anchor_date;
ALTER TABLE op_douyin_channel_daily DROP INDEX idx_douyin_channel_date;
ALTER TABLE op_douyin_dist_account_daily DROP INDEX idx_dist_account_date;
ALTER TABLE op_douyin_dist_product_daily DROP INDEX idx_dist_product_date;
ALTER TABLE op_douyin_dist_promote_hourly DROP INDEX idx_dist_promote_date;
ALTER TABLE op_douyin_funnel_daily DROP INDEX idx_douyin_funnel_date;
ALTER TABLE op_douyin_goods_daily DROP INDEX idx_douyin_goods_date;
ALTER TABLE op_douyin_live_daily DROP INDEX idx_douyin_live_date;

-- 天猫
ALTER TABLE op_tmall_cs_industry_keyword DROP INDEX idx_date;
ALTER TABLE op_tmall_cs_market_rank DROP INDEX idx_date;
ALTER TABLE op_tmall_shop_daily DROP INDEX idx_stat_date;

-- 小红书
ALTER TABLE op_xhs_cs_excellent_trend_daily DROP INDEX idx_stat_date;
ALTER TABLE op_xhs_cs_trend_daily DROP INDEX idx_stat_date;

-- 权限
ALTER TABLE role_permissions DROP INDEX idx_role_permissions_role_id;
ALTER TABLE user_roles DROP INDEX idx_user_roles_user_id;

-- 等sync完后单独跑：
-- ALTER TABLE trade_goods_202501 DROP INDEX idx_trade_id;
