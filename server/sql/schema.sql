-- BI Dashboard 数据库建表脚本
-- 方案：原始数据按月分表 + 预聚合表

CREATE DATABASE IF NOT EXISTS bi_dashboard DEFAULT CHARSET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE bi_dashboard;

-- =============================================
-- 1. 销售单原始数据表（按月分表模板）
-- 实际使用时创建: trade_202603, trade_202604 ...
-- =============================================
CREATE TABLE IF NOT EXISTS trade_template (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    trade_id        VARCHAR(64)    NOT NULL COMMENT '吉客云销售单ID',
    trade_no        VARCHAR(64)    NOT NULL COMMENT '销售单号',
    source_trade_no VARCHAR(64)    DEFAULT NULL COMMENT '原始订单号',
    trade_status    INT            NOT NULL COMMENT '销售单状态',
    trade_type      INT            DEFAULT NULL COMMENT '订单类型',
    shop_id         VARCHAR(64)    DEFAULT NULL COMMENT '店铺ID',
    shop_name       VARCHAR(100)   DEFAULT NULL COMMENT '店铺名称',
    warehouse_id    VARCHAR(64)    DEFAULT NULL COMMENT '仓库ID',
    warehouse_name  VARCHAR(100)   DEFAULT NULL COMMENT '仓库名称',
    pay_type        INT            DEFAULT NULL COMMENT '支付方式',
    pay_no          VARCHAR(400)   DEFAULT NULL COMMENT '支付单号',
    charge_currency VARCHAR(20)    DEFAULT 'CNY' COMMENT '结算币种',
    check_total     DECIMAL(14,2)  DEFAULT 0 COMMENT '核算金额',
    other_fee       DECIMAL(14,2)  DEFAULT 0 COMMENT '其他费用',
    seller_memo     VARCHAR(500)   DEFAULT NULL COMMENT '客服备注',
    buyer_memo      VARCHAR(500)   DEFAULT NULL COMMENT '买家备注',
    trade_time      DATETIME       DEFAULT NULL COMMENT '下单时间',
    created_time    DATETIME       DEFAULT NULL COMMENT '创建时间',
    audit_time      DATETIME       DEFAULT NULL COMMENT '审核时间',
    consign_time    DATETIME       DEFAULT NULL COMMENT '发货时间',
    complete_time   DATETIME       DEFAULT NULL COMMENT '完成时间',
    modified_time   DATETIME       DEFAULT NULL COMMENT '修改时间',
    sync_time       DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '同步时间',
    INDEX idx_trade_no (trade_no),
    INDEX idx_shop_id (shop_id),
    INDEX idx_trade_time (trade_time),
    INDEX idx_complete_time (complete_time),
    INDEX idx_sync_time (sync_time),
    UNIQUE INDEX uk_trade_id (trade_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='销售单原始数据（按月分表模板）';

-- =============================================
-- 2. 销售单商品明细表（按月分表模板）
-- =============================================
CREATE TABLE IF NOT EXISTS trade_goods_template (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    trade_id        VARCHAR(64)    NOT NULL COMMENT '销售单ID',
    trade_no        VARCHAR(64)    NOT NULL COMMENT '销售单号',
    goods_id        VARCHAR(64)    DEFAULT NULL COMMENT '商品ID',
    goods_no        VARCHAR(64)    DEFAULT NULL COMMENT '商品编码',
    goods_name      VARCHAR(200)   DEFAULT NULL COMMENT '商品名称',
    spec_name       VARCHAR(255)   DEFAULT NULL COMMENT '规格名称',
    barcode         VARCHAR(64)    DEFAULT NULL COMMENT '条码',
    sku_id          VARCHAR(64)    DEFAULT NULL COMMENT 'SKU ID',
    sell_count      DECIMAL(14,4)  DEFAULT 0 COMMENT '数量',
    sell_price      DECIMAL(14,4)  DEFAULT 0 COMMENT '单价',
    sell_total      DECIMAL(14,2)  DEFAULT 0 COMMENT '总金额',
    cost            DECIMAL(14,4)  DEFAULT 0 COMMENT '商品成本',
    discount_fee    DECIMAL(14,2)  DEFAULT 0 COMMENT '优惠金额',
    tax_fee         DECIMAL(14,2)  DEFAULT 0 COMMENT '税费',
    category_name   VARCHAR(100)   DEFAULT NULL COMMENT '商品分类',
    brand_name      VARCHAR(100)   DEFAULT NULL COMMENT '品牌',
    unit            VARCHAR(20)    DEFAULT NULL COMMENT '单位',
    shop_id         VARCHAR(64)    DEFAULT NULL COMMENT '店铺ID（冗余，加速聚合）',
    trade_time      DATETIME       DEFAULT NULL COMMENT '下单时间（冗余，加速聚合）',
    INDEX idx_trade_id (trade_id),
    INDEX idx_goods_no (goods_no),
    INDEX idx_sku_id (sku_id),
    INDEX idx_shop_id_trade_time (shop_id, trade_time),
    INDEX idx_category (category_name),
    INDEX idx_brand (brand_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='销售单商品明细（按月分表模板）';

-- =============================================
-- 3. 预聚合表：每日部门/店铺汇总
-- 看板直接查这张表，几千人并发无压力
-- =============================================
CREATE TABLE IF NOT EXISTS agg_daily_shop (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date       DATE           NOT NULL COMMENT '统计日期',
    shop_id         VARCHAR(64)    NOT NULL COMMENT '店铺ID',
    shop_name       VARCHAR(100)   DEFAULT NULL COMMENT '店铺名称',
    department      VARCHAR(50)    DEFAULT NULL COMMENT '所属部门: ecommerce/social/offline/distribution',
    order_count     INT            DEFAULT 0 COMMENT '订单数',
    goods_qty       DECIMAL(14,2)  DEFAULT 0 COMMENT '销售件数',
    sales_amount    DECIMAL(14,2)  DEFAULT 0 COMMENT '销售金额',
    cost_amount     DECIMAL(14,2)  DEFAULT 0 COMMENT '成本金额',
    profit_amount   DECIMAL(14,2)  DEFAULT 0 COMMENT '毛利金额',
    discount_amount DECIMAL(14,2)  DEFAULT 0 COMMENT '优惠金额',
    refund_amount   DECIMAL(14,2)  DEFAULT 0 COMMENT '退款金额',
    avg_price       DECIMAL(14,2)  DEFAULT 0 COMMENT '客单价',
    updated_at      DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE INDEX uk_date_shop (stat_date, shop_id),
    INDEX idx_department_date (department, stat_date),
    INDEX idx_stat_date (stat_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='每日店铺销售汇总（预聚合）';

-- =============================================
-- 4. 预聚合表：每日商品汇总
-- =============================================
CREATE TABLE IF NOT EXISTS agg_daily_goods (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_date       DATE           NOT NULL COMMENT '统计日期',
    goods_id        VARCHAR(64)    NOT NULL COMMENT '商品ID',
    goods_no        VARCHAR(64)    DEFAULT NULL COMMENT '商品编码',
    goods_name      VARCHAR(200)   DEFAULT NULL COMMENT '商品名称',
    category_name   VARCHAR(100)   DEFAULT NULL COMMENT '分类',
    brand_name      VARCHAR(100)   DEFAULT NULL COMMENT '品牌',
    shop_id         VARCHAR(64)    DEFAULT NULL COMMENT '店铺ID',
    department      VARCHAR(50)    DEFAULT NULL COMMENT '所属部门',
    sell_count      DECIMAL(14,2)  DEFAULT 0 COMMENT '销售数量',
    sell_amount     DECIMAL(14,2)  DEFAULT 0 COMMENT '销售金额',
    cost_amount     DECIMAL(14,2)  DEFAULT 0 COMMENT '成本金额',
    profit_amount   DECIMAL(14,2)  DEFAULT 0 COMMENT '毛利',
    avg_price       DECIMAL(14,4)  DEFAULT 0 COMMENT '均价',
    updated_at      DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE INDEX uk_date_goods_shop (stat_date, goods_id, shop_id),
    INDEX idx_department_date (department, stat_date),
    INDEX idx_category (category_name, stat_date),
    INDEX idx_brand (brand_name, stat_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='每日商品销售汇总（预聚合）';

-- =============================================
-- 5. 预聚合表：每月部门汇总（综合看板用）
-- =============================================
CREATE TABLE IF NOT EXISTS agg_monthly_department (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    stat_month      VARCHAR(7)     NOT NULL COMMENT '统计月份 yyyy-MM',
    department      VARCHAR(50)    NOT NULL COMMENT '部门',
    order_count     INT            DEFAULT 0 COMMENT '订单数',
    goods_qty       DECIMAL(14,2)  DEFAULT 0 COMMENT '销售件数',
    sales_amount    DECIMAL(14,2)  DEFAULT 0 COMMENT '销售金额',
    cost_amount     DECIMAL(14,2)  DEFAULT 0 COMMENT '成本金额',
    profit_amount   DECIMAL(14,2)  DEFAULT 0 COMMENT '毛利金额',
    refund_amount   DECIMAL(14,2)  DEFAULT 0 COMMENT '退款金额',
    avg_price       DECIMAL(14,2)  DEFAULT 0 COMMENT '客单价',
    updated_at      DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE INDEX uk_month_dept (stat_month, department),
    INDEX idx_stat_month (stat_month)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='每月部门销售汇总（预聚合）';

-- =============================================
-- 6. 销售渠道表（来自 erp.sales.get 接口）
-- 同时作为 店铺-部门映射 使用
-- =============================================
CREATE TABLE IF NOT EXISTS sales_channel (
    id                    BIGINT PRIMARY KEY AUTO_INCREMENT,
    channel_id            VARCHAR(64)    NOT NULL COMMENT '渠道ID（即shopId）',
    channel_code          VARCHAR(64)    DEFAULT NULL COMMENT '渠道编码',
    channel_name          VARCHAR(100)   DEFAULT NULL COMMENT '渠道名称',
    channel_type          VARCHAR(10)    DEFAULT NULL COMMENT '渠道类型: 0分销办公室/1直营网店/2直营门店/3销售办公室/4货主虚拟店/5分销虚拟店/6加盟门店/7内部交易渠道',
    channel_type_name     VARCHAR(50)    DEFAULT NULL COMMENT '渠道类型名称',
    online_plat_code      VARCHAR(50)    DEFAULT NULL COMMENT '平台编码',
    online_plat_name      VARCHAR(50)    DEFAULT NULL COMMENT '平台名称(淘宝/京东/抖音...)',
    channel_depart_id     VARCHAR(64)    DEFAULT NULL COMMENT '负责部门ID',
    channel_depart_name   VARCHAR(100)   DEFAULT NULL COMMENT '负责部门名称',
    cate_id               VARCHAR(64)    DEFAULT NULL COMMENT '渠道分类ID',
    cate_name             VARCHAR(100)   DEFAULT NULL COMMENT '渠道分类名称',
    company_id            VARCHAR(64)    DEFAULT NULL COMMENT '公司ID',
    company_name          VARCHAR(100)   DEFAULT NULL COMMENT '公司名称',
    company_code          VARCHAR(64)    DEFAULT NULL COMMENT '公司编码',
    depart_code           VARCHAR(64)    DEFAULT NULL COMMENT '部门编码',
    warehouse_code        VARCHAR(64)    DEFAULT NULL COMMENT '默认仓库编码',
    warehouse_name        VARCHAR(100)   DEFAULT NULL COMMENT '默认仓库名称',
    link_man              VARCHAR(50)    DEFAULT NULL COMMENT '联系人',
    link_tel              VARCHAR(50)    DEFAULT NULL COMMENT '联系电话',
    memo                  VARCHAR(500)   DEFAULT NULL COMMENT '备注',
    plat_shop_id          VARCHAR(100)   DEFAULT NULL COMMENT '平台店铺ID',
    plat_shop_name        VARCHAR(100)   DEFAULT NULL COMMENT '平台店铺名称',
    responsible_user      VARCHAR(50)    DEFAULT NULL COMMENT '渠道负责人',
    department            VARCHAR(50)    DEFAULT NULL COMMENT 'BI看板部门映射: ecommerce/social/offline/distribution',
    updated_at            DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE INDEX uk_channel_id (channel_id),
    INDEX idx_channel_type (channel_type),
    INDEX idx_cate_name (cate_name),
    INDEX idx_department (department),
    INDEX idx_depart_name (channel_depart_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='销售渠道信息（来自吉客云erp.sales.get）';

-- =============================================
-- 7. 数据同步记录表
-- =============================================
CREATE TABLE IF NOT EXISTS sync_log (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    sync_type       VARCHAR(50)    NOT NULL COMMENT '同步类型: trade/sales_summary',
    start_time      DATETIME       NOT NULL COMMENT '同步数据起始时间',
    end_time        DATETIME       NOT NULL COMMENT '同步数据截止时间',
    record_count    INT            DEFAULT 0 COMMENT '同步记录数',
    status          VARCHAR(20)    NOT NULL DEFAULT 'running' COMMENT '状态: running/success/failed',
    error_msg       TEXT           DEFAULT NULL COMMENT '错误信息',
    created_at      DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at     DATETIME       DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='数据同步日志';
