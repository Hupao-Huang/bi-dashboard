USE bi_dashboard;

CREATE TABLE IF NOT EXISTS op_jd_shop_daily (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date DATE NOT NULL COMMENT '统计日期',
    shop_name VARCHAR(100) NOT NULL COMMENT '店铺名称',
    visitors INT DEFAULT 0 COMMENT '访客数',
    visitors_change VARCHAR(20) DEFAULT NULL COMMENT '访客数变化',
    page_views INT DEFAULT 0 COMMENT '浏览量',
    page_views_change VARCHAR(20) DEFAULT NULL COMMENT '浏览量变化',
    avg_visit_depth DECIMAL(10,2) DEFAULT 0 COMMENT '人均浏览量',
    avg_stay_time DECIMAL(10,2) DEFAULT 0 COMMENT '平均停留时间(秒)',
    bounce_rate DECIMAL(10,4) DEFAULT 0 COMMENT '跳失率',
    pay_customers INT DEFAULT 0 COMMENT '成交客户数',
    pay_customers_change VARCHAR(20) DEFAULT NULL COMMENT '成交客户数变化',
    pay_count INT DEFAULT 0 COMMENT '成交件数',
    pay_count_change VARCHAR(20) DEFAULT NULL COMMENT '成交件数变化',
    pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '成交金额',
    pay_amount_change VARCHAR(20) DEFAULT NULL COMMENT '成交金额变化',
    pay_orders INT DEFAULT 0 COMMENT '成交订单数',
    unit_price DECIMAL(10,2) DEFAULT 0 COMMENT '客单价',
    conv_rate DECIMAL(10,4) DEFAULT 0 COMMENT '转化率',
    uv_value DECIMAL(10,2) DEFAULT 0 COMMENT 'UV价值',
    refund_amount DECIMAL(14,2) DEFAULT 0 COMMENT '退款金额',
    cart_customers INT DEFAULT 0 COMMENT '加购客户数',
    collect_customers INT DEFAULT 0 COMMENT '收藏客户数',
    UNIQUE INDEX uk_date_shop (stat_date, shop_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='京东店铺日销售数据';

CREATE TABLE IF NOT EXISTS op_jd_affiliate_daily (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date DATE NOT NULL COMMENT '统计日期',
    shop_name VARCHAR(100) NOT NULL COMMENT '店铺名称',
    referral_count INT DEFAULT 0 COMMENT '引入量',
    order_buyers INT DEFAULT 0 COMMENT '下单买家数',
    order_amount DECIMAL(14,2) DEFAULT 0 COMMENT '下单金额',
    est_commission DECIMAL(14,2) DEFAULT 0 COMMENT '预估佣金',
    complete_buyers INT DEFAULT 0 COMMENT '完成买家数',
    complete_amount DECIMAL(14,2) DEFAULT 0 COMMENT '完成金额',
    actual_commission DECIMAL(14,2) DEFAULT 0 COMMENT '实际佣金',
    UNIQUE INDEX uk_date_shop (stat_date, shop_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='京东联盟推广日数据(CPS)';

CREATE TABLE IF NOT EXISTS op_jd_customer_daily (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date DATE NOT NULL COMMENT '统计日期',
    shop_name VARCHAR(100) NOT NULL COMMENT '店铺名称',
    browse_customers INT DEFAULT 0 COMMENT '浏览客户数',
    cart_customers INT DEFAULT 0 COMMENT '加购客户数',
    order_customers INT DEFAULT 0 COMMENT '下单客户数',
    pay_customers INT DEFAULT 0 COMMENT '成交客户数',
    repurchase_customers INT DEFAULT 0 COMMENT '复购客户数',
    lost_customers INT DEFAULT 0 COMMENT '沉睡客户数',
    UNIQUE INDEX uk_date_shop (stat_date, shop_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='京东客户洞察日数据';

CREATE TABLE IF NOT EXISTS op_jd_customer_type_daily (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date DATE NOT NULL COMMENT '统计日期',
    shop_name VARCHAR(100) NOT NULL COMMENT '店铺名称',
    customer_type VARCHAR(20) NOT NULL COMMENT '客户类型(全部/老客/新客)',
    pay_customers INT DEFAULT 0 COMMENT '成交客户数',
    pay_pct DECIMAL(10,4) DEFAULT 0 COMMENT '成交客户占比',
    conv_rate DECIMAL(10,4) DEFAULT 0 COMMENT '成交转化率',
    unit_price DECIMAL(10,2) DEFAULT 0 COMMENT '客单价',
    unit_count DECIMAL(10,2) DEFAULT 0 COMMENT '客单件',
    item_price DECIMAL(10,2) DEFAULT 0 COMMENT '客件价',
    UNIQUE INDEX uk_date_shop_type (stat_date, shop_name, customer_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='京东新老客日数据';

CREATE TABLE IF NOT EXISTS op_jd_promo_sku_daily (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date DATE NOT NULL COMMENT '统计日期',
    shop_name VARCHAR(100) NOT NULL COMMENT '店铺名称',
    promo_type VARCHAR(50) NOT NULL COMMENT '活动类型(便宜包邮/百亿补贴/秒杀)',
    sku_id VARCHAR(64) DEFAULT NULL COMMENT 'SKU ID',
    goods_name VARCHAR(200) DEFAULT NULL COMMENT '商品名称',
    uv INT DEFAULT 0 COMMENT '活动UV',
    pv INT DEFAULT 0 COMMENT '活动PV',
    pay_count INT DEFAULT 0 COMMENT '成交件数',
    pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '成交金额',
    pay_users INT DEFAULT 0 COMMENT '成交用户数',
    pay_orders INT DEFAULT 0 COMMENT '成交订单数',
    avg_price DECIMAL(10,2) DEFAULT 0 COMMENT '客单价',
    INDEX idx_date_shop (stat_date, shop_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='京东营销活动SKU日数据';

CREATE TABLE IF NOT EXISTS op_jd_promo_daily (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date DATE NOT NULL COMMENT '统计日期',
    shop_name VARCHAR(100) NOT NULL COMMENT '店铺名称',
    promo_type VARCHAR(50) NOT NULL COMMENT '活动类型',
    pay_goods_count INT DEFAULT 0 COMMENT '成交商品种类',
    pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '成交金额',
    pay_count INT DEFAULT 0 COMMENT '成交件数',
    pay_users INT DEFAULT 0 COMMENT '成交用户数',
    conv_rate DECIMAL(10,4) DEFAULT 0 COMMENT '转化率',
    uv INT DEFAULT 0 COMMENT '活动UV',
    pv INT DEFAULT 0 COMMENT '活动PV',
    UNIQUE INDEX uk_date_shop_promo (stat_date, shop_name, promo_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='京东营销活动日汇总';

CREATE TABLE IF NOT EXISTS op_jd_industry_keyword (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date DATE NOT NULL COMMENT '统计日期',
    shop_name VARCHAR(100) NOT NULL COMMENT '店铺名称',
    keyword VARCHAR(200) NOT NULL COMMENT '关键词',
    search_rank VARCHAR(50) DEFAULT NULL COMMENT '搜索量排名',
    compete_rank VARCHAR(50) DEFAULT NULL COMMENT '竞争量排名',
    click_rank VARCHAR(50) DEFAULT NULL COMMENT '点击量排名',
    pay_amount_range VARCHAR(50) DEFAULT NULL COMMENT '成交额范围',
    conv_rate_range VARCHAR(50) DEFAULT NULL COMMENT '转化率范围',
    related_goods INT DEFAULT 0 COMMENT '关联商品数',
    cart_ref VARCHAR(50) DEFAULT NULL COMMENT '加车参考',
    top_brand VARCHAR(100) DEFAULT NULL COMMENT '热卖品牌',
    INDEX idx_date_shop (stat_date, shop_name),
    INDEX idx_keyword (keyword)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='京东行业关键词数据';
