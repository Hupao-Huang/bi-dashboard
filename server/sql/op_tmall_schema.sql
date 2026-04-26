-- 天猫运营数据表 (数据来源：生意参谋/万象台/淘宝联盟/数据银行/达摩盘/集客)
-- 前缀 op_tmall_ 与吉客云数据区分

USE bi_dashboard;

-- =============================================
-- 1. 生意参谋-店铺销售日数据
-- 来源: 天猫_{date}_{shop}_生意参谋_店铺销售数据.xlsx
-- =============================================
CREATE TABLE IF NOT EXISTS op_tmall_shop_daily (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date       DATE           NOT NULL COMMENT '统计日期',
    shop_name       VARCHAR(100)   NOT NULL COMMENT '店铺名称',
    -- 流量指标
    visitors        INT            DEFAULT 0 COMMENT '访客数',
    visitors_wireless INT          DEFAULT 0 COMMENT '无线端访客数',
    page_views      INT            DEFAULT 0 COMMENT '浏览量',
    product_visitors INT           DEFAULT 0 COMMENT '商品访客数',
    product_views   INT            DEFAULT 0 COMMENT '商品浏览量',
    avg_stay_time   DECIMAL(10,2)  DEFAULT 0 COMMENT '平均停留时长(秒)',
    bounce_rate     DECIMAL(8,6)   DEFAULT 0 COMMENT '跳失率',
    -- 转化指标
    cart_buyers     INT            DEFAULT 0 COMMENT '加购人数',
    cart_qty        INT            DEFAULT 0 COMMENT '加购件数',
    collect_buyers  INT            DEFAULT 0 COMMENT '商品收藏买家数',
    order_amount    DECIMAL(14,2)  DEFAULT 0 COMMENT '下单金额',
    order_buyers    INT            DEFAULT 0 COMMENT '下单买家数',
    order_qty       INT            DEFAULT 0 COMMENT '下单件数',
    order_conv_rate DECIMAL(8,6)   DEFAULT 0 COMMENT '下单转化率',
    pay_amount      DECIMAL(14,2)  DEFAULT 0 COMMENT '支付金额',
    pay_buyers      INT            DEFAULT 0 COMMENT '支付买家数',
    pay_qty         INT            DEFAULT 0 COMMENT '支付件数',
    pay_sub_orders  INT            DEFAULT 0 COMMENT '支付子订单数',
    pay_conv_rate   DECIMAL(8,6)   DEFAULT 0 COMMENT '支付转化率',
    unit_price      DECIMAL(10,2)  DEFAULT 0 COMMENT '客单价',
    uv_value        DECIMAL(10,2)  DEFAULT 0 COMMENT 'UV价值',
    -- 新老客
    old_visitors    INT            DEFAULT 0 COMMENT '老访客数',
    new_visitors    INT            DEFAULT 0 COMMENT '新访客数',
    pay_new_buyers  INT            DEFAULT 0 COMMENT '支付新买家数',
    pay_old_buyers  INT            DEFAULT 0 COMMENT '支付老买家数',
    old_buyer_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '老买家支付金额',
    -- 推广花费
    total_ad_cost   DECIMAL(14,2)  DEFAULT 0 COMMENT '全站推广花费',
    keyword_ad_cost DECIMAL(14,2)  DEFAULT 0 COMMENT '关键词推广花费',
    crowd_ad_cost   DECIMAL(14,2)  DEFAULT 0 COMMENT '精准人群推广花费',
    smart_ad_cost   DECIMAL(14,2)  DEFAULT 0 COMMENT '智能场景花费',
    taobaoke_fee    DECIMAL(14,2)  DEFAULT 0 COMMENT '淘宝客佣金',
    -- 退款与评价
    refund_amount   DECIMAL(14,2)  DEFAULT 0 COMMENT '成功退款金额',
    review_count    INT            DEFAULT 0 COMMENT '评价数',
    -- 会员
    member_total    INT            DEFAULT 0 COMMENT '会员总数',
    member_new      INT            DEFAULT 0 COMMENT '新增会员数',
    member_active   INT            DEFAULT 0 COMMENT '活跃会员数',
    member_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '会员成交金额',
    member_pay_buyers INT          DEFAULT 0 COMMENT '会员成交人数',
    -- 关注
    follow_count    INT            DEFAULT 0 COMMENT '关注店铺人数',
    updated_at      DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE INDEX uk_date_shop (stat_date, shop_name),
    INDEX idx_shop_name (shop_name),
    INDEX idx_stat_date (stat_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='天猫运营-生意参谋店铺销售日数据';

-- =============================================
-- 2. 生意参谋-商品销售日数据
-- 来源: 天猫_{date}_{shop}_生意参谋_商品销售数据.xlsx
-- =============================================
CREATE TABLE IF NOT EXISTS op_tmall_goods_daily (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date       DATE           NOT NULL COMMENT '统计日期',
    shop_name       VARCHAR(100)   NOT NULL COMMENT '店铺名称',
    product_id      VARCHAR(64)    NOT NULL COMMENT '商品ID',
    product_name    VARCHAR(500)   DEFAULT NULL COMMENT '商品名称',
    main_product_id VARCHAR(64)    DEFAULT NULL COMMENT '主商品ID',
    product_type    VARCHAR(20)    DEFAULT NULL COMMENT '商品类型',
    product_no      VARCHAR(64)    DEFAULT NULL COMMENT '货号',
    product_status  VARCHAR(20)    DEFAULT NULL COMMENT '商品状态',
    -- 流量
    visitors        INT            DEFAULT 0 COMMENT '商品访客数',
    page_views      INT            DEFAULT 0 COMMENT '商品浏览量',
    avg_stay_time   DECIMAL(10,2)  DEFAULT 0 COMMENT '平均停留时长',
    detail_bounce_rate VARCHAR(20) DEFAULT NULL COMMENT '商品详情页跳出率',
    -- 收藏加购
    collect_buyers  INT            DEFAULT 0 COMMENT '商品收藏人数',
    cart_qty        INT            DEFAULT 0 COMMENT '商品加购件数',
    cart_buyers     INT            DEFAULT 0 COMMENT '商品加购人数',
    -- 下单
    order_buyers    INT            DEFAULT 0 COMMENT '下单买家数',
    order_qty       INT            DEFAULT 0 COMMENT '下单件数',
    order_amount    DECIMAL(14,2)  DEFAULT 0 COMMENT '下单金额',
    order_conv_rate VARCHAR(20)    DEFAULT NULL COMMENT '下单转化率',
    -- 支付
    pay_buyers      INT            DEFAULT 0 COMMENT '支付买家数',
    pay_qty         INT            DEFAULT 0 COMMENT '支付件数',
    pay_amount      DECIMAL(14,2)  DEFAULT 0 COMMENT '支付金额',
    pay_conv_rate   VARCHAR(20)    DEFAULT NULL COMMENT '商品支付转化率',
    pay_new_buyers  INT            DEFAULT 0 COMMENT '支付新买家数',
    pay_old_buyers  INT            DEFAULT 0 COMMENT '支付老买家数',
    old_buyer_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '老买家支付金额',
    -- 其他
    uv_value        DECIMAL(10,4)  DEFAULT 0 COMMENT '访客平均价值',
    refund_amount   DECIMAL(14,2)  DEFAULT 0 COMMENT '成功退款金额',
    year_pay_amount DECIMAL(14,2)  DEFAULT 0 COMMENT '年累计支付金额',
    month_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '月累计支付金额',
    month_pay_qty   INT            DEFAULT 0 COMMENT '月累计支付件数',
    updated_at      DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE INDEX uk_date_shop_product (stat_date, shop_name, product_id),
    INDEX idx_shop_date (shop_name, stat_date),
    INDEX idx_product_id (product_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='天猫运营-生意参谋商品销售日数据';

-- =============================================
-- 3. 万象台-CPC推广场景日数据
-- 来源: 天猫_{date}_{shop}_万象台_营销场景数据.xlsx
-- =============================================
CREATE TABLE IF NOT EXISTS op_tmall_campaign_daily (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date       DATE           NOT NULL COMMENT '统计日期',
    shop_name       VARCHAR(100)   NOT NULL COMMENT '店铺名称',
    scene_id        VARCHAR(64)    DEFAULT NULL COMMENT '场景ID',
    scene_name      VARCHAR(200)   DEFAULT NULL COMMENT '场景名字',
    orig_scene_id   VARCHAR(64)    DEFAULT NULL COMMENT '原二级场景ID',
    orig_scene_name VARCHAR(200)   DEFAULT NULL COMMENT '原二级场景名字',
    -- 曝光点击
    impressions     BIGINT         DEFAULT 0 COMMENT '展现量',
    clicks          INT            DEFAULT 0 COMMENT '点击量',
    cost            DECIMAL(14,2)  DEFAULT 0 COMMENT '花费',
    click_rate      DECIMAL(8,6)   DEFAULT 0 COMMENT '点击率',
    avg_click_cost  DECIMAL(10,4)  DEFAULT 0 COMMENT '平均点击花费',
    cpm             DECIMAL(10,4)  DEFAULT 0 COMMENT '千次展现花费',
    -- 成交
    total_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '总成交金额',
    total_pay_count INT            DEFAULT 0 COMMENT '总成交笔数',
    direct_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '直接成交金额',
    indirect_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '间接成交金额',
    click_conv_rate DECIMAL(8,6)   DEFAULT 0 COMMENT '点击转化率',
    roi             DECIMAL(10,4)  DEFAULT 0 COMMENT '投入产出比',
    -- 加购收藏
    total_cart      INT            DEFAULT 0 COMMENT '总购物车数',
    cart_rate       DECIMAL(8,6)   DEFAULT 0 COMMENT '加购率',
    total_collect   INT            DEFAULT 0 COMMENT '总收藏数',
    -- 新客
    new_customer_count INT         DEFAULT 0 COMMENT '成交新客数',
    new_customer_rate VARCHAR(20)  DEFAULT NULL COMMENT '成交新客占比',
    -- 会员
    member_first_buy INT           DEFAULT 0 COMMENT '会员首购人数',
    member_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '会员成交金额',
    member_pay_count INT           DEFAULT 0 COMMENT '会员成交笔数',
    updated_at      DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE INDEX uk_date_shop_scene (stat_date, shop_name, scene_id),
    INDEX idx_shop_date (shop_name, stat_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='天猫运营-万象台CPC推广场景日数据';

-- =============================================
-- 4. 淘宝联盟-CPS推广日数据
-- 来源: 天猫_{date}_{shop}_淘宝联盟_营销场景数据.csv
-- =============================================
CREATE TABLE IF NOT EXISTS op_tmall_cps_daily (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date       DATE           NOT NULL COMMENT '统计日期',
    shop_name       VARCHAR(100)   NOT NULL COMMENT '店铺名称',
    plan_name       VARCHAR(100)   DEFAULT NULL COMMENT '数据内容(计划类型)',
    -- 点击
    clicks          INT            DEFAULT 0 COMMENT '点击量(进店量)',
    click_users     INT            DEFAULT 0 COMMENT '点击人数(进店人数)',
    click_conv_rate VARCHAR(20)    DEFAULT NULL COMMENT '点击转化率(付款转化率)',
    -- 付款
    pay_users       INT            DEFAULT 0 COMMENT '付款人数',
    pay_amount      DECIMAL(14,2)  DEFAULT 0 COMMENT '付款金额',
    pay_orders      INT            DEFAULT 0 COMMENT '付款笔数',
    pay_qty         INT            DEFAULT 0 COMMENT '付款件数',
    pay_commission  DECIMAL(14,2)  DEFAULT 0 COMMENT '付款佣金支出',
    pay_service_fee DECIMAL(14,2)  DEFAULT 0 COMMENT '付款服务费支出',
    pay_commission_rate VARCHAR(20) DEFAULT NULL COMMENT '付款佣金率',
    pay_total_cost  DECIMAL(14,2)  DEFAULT 0 COMMENT '付款支出费用',
    -- 结算
    settle_users    INT            DEFAULT 0 COMMENT '结算人数',
    settle_orders   INT            DEFAULT 0 COMMENT '结算笔数',
    settle_amount   DECIMAL(14,2)  DEFAULT 0 COMMENT '结算金额',
    settle_commission DECIMAL(14,2) DEFAULT 0 COMMENT '结算佣金支出',
    settle_service_fee DECIMAL(14,2) DEFAULT 0 COMMENT '结算服务费支出',
    settle_total_cost DECIMAL(14,2) DEFAULT 0 COMMENT '结算支出费用',
    -- 确认收货
    confirm_users   INT            DEFAULT 0 COMMENT '确认收货人数',
    confirm_amount  DECIMAL(14,2)  DEFAULT 0 COMMENT '确认收货金额',
    confirm_orders  INT            DEFAULT 0 COMMENT '确认收货笔数',
    per_item_cost   DECIMAL(10,4)  DEFAULT 0 COMMENT '单件商品付款支出费用',
    updated_at      DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE INDEX uk_date_shop_plan (stat_date, shop_name, plan_name),
    INDEX idx_shop_date (shop_name, stat_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='天猫运营-淘宝联盟CPS推广日数据';

-- =============================================
-- 5. 生意参谋-会员日数据 (JSON格式)
-- 来源: 天猫_{date}_{shop}_生意参谋_会员数据.json
-- 注意: JSON里是30天数组，每天一条记录
-- =============================================
CREATE TABLE IF NOT EXISTS op_tmall_member_daily (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date       DATE           NOT NULL COMMENT '统计日期',
    shop_name       VARCHAR(100)   NOT NULL COMMENT '店铺名称',
    paid_member_cnt INT            DEFAULT 0 COMMENT '支付会员数',
    member_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '会员支付金额',
    member_unit_price DECIMAL(10,2) DEFAULT 0 COMMENT '会员客单价',
    total_member_cnt INT           DEFAULT 0 COMMENT '会员总数',
    repurchase_rate DECIMAL(8,6)   DEFAULT 0 COMMENT '复购率',
    updated_at      DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE INDEX uk_date_shop (stat_date, shop_name),
    INDEX idx_shop_date (shop_name, stat_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='天猫运营-生意参谋会员日数据';

-- =============================================
-- 6. 数据银行-品牌人群日数据 (JSON格式)
-- 来源: 天猫_{date}_{shop}_数据银行_品牌数据.json
-- compare下多个指标的日期+数值数组
-- =============================================
CREATE TABLE IF NOT EXISTS op_tmall_brand_daily (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date       DATE           NOT NULL COMMENT '统计日期',
    shop_name       VARCHAR(100)   NOT NULL COMMENT '店铺名称',
    member_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '会员支付金额',
    customer_volume BIGINT         DEFAULT 0 COMMENT '消费者总量',
    loyal_volume    BIGINT         DEFAULT 0 COMMENT '忠诚人群量',
    awareness_volume BIGINT        DEFAULT 0 COMMENT '认知人群量',
    brand_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '品牌支付金额',
    deepen_ratio    DECIMAL(8,6)   DEFAULT 0 COMMENT '加深率',
    deepen_uv       BIGINT         DEFAULT 0 COMMENT '加深人数',
    purchase_volume BIGINT         DEFAULT 0 COMMENT '购买人群量',
    interest_volume BIGINT         DEFAULT 0 COMMENT '兴趣人群量',
    updated_at      DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE INDEX uk_date_shop (stat_date, shop_name),
    INDEX idx_shop_date (shop_name, stat_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='天猫运营-数据银行品牌人群日数据';

-- =============================================
-- 7. 达摩盘-人群画像日数据 (JSON格式)
-- 来源: 天猫_{date}_{shop}_达摩盘_人群数据.json
-- =============================================
CREATE TABLE IF NOT EXISTS op_tmall_crowd_daily (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date       DATE           NOT NULL COMMENT '统计日期',
    shop_name       VARCHAR(100)   NOT NULL COMMENT '店铺名称',
    coverage        BIGINT         DEFAULT 0 COMMENT '覆盖人数',
    ta_concentrate_ratio DECIMAL(8,6) DEFAULT 0 COMMENT 'TA浓度',
    ta_permeability_ratio DECIMAL(8,6) DEFAULT 0 COMMENT 'TA渗透率',
    ta_permeability_visit DECIMAL(8,6) DEFAULT 0 COMMENT 'TA渗透-访问',
    ta_permeability_repurchase DECIMAL(8,6) DEFAULT 0 COMMENT 'TA渗透-复购',
    ta_permeability_prospect DECIMAL(8,6) DEFAULT 0 COMMENT 'TA渗透-潜客',
    ta_permeability_purchase DECIMAL(8,6) DEFAULT 0 COMMENT 'TA渗透-首购',
    ta_permeability_interest DECIMAL(8,6) DEFAULT 0 COMMENT 'TA渗透-兴趣',
    shop_alipay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '店铺支付金额',
    shop_alipay_cnt INT            DEFAULT 0 COMMENT '店铺支付笔数',
    shop_alipay_uv  INT            DEFAULT 0 COMMENT '店铺支付人数',
    updated_at      DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE INDEX uk_date_shop (stat_date, shop_name),
    INDEX idx_shop_date (shop_name, stat_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='天猫运营-达摩盘人群画像日数据';

-- =============================================
-- 8. 集客-复购月数据
-- 来源: 天猫_{date}_{shop}_集客_复购数据.xlsx
-- 注意: 这是月度数据，不是日数据
-- =============================================
CREATE TABLE IF NOT EXISTS op_tmall_repurchase_monthly (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_month      VARCHAR(7)     NOT NULL COMMENT '统计月份 yyyy-MM',
    shop_name       VARCHAR(100)   NOT NULL COMMENT '店铺名称',
    category        VARCHAR(100)   DEFAULT NULL COMMENT '行业/类目',
    new_ratio       DECIMAL(8,4)   DEFAULT 0 COMMENT '新客占比',
    new_sales_ratio DECIMAL(8,4)   DEFAULT 0 COMMENT '新客销售额占比',
    new_repurchase_30d DECIMAL(8,4) DEFAULT 0 COMMENT '新客复购率(30天)',
    new_repurchase_60d DECIMAL(8,4) DEFAULT 0 COMMENT '新客复购率(60天)',
    new_repurchase_90d DECIMAL(8,4) DEFAULT 0 COMMENT '新客复购率(90天)',
    new_repurchase_180d DECIMAL(8,4) DEFAULT 0 COMMENT '新客复购率(180天)',
    new_repurchase_360d DECIMAL(8,4) DEFAULT 0 COMMENT '新客复购率(360天)',
    old_ratio       DECIMAL(8,4)   DEFAULT 0 COMMENT '老客占比',
    old_sales_ratio DECIMAL(8,4)   DEFAULT 0 COMMENT '老客销售额占比',
    old_repurchase_30d DECIMAL(8,4) DEFAULT 0 COMMENT '老客复购率(30天)',
    old_repurchase_60d DECIMAL(8,4) DEFAULT 0 COMMENT '老客复购率(60天)',
    old_repurchase_90d DECIMAL(8,4) DEFAULT 0 COMMENT '老客复购率(90天)',
    old_repurchase_180d DECIMAL(8,4) DEFAULT 0 COMMENT '老客复购率(180天)',
    old_repurchase_360d DECIMAL(8,4) DEFAULT 0 COMMENT '老客复购率(360天)',
    shop_repurchase_30d DECIMAL(8,4) DEFAULT 0 COMMENT '店铺复购率(30天)',
    shop_repurchase_180d DECIMAL(8,4) DEFAULT 0 COMMENT '店铺复购率(180天)',
    shop_repurchase_360d DECIMAL(8,4) DEFAULT 0 COMMENT '店铺复购率(360天)',
    lost_repurchase_rate DECIMAL(8,4) DEFAULT 0 COMMENT '流失客户回购率',
    last_repurchase_days INT       DEFAULT 0 COMMENT '最后一次回购间隔(天)',
    unit_price      DECIMAL(10,2)  DEFAULT 0 COMMENT '客单价(元)',
    updated_at      DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE INDEX uk_month_shop (stat_month, shop_name),
    INDEX idx_shop (shop_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='天猫运营-集客复购月数据';

-- =============================================
-- 9. 集客-行业对比月数据
-- 来源: 天猫_{date}_{shop}_集客_行业数据.xlsx
-- =============================================
CREATE TABLE IF NOT EXISTS op_tmall_industry_monthly (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_month      VARCHAR(7)     NOT NULL COMMENT '统计月份 yyyy-MM',
    shop_name       VARCHAR(100)   NOT NULL COMMENT '店铺名称',
    category        VARCHAR(100)   DEFAULT NULL COMMENT '行业/类目',
    value_type      VARCHAR(20)    NOT NULL COMMENT '取值方式: 平均值/最大值',
    new_ratio       DECIMAL(8,4)   DEFAULT 0 COMMENT '新客占比',
    new_sales_ratio DECIMAL(8,4)   DEFAULT 0 COMMENT '新客销售额占比',
    new_repurchase_30d DECIMAL(8,4) DEFAULT 0 COMMENT '新客复购率(30天)',
    new_repurchase_60d DECIMAL(8,4) DEFAULT 0 COMMENT '新客复购率(60天)',
    new_repurchase_90d DECIMAL(8,4) DEFAULT 0 COMMENT '新客复购率(90天)',
    new_repurchase_180d DECIMAL(8,4) DEFAULT 0 COMMENT '新客复购率(180天)',
    new_repurchase_360d DECIMAL(8,4) DEFAULT 0 COMMENT '新客复购率(360天)',
    old_ratio       DECIMAL(8,4)   DEFAULT 0 COMMENT '老客占比',
    old_sales_ratio DECIMAL(8,4)   DEFAULT 0 COMMENT '老客销售额占比',
    old_repurchase_30d DECIMAL(8,4) DEFAULT 0 COMMENT '老客复购率(30天)',
    old_repurchase_60d DECIMAL(8,4) DEFAULT 0 COMMENT '老客复购率(60天)',
    old_repurchase_90d DECIMAL(8,4) DEFAULT 0 COMMENT '老客复购率(90天)',
    old_repurchase_180d DECIMAL(8,4) DEFAULT 0 COMMENT '老客复购率(180天)',
    old_repurchase_360d DECIMAL(8,4) DEFAULT 0 COMMENT '老客复购率(360天)',
    shop_repurchase_30d DECIMAL(8,4) DEFAULT 0 COMMENT '店铺复购率(30天)',
    shop_repurchase_180d DECIMAL(8,4) DEFAULT 0 COMMENT '店铺复购率(180天)',
    shop_repurchase_360d DECIMAL(8,4) DEFAULT 0 COMMENT '店铺复购率(360天)',
    lost_repurchase_rate DECIMAL(8,4) DEFAULT 0 COMMENT '流失客户回购率',
    last_repurchase_days INT       DEFAULT 0 COMMENT '最后一次回购间隔(天)',
    unit_price      DECIMAL(10,2)  DEFAULT 0 COMMENT '客单价(元)',
    updated_at      DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE INDEX uk_month_shop_type (stat_month, shop_name, value_type),
    INDEX idx_shop (shop_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='天猫运营-集客行业对比月数据';

-- =============================================
-- 12. 万象台-CPC推广商品级明细日数据
-- 来源: 天猫_{date}_{shop}_万象台_营销明细数据.xlsx
-- 粒度: 日期 × 店铺 × 商品(主体ID)
-- =============================================
CREATE TABLE IF NOT EXISTS op_tmall_campaign_detail_daily (
    id                          BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date                   DATE           NOT NULL COMMENT '统计日期',
    shop_name                   VARCHAR(100)   NOT NULL COMMENT '店铺名称',
    product_id                  VARCHAR(32)    NOT NULL COMMENT '商品ID(Excel主体ID)',
    entity_type                 VARCHAR(20)    DEFAULT '商品' COMMENT '主体类型(通常为"商品")',
    product_name                VARCHAR(500)   DEFAULT NULL COMMENT '商品名称(主体名称)',
    -- 曝光点击
    impressions                 BIGINT         DEFAULT 0 COMMENT '展现量',
    clicks                      INT            DEFAULT 0 COMMENT '点击量',
    cost                        DECIMAL(14,2)  DEFAULT 0 COMMENT '花费',
    click_rate                  DECIMAL(8,6)   DEFAULT 0 COMMENT '点击率',
    avg_click_cost              DECIMAL(10,4)  DEFAULT 0 COMMENT '平均点击花费',
    cpm                         DECIMAL(10,4)  DEFAULT 0 COMMENT '千次展现花费',
    -- 预售成交
    presale_total_amount        DECIMAL(14,2)  DEFAULT 0 COMMENT '总预售成交金额',
    presale_total_count         INT            DEFAULT 0 COMMENT '总预售成交笔数',
    presale_direct_amount       DECIMAL(14,2)  DEFAULT 0 COMMENT '直接预售成交金额',
    presale_direct_count        INT            DEFAULT 0 COMMENT '直接预售成交笔数',
    presale_indirect_amount     DECIMAL(14,2)  DEFAULT 0 COMMENT '间接预售成交金额',
    presale_indirect_count      INT            DEFAULT 0 COMMENT '间接预售成交笔数',
    -- 成交
    direct_pay_amount           DECIMAL(14,2)  DEFAULT 0 COMMENT '直接成交金额',
    indirect_pay_amount         DECIMAL(14,2)  DEFAULT 0 COMMENT '间接成交金额',
    total_pay_amount            DECIMAL(14,2)  DEFAULT 0 COMMENT '总成交金额',
    total_pay_count             INT            DEFAULT 0 COMMENT '总成交笔数',
    direct_pay_count            INT            DEFAULT 0 COMMENT '直接成交笔数',
    indirect_pay_count          INT            DEFAULT 0 COMMENT '间接成交笔数',
    click_conv_rate             DECIMAL(8,6)   DEFAULT 0 COMMENT '点击转化率',
    roi                         DECIMAL(10,4)  DEFAULT 0 COMMENT '投入产出比',
    roi_with_presale            DECIMAL(10,4)  DEFAULT 0 COMMENT '含预售投产比',
    total_pay_cost              DECIMAL(14,2)  DEFAULT 0 COMMENT '总成交成本',
    -- 购物车
    total_cart                  INT            DEFAULT 0 COMMENT '总购物车数',
    direct_cart                 INT            DEFAULT 0 COMMENT '直接购物车数',
    indirect_cart               INT            DEFAULT 0 COMMENT '间接购物车数',
    cart_rate                   DECIMAL(8,6)   DEFAULT 0 COMMENT '加购率',
    cart_cost                   DECIMAL(14,2)  DEFAULT 0 COMMENT '加购成本',
    -- 收藏
    goods_collect_count         INT            DEFAULT 0 COMMENT '收藏宝贝数',
    shop_collect_count          INT            DEFAULT 0 COMMENT '收藏店铺数',
    shop_collect_cost           DECIMAL(14,2)  DEFAULT 0 COMMENT '店铺收藏成本',
    total_cart_collect          INT            DEFAULT 0 COMMENT '总收藏加购数',
    total_cart_collect_cost     DECIMAL(14,2)  DEFAULT 0 COMMENT '总收藏加购成本',
    goods_cart_collect          INT            DEFAULT 0 COMMENT '宝贝收藏加购数',
    goods_cart_collect_cost     DECIMAL(14,2)  DEFAULT 0 COMMENT '宝贝收藏加购成本',
    total_collect               INT            DEFAULT 0 COMMENT '总收藏数',
    goods_collect_cost          DECIMAL(14,2)  DEFAULT 0 COMMENT '宝贝收藏成本',
    goods_collect_rate          DECIMAL(8,6)   DEFAULT 0 COMMENT '宝贝收藏率',
    direct_goods_collect        INT            DEFAULT 0 COMMENT '直接收藏宝贝数',
    indirect_goods_collect      INT            DEFAULT 0 COMMENT '间接收藏宝贝数',
    -- 订单 / 其他互动
    place_order_count           INT            DEFAULT 0 COMMENT '拍下订单笔数',
    place_order_amount          DECIMAL(14,2)  DEFAULT 0 COMMENT '拍下订单金额',
    coupon_claim_count          INT            DEFAULT 0 COMMENT '优惠券领取量',
    shop_money_recharge_count   INT            DEFAULT 0 COMMENT '购物金充值笔数',
    shop_money_recharge_amount  DECIMAL(14,2)  DEFAULT 0 COMMENT '购物金充值金额',
    wangwang_consult_count      INT            DEFAULT 0 COMMENT '旺旺咨询量',
    -- 引导访问
    guide_visit_count           INT            DEFAULT 0 COMMENT '引导访问量',
    guide_visit_users           INT            DEFAULT 0 COMMENT '引导访问人数',
    guide_visit_potential       INT            DEFAULT 0 COMMENT '引导访问潜客数',
    guide_visit_potential_rate  DECIMAL(8,6)   DEFAULT 0 COMMENT '引导访问潜客占比',
    member_join_rate            DECIMAL(8,6)   DEFAULT 0 COMMENT '入会率',
    member_join_count           INT            DEFAULT 0 COMMENT '入会量',
    guide_visit_rate            DECIMAL(8,6)   DEFAULT 0 COMMENT '引导访问率',
    deep_visit_count            INT            DEFAULT 0 COMMENT '深度访问量',
    avg_page_views              DECIMAL(10,4)  DEFAULT 0 COMMENT '平均访问页面数',
    -- 成交人群
    new_customer_count          INT            DEFAULT 0 COMMENT '成交新客数',
    new_customer_rate           DECIMAL(8,6)   DEFAULT 0 COMMENT '成交新客占比',
    member_first_buy            INT            DEFAULT 0 COMMENT '会员首购人数',
    member_pay_amount           DECIMAL(14,2)  DEFAULT 0 COMMENT '会员成交金额',
    member_pay_count            INT            DEFAULT 0 COMMENT '会员成交笔数',
    buyer_count                 INT            DEFAULT 0 COMMENT '成交人数',
    avg_pay_count_per_user      DECIMAL(10,4)  DEFAULT 0 COMMENT '人均成交笔数',
    avg_pay_amount_per_user     DECIMAL(14,2)  DEFAULT 0 COMMENT '人均成交金额',
    -- 自然流量 / 平台助推
    natural_flow_pay_amount     DECIMAL(14,2)  DEFAULT 0 COMMENT '自然流量转化金额',
    natural_flow_impressions    BIGINT         DEFAULT 0 COMMENT '自然流量曝光量',
    platform_total_pay          DECIMAL(14,2)  DEFAULT 0 COMMENT '平台助推总成交',
    platform_direct_pay         DECIMAL(14,2)  DEFAULT 0 COMMENT '平台助推直接成交',
    platform_clicks             INT            DEFAULT 0 COMMENT '平台助推点击',
    created_at                  DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at                  DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_date_shop_product (stat_date, shop_name, product_id),
    KEY idx_date (stat_date),
    KEY idx_product (product_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='天猫万象台CPC推广-商品级明细(来源: 万象台_营销明细数据.xlsx)';
