-- v0.99 11 张运营表中文注释补全(P1-4)
-- 范围: op_douyin_goods_daily / op_douyin_live_daily / op_douyin_ad_material_daily /
--       op_douyin_dist_account_daily / op_douyin_dist_material_daily / op_douyin_dist_product_daily /
--       op_douyin_cs_feige_daily / op_jd_cs_workload_daily / op_jd_cs_sales_perf_daily /
--       op_kuaishou_cs_assessment_daily / op_xhs_cs_analysis_daily
-- 共 ~210 字段
-- 仅 ALTER MODIFY ... COMMENT, 不动字段类型/索引/UK
-- 客服业务 KPI 字段的"含义"按 RPA 表头/业务通用术语写, 不重新定义口径

-- =================== 表 1: op_douyin_goods_daily (抖音商品日数据 17 字段) ===================
ALTER TABLE op_douyin_goods_daily
  MODIFY COLUMN id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  MODIFY COLUMN stat_date DATE NOT NULL COMMENT '统计日期',
  MODIFY COLUMN shop_name VARCHAR(100) NOT NULL COMMENT '店铺名称',
  MODIFY COLUMN product_id VARCHAR(64) NOT NULL COMMENT '商品ID',
  MODIFY COLUMN product_name VARCHAR(255) DEFAULT '' COMMENT '商品名称',
  MODIFY COLUMN live_price VARCHAR(50) DEFAULT '' COMMENT '直播价',
  MODIFY COLUMN pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '支付金额',
  MODIFY COLUMN pay_qty INT DEFAULT 0 COMMENT '支付件数',
  MODIFY COLUMN presale_count INT DEFAULT 0 COMMENT '预售件数',
  MODIFY COLUMN click_uv INT DEFAULT 0 COMMENT '商品点击人数(UV)',
  MODIFY COLUMN click_rate DECIMAL(8,4) DEFAULT 0 COMMENT '点击率',
  MODIFY COLUMN conv_rate DECIMAL(8,4) DEFAULT 0 COMMENT '转化率',
  MODIFY COLUMN pre_refund_count INT DEFAULT 0 COMMENT '售前退款件数',
  MODIFY COLUMN pre_refund_amount DECIMAL(14,2) DEFAULT 0 COMMENT '售前退款金额',
  MODIFY COLUMN post_refund_count INT DEFAULT 0 COMMENT '售后退款件数',
  MODIFY COLUMN post_refund_amount DECIMAL(14,2) DEFAULT 0 COMMENT '售后退款金额',
  MODIFY COLUMN updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间';

-- =================== 表 2: op_douyin_live_daily (抖音直播日数据 22 字段) ===================
ALTER TABLE op_douyin_live_daily
  MODIFY COLUMN id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  MODIFY COLUMN stat_date DATE NOT NULL COMMENT '统计日期',
  MODIFY COLUMN shop_name VARCHAR(100) NOT NULL COMMENT '店铺名称',
  MODIFY COLUMN anchor_name VARCHAR(100) DEFAULT '' COMMENT '主播昵称',
  MODIFY COLUMN anchor_id VARCHAR(100) DEFAULT '' COMMENT '主播ID',
  MODIFY COLUMN start_time DATETIME DEFAULT NULL COMMENT '开播时间',
  MODIFY COLUMN end_time DATETIME DEFAULT NULL COMMENT '下播时间',
  MODIFY COLUMN duration_min DECIMAL(10,2) DEFAULT 0 COMMENT '直播时长(分钟)',
  MODIFY COLUMN comments INT DEFAULT 0 COMMENT '评论数',
  MODIFY COLUMN new_fans INT DEFAULT 0 COMMENT '新增粉丝数',
  MODIFY COLUMN product_count INT DEFAULT 0 COMMENT '直播商品数',
  MODIFY COLUMN product_exposure_uv INT DEFAULT 0 COMMENT '商品曝光人数(UV)',
  MODIFY COLUMN product_click_uv INT DEFAULT 0 COMMENT '商品点击人数(UV)',
  MODIFY COLUMN order_count INT DEFAULT 0 COMMENT '订单数',
  MODIFY COLUMN order_amount DECIMAL(14,2) DEFAULT 0 COMMENT '订单金额',
  MODIFY COLUMN pay_per_hour DECIMAL(14,2) DEFAULT 0 COMMENT '小时支付金额',
  MODIFY COLUMN pay_qty INT DEFAULT 0 COMMENT '支付件数',
  MODIFY COLUMN pay_uv INT DEFAULT 0 COMMENT '支付人数(UV)',
  MODIFY COLUMN refund_count INT DEFAULT 0 COMMENT '退款件数',
  MODIFY COLUMN refund_amount DECIMAL(14,2) DEFAULT 0 COMMENT '退款金额',
  MODIFY COLUMN net_order_count INT DEFAULT 0 COMMENT '净订单数',
  MODIFY COLUMN updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间';

-- =================== 表 3: op_douyin_ad_material_daily (抖音广告素材日数据 20 字段) ===================
ALTER TABLE op_douyin_ad_material_daily
  MODIFY COLUMN id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  MODIFY COLUMN stat_date DATE NOT NULL COMMENT '统计日期',
  MODIFY COLUMN shop_name VARCHAR(100) NOT NULL COMMENT '店铺名称',
  MODIFY COLUMN material_name VARCHAR(500) DEFAULT '' COMMENT '素材名称',
  MODIFY COLUMN material_id VARCHAR(100) DEFAULT '' COMMENT '素材ID',
  MODIFY COLUMN material_duration VARCHAR(50) DEFAULT '' COMMENT '素材时长',
  MODIFY COLUMN material_source VARCHAR(100) DEFAULT '' COMMENT '素材来源',
  MODIFY COLUMN net_roi DECIMAL(10,2) DEFAULT 0 COMMENT '净ROI(扣退款)',
  MODIFY COLUMN net_amount DECIMAL(14,2) DEFAULT 0 COMMENT '净成交金额',
  MODIFY COLUMN net_order_count INT DEFAULT 0 COMMENT '净订单数',
  MODIFY COLUMN refund_1h_rate DECIMAL(8,4) DEFAULT 0 COMMENT '1小时退款率',
  MODIFY COLUMN impressions INT DEFAULT 0 COMMENT '曝光数',
  MODIFY COLUMN clicks INT DEFAULT 0 COMMENT '点击数',
  MODIFY COLUMN click_rate DECIMAL(8,4) DEFAULT 0 COMMENT '点击率',
  MODIFY COLUMN conv_rate DECIMAL(8,4) DEFAULT 0 COMMENT '转化率',
  MODIFY COLUMN cost DECIMAL(14,2) DEFAULT 0 COMMENT '消耗金额',
  MODIFY COLUMN pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '支付金额',
  MODIFY COLUMN roi DECIMAL(10,2) DEFAULT 0 COMMENT 'ROI(成交金额/消耗)',
  MODIFY COLUMN cpm DECIMAL(10,2) DEFAULT 0 COMMENT '千次曝光成本',
  MODIFY COLUMN updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间';

-- =================== 表 4: op_douyin_dist_account_daily (抖音分销账户日数据 15 字段) ===================
ALTER TABLE op_douyin_dist_account_daily
  MODIFY COLUMN id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  MODIFY COLUMN stat_date DATE NOT NULL COMMENT '统计日期',
  MODIFY COLUMN douyin_name VARCHAR(200) DEFAULT '' COMMENT '抖音号昵称',
  MODIFY COLUMN douyin_id VARCHAR(100) DEFAULT '' COMMENT '抖音号ID',
  MODIFY COLUMN cost DECIMAL(14,2) DEFAULT 0 COMMENT '消耗金额',
  MODIFY COLUMN order_count INT DEFAULT 0 COMMENT '订单数',
  MODIFY COLUMN pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '支付金额',
  MODIFY COLUMN roi DECIMAL(10,2) DEFAULT 0 COMMENT 'ROI(成交金额/消耗)',
  MODIFY COLUMN order_cost DECIMAL(10,2) DEFAULT 0 COMMENT '单订单成本',
  MODIFY COLUMN user_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '用户实付金额',
  MODIFY COLUMN subsidy_amount DECIMAL(14,2) DEFAULT 0 COMMENT '补贴金额',
  MODIFY COLUMN net_roi DECIMAL(10,2) DEFAULT 0 COMMENT '净ROI(扣退款)',
  MODIFY COLUMN net_amount DECIMAL(14,2) DEFAULT 0 COMMENT '净成交金额',
  MODIFY COLUMN net_settle_rate DECIMAL(8,4) DEFAULT 0 COMMENT '净结算率',
  MODIFY COLUMN refund_1h_rate DECIMAL(8,4) DEFAULT 0 COMMENT '1小时退款率',
  MODIFY COLUMN updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间';

-- =================== 表 5: op_douyin_dist_material_daily (抖音分销素材日数据 22 字段) ===================
ALTER TABLE op_douyin_dist_material_daily
  MODIFY COLUMN id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  MODIFY COLUMN stat_date DATE NOT NULL COMMENT '统计日期',
  MODIFY COLUMN account_name VARCHAR(200) DEFAULT '' COMMENT '账号名称',
  MODIFY COLUMN material_id VARCHAR(100) DEFAULT '' COMMENT '素材ID',
  MODIFY COLUMN material_name VARCHAR(500) DEFAULT '' COMMENT '素材名称',
  MODIFY COLUMN impressions INT DEFAULT 0 COMMENT '曝光数',
  MODIFY COLUMN clicks INT DEFAULT 0 COMMENT '点击数',
  MODIFY COLUMN click_rate DECIMAL(8,4) DEFAULT 0 COMMENT '点击率',
  MODIFY COLUMN conv_rate DECIMAL(8,4) DEFAULT 0 COMMENT '转化率',
  MODIFY COLUMN cost DECIMAL(14,2) DEFAULT 0 COMMENT '消耗金额',
  MODIFY COLUMN order_count INT DEFAULT 0 COMMENT '订单数',
  MODIFY COLUMN pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '支付金额',
  MODIFY COLUMN roi DECIMAL(10,2) DEFAULT 0 COMMENT 'ROI(成交金额/消耗)',
  MODIFY COLUMN order_cost DECIMAL(10,2) DEFAULT 0 COMMENT '单订单成本',
  MODIFY COLUMN user_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '用户实付金额',
  MODIFY COLUMN cpm DECIMAL(10,2) DEFAULT 0 COMMENT '千次曝光成本',
  MODIFY COLUMN cpc DECIMAL(10,2) DEFAULT 0 COMMENT '单次点击成本',
  MODIFY COLUMN net_roi DECIMAL(10,2) DEFAULT 0 COMMENT '净ROI(扣退款)',
  MODIFY COLUMN net_amount DECIMAL(14,2) DEFAULT 0 COMMENT '净成交金额',
  MODIFY COLUMN net_order_count INT DEFAULT 0 COMMENT '净订单数',
  MODIFY COLUMN net_settle_rate DECIMAL(8,4) DEFAULT 0 COMMENT '净结算率',
  MODIFY COLUMN refund_1h_rate DECIMAL(8,4) DEFAULT 0 COMMENT '1小时退款率',
  MODIFY COLUMN updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间';

-- =================== 表 6: op_douyin_dist_product_daily (抖音分销商品日数据 16 字段) ===================
ALTER TABLE op_douyin_dist_product_daily
  MODIFY COLUMN id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  MODIFY COLUMN stat_date DATE NOT NULL COMMENT '统计日期',
  MODIFY COLUMN product_id VARCHAR(64) DEFAULT '' COMMENT '商品ID',
  MODIFY COLUMN product_name VARCHAR(255) DEFAULT '' COMMENT '商品名称',
  MODIFY COLUMN impressions INT DEFAULT 0 COMMENT '曝光数',
  MODIFY COLUMN clicks INT DEFAULT 0 COMMENT '点击数',
  MODIFY COLUMN click_rate DECIMAL(8,4) DEFAULT 0 COMMENT '点击率',
  MODIFY COLUMN conv_rate DECIMAL(8,4) DEFAULT 0 COMMENT '转化率',
  MODIFY COLUMN roi DECIMAL(10,2) DEFAULT 0 COMMENT 'ROI(成交金额/消耗)',
  MODIFY COLUMN order_cost DECIMAL(10,2) DEFAULT 0 COMMENT '单订单成本',
  MODIFY COLUMN user_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '用户实付金额',
  MODIFY COLUMN net_roi DECIMAL(10,2) DEFAULT 0 COMMENT '净ROI(扣退款)',
  MODIFY COLUMN net_amount DECIMAL(14,2) DEFAULT 0 COMMENT '净成交金额',
  MODIFY COLUMN net_order_cost DECIMAL(10,2) DEFAULT 0 COMMENT '净单订单成本',
  MODIFY COLUMN net_settle_rate DECIMAL(8,4) DEFAULT 0 COMMENT '净结算率',
  MODIFY COLUMN refund_1h_rate DECIMAL(8,4) DEFAULT 0 COMMENT '1小时退款率',
  MODIFY COLUMN updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间';

-- =================== 表 7: op_douyin_cs_feige_daily (抖音飞鸽客服日数据 26 字段) ===================
-- 客服 KPI 字段含义按 RPA 飞鸽工作台原始表头/平台通用术语写
ALTER TABLE op_douyin_cs_feige_daily
  MODIFY COLUMN id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  MODIFY COLUMN stat_date DATE NOT NULL COMMENT '统计日期',
  MODIFY COLUMN shop_name VARCHAR(128) NOT NULL COMMENT '店铺名称',
  MODIFY COLUMN online_cs_count INT DEFAULT 0 COMMENT '在线客服数',
  MODIFY COLUMN service_sessions INT DEFAULT 0 COMMENT '接待会话数',
  MODIFY COLUMN pending_reply_sessions INT DEFAULT 0 COMMENT '待回复会话数',
  MODIFY COLUMN received_users INT DEFAULT 0 COMMENT '接待人数',
  MODIFY COLUMN transfer_out_sessions INT DEFAULT 0 COMMENT '转接出会话数',
  MODIFY COLUMN valid_eval_count INT DEFAULT 0 COMMENT '有效评价数',
  MODIFY COLUMN valid_good_eval_count INT DEFAULT 0 COMMENT '有效好评数',
  MODIFY COLUMN valid_bad_eval_count INT DEFAULT 0 COMMENT '有效差评数',
  MODIFY COLUMN all_day_dissatisfaction_rate DECIMAL(10,4) DEFAULT 0 COMMENT '全天不满意率',
  MODIFY COLUMN all_day_satisfaction_rate DECIMAL(10,4) DEFAULT 0 COMMENT '全天满意率',
  MODIFY COLUMN all_day_avg_reply_seconds DECIMAL(10,4) DEFAULT 0 COMMENT '全天平均回复时长(秒)',
  MODIFY COLUMN all_day_first_reply_seconds DECIMAL(10,4) DEFAULT 0 COMMENT '全天首次回复时长(秒)',
  MODIFY COLUMN all_day_3min_reply_rate DECIMAL(10,4) DEFAULT 0 COMMENT '全天3分钟回复率',
  MODIFY COLUMN inquiry_users INT DEFAULT 0 COMMENT '询单人数',
  MODIFY COLUMN order_users INT DEFAULT 0 COMMENT '下单人数',
  MODIFY COLUMN pay_users INT DEFAULT 0 COMMENT '支付人数',
  MODIFY COLUMN refund_users INT DEFAULT 0 COMMENT '退款人数',
  MODIFY COLUMN inquiry_pay_amount DECIMAL(18,4) DEFAULT 0 COMMENT '询单支付金额',
  MODIFY COLUMN refund_amount DECIMAL(18,4) DEFAULT 0 COMMENT '退款金额',
  MODIFY COLUMN net_sales_amount DECIMAL(18,4) DEFAULT 0 COMMENT '净销售金额',
  MODIFY COLUMN inquiry_conv_rate DECIMAL(10,4) DEFAULT 0 COMMENT '询单转化率',
  MODIFY COLUMN raw_json JSON DEFAULT NULL COMMENT '原始数据JSON',
  MODIFY COLUMN updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间';

-- =================== 表 8: op_jd_cs_workload_daily (京东客服工作量日数据 26 字段) ===================
ALTER TABLE op_jd_cs_workload_daily
  MODIFY COLUMN id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  MODIFY COLUMN stat_date DATE NOT NULL COMMENT '统计日期',
  MODIFY COLUMN shop_name VARCHAR(128) NOT NULL COMMENT '店铺名称',
  MODIFY COLUMN on_duty_cs_count INT DEFAULT 0 COMMENT '在岗客服数',
  MODIFY COLUMN login_hours DECIMAL(10,4) DEFAULT 0 COMMENT '登录时长(小时)',
  MODIFY COLUMN shop_service_hours DECIMAL(10,4) DEFAULT 0 COMMENT '店铺服务时长(小时)',
  MODIFY COLUMN upv INT DEFAULT 0 COMMENT '咨询独立访客数',
  MODIFY COLUMN consult_count INT DEFAULT 0 COMMENT '咨询次数',
  MODIFY COLUMN receive_count INT DEFAULT 0 COMMENT '接待次数',
  MODIFY COLUMN connect_rate DECIMAL(10,4) DEFAULT 0 COMMENT '接通率',
  MODIFY COLUMN reply_rate DECIMAL(10,4) DEFAULT 0 COMMENT '回复率',
  MODIFY COLUMN resp_30s_rate DECIMAL(10,4) DEFAULT 0 COMMENT '30秒响应率',
  MODIFY COLUMN satisfaction_rate DECIMAL(10,4) DEFAULT 0 COMMENT '满意度',
  MODIFY COLUMN invite_eval_rate DECIMAL(10,4) DEFAULT 0 COMMENT '邀评率',
  MODIFY COLUMN avg_reply_msg_count DECIMAL(10,4) DEFAULT 0 COMMENT '平均回复消息数',
  MODIFY COLUMN timeout_reply_count INT DEFAULT 0 COMMENT '超时回复次数',
  MODIFY COLUMN avg_session_minutes DECIMAL(10,4) DEFAULT 0 COMMENT '平均会话时长(分钟)',
  MODIFY COLUMN message_consult_count INT DEFAULT 0 COMMENT '留言咨询数',
  MODIFY COLUMN message_assign_count INT DEFAULT 0 COMMENT '留言分配数',
  MODIFY COLUMN message_receive_count INT DEFAULT 0 COMMENT '留言接收数',
  MODIFY COLUMN message_reply_rate DECIMAL(10,4) DEFAULT 0 COMMENT '留言回复率',
  MODIFY COLUMN message_resp_rate DECIMAL(10,4) DEFAULT 0 COMMENT '留言响应率',
  MODIFY COLUMN merchant_message_rate DECIMAL(10,4) DEFAULT 0 COMMENT '商家留言率',
  MODIFY COLUMN resolve_rate DECIMAL(10,4) DEFAULT 0 COMMENT '解决率',
  MODIFY COLUMN updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间';

-- =================== 表 9: op_jd_cs_sales_perf_daily (京东客服销售业绩日数据 16 字段) ===================
ALTER TABLE op_jd_cs_sales_perf_daily
  MODIFY COLUMN id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  MODIFY COLUMN stat_date DATE NOT NULL COMMENT '统计日期',
  MODIFY COLUMN shop_name VARCHAR(128) NOT NULL COMMENT '店铺名称',
  MODIFY COLUMN on_duty_cs_count INT DEFAULT 0 COMMENT '在岗客服数',
  MODIFY COLUMN presale_receive_users INT DEFAULT 0 COMMENT '售前接待人数',
  MODIFY COLUMN order_users INT DEFAULT 0 COMMENT '下单人数',
  MODIFY COLUMN shipped_users INT DEFAULT 0 COMMENT '发货人数',
  MODIFY COLUMN order_count INT DEFAULT 0 COMMENT '订单数',
  MODIFY COLUMN shipped_order_count INT DEFAULT 0 COMMENT '发货订单数',
  MODIFY COLUMN order_goods_count INT DEFAULT 0 COMMENT '下单商品件数',
  MODIFY COLUMN shipped_goods_count INT DEFAULT 0 COMMENT '发货商品件数',
  MODIFY COLUMN order_goods_amount DECIMAL(18,4) DEFAULT 0 COMMENT '下单商品金额',
  MODIFY COLUMN shipped_goods_amount DECIMAL(18,4) DEFAULT 0 COMMENT '发货商品金额',
  MODIFY COLUMN consult_to_order_rate DECIMAL(10,4) DEFAULT 0 COMMENT '咨询下单转化率',
  MODIFY COLUMN consult_to_ship_rate DECIMAL(10,4) DEFAULT 0 COMMENT '咨询发货转化率',
  MODIFY COLUMN updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间';

-- =================== 表 10: op_kuaishou_cs_assessment_daily (快手客服考核日数据 22 字段) ===================
ALTER TABLE op_kuaishou_cs_assessment_daily
  MODIFY COLUMN id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  MODIFY COLUMN stat_date DATE NOT NULL COMMENT '统计日期',
  MODIFY COLUMN shop_name VARCHAR(128) NOT NULL COMMENT '店铺名称',
  MODIFY COLUMN reply_3min_rate_person DECIMAL(10,4) DEFAULT 0 COMMENT '3分钟回复率(按人)',
  MODIFY COLUMN reply_3min_rate_session DECIMAL(10,4) DEFAULT 0 COMMENT '3分钟回复率(按会话)',
  MODIFY COLUMN no_service_rate DECIMAL(10,4) DEFAULT 0 COMMENT '未接待率',
  MODIFY COLUMN consult_users INT DEFAULT 0 COMMENT '咨询人数',
  MODIFY COLUMN consult_times INT DEFAULT 0 COMMENT '咨询次数',
  MODIFY COLUMN manual_sessions INT DEFAULT 0 COMMENT '人工会话数',
  MODIFY COLUMN avg_reply_seconds DECIMAL(10,4) DEFAULT 0 COMMENT '平均回复时长(秒)',
  MODIFY COLUMN good_rate_person DECIMAL(10,4) DEFAULT 0 COMMENT '好评率(按人)',
  MODIFY COLUMN good_rate_session DECIMAL(10,4) DEFAULT 0 COMMENT '好评率(按会话)',
  MODIFY COLUMN bad_rate_person DECIMAL(10,4) DEFAULT 0 COMMENT '差评率(按人)',
  MODIFY COLUMN bad_rate_session DECIMAL(10,4) DEFAULT 0 COMMENT '差评率(按会话)',
  MODIFY COLUMN im_dissatisfaction_rate_person DECIMAL(10,4) DEFAULT 0 COMMENT 'IM不满意率(按人)',
  MODIFY COLUMN inquiry_users INT DEFAULT 0 COMMENT '询单人数',
  MODIFY COLUMN order_users INT DEFAULT 0 COMMENT '下单人数',
  MODIFY COLUMN pay_users INT DEFAULT 0 COMMENT '支付人数',
  MODIFY COLUMN inquiry_conv_rate DECIMAL(10,4) DEFAULT 0 COMMENT '询单转化率',
  MODIFY COLUMN inquiry_unit_price DECIMAL(18,4) DEFAULT 0 COMMENT '询单客单价',
  MODIFY COLUMN cs_sales_amount DECIMAL(18,4) DEFAULT 0 COMMENT '客服销售金额',
  MODIFY COLUMN updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间';

-- =================== 表 11: op_xhs_cs_analysis_daily (小红书客服分析日数据 22 字段) ===================
ALTER TABLE op_xhs_cs_analysis_daily
  MODIFY COLUMN id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  MODIFY COLUMN stat_date DATE NOT NULL COMMENT '统计日期',
  MODIFY COLUMN shop_name VARCHAR(128) NOT NULL COMMENT '店铺名称',
  MODIFY COLUMN case_count DECIMAL(18,4) DEFAULT 0 COMMENT '会话数',
  MODIFY COLUMN reply_case_count DECIMAL(18,4) DEFAULT 0 COMMENT '已回复会话数',
  MODIFY COLUMN reply_case_ratio DECIMAL(10,4) DEFAULT 0 COMMENT '会话回复率',
  MODIFY COLUMN avg_reply_duration_min DECIMAL(18,4) DEFAULT 0 COMMENT '平均回复时长(分钟)',
  MODIFY COLUMN inquiry_pay_case_ratio DECIMAL(10,4) DEFAULT 0 COMMENT '询单支付会话率',
  MODIFY COLUMN first_reply_45s_ratio DECIMAL(10,4) DEFAULT 0 COMMENT '首次45秒回复率',
  MODIFY COLUMN reply_in_3min_case_ratio DECIMAL(10,4) DEFAULT 0 COMMENT '3分钟回复会话率',
  MODIFY COLUMN pay_gmv DECIMAL(18,4) DEFAULT 0 COMMENT '支付GMV',
  MODIFY COLUMN inquiry_pay_gmv_ratio DECIMAL(10,4) DEFAULT 0 COMMENT '询单支付GMV占比',
  MODIFY COLUMN pay_pkg_count DECIMAL(18,4) DEFAULT 0 COMMENT '支付包裹数',
  MODIFY COLUMN inquiry_pay_pkg_count DECIMAL(18,4) DEFAULT 0 COMMENT '询单支付包裹数',
  MODIFY COLUMN positive_case_count DECIMAL(18,4) DEFAULT 0 COMMENT '正向会话数',
  MODIFY COLUMN positive_case_ratio DECIMAL(10,4) DEFAULT 0 COMMENT '正向会话占比',
  MODIFY COLUMN negative_case_count DECIMAL(18,4) DEFAULT 0 COMMENT '负向会话数',
  MODIFY COLUMN negative_case_ratio DECIMAL(10,4) DEFAULT 0 COMMENT '负向会话占比',
  MODIFY COLUMN evaluate_case_count DECIMAL(18,4) DEFAULT 0 COMMENT '评价会话数',
  MODIFY COLUMN evaluate_case_ratio DECIMAL(10,4) DEFAULT 0 COMMENT '评价会话占比',
  MODIFY COLUMN raw_json JSON DEFAULT NULL COMMENT '原始数据JSON',
  MODIFY COLUMN updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间';

-- 全量验证: 11 张表 blank-comment 字段应该都为 0
SELECT TABLE_NAME, COUNT(*) blank_field_count
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA='bi_dashboard'
  AND TABLE_NAME IN (
    'op_douyin_goods_daily','op_douyin_live_daily','op_douyin_ad_material_daily',
    'op_douyin_dist_account_daily','op_douyin_dist_material_daily','op_douyin_dist_product_daily',
    'op_douyin_cs_feige_daily','op_jd_cs_workload_daily','op_jd_cs_sales_perf_daily',
    'op_kuaishou_cs_assessment_daily','op_xhs_cs_analysis_daily'
  )
  AND (COLUMN_COMMENT IS NULL OR COLUMN_COMMENT='')
GROUP BY TABLE_NAME;
