-- 2026-04-20 天猫超市推广模块重做 + 双店支持
-- 1) 废弃旧推广表
DROP TABLE IF EXISTS op_tmall_cs_campaign_daily;

-- 2) shop_daily UK 加 shop_name
ALTER TABLE op_tmall_cs_shop_daily DROP INDEX uk_date;
ALTER TABLE op_tmall_cs_shop_daily ADD UNIQUE KEY uk_date_shop (stat_date, shop_name);

-- 3) 清空 shop_daily 旧数据（全是错的"天猫超市"单店）
TRUNCATE TABLE op_tmall_cs_shop_daily;

-- 4) 6 张新表 DDL 见 pdd_tmallcs_vip_tables.sql
