-- 快递仓储分析物化预聚合表
-- 数据源: trade_YYYYMM + trade_package_YYYYMM
-- 维度: ym × shop_name × warehouse_name × province (已 normalize)
-- 已固化过滤条件: 排取消单 + state 非空 + trade_type NOT IN (8,12) + 7 仓白名单
-- 用途: handler 在无 SKU 过滤时走物化表，秒级响应（原 7s → <200ms）
-- 重建: cmd/build-warehouse-flow-summary --ym=YYYY-MM
CREATE TABLE IF NOT EXISTS warehouse_flow_summary (
  ym             CHAR(7)      NOT NULL COMMENT '年月 YYYY-MM',
  shop_name      VARCHAR(255) NOT NULL DEFAULT '' COMMENT '渠道名',
  warehouse_name VARCHAR(255) NOT NULL COMMENT '仓库名',
  province       VARCHAR(64)  NOT NULL COMMENT '省份(已normalize)',
  orders         INT          NOT NULL DEFAULT 0 COMMENT '订单数',
  packages       INT          NOT NULL DEFAULT 0 COMMENT '包裹数',
  updated_at     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (ym, shop_name, warehouse_name, province),
  KEY idx_ym (ym),
  KEY idx_ym_warehouse (ym, warehouse_name),
  KEY idx_ym_province  (ym, province)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='快递仓储分析物化预聚合表';
