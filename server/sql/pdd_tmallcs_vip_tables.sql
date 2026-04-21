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
-- 店铺拆分: 2025-12-31 起天猫超市分为"天猫超市一盘货"(直发) 和 "天猫超市寄售"(仓库寄售)
-- RPA 文件夹原名 → 简称映射（在 import-tmallcs 代码 rpaShopToName 中实现）:
--   "杭州顺欧食品科技有限公司一盘货 -- 寄售" → "天猫超市一盘货"
--   "杭州顺欧食品科技有限公司寄售 -- 寄售"   → "天猫超市寄售"

-- 天猫超市经营概况(UK 加 shop_name，支持双店)
CREATE TABLE IF NOT EXISTS op_tmall_cs_shop_daily (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date DATE NOT NULL COMMENT '统计日期',
    shop_name VARCHAR(100) NOT NULL COMMENT '店铺名称(天猫超市一盘货/天猫超市寄售)',
    pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '成交金额',
    sub_order_avg_price DECIMAL(10,2) DEFAULT 0 COMMENT '子订单均价',
    avg_price DECIMAL(10,2) DEFAULT 0 COMMENT '客单价',
    ipv_uv INT DEFAULT 0 COMMENT 'IPVUV',
    pay_sub_orders INT DEFAULT 0 COMMENT '支付子订单数',
    pay_qty INT DEFAULT 0 COMMENT '支付商品件数',
    conv_rate DECIMAL(10,4) DEFAULT 0 COMMENT '转化率',
    pay_users INT DEFAULT 0 COMMENT '支付用户数',
    UNIQUE INDEX uk_date_shop (stat_date, shop_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='天猫超市经营概况日数据';

-- ⚠️ 已废弃: op_tmall_cs_campaign_daily
-- 原因: 三种推广文件(无界场景/智多星/淘客诊断) Excel 结构完全不同却共用一张表+同一 import 函数，
--   导致字段位置错位(如 cost 列实际存的是"场景ID"或"转化周期"字符串)。双店合并后还会互相覆盖。
-- 替代: 拆成 op_tmall_cs_wujie_scene_daily / wujie_detail_daily / smart_plan_daily /
--   smart_plan_detail_daily / taoke_daily 五张独立表，各自对应源文件结构。
-- 迁移: 2026-04-20 全量重导时 DROP 此表。

-- 无界-场景级推广(来源: 推广_无界场景数据.xlsx，69 列)
CREATE TABLE IF NOT EXISTS op_tmall_cs_wujie_scene_daily (
    id                          BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date                   DATE           NOT NULL COMMENT '统计日期',
    shop_name                   VARCHAR(100)   NOT NULL COMMENT '店铺名称',
    scene_id                    VARCHAR(64)    NOT NULL COMMENT '场景ID',
    scene_name                  VARCHAR(200)   DEFAULT NULL COMMENT '场景名字',
    orig_scene_id               VARCHAR(64)    DEFAULT NULL COMMENT '原二级场景ID',
    orig_scene_name             VARCHAR(200)   DEFAULT NULL COMMENT '原二级场景名字',
    impressions                 BIGINT         DEFAULT 0 COMMENT '展现量',
    clicks                      INT            DEFAULT 0 COMMENT '点击量',
    cost                        DECIMAL(14,2)  DEFAULT 0 COMMENT '花费',
    click_rate                  DECIMAL(8,6)   DEFAULT 0 COMMENT '点击率',
    avg_click_cost              DECIMAL(10,4)  DEFAULT 0 COMMENT '平均点击花费',
    cpm                         DECIMAL(10,4)  DEFAULT 0 COMMENT '千次展现花费',
    presale_total_amount        DECIMAL(14,2)  DEFAULT 0 COMMENT '总预售成交金额',
    presale_total_count         INT            DEFAULT 0 COMMENT '总预售成交笔数',
    presale_direct_amount       DECIMAL(14,2)  DEFAULT 0 COMMENT '直接预售成交金额',
    presale_direct_count        INT            DEFAULT 0 COMMENT '直接预售成交笔数',
    presale_indirect_amount     DECIMAL(14,2)  DEFAULT 0 COMMENT '间接预售成交金额',
    presale_indirect_count      INT            DEFAULT 0 COMMENT '间接预售成交笔数',
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
    total_cart                  INT            DEFAULT 0 COMMENT '总购物车数',
    direct_cart                 INT            DEFAULT 0 COMMENT '直接购物车数',
    indirect_cart               INT            DEFAULT 0 COMMENT '间接购物车数',
    cart_rate                   DECIMAL(8,6)   DEFAULT 0 COMMENT '加购率',
    cart_cost                   DECIMAL(14,2)  DEFAULT 0 COMMENT '加购成本',
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
    place_order_count           INT            DEFAULT 0 COMMENT '拍下订单笔数',
    place_order_amount          DECIMAL(14,2)  DEFAULT 0 COMMENT '拍下订单金额',
    coupon_claim_count          INT            DEFAULT 0 COMMENT '优惠券领取量',
    shop_money_recharge_count   INT            DEFAULT 0 COMMENT '购物金充值笔数',
    shop_money_recharge_amount  DECIMAL(14,2)  DEFAULT 0 COMMENT '购物金充值金额',
    wangwang_consult_count      INT            DEFAULT 0 COMMENT '旺旺咨询量',
    guide_visit_count           INT            DEFAULT 0 COMMENT '引导访问量',
    guide_visit_users           INT            DEFAULT 0 COMMENT '引导访问人数',
    guide_visit_potential       INT            DEFAULT 0 COMMENT '引导访问潜客数',
    guide_visit_potential_rate  DECIMAL(8,6)   DEFAULT 0 COMMENT '引导访问潜客占比',
    member_join_rate            DECIMAL(8,6)   DEFAULT 0 COMMENT '入会率',
    member_join_count           INT            DEFAULT 0 COMMENT '入会量',
    guide_visit_rate            DECIMAL(8,6)   DEFAULT 0 COMMENT '引导访问率',
    deep_visit_count            INT            DEFAULT 0 COMMENT '深度访问量',
    avg_page_views              DECIMAL(10,4)  DEFAULT 0 COMMENT '平均访问页面数',
    new_customer_count          INT            DEFAULT 0 COMMENT '成交新客数',
    new_customer_rate           DECIMAL(8,6)   DEFAULT 0 COMMENT '成交新客占比',
    member_first_buy            INT            DEFAULT 0 COMMENT '会员首购人数',
    member_pay_amount           DECIMAL(14,2)  DEFAULT 0 COMMENT '会员成交金额',
    member_pay_count            INT            DEFAULT 0 COMMENT '会员成交笔数',
    buyer_count                 INT            DEFAULT 0 COMMENT '成交人数',
    avg_pay_count_per_user      DECIMAL(10,4)  DEFAULT 0 COMMENT '人均成交笔数',
    avg_pay_amount_per_user     DECIMAL(14,2)  DEFAULT 0 COMMENT '人均成交金额',
    natural_flow_pay_amount     DECIMAL(14,2)  DEFAULT 0 COMMENT '自然流量转化金额',
    natural_flow_impressions    BIGINT         DEFAULT 0 COMMENT '自然流量曝光量',
    UNIQUE KEY uk_date_shop_scene (stat_date, shop_name, scene_id),
    KEY idx_date (stat_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='天猫超市无界-场景级推广日数据(来源: 推广_无界场景数据.xlsx)';

-- 无界-商品级明细(来源: 推广_无界明细数据.xlsx，68 列，比天猫万象台明细少 3 列平台助推)
CREATE TABLE IF NOT EXISTS op_tmall_cs_wujie_detail_daily (
    id                          BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date                   DATE           NOT NULL COMMENT '统计日期',
    shop_name                   VARCHAR(100)   NOT NULL COMMENT '店铺名称',
    product_id                  VARCHAR(32)    NOT NULL COMMENT '商品ID(主体ID)',
    entity_type                 VARCHAR(20)    DEFAULT '商品' COMMENT '主体类型',
    product_name                VARCHAR(500)   DEFAULT NULL COMMENT '商品名称',
    impressions                 BIGINT         DEFAULT 0 COMMENT '展现量',
    clicks                      INT            DEFAULT 0 COMMENT '点击量',
    cost                        DECIMAL(14,2)  DEFAULT 0 COMMENT '花费',
    click_rate                  DECIMAL(8,6)   DEFAULT 0 COMMENT '点击率',
    avg_click_cost              DECIMAL(10,4)  DEFAULT 0 COMMENT '平均点击花费',
    cpm                         DECIMAL(10,4)  DEFAULT 0 COMMENT '千次展现花费',
    presale_total_amount        DECIMAL(14,2)  DEFAULT 0 COMMENT '总预售成交金额',
    presale_total_count         INT            DEFAULT 0 COMMENT '总预售成交笔数',
    presale_direct_amount       DECIMAL(14,2)  DEFAULT 0 COMMENT '直接预售成交金额',
    presale_direct_count        INT            DEFAULT 0 COMMENT '直接预售成交笔数',
    presale_indirect_amount     DECIMAL(14,2)  DEFAULT 0 COMMENT '间接预售成交金额',
    presale_indirect_count      INT            DEFAULT 0 COMMENT '间接预售成交笔数',
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
    total_cart                  INT            DEFAULT 0 COMMENT '总购物车数',
    direct_cart                 INT            DEFAULT 0 COMMENT '直接购物车数',
    indirect_cart               INT            DEFAULT 0 COMMENT '间接购物车数',
    cart_rate                   DECIMAL(8,6)   DEFAULT 0 COMMENT '加购率',
    cart_cost                   DECIMAL(14,2)  DEFAULT 0 COMMENT '加购成本',
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
    place_order_count           INT            DEFAULT 0 COMMENT '拍下订单笔数',
    place_order_amount          DECIMAL(14,2)  DEFAULT 0 COMMENT '拍下订单金额',
    coupon_claim_count          INT            DEFAULT 0 COMMENT '优惠券领取量',
    shop_money_recharge_count   INT            DEFAULT 0 COMMENT '购物金充值笔数',
    shop_money_recharge_amount  DECIMAL(14,2)  DEFAULT 0 COMMENT '购物金充值金额',
    wangwang_consult_count      INT            DEFAULT 0 COMMENT '旺旺咨询量',
    guide_visit_count           INT            DEFAULT 0 COMMENT '引导访问量',
    guide_visit_users           INT            DEFAULT 0 COMMENT '引导访问人数',
    guide_visit_potential       INT            DEFAULT 0 COMMENT '引导访问潜客数',
    guide_visit_potential_rate  DECIMAL(8,6)   DEFAULT 0 COMMENT '引导访问潜客占比',
    member_join_rate            DECIMAL(8,6)   DEFAULT 0 COMMENT '入会率',
    member_join_count           INT            DEFAULT 0 COMMENT '入会量',
    guide_visit_rate            DECIMAL(8,6)   DEFAULT 0 COMMENT '引导访问率',
    deep_visit_count            INT            DEFAULT 0 COMMENT '深度访问量',
    avg_page_views              DECIMAL(10,4)  DEFAULT 0 COMMENT '平均访问页面数',
    new_customer_count          INT            DEFAULT 0 COMMENT '成交新客数',
    new_customer_rate           DECIMAL(8,6)   DEFAULT 0 COMMENT '成交新客占比',
    member_first_buy            INT            DEFAULT 0 COMMENT '会员首购人数',
    member_pay_amount           DECIMAL(14,2)  DEFAULT 0 COMMENT '会员成交金额',
    member_pay_count            INT            DEFAULT 0 COMMENT '会员成交笔数',
    buyer_count                 INT            DEFAULT 0 COMMENT '成交人数',
    avg_pay_count_per_user      DECIMAL(10,4)  DEFAULT 0 COMMENT '人均成交笔数',
    avg_pay_amount_per_user     DECIMAL(14,2)  DEFAULT 0 COMMENT '人均成交金额',
    natural_flow_pay_amount     DECIMAL(14,2)  DEFAULT 0 COMMENT '自然流量转化金额',
    natural_flow_impressions    BIGINT         DEFAULT 0 COMMENT '自然流量曝光量',
    UNIQUE KEY uk_date_shop_product (stat_date, shop_name, product_id),
    KEY idx_date (stat_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='天猫超市无界-商品级明细日数据(来源: 推广_无界明细数据.xlsx)';

-- 智多星-总览(来源: 推广_智多星.xlsx，16 列，sheet="数据总览")
CREATE TABLE IF NOT EXISTS op_tmall_cs_smart_plan_daily (
    id                  BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date           DATE           NOT NULL COMMENT '统计日期',
    shop_name           VARCHAR(100)   NOT NULL COMMENT '店铺名称',
    convert_cycle       VARCHAR(20)    DEFAULT NULL COMMENT '转化周期(如 15天)',
    campaign_scene      VARCHAR(50)    DEFAULT NULL COMMENT '投放场景',
    cost                DECIMAL(14,4)  DEFAULT 0 COMMENT '消耗(元)',
    impressions         BIGINT         DEFAULT 0 COMMENT '曝光量',
    clicks              INT            DEFAULT 0 COMMENT '点击量',
    click_rate          DECIMAL(8,6)   DEFAULT 0 COMMENT '点击率',
    avg_click_cost      DECIMAL(10,4)  DEFAULT 0 COMMENT '点击成本',
    cart_count          INT            DEFAULT 0 COMMENT '加购数',
    collect_count       INT            DEFAULT 0 COMMENT '收藏数',
    direct_pay_count    INT            DEFAULT 0 COMMENT '直接成交笔数',
    total_pay_count     INT            DEFAULT 0 COMMENT '总成交笔数',
    direct_pay_amount   DECIMAL(14,4)  DEFAULT 0 COMMENT '直接成交金额',
    total_pay_amount    DECIMAL(14,4)  DEFAULT 0 COMMENT '总成交金额',
    pay_conv_rate       DECIMAL(8,6)   DEFAULT 0 COMMENT '成交转化率',
    roi                 DECIMAL(10,4)  DEFAULT 0 COMMENT 'ROI',
    UNIQUE KEY uk_date_shop_scene (stat_date, shop_name, campaign_scene),
    KEY idx_date (stat_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='天猫超市智多星-总览日数据(来源: 推广_智多星.xlsx)';

-- 智多星-商品/计划明细(来源: 推广_智多星_明细.xlsx，20 列，sheet="商品效果数据")
CREATE TABLE IF NOT EXISTS op_tmall_cs_smart_plan_detail_daily (
    id                  BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date           DATE           NOT NULL COMMENT '统计日期',
    shop_name           VARCHAR(100)   NOT NULL COMMENT '店铺名称',
    plan_id             VARCHAR(64)    NOT NULL COMMENT '计划id',
    plan_name           VARCHAR(200)   DEFAULT NULL COMMENT '计划名称',
    product_id          VARCHAR(32)    NOT NULL COMMENT '宝贝id',
    product_name        VARCHAR(500)   DEFAULT NULL COMMENT '宝贝名称',
    convert_cycle       VARCHAR(20)    DEFAULT NULL COMMENT '转化周期',
    campaign_scene      VARCHAR(50)    DEFAULT NULL COMMENT '投放场景',
    cost                DECIMAL(14,4)  DEFAULT 0 COMMENT '消耗(元)',
    impressions         BIGINT         DEFAULT 0 COMMENT '曝光量',
    clicks              INT            DEFAULT 0 COMMENT '点击量',
    click_rate          DECIMAL(8,6)   DEFAULT 0 COMMENT '点击率',
    avg_click_cost      DECIMAL(10,4)  DEFAULT 0 COMMENT '点击成本',
    cart_count          INT            DEFAULT 0 COMMENT '加购数',
    collect_count       INT            DEFAULT 0 COMMENT '收藏数',
    direct_pay_count    INT            DEFAULT 0 COMMENT '直接成交笔数',
    total_pay_count     INT            DEFAULT 0 COMMENT '总成交笔数',
    direct_pay_amount   DECIMAL(14,4)  DEFAULT 0 COMMENT '直接成交金额',
    total_pay_amount    DECIMAL(14,4)  DEFAULT 0 COMMENT '总成交金额',
    pay_conv_rate       DECIMAL(8,6)   DEFAULT 0 COMMENT '成交转化率',
    roi                 DECIMAL(10,4)  DEFAULT 0 COMMENT 'ROI',
    UNIQUE KEY uk_date_shop_plan_product (stat_date, shop_name, plan_id, product_id),
    KEY idx_date (stat_date),
    KEY idx_plan (plan_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='天猫超市智多星-计划×商品明细日数据(来源: 推广_智多星_明细.xlsx)';

-- 淘客诊断(来源: 推广_淘客诊断.xlsx，12 列，sheet="data"，每次含最近多天滚动数据)
CREATE TABLE IF NOT EXISTS op_tmall_cs_taoke_daily (
    id                          BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date                   DATE           NOT NULL COMMENT '统计日期',
    shop_name                   VARCHAR(100)   NOT NULL COMMENT '店铺名称',
    taoke_pay_amount            DECIMAL(14,4)  DEFAULT 0 COMMENT '淘客成交金额',
    taoke_pay_penetration       DECIMAL(10,8)  DEFAULT 0 COMMENT '淘客成交渗透',
    taoke_total_cost            DECIMAL(14,4)  DEFAULT 0 COMMENT '淘客总投入费用',
    taoke_roi                   DECIMAL(10,4)  DEFAULT 0 COMMENT '淘客投入ROI',
    taoke_pay_users             INT            DEFAULT 0 COMMENT '淘客支付用户数',
    taoke_pay_users_penetration DECIMAL(10,8)  DEFAULT 0 COMMENT '淘客支付用户数渗透',
    taoke_new_density           DECIMAL(10,4)  DEFAULT 0 COMMENT '淘客新客浓度',
    taoke_channel_avg_price     DECIMAL(10,4)  DEFAULT 0 COMMENT '淘客渠道客单价',
    taoke_active_goods          INT            DEFAULT 0 COMMENT '淘客动销商品数',
    overall_active_goods        INT            DEFAULT 0 COMMENT '整体动销商品数',
    taoke_goods_penetration     DECIMAL(10,8)  DEFAULT 0 COMMENT '淘客商品渗透率',
    UNIQUE KEY uk_date_shop (stat_date, shop_name),
    KEY idx_date (stat_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='天猫超市淘客诊断日数据(来源: 推广_淘客诊断.xlsx)';

-- 商品级销售(来源: 销售数据_商品.xlsx，24 列，sheet="data"，SKU 粒度)
CREATE TABLE IF NOT EXISTS op_tmall_cs_goods_daily (
    id                          BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date                   DATE           NOT NULL COMMENT '统计日期',
    shop_name                   VARCHAR(100)   NOT NULL COMMENT '店铺名称',
    product_id                  VARCHAR(32)    NOT NULL COMMENT '商品ID',
    sku_id                      VARCHAR(32)    NOT NULL DEFAULT '0' COMMENT 'SKUID(无 SKU 时填 0)',
    sku_attr                    VARCHAR(500)   DEFAULT NULL COMMENT 'SKU属性',
    product_name                VARCHAR(500)   DEFAULT NULL COMMENT '商品名称',
    product_image               VARCHAR(500)   DEFAULT NULL COMMENT '商品图片',
    category_l4                 VARCHAR(100)   DEFAULT NULL COMMENT '四级类目',
    region                      VARCHAR(50)    DEFAULT NULL COMMENT '区域',
    brand                       VARCHAR(100)   DEFAULT NULL COMMENT '品牌',
    supplier_name               VARCHAR(200)   DEFAULT NULL COMMENT '供应商名称',
    pay_amount                  DECIMAL(14,4)  DEFAULT 0 COMMENT '支付金额',
    pay_qty                     INT            DEFAULT 0 COMMENT '支付商品件数',
    pay_users                   INT            DEFAULT 0 COMMENT '支付用户数',
    cart_users                  INT            DEFAULT 0 COMMENT '加购用户数',
    cart_qty                    INT            DEFAULT 0 COMMENT '加购件数',
    refund_init_amount          DECIMAL(14,4)  DEFAULT 0 COMMENT '发起退款金额',
    refund_init_sub_orders      INT            DEFAULT 0 COMMENT '发起退款子订单数',
    refund_init_qty             INT            DEFAULT 0 COMMENT '发起退款商品件数',
    pay_amount_ex_refund        DECIMAL(14,4)  DEFAULT 0 COMMENT '支付金额(剔退款)',
    pay_sub_orders_ex_refund    INT            DEFAULT 0 COMMENT '支付子订单数(剔退款)',
    refund_success_amount       DECIMAL(14,4)  DEFAULT 0 COMMENT '退款成功金额',
    refund_success_sub_orders   INT            DEFAULT 0 COMMENT '退款成功子订单数',
    refund_success_qty          INT            DEFAULT 0 COMMENT '退款成功商品件数',
    unit_price                  DECIMAL(12,6)  DEFAULT 0 COMMENT '件单价',
    UNIQUE KEY uk_date_shop_product_sku (stat_date, shop_name, product_id, sku_id),
    KEY idx_date (stat_date),
    KEY idx_product (product_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='天猫超市商品级销售日数据(来源: 销售数据_商品.xlsx)';

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
