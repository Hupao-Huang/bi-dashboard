-- H3: 删除 idx_trade_status（cardinality=1，全是9090已发货，无区分度）
-- 跳过 trade_202501（sync在写）
ALTER TABLE trade_202503 DROP INDEX idx_trade_status;
ALTER TABLE trade_202504 DROP INDEX idx_trade_status;
ALTER TABLE trade_202505 DROP INDEX idx_trade_status;
ALTER TABLE trade_202506 DROP INDEX idx_trade_status;
ALTER TABLE trade_202507 DROP INDEX idx_trade_status;
ALTER TABLE trade_202508 DROP INDEX idx_trade_status;
ALTER TABLE trade_202509 DROP INDEX idx_trade_status;
ALTER TABLE trade_202510 DROP INDEX idx_trade_status;
ALTER TABLE trade_202511 DROP INDEX idx_trade_status;
ALTER TABLE trade_202512 DROP INDEX idx_trade_status;
ALTER TABLE trade_202601 DROP INDEX idx_trade_status;
ALTER TABLE trade_202602 DROP INDEX idx_trade_status;
ALTER TABLE trade_202603 DROP INDEX idx_trade_status;
ALTER TABLE trade_202604 DROP INDEX idx_trade_status;
-- 模板表也删（避免以后新建月份表又复制这个无用索引）
ALTER TABLE trade_template DROP INDEX idx_trade_status;
