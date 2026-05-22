-- v1.71.2 (2026-05-22) — gross_profit_rate / tax_gross_profit_rate 精度扩容
--
-- 起因: 跑哥 5/22 决策修 FlexFloat 支持百分号字符串 (按字面值存),
--      实际数据极端值高达 6,652,600% (赔付/退款场景), DECIMAL(8,4) 装不下.
--
-- 影响: 旧字段 DECIMAL(8,4) 最大 ±9999.9999, 100 行实测超界.
--      回填时 13 天事务因 Error 1264 (Out of range) 整批回滚, 保留旧 0 数据.
--
-- 修复: ALTER 到 DECIMAL(14,4), 最大 ±9999999999.9999 (10B), 一劳永逸.
--
-- 跑哥已在生产手工执行 (2026-05-22 09:50), 本 SQL 仅作记录, 新部署或 reseed 时复用.

ALTER TABLE sales_goods_summary
  MODIFY COLUMN gross_profit_rate     DECIMAL(14,4) DEFAULT NULL COMMENT '毛利率(按字面值,如55.30表示55.30%)',
  MODIFY COLUMN tax_gross_profit_rate DECIMAL(14,4) DEFAULT NULL COMMENT '含税毛利率(按字面值)';

-- 月表 sales_goods_summary_monthly 同步扩容 (保持 schema 一致)
ALTER TABLE sales_goods_summary_monthly
  MODIFY COLUMN gross_profit_rate     DECIMAL(14,4) DEFAULT NULL COMMENT '毛利率(按字面值,如55.30表示55.30%)',
  MODIFY COLUMN tax_gross_profit_rate DECIMAL(14,4) DEFAULT NULL COMMENT '含税毛利率(按字面值)';
