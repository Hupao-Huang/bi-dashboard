-- 兼容旧代码: 把已删的 op_tmall_cs_campaign_daily 做成从 3 张新推广表 UNION ALL 的 VIEW
-- 让 dashboard.go 里旧的 SELECT ... FROM op_tmall_cs_campaign_daily 查询继续可用
-- 注意:
-- 1) VIEW 不可写，只读；导入由各自新表直接写入
-- 2) 淘客没有 clicks/impressions，填 0
-- 3) 这是过渡层，后续 dashboard.go 可重构直接查各原子表
CREATE OR REPLACE VIEW op_tmall_cs_campaign_daily AS
SELECT stat_date, shop_name, '无界' AS promo_type,
       SUM(cost) AS cost,
       SUM(total_pay_amount) AS pay_amount,
       SUM(clicks) AS clicks,
       SUM(impressions) AS impressions
FROM op_tmall_cs_wujie_scene_daily
GROUP BY stat_date, shop_name
UNION ALL
SELECT stat_date, shop_name, '智多星' AS promo_type,
       SUM(cost) AS cost,
       SUM(total_pay_amount) AS pay_amount,
       SUM(clicks) AS clicks,
       SUM(impressions) AS impressions
FROM op_tmall_cs_smart_plan_daily
GROUP BY stat_date, shop_name
UNION ALL
SELECT stat_date, shop_name, '淘客' AS promo_type,
       taoke_total_cost AS cost,
       taoke_pay_amount AS pay_amount,
       0 AS clicks,
       0 AS impressions
FROM op_tmall_cs_taoke_daily;
