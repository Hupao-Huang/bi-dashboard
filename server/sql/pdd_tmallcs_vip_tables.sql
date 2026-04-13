USE bi_dashboard;

-- ============ 拼多多 ============
-- 拼多多交易概况
CREATE TABLE IF NOT EXISTS op_pdd_shop_daily (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date DATE NOT NULL COMMENT '统计日期',
    shop_name VARCHAR(100) NOT NULL COMMENT '店铺名称',
    pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '成交金额',
    pay_amount_change VARCHAR(20) DEFAULT NULL COMMENT '成交金额变化',
    pay_count INT DEFAULT 0 COMMENT '成交件数',
    pay_count_change VARCHAR(20) DEFAULT NULL COMMENT '成交件数变化',
    pay_orders INT DEFAULT 0 COMMENT '成交订单数',
    pay_orders_change VARCHAR(20) DEFAULT NULL COMMENT '成交订单数变化',
    conv_rate DECIMAL(10,4) DEFAULT 0 COMMENT '成交转化率',
    conv_rate_change VARCHAR(20) DEFAULT NULL COMMENT '转化率变化',
    unit_price DECIMAL(10,2) DEFAULT 0 COMMENT '客单价',
    unit_price_change VARCHAR(20) DEFAULT NULL COMMENT '客单价变化',
    pay_orders_pct DECIMAL(10,4) DEFAULT 0 COMMENT '成交订单占比',
    UNIQUE INDEX uk_date_shop (stat_date, shop_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='拼多多店铺交易概况日数据';

-- 拼多多商品概况
CREATE TABLE IF NOT EXISTS op_pdd_goods_daily (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date DATE NOT NULL COMMENT '统计日期',
    shop_name VARCHAR(100) NOT NULL COMMENT '店铺名称',
    goods_visitors INT DEFAULT 0 COMMENT '商品访客数',
    goods_views INT DEFAULT 0 COMMENT '商品浏览量',
    goods_collect INT DEFAULT 0 COMMENT '商品收藏用户数',
    sale_goods_count INT DEFAULT 0 COMMENT '动销商品数',
    pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '成交金额',
    pay_count INT DEFAULT 0 COMMENT '成交件数',
    UNIQUE INDEX uk_date_shop (stat_date, shop_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='拼多多商品概况日数据';

-- 拼多多推广数据（商品推广CPC）
CREATE TABLE IF NOT EXISTS op_pdd_campaign_daily (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date DATE NOT NULL COMMENT '统计日期',
    shop_name VARCHAR(100) NOT NULL COMMENT '店铺名称',
    promo_type VARCHAR(50) NOT NULL COMMENT '推广类型(商品推广/明星店铺/直播推广)',
    cost DECIMAL(14,2) DEFAULT 0 COMMENT '总花费',
    pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '成交金额(订单口径)',
    roi DECIMAL(10,2) DEFAULT 0 COMMENT '投产比',
    real_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '实际成交金额',
    real_roi DECIMAL(10,2) DEFAULT 0 COMMENT '实际投产比',
    pay_orders INT DEFAULT 0 COMMENT '成交订单数',
    impressions INT DEFAULT 0 COMMENT '曝光次数',
    clicks INT DEFAULT 0 COMMENT '点击数',
    UNIQUE INDEX uk_date_shop_type (stat_date, shop_name, promo_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='拼多多推广日数据';

-- 拼多多多多视频数据
CREATE TABLE IF NOT EXISTS op_pdd_video_daily (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date DATE NOT NULL COMMENT '统计日期',
    shop_name VARCHAR(100) NOT NULL COMMENT '店铺名称',
    total_gmv DECIMAL(14,2) DEFAULT 0 COMMENT '总GMV',
    order_count INT DEFAULT 0 COMMENT '订单数',
    order_uv INT DEFAULT 0 COMMENT '下单用户数',
    feed_count INT DEFAULT 0 COMMENT '作品数',
    video_view_cnt INT DEFAULT 0 COMMENT '视频播放量',
    goods_click_cnt INT DEFAULT 0 COMMENT '商品点击数',
    UNIQUE INDEX uk_date_shop (stat_date, shop_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='拼多多多多视频日数据';

-- ============ 天猫超市 ============
-- 天猫超市经营概况
CREATE TABLE IF NOT EXISTS op_tmall_cs_shop_daily (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date DATE NOT NULL COMMENT '统计日期',
    shop_name VARCHAR(100) DEFAULT '天猫超市' COMMENT '店铺名称',
    pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '成交金额',
    pay_count INT DEFAULT 0 COMMENT '成交件数',
    pay_orders INT DEFAULT 0 COMMENT '成交订单数',
    visitors INT DEFAULT 0 COMMENT '访客数',
    page_views INT DEFAULT 0 COMMENT '浏览量',
    conv_rate DECIMAL(10,4) DEFAULT 0 COMMENT '转化率',
    unit_price DECIMAL(10,2) DEFAULT 0 COMMENT '客单价',
    refund_amount DECIMAL(14,2) DEFAULT 0 COMMENT '退款金额',
    UNIQUE INDEX uk_date (stat_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='天猫超市经营概况日数据';

-- 天猫超市推广数据
CREATE TABLE IF NOT EXISTS op_tmall_cs_campaign_daily (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date DATE NOT NULL COMMENT '统计日期',
    shop_name VARCHAR(100) DEFAULT '天猫超市' COMMENT '店铺名称',
    promo_type VARCHAR(50) NOT NULL COMMENT '推广类型(无界场景/淘客)',
    cost DECIMAL(14,2) DEFAULT 0 COMMENT '花费',
    pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '成交金额',
    roi DECIMAL(10,2) DEFAULT 0 COMMENT 'ROI',
    clicks INT DEFAULT 0 COMMENT '点击数',
    impressions INT DEFAULT 0 COMMENT '展现量',
    UNIQUE INDEX uk_date_type (stat_date, promo_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='天猫超市推广日数据';

-- ============ 唯品会 ============
-- 唯品会经营数据
CREATE TABLE IF NOT EXISTS op_vip_shop_daily (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date DATE NOT NULL COMMENT '统计日期',
    shop_name VARCHAR(100) DEFAULT '自营' COMMENT '店铺名称',
    pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '成交金额',
    pay_count INT DEFAULT 0 COMMENT '成交件数',
    pay_orders INT DEFAULT 0 COMMENT '成交订单数',
    visitors INT DEFAULT 0 COMMENT '访客数',
    cancel_amount DECIMAL(14,2) DEFAULT 0 COMMENT '取消金额',
    refund_amount DECIMAL(14,2) DEFAULT 0 COMMENT '退款金额',
    UNIQUE INDEX uk_date_shop (stat_date, shop_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='唯品会经营日数据';
