-- 到货单视角: 补来源采购单号 + 来源单据类型 + 到货单号索引
ALTER TABLE ys_inspection
  ADD COLUMN source_order_code VARCHAR(64) DEFAULT NULL COMMENT '来源采购订单号' AFTER vsourcecode,
  ADD COLUMN source_bill_type VARCHAR(64) DEFAULT NULL COMMENT '来源单据类型 pu_arrivalorder采购到货/po_osm_arrive_order委外到货' AFTER source_order_code,
  ADD KEY idx_arrival (vsourcecode);

UPDATE ys_inspection SET
  source_order_code = JSON_UNQUOTE(JSON_EXTRACT(raw_json,'$.sourceOrderCode')),
  source_bill_type  = JSON_UNQUOTE(JSON_EXTRACT(raw_json,'$.sourcebilltype'))
WHERE raw_json IS NOT NULL;
