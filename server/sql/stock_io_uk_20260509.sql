-- v0.87 stock_in_log/stock_out_log 加 UK + dedupe (2026-05-09)
-- 解决: stock_in_log 同一同步任务内 API 返回重复行造成 163 个重复组
-- 操作顺序: 备份 → dedupe → ADD UK
-- 配套代码: server/cmd/sync-stock-io/main.go 改成 collect → tx.Begin → DELETE → INSERT ON DUPLICATE KEY UPDATE → Commit (事务化兼顾防 API 失败空窗)

-- step 1: 备份原表 (执行时间: 2026-05-09 10:25)
-- DROP TABLE IF EXISTS stock_in_log_bak_20260509;
-- CREATE TABLE stock_in_log_bak_20260509 AS SELECT * FROM stock_in_log;

-- step 2: dedupe 保留每组 MIN(id)
-- DELETE s1 FROM stock_in_log s1
-- INNER JOIN (
--   SELECT MIN(id) AS min_id, goodsdoc_no, goods_no, sku_barcode, batch_no
--   FROM stock_in_log
--   GROUP BY goodsdoc_no, goods_no, sku_barcode, batch_no
--   HAVING COUNT(*) > 1
-- ) t ON s1.goodsdoc_no = t.goodsdoc_no AND s1.goods_no = t.goods_no
--    AND s1.sku_barcode = t.sku_barcode AND s1.batch_no = t.batch_no
--    AND s1.id > t.min_id;
-- 结果: 2062 → 1893, 删 169 行重复

-- step 3: 加 UK
ALTER TABLE stock_in_log ADD UNIQUE KEY uk_doc_sku_batch (goodsdoc_no, goods_no, sku_barcode, batch_no);
ALTER TABLE stock_out_log ADD UNIQUE KEY uk_rec_id (rec_id);

-- 回滚 (如果出问题):
-- DROP TABLE stock_in_log;
-- RENAME TABLE stock_in_log_bak_20260509 TO stock_in_log;
-- ALTER TABLE stock_out_log DROP INDEX uk_rec_id;
