-- 调拨单主表 (吉客云 erp.allocate.get)
-- 用于特殊渠道(京东自营/天猫超市寄售/朴朴)按调拨单算销售额
CREATE TABLE IF NOT EXISTS allocate_orders (
  id BIGINT NOT NULL AUTO_INCREMENT,
  allocate_no VARCHAR(64) NOT NULL COMMENT '调拨单号',
  allocate_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT '吉客云内部ID',
  in_warehouse_code VARCHAR(64) NOT NULL DEFAULT '' COMMENT '入库仓编号',
  in_warehouse_name VARCHAR(128) NOT NULL DEFAULT '' COMMENT '入库仓名',
  out_warehouse_code VARCHAR(64) NOT NULL DEFAULT '' COMMENT '出库仓编号',
  status INT NOT NULL DEFAULT 0 COMMENT '单据状态 0草稿 1待审 2已审 3已关闭 10审中 20已完成',
  in_status INT NOT NULL DEFAULT 0 COMMENT '入库状态 1待入 2部分入 3入库完成',
  out_status INT NOT NULL DEFAULT 0 COMMENT '出库状态 1待出 2部分出 3出库完成',
  total_amount DECIMAL(14,2) NOT NULL DEFAULT 0.00 COMMENT '调拨总金额(接口给的)',
  sku_count INT NOT NULL DEFAULT 0 COMMENT 'SKU数',
  source_no VARCHAR(64) NOT NULL DEFAULT '' COMMENT '来源单号',
  gmt_create DATETIME NULL COMMENT '创建时间',
  gmt_modified DATETIME NULL COMMENT '修改时间(入库完成时是入库完成时刻)',
  audit_date DATETIME NULL COMMENT '审核时间',
  stat_date DATE NULL COMMENT '销售统计日(=gmt_modified转日期, in_status=3时算)',
  channel_key VARCHAR(20) NOT NULL DEFAULT '' COMMENT '对应渠道key 京东/猫超/朴朴',
  synced_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '同步时间',
  PRIMARY KEY (id),
  UNIQUE KEY uk_allocate_no (allocate_no),
  KEY idx_in_wh_stat (in_warehouse_code, stat_date),
  KEY idx_channel_stat (channel_key, stat_date),
  KEY idx_in_status (in_status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='调拨单主表(特殊渠道按调拨算销售额)';

-- 调拨单明细
CREATE TABLE IF NOT EXISTS allocate_details (
  id BIGINT NOT NULL AUTO_INCREMENT,
  allocate_no VARCHAR(64) NOT NULL COMMENT '调拨单号',
  out_sku_code VARCHAR(64) NOT NULL DEFAULT '' COMMENT '外部货品编号',
  goods_no VARCHAR(64) NOT NULL DEFAULT '' COMMENT '内部货品编号(吉客云SKU)',
  goods_name VARCHAR(255) NOT NULL DEFAULT '' COMMENT '货品名',
  sku_name VARCHAR(255) NOT NULL DEFAULT '' COMMENT 'SKU名',
  sku_barcode VARCHAR(128) NOT NULL DEFAULT '' COMMENT '条码',
  sku_count DECIMAL(14,4) NOT NULL DEFAULT 0.0000 COMMENT '调拨数量',
  out_count DECIMAL(14,4) NOT NULL DEFAULT 0.0000 COMMENT '已出库数量',
  in_count DECIMAL(14,4) NOT NULL DEFAULT 0.0000 COMMENT '已入库数量',
  sku_price DECIMAL(14,4) NOT NULL DEFAULT 0.0000 COMMENT '接口单价',
  total_amount DECIMAL(14,2) NOT NULL DEFAULT 0.00 COMMENT '行金额(接口)',
  excel_price DECIMAL(14,4) NOT NULL DEFAULT 0.0000 COMMENT 'Excel价格表的价',
  excel_amount DECIMAL(14,2) NOT NULL DEFAULT 0.00 COMMENT '用Excel价算出的销售额',
  price_source VARCHAR(20) NOT NULL DEFAULT '' COMMENT '价格来源 excel/api/missing',
  channel_key VARCHAR(20) NOT NULL DEFAULT '' COMMENT '对应渠道key',
  stat_date DATE NULL COMMENT '销售统计日(冗余主表的, 方便查)',
  in_status INT NOT NULL DEFAULT 0 COMMENT '冗余主表in_status, 方便筛已入库完成的',
  synced_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '同步时间',
  PRIMARY KEY (id),
  UNIQUE KEY uk_allocate_sku (allocate_no, goods_no, sku_barcode),
  KEY idx_channel_stat (channel_key, stat_date),
  KEY idx_goods_no (goods_no)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='调拨单明细(每行=一个SKU的调拨)';

-- 特殊渠道价格表 (从 桌面/价格体系.xlsx 导入)
CREATE TABLE IF NOT EXISTS channel_special_price (
  id BIGINT NOT NULL AUTO_INCREMENT,
  channel_key VARCHAR(20) NOT NULL COMMENT '渠道key 京东/猫超/朴朴',
  goods_no VARCHAR(64) NOT NULL DEFAULT '' COMMENT '商品编码',
  barcode VARCHAR(128) NOT NULL DEFAULT '' COMMENT '条码',
  goods_name VARCHAR(255) NOT NULL DEFAULT '' COMMENT '商品名',
  price DECIMAL(14,4) NOT NULL DEFAULT 0.0000 COMMENT '采购价/单价',
  source_xlsx VARCHAR(255) NOT NULL DEFAULT '' COMMENT '来源 xlsx 路径',
  imported_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '导入时间',
  PRIMARY KEY (id),
  UNIQUE KEY uk_channel_goods (channel_key, goods_no),
  KEY idx_barcode (barcode)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='特殊渠道价格表(京东/猫超/朴朴各自维护)';
