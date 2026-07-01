-- 销售日报渠道映射(店铺 → 渠道 → 平台)
CREATE TABLE IF NOT EXISTS dim_sales_channel_map (
  shop_name  VARCHAR(200) NOT NULL COMMENT '店铺名(= 销售渠道I,对 trade.shop_name)',
  channel    VARCHAR(50)  NOT NULL COMMENT '渠道(抖音/天猫/拼多多/京东/唯品会/分销/私域/线下/新零售/其它)',
  platform   VARCHAR(20)  NOT NULL COMMENT '平台(社媒/电商/其他)',
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (shop_name)
) COMMENT='销售日报-渠道映射' COLLATE=utf8mb4_0900_ai_ci;

-- 销售日报箱规托规(货品 → 每箱瓶数 / 每托箱数)
CREATE TABLE IF NOT EXISTS dim_goods_pack_spec (
  goods_no       VARCHAR(64)   NOT NULL COMMENT '货品编码(对 trade_goods.goods_no / goods.goods_no)',
  box_qty        DECIMAL(14,4) NULL COMMENT '箱规=每箱单瓶数',
  pallet_box_qty DECIMAL(14,4) NULL COMMENT '托规=每托箱数',
  updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (goods_no)
) COMMENT='销售日报-箱规托规' COLLATE=utf8mb4_0900_ai_ci;
