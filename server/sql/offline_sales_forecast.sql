CREATE TABLE IF NOT EXISTS offline_sales_forecast (
  id BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键',
  ym CHAR(7) NOT NULL COMMENT '预测月份 YYYY-MM',
  region VARCHAR(32) NOT NULL COMMENT '大区名称',
  sku_code VARCHAR(64) NOT NULL COMMENT 'SKU 编码',
  goods_name VARCHAR(255) DEFAULT NULL COMMENT '货品名称（冗余存,便于查询）',
  forecast_qty INT NOT NULL DEFAULT 0 COMMENT '预测销量(件)',
  operator VARCHAR(64) DEFAULT NULL COMMENT '最后修改人',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  UNIQUE KEY uk_ym_region_sku (ym, region, sku_code),
  KEY idx_ym (ym),
  KEY idx_sku (sku_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='线下部门 SKU×大区 月度销量预测';
