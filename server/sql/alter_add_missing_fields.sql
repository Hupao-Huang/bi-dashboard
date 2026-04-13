-- ============================================================
-- 用途：为已有表补充缺失字段
-- 日期：2026-03-28
-- 说明：
--   1. trade 分表（202509~202603）及模板表新增56个字段
--   2. trade_goods 分表（202509~202603）及模板表新增4个字段
--   3. sales_goods_summary 表新增3个字段
--   4. stock_quantity 表新增6个字段
-- 注意：每条ALTER TABLE独立一句，方便排错
-- ============================================================

-- ============================================================
-- 第一部分：trade 表新增字段
-- 适用表：trade_template（模板）、trade_202509 ~ trade_202603
-- state, city, district, town, country, zip 已存在，不重复添加
-- ============================================================

-- ---------- trade_template ----------
ALTER TABLE trade_template ADD COLUMN customer_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户折让金额' AFTER flag_names;
ALTER TABLE trade_template ADD COLUMN customer_post_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户运费' AFTER customer_discount_fee;
ALTER TABLE trade_template ADD COLUMN customer_discount DECIMAL(8,4) DEFAULT NULL COMMENT '客户折扣' AFTER customer_post_fee;
ALTER TABLE trade_template ADD COLUMN customer_total_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户总费用' AFTER customer_discount;
ALTER TABLE trade_template ADD COLUMN customer_account VARCHAR(100) DEFAULT NULL COMMENT '客户账号' AFTER customer_total_fee;
ALTER TABLE trade_template ADD COLUMN customer_code VARCHAR(64) DEFAULT NULL COMMENT '客户编码' AFTER customer_account;
ALTER TABLE trade_template ADD COLUMN customer_grade_name VARCHAR(100) DEFAULT NULL COMMENT '客户等级' AFTER customer_code;
ALTER TABLE trade_template ADD COLUMN customer_tags VARCHAR(500) DEFAULT NULL COMMENT '客户标签' AFTER customer_grade_name;
ALTER TABLE trade_template ADD COLUMN buyer_open_uid VARCHAR(200) DEFAULT NULL COMMENT '买家平台唯一标识' AFTER customer_tags;
ALTER TABLE trade_template ADD COLUMN black_list INT DEFAULT NULL COMMENT '是否黑名单' AFTER buyer_open_uid;
ALTER TABLE trade_template ADD COLUMN invoice_amount DECIMAL(14,2) DEFAULT NULL COMMENT '发票金额' AFTER black_list;
ALTER TABLE trade_template ADD COLUMN invoice_type INT DEFAULT NULL COMMENT '发票类型' AFTER invoice_amount;
ALTER TABLE trade_template ADD COLUMN invoice_code VARCHAR(100) DEFAULT NULL COMMENT '发票编号' AFTER invoice_type;
ALTER TABLE trade_template ADD COLUMN charge_exchange_rate DECIMAL(10,4) DEFAULT NULL COMMENT '汇率' AFTER invoice_code;
ALTER TABLE trade_template ADD COLUMN charge_currency_code VARCHAR(20) DEFAULT NULL COMMENT '结算币种编码' AFTER charge_exchange_rate;
ALTER TABLE trade_template ADD COLUMN local_currency_code VARCHAR(20) DEFAULT NULL COMMENT '本地币种编码' AFTER charge_currency_code;
ALTER TABLE trade_template ADD COLUMN first_payment DECIMAL(14,2) DEFAULT NULL COMMENT '首款' AFTER local_currency_code;
ALTER TABLE trade_template ADD COLUMN final_payment DECIMAL(14,2) DEFAULT NULL COMMENT '尾款' AFTER first_payment;
ALTER TABLE trade_template ADD COLUMN received_total DECIMAL(14,2) DEFAULT NULL COMMENT '已收金额' AFTER final_payment;
ALTER TABLE trade_template ADD COLUMN first_paytime DATETIME DEFAULT NULL COMMENT '首次付款时间' AFTER received_total;
ALTER TABLE trade_template ADD COLUMN final_paytime DATETIME DEFAULT NULL COMMENT '最终付款时间' AFTER first_paytime;
ALTER TABLE trade_template ADD COLUMN fin_receipt_time DATETIME DEFAULT NULL COMMENT '财务收款时间' AFTER final_paytime;
ALTER TABLE trade_template ADD COLUMN payer_name VARCHAR(200) DEFAULT NULL COMMENT '付款方名称' AFTER fin_receipt_time;
ALTER TABLE trade_template ADD COLUMN payer_phone VARCHAR(50) DEFAULT NULL COMMENT '付款方电话' AFTER payer_name;
ALTER TABLE trade_template ADD COLUMN payer_regno VARCHAR(100) DEFAULT NULL COMMENT '付款方注册号' AFTER payer_phone;
ALTER TABLE trade_template ADD COLUMN payer_bank_account VARCHAR(100) DEFAULT NULL COMMENT '付款方银行账号' AFTER payer_regno;
ALTER TABLE trade_template ADD COLUMN payer_bank_name VARCHAR(200) DEFAULT NULL COMMENT '付款方银行名称' AFTER payer_bank_account;
ALTER TABLE trade_template ADD COLUMN logistic_code VARCHAR(50) DEFAULT NULL COMMENT '物流编码' AFTER payer_bank_name;
ALTER TABLE trade_template ADD COLUMN logistic_type INT DEFAULT NULL COMMENT '物流类型' AFTER logistic_code;
ALTER TABLE trade_template ADD COLUMN extra_logistic_no TEXT DEFAULT NULL COMMENT '额外物流单号JSON' AFTER logistic_type;
ALTER TABLE trade_template ADD COLUMN package_weight DECIMAL(14,2) DEFAULT NULL COMMENT '包裹重量' AFTER extra_logistic_no;
ALTER TABLE trade_template ADD COLUMN estimate_weight DECIMAL(14,2) DEFAULT NULL COMMENT '预估重量' AFTER package_weight;
ALTER TABLE trade_template ADD COLUMN estimate_volume DECIMAL(14,2) DEFAULT NULL COMMENT '预估体积' AFTER estimate_weight;
ALTER TABLE trade_template ADD COLUMN stockout_no VARCHAR(64) DEFAULT NULL COMMENT '出库单号' AFTER estimate_volume;
ALTER TABLE trade_template ADD COLUMN last_ship_time DATETIME DEFAULT NULL COMMENT '最后发货时间' AFTER stockout_no;
ALTER TABLE trade_template ADD COLUMN signing_time DATETIME DEFAULT NULL COMMENT '签收时间' AFTER last_ship_time;
ALTER TABLE trade_template ADD COLUMN review_time DATETIME DEFAULT NULL COMMENT '复核时间' AFTER signing_time;
ALTER TABLE trade_template ADD COLUMN confirm_time DATETIME DEFAULT NULL COMMENT '确认时间' AFTER review_time;
ALTER TABLE trade_template ADD COLUMN activation_time DATETIME DEFAULT NULL COMMENT '激活时间' AFTER confirm_time;
ALTER TABLE trade_template ADD COLUMN notify_pick_time DATETIME DEFAULT NULL COMMENT '通知拣货时间' AFTER activation_time;
ALTER TABLE trade_template ADD COLUMN settle_audit_time DATETIME DEFAULT NULL COMMENT '结算审核时间' AFTER notify_pick_time;
ALTER TABLE trade_template ADD COLUMN plat_complete_time DATETIME DEFAULT NULL COMMENT '平台完成时间' AFTER settle_audit_time;
ALTER TABLE trade_template ADD COLUMN reviewer VARCHAR(100) DEFAULT NULL COMMENT '复核人' AFTER plat_complete_time;
ALTER TABLE trade_template ADD COLUMN auditor VARCHAR(100) DEFAULT NULL COMMENT '审核人' AFTER reviewer;
ALTER TABLE trade_template ADD COLUMN register VARCHAR(100) DEFAULT NULL COMMENT '登记人' AFTER auditor;
ALTER TABLE trade_template ADD COLUMN seller VARCHAR(100) DEFAULT NULL COMMENT '业务员' AFTER register;
ALTER TABLE trade_template ADD COLUMN shop_type_code VARCHAR(50) DEFAULT NULL COMMENT '店铺平台编码' AFTER seller;
ALTER TABLE trade_template ADD COLUMN agent_shop_name VARCHAR(200) DEFAULT NULL COMMENT '代理店铺名' AFTER shop_type_code;
ALTER TABLE trade_template ADD COLUMN source_after_no VARCHAR(64) DEFAULT NULL COMMENT '售后单号' AFTER agent_shop_name;
ALTER TABLE trade_template ADD COLUMN country_code VARCHAR(20) DEFAULT NULL COMMENT '国家编码' AFTER source_after_no;
ALTER TABLE trade_template ADD COLUMN city_code VARCHAR(20) DEFAULT NULL COMMENT '城市编码' AFTER country_code;
ALTER TABLE trade_template ADD COLUMN sys_flag_ids VARCHAR(500) DEFAULT NULL COMMENT '系统标记ID' AFTER city_code;
ALTER TABLE trade_template ADD COLUMN special_reminding VARCHAR(500) DEFAULT NULL COMMENT '特殊提醒' AFTER sys_flag_ids;
ALTER TABLE trade_template ADD COLUMN abnormal_description VARCHAR(500) DEFAULT NULL COMMENT '异常描述' AFTER special_reminding;
ALTER TABLE trade_template ADD COLUMN append_memo TEXT DEFAULT NULL COMMENT '追加备注' AFTER abnormal_description;
ALTER TABLE trade_template ADD COLUMN ticket_code_list TEXT DEFAULT NULL COMMENT '提货码列表JSON' AFTER append_memo;
ALTER TABLE trade_template ADD COLUMN all_compass_source_content_type VARCHAR(500) DEFAULT NULL COMMENT '全域来源内容类型' AFTER ticket_code_list;

-- ---------- trade_202509 ----------
ALTER TABLE trade_202509 ADD COLUMN customer_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户折让金额' AFTER flag_names;
ALTER TABLE trade_202509 ADD COLUMN customer_post_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户运费' AFTER customer_discount_fee;
ALTER TABLE trade_202509 ADD COLUMN customer_discount DECIMAL(8,4) DEFAULT NULL COMMENT '客户折扣' AFTER customer_post_fee;
ALTER TABLE trade_202509 ADD COLUMN customer_total_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户总费用' AFTER customer_discount;
ALTER TABLE trade_202509 ADD COLUMN customer_account VARCHAR(100) DEFAULT NULL COMMENT '客户账号' AFTER customer_total_fee;
ALTER TABLE trade_202509 ADD COLUMN customer_code VARCHAR(64) DEFAULT NULL COMMENT '客户编码' AFTER customer_account;
ALTER TABLE trade_202509 ADD COLUMN customer_grade_name VARCHAR(100) DEFAULT NULL COMMENT '客户等级' AFTER customer_code;
ALTER TABLE trade_202509 ADD COLUMN customer_tags VARCHAR(500) DEFAULT NULL COMMENT '客户标签' AFTER customer_grade_name;
ALTER TABLE trade_202509 ADD COLUMN buyer_open_uid VARCHAR(200) DEFAULT NULL COMMENT '买家平台唯一标识' AFTER customer_tags;
ALTER TABLE trade_202509 ADD COLUMN black_list INT DEFAULT NULL COMMENT '是否黑名单' AFTER buyer_open_uid;
ALTER TABLE trade_202509 ADD COLUMN invoice_amount DECIMAL(14,2) DEFAULT NULL COMMENT '发票金额' AFTER black_list;
ALTER TABLE trade_202509 ADD COLUMN invoice_type INT DEFAULT NULL COMMENT '发票类型' AFTER invoice_amount;
ALTER TABLE trade_202509 ADD COLUMN invoice_code VARCHAR(100) DEFAULT NULL COMMENT '发票编号' AFTER invoice_type;
ALTER TABLE trade_202509 ADD COLUMN charge_exchange_rate DECIMAL(10,4) DEFAULT NULL COMMENT '汇率' AFTER invoice_code;
ALTER TABLE trade_202509 ADD COLUMN charge_currency_code VARCHAR(20) DEFAULT NULL COMMENT '结算币种编码' AFTER charge_exchange_rate;
ALTER TABLE trade_202509 ADD COLUMN local_currency_code VARCHAR(20) DEFAULT NULL COMMENT '本地币种编码' AFTER charge_currency_code;
ALTER TABLE trade_202509 ADD COLUMN first_payment DECIMAL(14,2) DEFAULT NULL COMMENT '首款' AFTER local_currency_code;
ALTER TABLE trade_202509 ADD COLUMN final_payment DECIMAL(14,2) DEFAULT NULL COMMENT '尾款' AFTER first_payment;
ALTER TABLE trade_202509 ADD COLUMN received_total DECIMAL(14,2) DEFAULT NULL COMMENT '已收金额' AFTER final_payment;
ALTER TABLE trade_202509 ADD COLUMN first_paytime DATETIME DEFAULT NULL COMMENT '首次付款时间' AFTER received_total;
ALTER TABLE trade_202509 ADD COLUMN final_paytime DATETIME DEFAULT NULL COMMENT '最终付款时间' AFTER first_paytime;
ALTER TABLE trade_202509 ADD COLUMN fin_receipt_time DATETIME DEFAULT NULL COMMENT '财务收款时间' AFTER final_paytime;
ALTER TABLE trade_202509 ADD COLUMN payer_name VARCHAR(200) DEFAULT NULL COMMENT '付款方名称' AFTER fin_receipt_time;
ALTER TABLE trade_202509 ADD COLUMN payer_phone VARCHAR(50) DEFAULT NULL COMMENT '付款方电话' AFTER payer_name;
ALTER TABLE trade_202509 ADD COLUMN payer_regno VARCHAR(100) DEFAULT NULL COMMENT '付款方注册号' AFTER payer_phone;
ALTER TABLE trade_202509 ADD COLUMN payer_bank_account VARCHAR(100) DEFAULT NULL COMMENT '付款方银行账号' AFTER payer_regno;
ALTER TABLE trade_202509 ADD COLUMN payer_bank_name VARCHAR(200) DEFAULT NULL COMMENT '付款方银行名称' AFTER payer_bank_account;
ALTER TABLE trade_202509 ADD COLUMN logistic_code VARCHAR(50) DEFAULT NULL COMMENT '物流编码' AFTER payer_bank_name;
ALTER TABLE trade_202509 ADD COLUMN logistic_type INT DEFAULT NULL COMMENT '物流类型' AFTER logistic_code;
ALTER TABLE trade_202509 ADD COLUMN extra_logistic_no TEXT DEFAULT NULL COMMENT '额外物流单号JSON' AFTER logistic_type;
ALTER TABLE trade_202509 ADD COLUMN package_weight DECIMAL(14,2) DEFAULT NULL COMMENT '包裹重量' AFTER extra_logistic_no;
ALTER TABLE trade_202509 ADD COLUMN estimate_weight DECIMAL(14,2) DEFAULT NULL COMMENT '预估重量' AFTER package_weight;
ALTER TABLE trade_202509 ADD COLUMN estimate_volume DECIMAL(14,2) DEFAULT NULL COMMENT '预估体积' AFTER estimate_weight;
ALTER TABLE trade_202509 ADD COLUMN stockout_no VARCHAR(64) DEFAULT NULL COMMENT '出库单号' AFTER estimate_volume;
ALTER TABLE trade_202509 ADD COLUMN last_ship_time DATETIME DEFAULT NULL COMMENT '最后发货时间' AFTER stockout_no;
ALTER TABLE trade_202509 ADD COLUMN signing_time DATETIME DEFAULT NULL COMMENT '签收时间' AFTER last_ship_time;
ALTER TABLE trade_202509 ADD COLUMN review_time DATETIME DEFAULT NULL COMMENT '复核时间' AFTER signing_time;
ALTER TABLE trade_202509 ADD COLUMN confirm_time DATETIME DEFAULT NULL COMMENT '确认时间' AFTER review_time;
ALTER TABLE trade_202509 ADD COLUMN activation_time DATETIME DEFAULT NULL COMMENT '激活时间' AFTER confirm_time;
ALTER TABLE trade_202509 ADD COLUMN notify_pick_time DATETIME DEFAULT NULL COMMENT '通知拣货时间' AFTER activation_time;
ALTER TABLE trade_202509 ADD COLUMN settle_audit_time DATETIME DEFAULT NULL COMMENT '结算审核时间' AFTER notify_pick_time;
ALTER TABLE trade_202509 ADD COLUMN plat_complete_time DATETIME DEFAULT NULL COMMENT '平台完成时间' AFTER settle_audit_time;
ALTER TABLE trade_202509 ADD COLUMN reviewer VARCHAR(100) DEFAULT NULL COMMENT '复核人' AFTER plat_complete_time;
ALTER TABLE trade_202509 ADD COLUMN auditor VARCHAR(100) DEFAULT NULL COMMENT '审核人' AFTER reviewer;
ALTER TABLE trade_202509 ADD COLUMN register VARCHAR(100) DEFAULT NULL COMMENT '登记人' AFTER auditor;
ALTER TABLE trade_202509 ADD COLUMN seller VARCHAR(100) DEFAULT NULL COMMENT '业务员' AFTER register;
ALTER TABLE trade_202509 ADD COLUMN shop_type_code VARCHAR(50) DEFAULT NULL COMMENT '店铺平台编码' AFTER seller;
ALTER TABLE trade_202509 ADD COLUMN agent_shop_name VARCHAR(200) DEFAULT NULL COMMENT '代理店铺名' AFTER shop_type_code;
ALTER TABLE trade_202509 ADD COLUMN source_after_no VARCHAR(64) DEFAULT NULL COMMENT '售后单号' AFTER agent_shop_name;
ALTER TABLE trade_202509 ADD COLUMN country_code VARCHAR(20) DEFAULT NULL COMMENT '国家编码' AFTER source_after_no;
ALTER TABLE trade_202509 ADD COLUMN city_code VARCHAR(20) DEFAULT NULL COMMENT '城市编码' AFTER country_code;
ALTER TABLE trade_202509 ADD COLUMN sys_flag_ids VARCHAR(500) DEFAULT NULL COMMENT '系统标记ID' AFTER city_code;
ALTER TABLE trade_202509 ADD COLUMN special_reminding VARCHAR(500) DEFAULT NULL COMMENT '特殊提醒' AFTER sys_flag_ids;
ALTER TABLE trade_202509 ADD COLUMN abnormal_description VARCHAR(500) DEFAULT NULL COMMENT '异常描述' AFTER special_reminding;
ALTER TABLE trade_202509 ADD COLUMN append_memo TEXT DEFAULT NULL COMMENT '追加备注' AFTER abnormal_description;
ALTER TABLE trade_202509 ADD COLUMN ticket_code_list TEXT DEFAULT NULL COMMENT '提货码列表JSON' AFTER append_memo;
ALTER TABLE trade_202509 ADD COLUMN all_compass_source_content_type VARCHAR(500) DEFAULT NULL COMMENT '全域来源内容类型' AFTER ticket_code_list;

-- ---------- trade_202510 ----------
ALTER TABLE trade_202510 ADD COLUMN customer_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户折让金额' AFTER flag_names;
ALTER TABLE trade_202510 ADD COLUMN customer_post_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户运费' AFTER customer_discount_fee;
ALTER TABLE trade_202510 ADD COLUMN customer_discount DECIMAL(8,4) DEFAULT NULL COMMENT '客户折扣' AFTER customer_post_fee;
ALTER TABLE trade_202510 ADD COLUMN customer_total_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户总费用' AFTER customer_discount;
ALTER TABLE trade_202510 ADD COLUMN customer_account VARCHAR(100) DEFAULT NULL COMMENT '客户账号' AFTER customer_total_fee;
ALTER TABLE trade_202510 ADD COLUMN customer_code VARCHAR(64) DEFAULT NULL COMMENT '客户编码' AFTER customer_account;
ALTER TABLE trade_202510 ADD COLUMN customer_grade_name VARCHAR(100) DEFAULT NULL COMMENT '客户等级' AFTER customer_code;
ALTER TABLE trade_202510 ADD COLUMN customer_tags VARCHAR(500) DEFAULT NULL COMMENT '客户标签' AFTER customer_grade_name;
ALTER TABLE trade_202510 ADD COLUMN buyer_open_uid VARCHAR(200) DEFAULT NULL COMMENT '买家平台唯一标识' AFTER customer_tags;
ALTER TABLE trade_202510 ADD COLUMN black_list INT DEFAULT NULL COMMENT '是否黑名单' AFTER buyer_open_uid;
ALTER TABLE trade_202510 ADD COLUMN invoice_amount DECIMAL(14,2) DEFAULT NULL COMMENT '发票金额' AFTER black_list;
ALTER TABLE trade_202510 ADD COLUMN invoice_type INT DEFAULT NULL COMMENT '发票类型' AFTER invoice_amount;
ALTER TABLE trade_202510 ADD COLUMN invoice_code VARCHAR(100) DEFAULT NULL COMMENT '发票编号' AFTER invoice_type;
ALTER TABLE trade_202510 ADD COLUMN charge_exchange_rate DECIMAL(10,4) DEFAULT NULL COMMENT '汇率' AFTER invoice_code;
ALTER TABLE trade_202510 ADD COLUMN charge_currency_code VARCHAR(20) DEFAULT NULL COMMENT '结算币种编码' AFTER charge_exchange_rate;
ALTER TABLE trade_202510 ADD COLUMN local_currency_code VARCHAR(20) DEFAULT NULL COMMENT '本地币种编码' AFTER charge_currency_code;
ALTER TABLE trade_202510 ADD COLUMN first_payment DECIMAL(14,2) DEFAULT NULL COMMENT '首款' AFTER local_currency_code;
ALTER TABLE trade_202510 ADD COLUMN final_payment DECIMAL(14,2) DEFAULT NULL COMMENT '尾款' AFTER first_payment;
ALTER TABLE trade_202510 ADD COLUMN received_total DECIMAL(14,2) DEFAULT NULL COMMENT '已收金额' AFTER final_payment;
ALTER TABLE trade_202510 ADD COLUMN first_paytime DATETIME DEFAULT NULL COMMENT '首次付款时间' AFTER received_total;
ALTER TABLE trade_202510 ADD COLUMN final_paytime DATETIME DEFAULT NULL COMMENT '最终付款时间' AFTER first_paytime;
ALTER TABLE trade_202510 ADD COLUMN fin_receipt_time DATETIME DEFAULT NULL COMMENT '财务收款时间' AFTER final_paytime;
ALTER TABLE trade_202510 ADD COLUMN payer_name VARCHAR(200) DEFAULT NULL COMMENT '付款方名称' AFTER fin_receipt_time;
ALTER TABLE trade_202510 ADD COLUMN payer_phone VARCHAR(50) DEFAULT NULL COMMENT '付款方电话' AFTER payer_name;
ALTER TABLE trade_202510 ADD COLUMN payer_regno VARCHAR(100) DEFAULT NULL COMMENT '付款方注册号' AFTER payer_phone;
ALTER TABLE trade_202510 ADD COLUMN payer_bank_account VARCHAR(100) DEFAULT NULL COMMENT '付款方银行账号' AFTER payer_regno;
ALTER TABLE trade_202510 ADD COLUMN payer_bank_name VARCHAR(200) DEFAULT NULL COMMENT '付款方银行名称' AFTER payer_bank_account;
ALTER TABLE trade_202510 ADD COLUMN logistic_code VARCHAR(50) DEFAULT NULL COMMENT '物流编码' AFTER payer_bank_name;
ALTER TABLE trade_202510 ADD COLUMN logistic_type INT DEFAULT NULL COMMENT '物流类型' AFTER logistic_code;
ALTER TABLE trade_202510 ADD COLUMN extra_logistic_no TEXT DEFAULT NULL COMMENT '额外物流单号JSON' AFTER logistic_type;
ALTER TABLE trade_202510 ADD COLUMN package_weight DECIMAL(14,2) DEFAULT NULL COMMENT '包裹重量' AFTER extra_logistic_no;
ALTER TABLE trade_202510 ADD COLUMN estimate_weight DECIMAL(14,2) DEFAULT NULL COMMENT '预估重量' AFTER package_weight;
ALTER TABLE trade_202510 ADD COLUMN estimate_volume DECIMAL(14,2) DEFAULT NULL COMMENT '预估体积' AFTER estimate_weight;
ALTER TABLE trade_202510 ADD COLUMN stockout_no VARCHAR(64) DEFAULT NULL COMMENT '出库单号' AFTER estimate_volume;
ALTER TABLE trade_202510 ADD COLUMN last_ship_time DATETIME DEFAULT NULL COMMENT '最后发货时间' AFTER stockout_no;
ALTER TABLE trade_202510 ADD COLUMN signing_time DATETIME DEFAULT NULL COMMENT '签收时间' AFTER last_ship_time;
ALTER TABLE trade_202510 ADD COLUMN review_time DATETIME DEFAULT NULL COMMENT '复核时间' AFTER signing_time;
ALTER TABLE trade_202510 ADD COLUMN confirm_time DATETIME DEFAULT NULL COMMENT '确认时间' AFTER review_time;
ALTER TABLE trade_202510 ADD COLUMN activation_time DATETIME DEFAULT NULL COMMENT '激活时间' AFTER confirm_time;
ALTER TABLE trade_202510 ADD COLUMN notify_pick_time DATETIME DEFAULT NULL COMMENT '通知拣货时间' AFTER activation_time;
ALTER TABLE trade_202510 ADD COLUMN settle_audit_time DATETIME DEFAULT NULL COMMENT '结算审核时间' AFTER notify_pick_time;
ALTER TABLE trade_202510 ADD COLUMN plat_complete_time DATETIME DEFAULT NULL COMMENT '平台完成时间' AFTER settle_audit_time;
ALTER TABLE trade_202510 ADD COLUMN reviewer VARCHAR(100) DEFAULT NULL COMMENT '复核人' AFTER plat_complete_time;
ALTER TABLE trade_202510 ADD COLUMN auditor VARCHAR(100) DEFAULT NULL COMMENT '审核人' AFTER reviewer;
ALTER TABLE trade_202510 ADD COLUMN register VARCHAR(100) DEFAULT NULL COMMENT '登记人' AFTER auditor;
ALTER TABLE trade_202510 ADD COLUMN seller VARCHAR(100) DEFAULT NULL COMMENT '业务员' AFTER register;
ALTER TABLE trade_202510 ADD COLUMN shop_type_code VARCHAR(50) DEFAULT NULL COMMENT '店铺平台编码' AFTER seller;
ALTER TABLE trade_202510 ADD COLUMN agent_shop_name VARCHAR(200) DEFAULT NULL COMMENT '代理店铺名' AFTER shop_type_code;
ALTER TABLE trade_202510 ADD COLUMN source_after_no VARCHAR(64) DEFAULT NULL COMMENT '售后单号' AFTER agent_shop_name;
ALTER TABLE trade_202510 ADD COLUMN country_code VARCHAR(20) DEFAULT NULL COMMENT '国家编码' AFTER source_after_no;
ALTER TABLE trade_202510 ADD COLUMN city_code VARCHAR(20) DEFAULT NULL COMMENT '城市编码' AFTER country_code;
ALTER TABLE trade_202510 ADD COLUMN sys_flag_ids VARCHAR(500) DEFAULT NULL COMMENT '系统标记ID' AFTER city_code;
ALTER TABLE trade_202510 ADD COLUMN special_reminding VARCHAR(500) DEFAULT NULL COMMENT '特殊提醒' AFTER sys_flag_ids;
ALTER TABLE trade_202510 ADD COLUMN abnormal_description VARCHAR(500) DEFAULT NULL COMMENT '异常描述' AFTER special_reminding;
ALTER TABLE trade_202510 ADD COLUMN append_memo TEXT DEFAULT NULL COMMENT '追加备注' AFTER abnormal_description;
ALTER TABLE trade_202510 ADD COLUMN ticket_code_list TEXT DEFAULT NULL COMMENT '提货码列表JSON' AFTER append_memo;
ALTER TABLE trade_202510 ADD COLUMN all_compass_source_content_type VARCHAR(500) DEFAULT NULL COMMENT '全域来源内容类型' AFTER ticket_code_list;

-- ---------- trade_202511 ----------
ALTER TABLE trade_202511 ADD COLUMN customer_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户折让金额' AFTER flag_names;
ALTER TABLE trade_202511 ADD COLUMN customer_post_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户运费' AFTER customer_discount_fee;
ALTER TABLE trade_202511 ADD COLUMN customer_discount DECIMAL(8,4) DEFAULT NULL COMMENT '客户折扣' AFTER customer_post_fee;
ALTER TABLE trade_202511 ADD COLUMN customer_total_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户总费用' AFTER customer_discount;
ALTER TABLE trade_202511 ADD COLUMN customer_account VARCHAR(100) DEFAULT NULL COMMENT '客户账号' AFTER customer_total_fee;
ALTER TABLE trade_202511 ADD COLUMN customer_code VARCHAR(64) DEFAULT NULL COMMENT '客户编码' AFTER customer_account;
ALTER TABLE trade_202511 ADD COLUMN customer_grade_name VARCHAR(100) DEFAULT NULL COMMENT '客户等级' AFTER customer_code;
ALTER TABLE trade_202511 ADD COLUMN customer_tags VARCHAR(500) DEFAULT NULL COMMENT '客户标签' AFTER customer_grade_name;
ALTER TABLE trade_202511 ADD COLUMN buyer_open_uid VARCHAR(200) DEFAULT NULL COMMENT '买家平台唯一标识' AFTER customer_tags;
ALTER TABLE trade_202511 ADD COLUMN black_list INT DEFAULT NULL COMMENT '是否黑名单' AFTER buyer_open_uid;
ALTER TABLE trade_202511 ADD COLUMN invoice_amount DECIMAL(14,2) DEFAULT NULL COMMENT '发票金额' AFTER black_list;
ALTER TABLE trade_202511 ADD COLUMN invoice_type INT DEFAULT NULL COMMENT '发票类型' AFTER invoice_amount;
ALTER TABLE trade_202511 ADD COLUMN invoice_code VARCHAR(100) DEFAULT NULL COMMENT '发票编号' AFTER invoice_type;
ALTER TABLE trade_202511 ADD COLUMN charge_exchange_rate DECIMAL(10,4) DEFAULT NULL COMMENT '汇率' AFTER invoice_code;
ALTER TABLE trade_202511 ADD COLUMN charge_currency_code VARCHAR(20) DEFAULT NULL COMMENT '结算币种编码' AFTER charge_exchange_rate;
ALTER TABLE trade_202511 ADD COLUMN local_currency_code VARCHAR(20) DEFAULT NULL COMMENT '本地币种编码' AFTER charge_currency_code;
ALTER TABLE trade_202511 ADD COLUMN first_payment DECIMAL(14,2) DEFAULT NULL COMMENT '首款' AFTER local_currency_code;
ALTER TABLE trade_202511 ADD COLUMN final_payment DECIMAL(14,2) DEFAULT NULL COMMENT '尾款' AFTER first_payment;
ALTER TABLE trade_202511 ADD COLUMN received_total DECIMAL(14,2) DEFAULT NULL COMMENT '已收金额' AFTER final_payment;
ALTER TABLE trade_202511 ADD COLUMN first_paytime DATETIME DEFAULT NULL COMMENT '首次付款时间' AFTER received_total;
ALTER TABLE trade_202511 ADD COLUMN final_paytime DATETIME DEFAULT NULL COMMENT '最终付款时间' AFTER first_paytime;
ALTER TABLE trade_202511 ADD COLUMN fin_receipt_time DATETIME DEFAULT NULL COMMENT '财务收款时间' AFTER final_paytime;
ALTER TABLE trade_202511 ADD COLUMN payer_name VARCHAR(200) DEFAULT NULL COMMENT '付款方名称' AFTER fin_receipt_time;
ALTER TABLE trade_202511 ADD COLUMN payer_phone VARCHAR(50) DEFAULT NULL COMMENT '付款方电话' AFTER payer_name;
ALTER TABLE trade_202511 ADD COLUMN payer_regno VARCHAR(100) DEFAULT NULL COMMENT '付款方注册号' AFTER payer_phone;
ALTER TABLE trade_202511 ADD COLUMN payer_bank_account VARCHAR(100) DEFAULT NULL COMMENT '付款方银行账号' AFTER payer_regno;
ALTER TABLE trade_202511 ADD COLUMN payer_bank_name VARCHAR(200) DEFAULT NULL COMMENT '付款方银行名称' AFTER payer_bank_account;
ALTER TABLE trade_202511 ADD COLUMN logistic_code VARCHAR(50) DEFAULT NULL COMMENT '物流编码' AFTER payer_bank_name;
ALTER TABLE trade_202511 ADD COLUMN logistic_type INT DEFAULT NULL COMMENT '物流类型' AFTER logistic_code;
ALTER TABLE trade_202511 ADD COLUMN extra_logistic_no TEXT DEFAULT NULL COMMENT '额外物流单号JSON' AFTER logistic_type;
ALTER TABLE trade_202511 ADD COLUMN package_weight DECIMAL(14,2) DEFAULT NULL COMMENT '包裹重量' AFTER extra_logistic_no;
ALTER TABLE trade_202511 ADD COLUMN estimate_weight DECIMAL(14,2) DEFAULT NULL COMMENT '预估重量' AFTER package_weight;
ALTER TABLE trade_202511 ADD COLUMN estimate_volume DECIMAL(14,2) DEFAULT NULL COMMENT '预估体积' AFTER estimate_weight;
ALTER TABLE trade_202511 ADD COLUMN stockout_no VARCHAR(64) DEFAULT NULL COMMENT '出库单号' AFTER estimate_volume;
ALTER TABLE trade_202511 ADD COLUMN last_ship_time DATETIME DEFAULT NULL COMMENT '最后发货时间' AFTER stockout_no;
ALTER TABLE trade_202511 ADD COLUMN signing_time DATETIME DEFAULT NULL COMMENT '签收时间' AFTER last_ship_time;
ALTER TABLE trade_202511 ADD COLUMN review_time DATETIME DEFAULT NULL COMMENT '复核时间' AFTER signing_time;
ALTER TABLE trade_202511 ADD COLUMN confirm_time DATETIME DEFAULT NULL COMMENT '确认时间' AFTER review_time;
ALTER TABLE trade_202511 ADD COLUMN activation_time DATETIME DEFAULT NULL COMMENT '激活时间' AFTER confirm_time;
ALTER TABLE trade_202511 ADD COLUMN notify_pick_time DATETIME DEFAULT NULL COMMENT '通知拣货时间' AFTER activation_time;
ALTER TABLE trade_202511 ADD COLUMN settle_audit_time DATETIME DEFAULT NULL COMMENT '结算审核时间' AFTER notify_pick_time;
ALTER TABLE trade_202511 ADD COLUMN plat_complete_time DATETIME DEFAULT NULL COMMENT '平台完成时间' AFTER settle_audit_time;
ALTER TABLE trade_202511 ADD COLUMN reviewer VARCHAR(100) DEFAULT NULL COMMENT '复核人' AFTER plat_complete_time;
ALTER TABLE trade_202511 ADD COLUMN auditor VARCHAR(100) DEFAULT NULL COMMENT '审核人' AFTER reviewer;
ALTER TABLE trade_202511 ADD COLUMN register VARCHAR(100) DEFAULT NULL COMMENT '登记人' AFTER auditor;
ALTER TABLE trade_202511 ADD COLUMN seller VARCHAR(100) DEFAULT NULL COMMENT '业务员' AFTER register;
ALTER TABLE trade_202511 ADD COLUMN shop_type_code VARCHAR(50) DEFAULT NULL COMMENT '店铺平台编码' AFTER seller;
ALTER TABLE trade_202511 ADD COLUMN agent_shop_name VARCHAR(200) DEFAULT NULL COMMENT '代理店铺名' AFTER shop_type_code;
ALTER TABLE trade_202511 ADD COLUMN source_after_no VARCHAR(64) DEFAULT NULL COMMENT '售后单号' AFTER agent_shop_name;
ALTER TABLE trade_202511 ADD COLUMN country_code VARCHAR(20) DEFAULT NULL COMMENT '国家编码' AFTER source_after_no;
ALTER TABLE trade_202511 ADD COLUMN city_code VARCHAR(20) DEFAULT NULL COMMENT '城市编码' AFTER country_code;
ALTER TABLE trade_202511 ADD COLUMN sys_flag_ids VARCHAR(500) DEFAULT NULL COMMENT '系统标记ID' AFTER city_code;
ALTER TABLE trade_202511 ADD COLUMN special_reminding VARCHAR(500) DEFAULT NULL COMMENT '特殊提醒' AFTER sys_flag_ids;
ALTER TABLE trade_202511 ADD COLUMN abnormal_description VARCHAR(500) DEFAULT NULL COMMENT '异常描述' AFTER special_reminding;
ALTER TABLE trade_202511 ADD COLUMN append_memo TEXT DEFAULT NULL COMMENT '追加备注' AFTER abnormal_description;
ALTER TABLE trade_202511 ADD COLUMN ticket_code_list TEXT DEFAULT NULL COMMENT '提货码列表JSON' AFTER append_memo;
ALTER TABLE trade_202511 ADD COLUMN all_compass_source_content_type VARCHAR(500) DEFAULT NULL COMMENT '全域来源内容类型' AFTER ticket_code_list;

-- ---------- trade_202512 ----------
ALTER TABLE trade_202512 ADD COLUMN customer_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户折让金额' AFTER flag_names;
ALTER TABLE trade_202512 ADD COLUMN customer_post_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户运费' AFTER customer_discount_fee;
ALTER TABLE trade_202512 ADD COLUMN customer_discount DECIMAL(8,4) DEFAULT NULL COMMENT '客户折扣' AFTER customer_post_fee;
ALTER TABLE trade_202512 ADD COLUMN customer_total_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户总费用' AFTER customer_discount;
ALTER TABLE trade_202512 ADD COLUMN customer_account VARCHAR(100) DEFAULT NULL COMMENT '客户账号' AFTER customer_total_fee;
ALTER TABLE trade_202512 ADD COLUMN customer_code VARCHAR(64) DEFAULT NULL COMMENT '客户编码' AFTER customer_account;
ALTER TABLE trade_202512 ADD COLUMN customer_grade_name VARCHAR(100) DEFAULT NULL COMMENT '客户等级' AFTER customer_code;
ALTER TABLE trade_202512 ADD COLUMN customer_tags VARCHAR(500) DEFAULT NULL COMMENT '客户标签' AFTER customer_grade_name;
ALTER TABLE trade_202512 ADD COLUMN buyer_open_uid VARCHAR(200) DEFAULT NULL COMMENT '买家平台唯一标识' AFTER customer_tags;
ALTER TABLE trade_202512 ADD COLUMN black_list INT DEFAULT NULL COMMENT '是否黑名单' AFTER buyer_open_uid;
ALTER TABLE trade_202512 ADD COLUMN invoice_amount DECIMAL(14,2) DEFAULT NULL COMMENT '发票金额' AFTER black_list;
ALTER TABLE trade_202512 ADD COLUMN invoice_type INT DEFAULT NULL COMMENT '发票类型' AFTER invoice_amount;
ALTER TABLE trade_202512 ADD COLUMN invoice_code VARCHAR(100) DEFAULT NULL COMMENT '发票编号' AFTER invoice_type;
ALTER TABLE trade_202512 ADD COLUMN charge_exchange_rate DECIMAL(10,4) DEFAULT NULL COMMENT '汇率' AFTER invoice_code;
ALTER TABLE trade_202512 ADD COLUMN charge_currency_code VARCHAR(20) DEFAULT NULL COMMENT '结算币种编码' AFTER charge_exchange_rate;
ALTER TABLE trade_202512 ADD COLUMN local_currency_code VARCHAR(20) DEFAULT NULL COMMENT '本地币种编码' AFTER charge_currency_code;
ALTER TABLE trade_202512 ADD COLUMN first_payment DECIMAL(14,2) DEFAULT NULL COMMENT '首款' AFTER local_currency_code;
ALTER TABLE trade_202512 ADD COLUMN final_payment DECIMAL(14,2) DEFAULT NULL COMMENT '尾款' AFTER first_payment;
ALTER TABLE trade_202512 ADD COLUMN received_total DECIMAL(14,2) DEFAULT NULL COMMENT '已收金额' AFTER final_payment;
ALTER TABLE trade_202512 ADD COLUMN first_paytime DATETIME DEFAULT NULL COMMENT '首次付款时间' AFTER received_total;
ALTER TABLE trade_202512 ADD COLUMN final_paytime DATETIME DEFAULT NULL COMMENT '最终付款时间' AFTER first_paytime;
ALTER TABLE trade_202512 ADD COLUMN fin_receipt_time DATETIME DEFAULT NULL COMMENT '财务收款时间' AFTER final_paytime;
ALTER TABLE trade_202512 ADD COLUMN payer_name VARCHAR(200) DEFAULT NULL COMMENT '付款方名称' AFTER fin_receipt_time;
ALTER TABLE trade_202512 ADD COLUMN payer_phone VARCHAR(50) DEFAULT NULL COMMENT '付款方电话' AFTER payer_name;
ALTER TABLE trade_202512 ADD COLUMN payer_regno VARCHAR(100) DEFAULT NULL COMMENT '付款方注册号' AFTER payer_phone;
ALTER TABLE trade_202512 ADD COLUMN payer_bank_account VARCHAR(100) DEFAULT NULL COMMENT '付款方银行账号' AFTER payer_regno;
ALTER TABLE trade_202512 ADD COLUMN payer_bank_name VARCHAR(200) DEFAULT NULL COMMENT '付款方银行名称' AFTER payer_bank_account;
ALTER TABLE trade_202512 ADD COLUMN logistic_code VARCHAR(50) DEFAULT NULL COMMENT '物流编码' AFTER payer_bank_name;
ALTER TABLE trade_202512 ADD COLUMN logistic_type INT DEFAULT NULL COMMENT '物流类型' AFTER logistic_code;
ALTER TABLE trade_202512 ADD COLUMN extra_logistic_no TEXT DEFAULT NULL COMMENT '额外物流单号JSON' AFTER logistic_type;
ALTER TABLE trade_202512 ADD COLUMN package_weight DECIMAL(14,2) DEFAULT NULL COMMENT '包裹重量' AFTER extra_logistic_no;
ALTER TABLE trade_202512 ADD COLUMN estimate_weight DECIMAL(14,2) DEFAULT NULL COMMENT '预估重量' AFTER package_weight;
ALTER TABLE trade_202512 ADD COLUMN estimate_volume DECIMAL(14,2) DEFAULT NULL COMMENT '预估体积' AFTER estimate_weight;
ALTER TABLE trade_202512 ADD COLUMN stockout_no VARCHAR(64) DEFAULT NULL COMMENT '出库单号' AFTER estimate_volume;
ALTER TABLE trade_202512 ADD COLUMN last_ship_time DATETIME DEFAULT NULL COMMENT '最后发货时间' AFTER stockout_no;
ALTER TABLE trade_202512 ADD COLUMN signing_time DATETIME DEFAULT NULL COMMENT '签收时间' AFTER last_ship_time;
ALTER TABLE trade_202512 ADD COLUMN review_time DATETIME DEFAULT NULL COMMENT '复核时间' AFTER signing_time;
ALTER TABLE trade_202512 ADD COLUMN confirm_time DATETIME DEFAULT NULL COMMENT '确认时间' AFTER review_time;
ALTER TABLE trade_202512 ADD COLUMN activation_time DATETIME DEFAULT NULL COMMENT '激活时间' AFTER confirm_time;
ALTER TABLE trade_202512 ADD COLUMN notify_pick_time DATETIME DEFAULT NULL COMMENT '通知拣货时间' AFTER activation_time;
ALTER TABLE trade_202512 ADD COLUMN settle_audit_time DATETIME DEFAULT NULL COMMENT '结算审核时间' AFTER notify_pick_time;
ALTER TABLE trade_202512 ADD COLUMN plat_complete_time DATETIME DEFAULT NULL COMMENT '平台完成时间' AFTER settle_audit_time;
ALTER TABLE trade_202512 ADD COLUMN reviewer VARCHAR(100) DEFAULT NULL COMMENT '复核人' AFTER plat_complete_time;
ALTER TABLE trade_202512 ADD COLUMN auditor VARCHAR(100) DEFAULT NULL COMMENT '审核人' AFTER reviewer;
ALTER TABLE trade_202512 ADD COLUMN register VARCHAR(100) DEFAULT NULL COMMENT '登记人' AFTER auditor;
ALTER TABLE trade_202512 ADD COLUMN seller VARCHAR(100) DEFAULT NULL COMMENT '业务员' AFTER register;
ALTER TABLE trade_202512 ADD COLUMN shop_type_code VARCHAR(50) DEFAULT NULL COMMENT '店铺平台编码' AFTER seller;
ALTER TABLE trade_202512 ADD COLUMN agent_shop_name VARCHAR(200) DEFAULT NULL COMMENT '代理店铺名' AFTER shop_type_code;
ALTER TABLE trade_202512 ADD COLUMN source_after_no VARCHAR(64) DEFAULT NULL COMMENT '售后单号' AFTER agent_shop_name;
ALTER TABLE trade_202512 ADD COLUMN country_code VARCHAR(20) DEFAULT NULL COMMENT '国家编码' AFTER source_after_no;
ALTER TABLE trade_202512 ADD COLUMN city_code VARCHAR(20) DEFAULT NULL COMMENT '城市编码' AFTER country_code;
ALTER TABLE trade_202512 ADD COLUMN sys_flag_ids VARCHAR(500) DEFAULT NULL COMMENT '系统标记ID' AFTER city_code;
ALTER TABLE trade_202512 ADD COLUMN special_reminding VARCHAR(500) DEFAULT NULL COMMENT '特殊提醒' AFTER sys_flag_ids;
ALTER TABLE trade_202512 ADD COLUMN abnormal_description VARCHAR(500) DEFAULT NULL COMMENT '异常描述' AFTER special_reminding;
ALTER TABLE trade_202512 ADD COLUMN append_memo TEXT DEFAULT NULL COMMENT '追加备注' AFTER abnormal_description;
ALTER TABLE trade_202512 ADD COLUMN ticket_code_list TEXT DEFAULT NULL COMMENT '提货码列表JSON' AFTER append_memo;
ALTER TABLE trade_202512 ADD COLUMN all_compass_source_content_type VARCHAR(500) DEFAULT NULL COMMENT '全域来源内容类型' AFTER ticket_code_list;

-- ---------- trade_202601 ----------
ALTER TABLE trade_202601 ADD COLUMN customer_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户折让金额' AFTER flag_names;
ALTER TABLE trade_202601 ADD COLUMN customer_post_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户运费' AFTER customer_discount_fee;
ALTER TABLE trade_202601 ADD COLUMN customer_discount DECIMAL(8,4) DEFAULT NULL COMMENT '客户折扣' AFTER customer_post_fee;
ALTER TABLE trade_202601 ADD COLUMN customer_total_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户总费用' AFTER customer_discount;
ALTER TABLE trade_202601 ADD COLUMN customer_account VARCHAR(100) DEFAULT NULL COMMENT '客户账号' AFTER customer_total_fee;
ALTER TABLE trade_202601 ADD COLUMN customer_code VARCHAR(64) DEFAULT NULL COMMENT '客户编码' AFTER customer_account;
ALTER TABLE trade_202601 ADD COLUMN customer_grade_name VARCHAR(100) DEFAULT NULL COMMENT '客户等级' AFTER customer_code;
ALTER TABLE trade_202601 ADD COLUMN customer_tags VARCHAR(500) DEFAULT NULL COMMENT '客户标签' AFTER customer_grade_name;
ALTER TABLE trade_202601 ADD COLUMN buyer_open_uid VARCHAR(200) DEFAULT NULL COMMENT '买家平台唯一标识' AFTER customer_tags;
ALTER TABLE trade_202601 ADD COLUMN black_list INT DEFAULT NULL COMMENT '是否黑名单' AFTER buyer_open_uid;
ALTER TABLE trade_202601 ADD COLUMN invoice_amount DECIMAL(14,2) DEFAULT NULL COMMENT '发票金额' AFTER black_list;
ALTER TABLE trade_202601 ADD COLUMN invoice_type INT DEFAULT NULL COMMENT '发票类型' AFTER invoice_amount;
ALTER TABLE trade_202601 ADD COLUMN invoice_code VARCHAR(100) DEFAULT NULL COMMENT '发票编号' AFTER invoice_type;
ALTER TABLE trade_202601 ADD COLUMN charge_exchange_rate DECIMAL(10,4) DEFAULT NULL COMMENT '汇率' AFTER invoice_code;
ALTER TABLE trade_202601 ADD COLUMN charge_currency_code VARCHAR(20) DEFAULT NULL COMMENT '结算币种编码' AFTER charge_exchange_rate;
ALTER TABLE trade_202601 ADD COLUMN local_currency_code VARCHAR(20) DEFAULT NULL COMMENT '本地币种编码' AFTER charge_currency_code;
ALTER TABLE trade_202601 ADD COLUMN first_payment DECIMAL(14,2) DEFAULT NULL COMMENT '首款' AFTER local_currency_code;
ALTER TABLE trade_202601 ADD COLUMN final_payment DECIMAL(14,2) DEFAULT NULL COMMENT '尾款' AFTER first_payment;
ALTER TABLE trade_202601 ADD COLUMN received_total DECIMAL(14,2) DEFAULT NULL COMMENT '已收金额' AFTER final_payment;
ALTER TABLE trade_202601 ADD COLUMN first_paytime DATETIME DEFAULT NULL COMMENT '首次付款时间' AFTER received_total;
ALTER TABLE trade_202601 ADD COLUMN final_paytime DATETIME DEFAULT NULL COMMENT '最终付款时间' AFTER first_paytime;
ALTER TABLE trade_202601 ADD COLUMN fin_receipt_time DATETIME DEFAULT NULL COMMENT '财务收款时间' AFTER final_paytime;
ALTER TABLE trade_202601 ADD COLUMN payer_name VARCHAR(200) DEFAULT NULL COMMENT '付款方名称' AFTER fin_receipt_time;
ALTER TABLE trade_202601 ADD COLUMN payer_phone VARCHAR(50) DEFAULT NULL COMMENT '付款方电话' AFTER payer_name;
ALTER TABLE trade_202601 ADD COLUMN payer_regno VARCHAR(100) DEFAULT NULL COMMENT '付款方注册号' AFTER payer_phone;
ALTER TABLE trade_202601 ADD COLUMN payer_bank_account VARCHAR(100) DEFAULT NULL COMMENT '付款方银行账号' AFTER payer_regno;
ALTER TABLE trade_202601 ADD COLUMN payer_bank_name VARCHAR(200) DEFAULT NULL COMMENT '付款方银行名称' AFTER payer_bank_account;
ALTER TABLE trade_202601 ADD COLUMN logistic_code VARCHAR(50) DEFAULT NULL COMMENT '物流编码' AFTER payer_bank_name;
ALTER TABLE trade_202601 ADD COLUMN logistic_type INT DEFAULT NULL COMMENT '物流类型' AFTER logistic_code;
ALTER TABLE trade_202601 ADD COLUMN extra_logistic_no TEXT DEFAULT NULL COMMENT '额外物流单号JSON' AFTER logistic_type;
ALTER TABLE trade_202601 ADD COLUMN package_weight DECIMAL(14,2) DEFAULT NULL COMMENT '包裹重量' AFTER extra_logistic_no;
ALTER TABLE trade_202601 ADD COLUMN estimate_weight DECIMAL(14,2) DEFAULT NULL COMMENT '预估重量' AFTER package_weight;
ALTER TABLE trade_202601 ADD COLUMN estimate_volume DECIMAL(14,2) DEFAULT NULL COMMENT '预估体积' AFTER estimate_weight;
ALTER TABLE trade_202601 ADD COLUMN stockout_no VARCHAR(64) DEFAULT NULL COMMENT '出库单号' AFTER estimate_volume;
ALTER TABLE trade_202601 ADD COLUMN last_ship_time DATETIME DEFAULT NULL COMMENT '最后发货时间' AFTER stockout_no;
ALTER TABLE trade_202601 ADD COLUMN signing_time DATETIME DEFAULT NULL COMMENT '签收时间' AFTER last_ship_time;
ALTER TABLE trade_202601 ADD COLUMN review_time DATETIME DEFAULT NULL COMMENT '复核时间' AFTER signing_time;
ALTER TABLE trade_202601 ADD COLUMN confirm_time DATETIME DEFAULT NULL COMMENT '确认时间' AFTER review_time;
ALTER TABLE trade_202601 ADD COLUMN activation_time DATETIME DEFAULT NULL COMMENT '激活时间' AFTER confirm_time;
ALTER TABLE trade_202601 ADD COLUMN notify_pick_time DATETIME DEFAULT NULL COMMENT '通知拣货时间' AFTER activation_time;
ALTER TABLE trade_202601 ADD COLUMN settle_audit_time DATETIME DEFAULT NULL COMMENT '结算审核时间' AFTER notify_pick_time;
ALTER TABLE trade_202601 ADD COLUMN plat_complete_time DATETIME DEFAULT NULL COMMENT '平台完成时间' AFTER settle_audit_time;
ALTER TABLE trade_202601 ADD COLUMN reviewer VARCHAR(100) DEFAULT NULL COMMENT '复核人' AFTER plat_complete_time;
ALTER TABLE trade_202601 ADD COLUMN auditor VARCHAR(100) DEFAULT NULL COMMENT '审核人' AFTER reviewer;
ALTER TABLE trade_202601 ADD COLUMN register VARCHAR(100) DEFAULT NULL COMMENT '登记人' AFTER auditor;
ALTER TABLE trade_202601 ADD COLUMN seller VARCHAR(100) DEFAULT NULL COMMENT '业务员' AFTER register;
ALTER TABLE trade_202601 ADD COLUMN shop_type_code VARCHAR(50) DEFAULT NULL COMMENT '店铺平台编码' AFTER seller;
ALTER TABLE trade_202601 ADD COLUMN agent_shop_name VARCHAR(200) DEFAULT NULL COMMENT '代理店铺名' AFTER shop_type_code;
ALTER TABLE trade_202601 ADD COLUMN source_after_no VARCHAR(64) DEFAULT NULL COMMENT '售后单号' AFTER agent_shop_name;
ALTER TABLE trade_202601 ADD COLUMN country_code VARCHAR(20) DEFAULT NULL COMMENT '国家编码' AFTER source_after_no;
ALTER TABLE trade_202601 ADD COLUMN city_code VARCHAR(20) DEFAULT NULL COMMENT '城市编码' AFTER country_code;
ALTER TABLE trade_202601 ADD COLUMN sys_flag_ids VARCHAR(500) DEFAULT NULL COMMENT '系统标记ID' AFTER city_code;
ALTER TABLE trade_202601 ADD COLUMN special_reminding VARCHAR(500) DEFAULT NULL COMMENT '特殊提醒' AFTER sys_flag_ids;
ALTER TABLE trade_202601 ADD COLUMN abnormal_description VARCHAR(500) DEFAULT NULL COMMENT '异常描述' AFTER special_reminding;
ALTER TABLE trade_202601 ADD COLUMN append_memo TEXT DEFAULT NULL COMMENT '追加备注' AFTER abnormal_description;
ALTER TABLE trade_202601 ADD COLUMN ticket_code_list TEXT DEFAULT NULL COMMENT '提货码列表JSON' AFTER append_memo;
ALTER TABLE trade_202601 ADD COLUMN all_compass_source_content_type VARCHAR(500) DEFAULT NULL COMMENT '全域来源内容类型' AFTER ticket_code_list;

-- ---------- trade_202602 ----------
ALTER TABLE trade_202602 ADD COLUMN customer_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户折让金额' AFTER flag_names;
ALTER TABLE trade_202602 ADD COLUMN customer_post_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户运费' AFTER customer_discount_fee;
ALTER TABLE trade_202602 ADD COLUMN customer_discount DECIMAL(8,4) DEFAULT NULL COMMENT '客户折扣' AFTER customer_post_fee;
ALTER TABLE trade_202602 ADD COLUMN customer_total_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户总费用' AFTER customer_discount;
ALTER TABLE trade_202602 ADD COLUMN customer_account VARCHAR(100) DEFAULT NULL COMMENT '客户账号' AFTER customer_total_fee;
ALTER TABLE trade_202602 ADD COLUMN customer_code VARCHAR(64) DEFAULT NULL COMMENT '客户编码' AFTER customer_account;
ALTER TABLE trade_202602 ADD COLUMN customer_grade_name VARCHAR(100) DEFAULT NULL COMMENT '客户等级' AFTER customer_code;
ALTER TABLE trade_202602 ADD COLUMN customer_tags VARCHAR(500) DEFAULT NULL COMMENT '客户标签' AFTER customer_grade_name;
ALTER TABLE trade_202602 ADD COLUMN buyer_open_uid VARCHAR(200) DEFAULT NULL COMMENT '买家平台唯一标识' AFTER customer_tags;
ALTER TABLE trade_202602 ADD COLUMN black_list INT DEFAULT NULL COMMENT '是否黑名单' AFTER buyer_open_uid;
ALTER TABLE trade_202602 ADD COLUMN invoice_amount DECIMAL(14,2) DEFAULT NULL COMMENT '发票金额' AFTER black_list;
ALTER TABLE trade_202602 ADD COLUMN invoice_type INT DEFAULT NULL COMMENT '发票类型' AFTER invoice_amount;
ALTER TABLE trade_202602 ADD COLUMN invoice_code VARCHAR(100) DEFAULT NULL COMMENT '发票编号' AFTER invoice_type;
ALTER TABLE trade_202602 ADD COLUMN charge_exchange_rate DECIMAL(10,4) DEFAULT NULL COMMENT '汇率' AFTER invoice_code;
ALTER TABLE trade_202602 ADD COLUMN charge_currency_code VARCHAR(20) DEFAULT NULL COMMENT '结算币种编码' AFTER charge_exchange_rate;
ALTER TABLE trade_202602 ADD COLUMN local_currency_code VARCHAR(20) DEFAULT NULL COMMENT '本地币种编码' AFTER charge_currency_code;
ALTER TABLE trade_202602 ADD COLUMN first_payment DECIMAL(14,2) DEFAULT NULL COMMENT '首款' AFTER local_currency_code;
ALTER TABLE trade_202602 ADD COLUMN final_payment DECIMAL(14,2) DEFAULT NULL COMMENT '尾款' AFTER first_payment;
ALTER TABLE trade_202602 ADD COLUMN received_total DECIMAL(14,2) DEFAULT NULL COMMENT '已收金额' AFTER final_payment;
ALTER TABLE trade_202602 ADD COLUMN first_paytime DATETIME DEFAULT NULL COMMENT '首次付款时间' AFTER received_total;
ALTER TABLE trade_202602 ADD COLUMN final_paytime DATETIME DEFAULT NULL COMMENT '最终付款时间' AFTER first_paytime;
ALTER TABLE trade_202602 ADD COLUMN fin_receipt_time DATETIME DEFAULT NULL COMMENT '财务收款时间' AFTER final_paytime;
ALTER TABLE trade_202602 ADD COLUMN payer_name VARCHAR(200) DEFAULT NULL COMMENT '付款方名称' AFTER fin_receipt_time;
ALTER TABLE trade_202602 ADD COLUMN payer_phone VARCHAR(50) DEFAULT NULL COMMENT '付款方电话' AFTER payer_name;
ALTER TABLE trade_202602 ADD COLUMN payer_regno VARCHAR(100) DEFAULT NULL COMMENT '付款方注册号' AFTER payer_phone;
ALTER TABLE trade_202602 ADD COLUMN payer_bank_account VARCHAR(100) DEFAULT NULL COMMENT '付款方银行账号' AFTER payer_regno;
ALTER TABLE trade_202602 ADD COLUMN payer_bank_name VARCHAR(200) DEFAULT NULL COMMENT '付款方银行名称' AFTER payer_bank_account;
ALTER TABLE trade_202602 ADD COLUMN logistic_code VARCHAR(50) DEFAULT NULL COMMENT '物流编码' AFTER payer_bank_name;
ALTER TABLE trade_202602 ADD COLUMN logistic_type INT DEFAULT NULL COMMENT '物流类型' AFTER logistic_code;
ALTER TABLE trade_202602 ADD COLUMN extra_logistic_no TEXT DEFAULT NULL COMMENT '额外物流单号JSON' AFTER logistic_type;
ALTER TABLE trade_202602 ADD COLUMN package_weight DECIMAL(14,2) DEFAULT NULL COMMENT '包裹重量' AFTER extra_logistic_no;
ALTER TABLE trade_202602 ADD COLUMN estimate_weight DECIMAL(14,2) DEFAULT NULL COMMENT '预估重量' AFTER package_weight;
ALTER TABLE trade_202602 ADD COLUMN estimate_volume DECIMAL(14,2) DEFAULT NULL COMMENT '预估体积' AFTER estimate_weight;
ALTER TABLE trade_202602 ADD COLUMN stockout_no VARCHAR(64) DEFAULT NULL COMMENT '出库单号' AFTER estimate_volume;
ALTER TABLE trade_202602 ADD COLUMN last_ship_time DATETIME DEFAULT NULL COMMENT '最后发货时间' AFTER stockout_no;
ALTER TABLE trade_202602 ADD COLUMN signing_time DATETIME DEFAULT NULL COMMENT '签收时间' AFTER last_ship_time;
ALTER TABLE trade_202602 ADD COLUMN review_time DATETIME DEFAULT NULL COMMENT '复核时间' AFTER signing_time;
ALTER TABLE trade_202602 ADD COLUMN confirm_time DATETIME DEFAULT NULL COMMENT '确认时间' AFTER review_time;
ALTER TABLE trade_202602 ADD COLUMN activation_time DATETIME DEFAULT NULL COMMENT '激活时间' AFTER confirm_time;
ALTER TABLE trade_202602 ADD COLUMN notify_pick_time DATETIME DEFAULT NULL COMMENT '通知拣货时间' AFTER activation_time;
ALTER TABLE trade_202602 ADD COLUMN settle_audit_time DATETIME DEFAULT NULL COMMENT '结算审核时间' AFTER notify_pick_time;
ALTER TABLE trade_202602 ADD COLUMN plat_complete_time DATETIME DEFAULT NULL COMMENT '平台完成时间' AFTER settle_audit_time;
ALTER TABLE trade_202602 ADD COLUMN reviewer VARCHAR(100) DEFAULT NULL COMMENT '复核人' AFTER plat_complete_time;
ALTER TABLE trade_202602 ADD COLUMN auditor VARCHAR(100) DEFAULT NULL COMMENT '审核人' AFTER reviewer;
ALTER TABLE trade_202602 ADD COLUMN register VARCHAR(100) DEFAULT NULL COMMENT '登记人' AFTER auditor;
ALTER TABLE trade_202602 ADD COLUMN seller VARCHAR(100) DEFAULT NULL COMMENT '业务员' AFTER register;
ALTER TABLE trade_202602 ADD COLUMN shop_type_code VARCHAR(50) DEFAULT NULL COMMENT '店铺平台编码' AFTER seller;
ALTER TABLE trade_202602 ADD COLUMN agent_shop_name VARCHAR(200) DEFAULT NULL COMMENT '代理店铺名' AFTER shop_type_code;
ALTER TABLE trade_202602 ADD COLUMN source_after_no VARCHAR(64) DEFAULT NULL COMMENT '售后单号' AFTER agent_shop_name;
ALTER TABLE trade_202602 ADD COLUMN country_code VARCHAR(20) DEFAULT NULL COMMENT '国家编码' AFTER source_after_no;
ALTER TABLE trade_202602 ADD COLUMN city_code VARCHAR(20) DEFAULT NULL COMMENT '城市编码' AFTER country_code;
ALTER TABLE trade_202602 ADD COLUMN sys_flag_ids VARCHAR(500) DEFAULT NULL COMMENT '系统标记ID' AFTER city_code;
ALTER TABLE trade_202602 ADD COLUMN special_reminding VARCHAR(500) DEFAULT NULL COMMENT '特殊提醒' AFTER sys_flag_ids;
ALTER TABLE trade_202602 ADD COLUMN abnormal_description VARCHAR(500) DEFAULT NULL COMMENT '异常描述' AFTER special_reminding;
ALTER TABLE trade_202602 ADD COLUMN append_memo TEXT DEFAULT NULL COMMENT '追加备注' AFTER abnormal_description;
ALTER TABLE trade_202602 ADD COLUMN ticket_code_list TEXT DEFAULT NULL COMMENT '提货码列表JSON' AFTER append_memo;
ALTER TABLE trade_202602 ADD COLUMN all_compass_source_content_type VARCHAR(500) DEFAULT NULL COMMENT '全域来源内容类型' AFTER ticket_code_list;

-- ---------- trade_202603 ----------
ALTER TABLE trade_202603 ADD COLUMN customer_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户折让金额' AFTER flag_names;
ALTER TABLE trade_202603 ADD COLUMN customer_post_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户运费' AFTER customer_discount_fee;
ALTER TABLE trade_202603 ADD COLUMN customer_discount DECIMAL(8,4) DEFAULT NULL COMMENT '客户折扣' AFTER customer_post_fee;
ALTER TABLE trade_202603 ADD COLUMN customer_total_fee DECIMAL(14,2) DEFAULT NULL COMMENT '客户总费用' AFTER customer_discount;
ALTER TABLE trade_202603 ADD COLUMN customer_account VARCHAR(100) DEFAULT NULL COMMENT '客户账号' AFTER customer_total_fee;
ALTER TABLE trade_202603 ADD COLUMN customer_code VARCHAR(64) DEFAULT NULL COMMENT '客户编码' AFTER customer_account;
ALTER TABLE trade_202603 ADD COLUMN customer_grade_name VARCHAR(100) DEFAULT NULL COMMENT '客户等级' AFTER customer_code;
ALTER TABLE trade_202603 ADD COLUMN customer_tags VARCHAR(500) DEFAULT NULL COMMENT '客户标签' AFTER customer_grade_name;
ALTER TABLE trade_202603 ADD COLUMN buyer_open_uid VARCHAR(200) DEFAULT NULL COMMENT '买家平台唯一标识' AFTER customer_tags;
ALTER TABLE trade_202603 ADD COLUMN black_list INT DEFAULT NULL COMMENT '是否黑名单' AFTER buyer_open_uid;
ALTER TABLE trade_202603 ADD COLUMN invoice_amount DECIMAL(14,2) DEFAULT NULL COMMENT '发票金额' AFTER black_list;
ALTER TABLE trade_202603 ADD COLUMN invoice_type INT DEFAULT NULL COMMENT '发票类型' AFTER invoice_amount;
ALTER TABLE trade_202603 ADD COLUMN invoice_code VARCHAR(100) DEFAULT NULL COMMENT '发票编号' AFTER invoice_type;
ALTER TABLE trade_202603 ADD COLUMN charge_exchange_rate DECIMAL(10,4) DEFAULT NULL COMMENT '汇率' AFTER invoice_code;
ALTER TABLE trade_202603 ADD COLUMN charge_currency_code VARCHAR(20) DEFAULT NULL COMMENT '结算币种编码' AFTER charge_exchange_rate;
ALTER TABLE trade_202603 ADD COLUMN local_currency_code VARCHAR(20) DEFAULT NULL COMMENT '本地币种编码' AFTER charge_currency_code;
ALTER TABLE trade_202603 ADD COLUMN first_payment DECIMAL(14,2) DEFAULT NULL COMMENT '首款' AFTER local_currency_code;
ALTER TABLE trade_202603 ADD COLUMN final_payment DECIMAL(14,2) DEFAULT NULL COMMENT '尾款' AFTER first_payment;
ALTER TABLE trade_202603 ADD COLUMN received_total DECIMAL(14,2) DEFAULT NULL COMMENT '已收金额' AFTER final_payment;
ALTER TABLE trade_202603 ADD COLUMN first_paytime DATETIME DEFAULT NULL COMMENT '首次付款时间' AFTER received_total;
ALTER TABLE trade_202603 ADD COLUMN final_paytime DATETIME DEFAULT NULL COMMENT '最终付款时间' AFTER first_paytime;
ALTER TABLE trade_202603 ADD COLUMN fin_receipt_time DATETIME DEFAULT NULL COMMENT '财务收款时间' AFTER final_paytime;
ALTER TABLE trade_202603 ADD COLUMN payer_name VARCHAR(200) DEFAULT NULL COMMENT '付款方名称' AFTER fin_receipt_time;
ALTER TABLE trade_202603 ADD COLUMN payer_phone VARCHAR(50) DEFAULT NULL COMMENT '付款方电话' AFTER payer_name;
ALTER TABLE trade_202603 ADD COLUMN payer_regno VARCHAR(100) DEFAULT NULL COMMENT '付款方注册号' AFTER payer_phone;
ALTER TABLE trade_202603 ADD COLUMN payer_bank_account VARCHAR(100) DEFAULT NULL COMMENT '付款方银行账号' AFTER payer_regno;
ALTER TABLE trade_202603 ADD COLUMN payer_bank_name VARCHAR(200) DEFAULT NULL COMMENT '付款方银行名称' AFTER payer_bank_account;
ALTER TABLE trade_202603 ADD COLUMN logistic_code VARCHAR(50) DEFAULT NULL COMMENT '物流编码' AFTER payer_bank_name;
ALTER TABLE trade_202603 ADD COLUMN logistic_type INT DEFAULT NULL COMMENT '物流类型' AFTER logistic_code;
ALTER TABLE trade_202603 ADD COLUMN extra_logistic_no TEXT DEFAULT NULL COMMENT '额外物流单号JSON' AFTER logistic_type;
ALTER TABLE trade_202603 ADD COLUMN package_weight DECIMAL(14,2) DEFAULT NULL COMMENT '包裹重量' AFTER extra_logistic_no;
ALTER TABLE trade_202603 ADD COLUMN estimate_weight DECIMAL(14,2) DEFAULT NULL COMMENT '预估重量' AFTER package_weight;
ALTER TABLE trade_202603 ADD COLUMN estimate_volume DECIMAL(14,2) DEFAULT NULL COMMENT '预估体积' AFTER estimate_weight;
ALTER TABLE trade_202603 ADD COLUMN stockout_no VARCHAR(64) DEFAULT NULL COMMENT '出库单号' AFTER estimate_volume;
ALTER TABLE trade_202603 ADD COLUMN last_ship_time DATETIME DEFAULT NULL COMMENT '最后发货时间' AFTER stockout_no;
ALTER TABLE trade_202603 ADD COLUMN signing_time DATETIME DEFAULT NULL COMMENT '签收时间' AFTER last_ship_time;
ALTER TABLE trade_202603 ADD COLUMN review_time DATETIME DEFAULT NULL COMMENT '复核时间' AFTER signing_time;
ALTER TABLE trade_202603 ADD COLUMN confirm_time DATETIME DEFAULT NULL COMMENT '确认时间' AFTER review_time;
ALTER TABLE trade_202603 ADD COLUMN activation_time DATETIME DEFAULT NULL COMMENT '激活时间' AFTER confirm_time;
ALTER TABLE trade_202603 ADD COLUMN notify_pick_time DATETIME DEFAULT NULL COMMENT '通知拣货时间' AFTER activation_time;
ALTER TABLE trade_202603 ADD COLUMN settle_audit_time DATETIME DEFAULT NULL COMMENT '结算审核时间' AFTER notify_pick_time;
ALTER TABLE trade_202603 ADD COLUMN plat_complete_time DATETIME DEFAULT NULL COMMENT '平台完成时间' AFTER settle_audit_time;
ALTER TABLE trade_202603 ADD COLUMN reviewer VARCHAR(100) DEFAULT NULL COMMENT '复核人' AFTER plat_complete_time;
ALTER TABLE trade_202603 ADD COLUMN auditor VARCHAR(100) DEFAULT NULL COMMENT '审核人' AFTER reviewer;
ALTER TABLE trade_202603 ADD COLUMN register VARCHAR(100) DEFAULT NULL COMMENT '登记人' AFTER auditor;
ALTER TABLE trade_202603 ADD COLUMN seller VARCHAR(100) DEFAULT NULL COMMENT '业务员' AFTER register;
ALTER TABLE trade_202603 ADD COLUMN shop_type_code VARCHAR(50) DEFAULT NULL COMMENT '店铺平台编码' AFTER seller;
ALTER TABLE trade_202603 ADD COLUMN agent_shop_name VARCHAR(200) DEFAULT NULL COMMENT '代理店铺名' AFTER shop_type_code;
ALTER TABLE trade_202603 ADD COLUMN source_after_no VARCHAR(64) DEFAULT NULL COMMENT '售后单号' AFTER agent_shop_name;
ALTER TABLE trade_202603 ADD COLUMN country_code VARCHAR(20) DEFAULT NULL COMMENT '国家编码' AFTER source_after_no;
ALTER TABLE trade_202603 ADD COLUMN city_code VARCHAR(20) DEFAULT NULL COMMENT '城市编码' AFTER country_code;
ALTER TABLE trade_202603 ADD COLUMN sys_flag_ids VARCHAR(500) DEFAULT NULL COMMENT '系统标记ID' AFTER city_code;
ALTER TABLE trade_202603 ADD COLUMN special_reminding VARCHAR(500) DEFAULT NULL COMMENT '特殊提醒' AFTER sys_flag_ids;
ALTER TABLE trade_202603 ADD COLUMN abnormal_description VARCHAR(500) DEFAULT NULL COMMENT '异常描述' AFTER special_reminding;
ALTER TABLE trade_202603 ADD COLUMN append_memo TEXT DEFAULT NULL COMMENT '追加备注' AFTER abnormal_description;
ALTER TABLE trade_202603 ADD COLUMN ticket_code_list TEXT DEFAULT NULL COMMENT '提货码列表JSON' AFTER append_memo;
ALTER TABLE trade_202603 ADD COLUMN all_compass_source_content_type VARCHAR(500) DEFAULT NULL COMMENT '全域来源内容类型' AFTER ticket_code_list;


-- ============================================================
-- 第二部分：trade_goods 表新增字段
-- 适用表：trade_goods_template、trade_goods_202509 ~ trade_goods_202603
-- ============================================================

-- ---------- trade_goods_template ----------
ALTER TABLE trade_goods_template ADD COLUMN goods_plat_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '平台商品折扣';
ALTER TABLE trade_goods_template ADD COLUMN is_presell INT DEFAULT NULL COMMENT '是否预售';
ALTER TABLE trade_goods_template ADD COLUMN share_order_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '分摊订单折扣';
ALTER TABLE trade_goods_template ADD COLUMN share_order_plat_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '分摊平台折扣';

-- ---------- trade_goods_202509 ----------
ALTER TABLE trade_goods_202509 ADD COLUMN goods_plat_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '平台商品折扣';
ALTER TABLE trade_goods_202509 ADD COLUMN is_presell INT DEFAULT NULL COMMENT '是否预售';
ALTER TABLE trade_goods_202509 ADD COLUMN share_order_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '分摊订单折扣';
ALTER TABLE trade_goods_202509 ADD COLUMN share_order_plat_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '分摊平台折扣';

-- ---------- trade_goods_202510 ----------
ALTER TABLE trade_goods_202510 ADD COLUMN goods_plat_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '平台商品折扣';
ALTER TABLE trade_goods_202510 ADD COLUMN is_presell INT DEFAULT NULL COMMENT '是否预售';
ALTER TABLE trade_goods_202510 ADD COLUMN share_order_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '分摊订单折扣';
ALTER TABLE trade_goods_202510 ADD COLUMN share_order_plat_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '分摊平台折扣';

-- ---------- trade_goods_202511 ----------
ALTER TABLE trade_goods_202511 ADD COLUMN goods_plat_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '平台商品折扣';
ALTER TABLE trade_goods_202511 ADD COLUMN is_presell INT DEFAULT NULL COMMENT '是否预售';
ALTER TABLE trade_goods_202511 ADD COLUMN share_order_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '分摊订单折扣';
ALTER TABLE trade_goods_202511 ADD COLUMN share_order_plat_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '分摊平台折扣';

-- ---------- trade_goods_202512 ----------
ALTER TABLE trade_goods_202512 ADD COLUMN goods_plat_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '平台商品折扣';
ALTER TABLE trade_goods_202512 ADD COLUMN is_presell INT DEFAULT NULL COMMENT '是否预售';
ALTER TABLE trade_goods_202512 ADD COLUMN share_order_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '分摊订单折扣';
ALTER TABLE trade_goods_202512 ADD COLUMN share_order_plat_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '分摊平台折扣';

-- ---------- trade_goods_202601 ----------
ALTER TABLE trade_goods_202601 ADD COLUMN goods_plat_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '平台商品折扣';
ALTER TABLE trade_goods_202601 ADD COLUMN is_presell INT DEFAULT NULL COMMENT '是否预售';
ALTER TABLE trade_goods_202601 ADD COLUMN share_order_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '分摊订单折扣';
ALTER TABLE trade_goods_202601 ADD COLUMN share_order_plat_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '分摊平台折扣';

-- ---------- trade_goods_202602 ----------
ALTER TABLE trade_goods_202602 ADD COLUMN goods_plat_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '平台商品折扣';
ALTER TABLE trade_goods_202602 ADD COLUMN is_presell INT DEFAULT NULL COMMENT '是否预售';
ALTER TABLE trade_goods_202602 ADD COLUMN share_order_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '分摊订单折扣';
ALTER TABLE trade_goods_202602 ADD COLUMN share_order_plat_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '分摊平台折扣';

-- ---------- trade_goods_202603 ----------
ALTER TABLE trade_goods_202603 ADD COLUMN goods_plat_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '平台商品折扣';
ALTER TABLE trade_goods_202603 ADD COLUMN is_presell INT DEFAULT NULL COMMENT '是否预售';
ALTER TABLE trade_goods_202603 ADD COLUMN share_order_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '分摊订单折扣';
ALTER TABLE trade_goods_202603 ADD COLUMN share_order_plat_discount_fee DECIMAL(14,2) DEFAULT NULL COMMENT '分摊平台折扣';


-- ============================================================
-- 第三部分：sales_goods_summary 表新增字段
-- ============================================================

ALTER TABLE sales_goods_summary ADD COLUMN default_vend_id VARCHAR(64) DEFAULT NULL COMMENT '默认供应商ID';
ALTER TABLE sales_goods_summary ADD COLUMN unique_id VARCHAR(64) DEFAULT NULL COMMENT '唯一标识';
ALTER TABLE sales_goods_summary ADD COLUMN unique_sku_id VARCHAR(64) DEFAULT NULL COMMENT 'SKU唯一标识';


-- ============================================================
-- 第四部分：stock_quantity 表新增字段
-- ============================================================

ALTER TABLE stock_quantity ADD COLUMN is_batch_management INT DEFAULT NULL COMMENT '是否批次管理';
ALTER TABLE stock_quantity ADD COLUMN last_purch_no_tax_price DECIMAL(14,4) DEFAULT NULL COMMENT '最近采购未税价';
ALTER TABLE stock_quantity ADD COLUMN last_purch_price DECIMAL(14,4) DEFAULT NULL COMMENT '最近采购价';
ALTER TABLE stock_quantity ADD COLUMN locking_quantity DECIMAL(14,4) DEFAULT NULL COMMENT '锁定中数量';
ALTER TABLE stock_quantity ADD COLUMN owner_id VARCHAR(64) DEFAULT NULL COMMENT '货主ID';
ALTER TABLE stock_quantity ADD COLUMN owner_type INT DEFAULT NULL COMMENT '货主类型';
