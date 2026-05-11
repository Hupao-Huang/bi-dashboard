package handler

import (
	"database/sql"
	"log"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

type permissionSeed struct {
	Code string
	Name string
	Type string
}

type roleSeed struct {
	Code        string
	Name        string
	Description string
}

var permissionSeeds = []permissionSeed{
	{Code: "overview:view", Name: "综合看板", Type: "page"},
	{Code: "brand:view", Name: "品牌中心", Type: "menu"},
	{Code: "ecommerce:view", Name: "电商部门", Type: "menu"},
	{Code: "ecommerce.store_preview:view", Name: "电商-店铺数据预览", Type: "page"},
	{Code: "ecommerce.store_dashboard:view", Name: "电商-店铺看板", Type: "page"},
	{Code: "ecommerce.product_dashboard:view", Name: "电商-货品看板", Type: "page"},
	{Code: "ecommerce.marketing_cost:view", Name: "电商-营销费用", Type: "page"},
	{Code: "ecommerce.special_channel_allot:view", Name: "电商-特殊渠道调拨对账", Type: "page"},
	{Code: "social:view", Name: "社媒部门", Type: "menu"},
	{Code: "social.store_preview:view", Name: "社媒-店铺数据预览", Type: "page"},
	{Code: "social.store_dashboard:view", Name: "社媒-店铺看板", Type: "page"},
	{Code: "social.product_dashboard:view", Name: "社媒-货品看板", Type: "page"},
	{Code: "social.feigua:view", Name: "社媒-飞瓜看板", Type: "page"},
	{Code: "social.marketing:view", Name: "社媒-营销看板", Type: "page"},
	{Code: "offline:view", Name: "线下部门", Type: "menu"},
	{Code: "offline.store_preview:view", Name: "线下-店铺数据预览", Type: "page"},
	{Code: "offline.store_dashboard:view", Name: "线下-店铺看板", Type: "page"},
	{Code: "offline.product_dashboard:view", Name: "线下-货品看板", Type: "page"},
	{Code: "offline.high_value_customers:view", Name: "线下-高价值客户", Type: "page"},
	{Code: "offline.turnover_expiry:view", Name: "线下-周转率及临期", Type: "page"},
	{Code: "offline.ka_monthly:view", Name: "线下-KA月度统计", Type: "page"},
	{Code: "offline.target:view", Name: "线下-目标管理查看", Type: "page"},
	{Code: "offline.target:edit", Name: "线下-目标管理编辑", Type: "action"},
	{Code: "offline.sales_forecast:view", Name: "线下-销量预测管理查看", Type: "page"},
	{Code: "offline.sales_forecast:edit", Name: "线下-销量预测管理编辑", Type: "action"},
	{Code: "distribution:view", Name: "分销部门", Type: "menu"},
	{Code: "distribution.store_preview:view", Name: "分销-店铺数据预览", Type: "page"},
	{Code: "distribution.store_dashboard:view", Name: "分销-店铺看板", Type: "page"},
	{Code: "distribution.product_dashboard:view", Name: "分销-货品看板", Type: "page"},
	{Code: "distribution.customer_analysis:view", Name: "分销-客户分析", Type: "page"},
	{Code: "distribution.customer_list:edit", Name: "分销-客户名单管理", Type: "action"},
	{Code: "instant_retail:view", Name: "即时零售部", Type: "menu"},
	{Code: "instant_retail.store_preview:view", Name: "即时零售-店铺数据预览", Type: "page"},
	{Code: "instant_retail.store_dashboard:view", Name: "即时零售-店铺看板", Type: "page"},
	{Code: "instant_retail.product_dashboard:view", Name: "即时零售-货品看板", Type: "page"},
	{Code: "instant_retail.special_channel_allot:view", Name: "即时零售-特殊渠道调拨对账", Type: "page"},
	{Code: "finance:view", Name: "财务部门", Type: "menu"},
	{Code: "finance.overview:view", Name: "财务-利润总览", Type: "page"},
	{Code: "finance.department_profit:view", Name: "财务-部门利润分析", Type: "page"},
	{Code: "finance.monthly_profit:view", Name: "财务-月度利润统计", Type: "page"},
	{Code: "finance.product_profit:view", Name: "财务-产品利润统计", Type: "page"},
	{Code: "finance.expense:view", Name: "财务-费控管理", Type: "page"},
	{Code: "finance.report:view", Name: "财务-财务报表", Type: "page"},
	{Code: "finance.report:import", Name: "财务-财务报表导入", Type: "action"},
	{Code: "customer:view", Name: "客服部门", Type: "menu"},
	{Code: "customer.overview:view", Name: "客服-客服总览", Type: "page"},
	{Code: "supply_chain:view", Name: "供应链管理", Type: "menu"},
	{Code: "supply_chain.plan_dashboard:view", Name: "供应链-计划看板", Type: "page"},
	{Code: "supply_chain.inventory_warning:view", Name: "供应链-库存预警", Type: "page"},
	{Code: "supply_chain.logistics_analysis:view", Name: "供应链-快递仓储分析", Type: "page"},
	{Code: "supply_chain.daily_alerts:view", Name: "供应链-每日预警", Type: "page"},
	{Code: "supply_chain.monthly_billing:view", Name: "供应链-月度账单分析", Type: "page"},
	{Code: "profit:view", Name: "查看利润", Type: "field"},
	{Code: "cost:view", Name: "查看成本", Type: "field"},
	{Code: "gross_margin:view", Name: "查看毛利率", Type: "field"},
	{Code: "user.manage", Name: "用户管理", Type: "action"},
	{Code: "role.manage", Name: "角色管理", Type: "action"},
	{Code: "feedback.manage", Name: "反馈管理", Type: "action"},
	{Code: "notice.manage", Name: "公告管理", Type: "action"},
	{Code: "channel.manage", Name: "渠道管理", Type: "action"},
	{Code: "data:export", Name: "数据导出", Type: "action"},
}

var roleSeeds = []roleSeed{
	{Code: "super_admin", Name: "超级管理员", Description: "拥有全部权限"},
	{Code: "management", Name: "管理层", Description: "查看全局经营数据"},
	{Code: "dept_manager", Name: "部门负责人", Description: "按部门查看业务数据"},
	{Code: "operator", Name: "运营", Description: "查看分配的平台和店铺"},
	{Code: "finance", Name: "财务", Description: "查看财务与利润数据"},
	{Code: "supply_chain", Name: "供应链", Description: "查看供应链相关数据"},
}

var roleDefaultPermissions = map[string][]string{
	"management": {
		"overview:view",
		"brand:view",
		"ecommerce:view", "ecommerce.store_preview:view", "ecommerce.store_dashboard:view", "ecommerce.product_dashboard:view", "ecommerce.marketing_cost:view", "ecommerce.special_channel_allot:view",
		"social:view", "social.store_preview:view", "social.store_dashboard:view", "social.product_dashboard:view", "social.feigua:view",
		"offline:view", "offline.store_preview:view", "offline.store_dashboard:view", "offline.product_dashboard:view", "offline.high_value_customers:view", "offline.turnover_expiry:view", "offline.ka_monthly:view",
		"distribution:view", "distribution.store_preview:view", "distribution.store_dashboard:view", "distribution.product_dashboard:view", "distribution.customer_analysis:view", "distribution.customer_list:edit",
		"instant_retail:view", "instant_retail.store_preview:view", "instant_retail.store_dashboard:view", "instant_retail.product_dashboard:view", "instant_retail.special_channel_allot:view",
		"finance:view", "finance.overview:view", "finance.department_profit:view", "finance.monthly_profit:view", "finance.product_profit:view", "finance.expense:view", "finance.report:view",
		"customer:view", "customer.overview:view",
		"supply_chain:view", "supply_chain.plan_dashboard:view", "supply_chain.inventory_warning:view", "supply_chain.logistics_analysis:view", "supply_chain.daily_alerts:view", "supply_chain.monthly_billing:view",
		"profit:view", "cost:view", "gross_margin:view",
	},
	"dept_manager": {
		"overview:view",
		"brand:view",
		"ecommerce:view", "ecommerce.store_preview:view", "ecommerce.store_dashboard:view", "ecommerce.product_dashboard:view", "ecommerce.marketing_cost:view", "ecommerce.special_channel_allot:view",
		"social:view", "social.store_preview:view", "social.store_dashboard:view", "social.product_dashboard:view", "social.feigua:view",
		"offline:view", "offline.store_preview:view", "offline.store_dashboard:view", "offline.product_dashboard:view", "offline.high_value_customers:view", "offline.turnover_expiry:view", "offline.ka_monthly:view",
		"distribution:view", "distribution.store_preview:view", "distribution.store_dashboard:view", "distribution.product_dashboard:view", "distribution.customer_analysis:view", "distribution.customer_list:edit",
		"instant_retail:view", "instant_retail.store_preview:view", "instant_retail.store_dashboard:view", "instant_retail.product_dashboard:view", "instant_retail.special_channel_allot:view",
		"customer:view", "customer.overview:view",
	},
	"operator": {
		"brand:view",
		"ecommerce:view", "ecommerce.store_preview:view", "ecommerce.store_dashboard:view", "ecommerce.product_dashboard:view", "ecommerce.marketing_cost:view", "ecommerce.special_channel_allot:view",
		"social:view", "social.store_preview:view", "social.store_dashboard:view", "social.product_dashboard:view", "social.feigua:view",
		"offline:view", "offline.store_preview:view", "offline.store_dashboard:view", "offline.product_dashboard:view", "offline.high_value_customers:view", "offline.turnover_expiry:view", "offline.ka_monthly:view",
		"distribution:view", "distribution.store_preview:view", "distribution.store_dashboard:view", "distribution.product_dashboard:view", "distribution.customer_analysis:view", "distribution.customer_list:edit",
		"instant_retail:view", "instant_retail.store_preview:view", "instant_retail.store_dashboard:view", "instant_retail.product_dashboard:view", "instant_retail.special_channel_allot:view",
		"customer:view", "customer.overview:view",
	},
	"finance": {
		"overview:view",
		"brand:view",
		"finance:view", "finance.overview:view", "finance.department_profit:view", "finance.monthly_profit:view", "finance.product_profit:view", "finance.expense:view", "finance.report:view", "finance.report:import",
		"profit:view", "cost:view", "gross_margin:view",
	},
	"supply_chain": {
		"brand:view",
		"supply_chain:view", "supply_chain.plan_dashboard:view", "supply_chain.inventory_warning:view", "supply_chain.logistics_analysis:view", "supply_chain.daily_alerts:view", "supply_chain.monthly_billing:view",
	},
}

func allPermissionCodes() []string {
	codes := make([]string, 0, len(permissionSeeds))
	for _, seed := range permissionSeeds {
		codes = append(codes, seed.Code)
	}
	return codes
}

func EnsureAuthSchemaAndSeed(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			username VARCHAR(64) NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			real_name VARCHAR(64) DEFAULT NULL,
			status VARCHAR(20) NOT NULL DEFAULT 'active',
			last_login_at DATETIME DEFAULT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_username (username)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='系统用户'`,
		`CREATE TABLE IF NOT EXISTS roles (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			code VARCHAR(64) NOT NULL,
			name VARCHAR(100) NOT NULL,
			description VARCHAR(255) DEFAULT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_role_code (code)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='系统角色'`,
		`CREATE TABLE IF NOT EXISTS permissions (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			code VARCHAR(128) NOT NULL,
			name VARCHAR(100) NOT NULL,
			type VARCHAR(20) NOT NULL DEFAULT 'page',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_permission_code (code)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='权限点'`,
		`CREATE TABLE IF NOT EXISTS user_roles (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			user_id BIGINT NOT NULL,
			role_id BIGINT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE KEY uk_user_role (user_id, role_id),
			KEY idx_user_roles_user_id (user_id),
			KEY idx_user_roles_role_id (role_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户角色关联'`,
		`CREATE TABLE IF NOT EXISTS role_permissions (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			role_id BIGINT NOT NULL,
			permission_id BIGINT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE KEY uk_role_permission (role_id, permission_id),
			KEY idx_role_permissions_role_id (role_id),
			KEY idx_role_permissions_permission_id (permission_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='角色权限关联'`,
		`CREATE TABLE IF NOT EXISTS data_scopes (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			subject_type VARCHAR(16) NOT NULL,
			subject_id BIGINT NOT NULL,
			scope_type VARCHAR(20) NOT NULL,
			scope_value VARCHAR(128) NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE KEY uk_data_scope (subject_type, subject_id, scope_type, scope_value),
			KEY idx_data_scope_subject (subject_type, subject_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='数据权限范围'`,
		`CREATE TABLE IF NOT EXISTS user_sessions (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			user_id BIGINT NOT NULL,
			token_hash CHAR(64) NOT NULL,
			expires_at DATETIME NOT NULL,
			last_active_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			ip VARCHAR(64) DEFAULT NULL,
			user_agent VARCHAR(255) DEFAULT NULL,
			UNIQUE KEY uk_token_hash (token_hash),
			KEY idx_user_sessions_user_id (user_id),
			KEY idx_user_sessions_expires_at (expires_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='登录会话'`,

		`CREATE TABLE IF NOT EXISTS feedback (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			user_id BIGINT NOT NULL,
			username VARCHAR(64) NOT NULL,
			real_name VARCHAR(64) NOT NULL DEFAULT '',
			title VARCHAR(255) NOT NULL,
			content TEXT NOT NULL,
			page_url VARCHAR(500) DEFAULT '',
			attachments JSON DEFAULT NULL COMMENT '附件列表，JSON数组',
			status VARCHAR(20) NOT NULL DEFAULT 'pending' COMMENT 'pending/processing/resolved/closed',
			reply TEXT DEFAULT NULL COMMENT '管理员回复',
			replied_by VARCHAR(64) DEFAULT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			KEY idx_feedback_user_id (user_id),
			KEY idx_feedback_status (status),
			KEY idx_feedback_created_at (created_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户反馈'`,

		`CREATE TABLE IF NOT EXISTS audit_logs (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			user_id BIGINT DEFAULT NULL,
			username VARCHAR(64) NOT NULL DEFAULT '',
			real_name VARCHAR(64) NOT NULL DEFAULT '',
			action VARCHAR(32) NOT NULL COMMENT 'page_view/export/login/logout/permission_change',
			resource VARCHAR(255) NOT NULL DEFAULT '',
			detail TEXT DEFAULT NULL,
			ip VARCHAR(64) DEFAULT '',
			user_agent VARCHAR(255) DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			KEY idx_audit_user_id (user_id),
			KEY idx_audit_action (action),
			KEY idx_audit_created_at (created_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='操作审计日志'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_live_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			shop_name VARCHAR(100) NOT NULL,
			anchor_name VARCHAR(100) DEFAULT '',
			anchor_id VARCHAR(100) DEFAULT '',
			start_time DATETIME DEFAULT NULL,
			end_time DATETIME DEFAULT NULL,
			duration_min DECIMAL(10,2) DEFAULT 0,
			exposure_uv INT DEFAULT 0 COMMENT '曝光人数',
			watch_uv INT DEFAULT 0 COMMENT '观看人数',
			watch_pv INT DEFAULT 0 COMMENT '观看次数',
			max_online INT DEFAULT 0 COMMENT '最高在线',
			avg_online INT DEFAULT 0 COMMENT '平均在线',
			avg_watch_min DECIMAL(10,2) DEFAULT 0 COMMENT '人均观看时长(分)',
			comments INT DEFAULT 0,
			new_fans INT DEFAULT 0,
			product_count INT DEFAULT 0,
			product_exposure_uv INT DEFAULT 0,
			product_click_uv INT DEFAULT 0,
			order_count INT DEFAULT 0,
			order_amount DECIMAL(14,2) DEFAULT 0,
			pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '用户支付金额',
			pay_per_hour DECIMAL(14,2) DEFAULT 0,
			pay_qty INT DEFAULT 0,
			pay_uv INT DEFAULT 0,
			refund_count INT DEFAULT 0,
			refund_amount DECIMAL(14,2) DEFAULT 0,
			ad_cost_bindshop DECIMAL(14,2) DEFAULT 0 COMMENT '投放消耗(店铺绑定)',
			ad_cost_invested DECIMAL(14,2) DEFAULT 0 COMMENT '投放消耗(店铺被投)',
			net_order_amount DECIMAL(14,2) DEFAULT 0 COMMENT '净成交金额',
			net_order_count INT DEFAULT 0,
			refund_1h_rate DECIMAL(8,4) DEFAULT 0 COMMENT '1小时退款率',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			KEY idx_douyin_live_date (stat_date),
			KEY idx_douyin_live_shop (shop_name)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音自营直播数据'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_goods_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			shop_name VARCHAR(100) NOT NULL,
			product_id VARCHAR(64) NOT NULL,
			product_name VARCHAR(255) DEFAULT '',
			explain_count INT DEFAULT 0 COMMENT '讲解次数',
			live_price VARCHAR(50) DEFAULT '',
			pay_amount DECIMAL(14,2) DEFAULT 0,
			pay_qty INT DEFAULT 0,
			presale_count INT DEFAULT 0,
			click_uv INT DEFAULT 0,
			click_rate DECIMAL(8,4) DEFAULT 0,
			conv_rate DECIMAL(8,4) DEFAULT 0,
			cpm_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '千次曝光支付金额',
			pre_refund_count INT DEFAULT 0,
			pre_refund_amount DECIMAL(14,2) DEFAULT 0,
			post_refund_count INT DEFAULT 0,
			post_refund_amount DECIMAL(14,2) DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_douyin_goods (stat_date, shop_name, product_id),
			KEY idx_douyin_goods_date (stat_date)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音自营商品数据'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_dist_product_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			account_name VARCHAR(200) NOT NULL COMMENT '投放账户(文件夹名)',
			product_id VARCHAR(64) NOT NULL,
			product_name VARCHAR(255) DEFAULT '',
			impressions INT DEFAULT 0,
			clicks INT DEFAULT 0,
			click_rate DECIMAL(8,4) DEFAULT 0,
			conv_rate DECIMAL(8,4) DEFAULT 0,
			cost DECIMAL(14,2) DEFAULT 0 COMMENT '整体消耗',
			pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '整体成交金额',
			roi DECIMAL(10,2) DEFAULT 0,
			order_cost DECIMAL(10,2) DEFAULT 0,
			user_pay_amount DECIMAL(14,2) DEFAULT 0,
			subsidy_amount DECIMAL(14,2) DEFAULT 0 COMMENT '平台补贴',
			net_roi DECIMAL(10,2) DEFAULT 0,
			net_amount DECIMAL(14,2) DEFAULT 0,
			net_order_cost DECIMAL(10,2) DEFAULT 0,
			net_settle_rate DECIMAL(8,4) DEFAULT 0,
			refund_1h_rate DECIMAL(8,4) DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_dist_product (stat_date, account_name, product_id),
			KEY idx_dist_product_date (stat_date)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音分销推商品'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_dist_account_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			account_name VARCHAR(200) NOT NULL COMMENT '投放账户(文件夹名)',
			douyin_name VARCHAR(200) DEFAULT '',
			douyin_id VARCHAR(100) DEFAULT '',
			cost DECIMAL(14,2) DEFAULT 0,
			order_count INT DEFAULT 0,
			pay_amount DECIMAL(14,2) DEFAULT 0,
			roi DECIMAL(10,2) DEFAULT 0,
			order_cost DECIMAL(10,2) DEFAULT 0,
			user_pay_amount DECIMAL(14,2) DEFAULT 0,
			subsidy_amount DECIMAL(14,2) DEFAULT 0,
			net_roi DECIMAL(10,2) DEFAULT 0,
			net_amount DECIMAL(14,2) DEFAULT 0,
			net_settle_rate DECIMAL(8,4) DEFAULT 0,
			refund_1h_rate DECIMAL(8,4) DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_dist_account (stat_date, account_name, douyin_name),
			KEY idx_dist_account_date (stat_date)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音分销推抖音号'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_channel_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			shop_name VARCHAR(100) NOT NULL,
			channel_name VARCHAR(100) NOT NULL COMMENT '渠道名(推荐feed/直播广场/同城等)',
			watch_ucnt INT DEFAULT 0 COMMENT '观看人数',
			watch_cnt INT DEFAULT 0 COMMENT '观看次数',
			avg_watch_duration DECIMAL(10,2) DEFAULT 0 COMMENT '平均观看时长(秒)',
			pay_amt DECIMAL(14,2) DEFAULT 0 COMMENT '成交金额',
			pay_cnt INT DEFAULT 0 COMMENT '成交人数',
			avg_pay_order_amt DECIMAL(10,2) DEFAULT 0 COMMENT '客单价',
			watch_pay_cnt_ratio DECIMAL(8,4) DEFAULT 0 COMMENT '观看-成交率',
			interact_watch_cnt_ratio DECIMAL(8,4) DEFAULT 0 COMMENT '互动-观看率',
			ad_costed_amt DECIMAL(14,2) DEFAULT 0 COMMENT '广告消耗',
			stat_cost DECIMAL(14,2) DEFAULT 0 COMMENT '统计消耗',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_douyin_channel (stat_date, shop_name, channel_name),
			KEY idx_douyin_channel_date (stat_date)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音自营渠道分析'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_funnel_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			shop_name VARCHAR(100) NOT NULL,
			step_name VARCHAR(100) NOT NULL COMMENT '漏斗步骤名',
			step_value BIGINT DEFAULT 0 COMMENT '人数/金额',
			step_order INT DEFAULT 0 COMMENT '排序',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_douyin_funnel (stat_date, shop_name, step_name),
			KEY idx_douyin_funnel_date (stat_date)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音自营转化漏斗'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_ad_live_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			shop_name VARCHAR(100) NOT NULL,
			douyin_name VARCHAR(200) DEFAULT '',
			douyin_id VARCHAR(100) DEFAULT '',
			net_roi DECIMAL(10,2) DEFAULT 0,
			net_amount DECIMAL(14,2) DEFAULT 0 COMMENT '净成交金额',
			net_order_count INT DEFAULT 0,
			net_order_cost DECIMAL(10,2) DEFAULT 0,
			user_pay_amount DECIMAL(14,2) DEFAULT 0,
			net_settle_rate DECIMAL(8,4) DEFAULT 0,
			refund_1h_rate DECIMAL(8,4) DEFAULT 0,
			impressions INT DEFAULT 0 COMMENT '展示次数',
			clicks INT DEFAULT 0,
			click_rate DECIMAL(8,4) DEFAULT 0,
			conv_rate DECIMAL(8,4) DEFAULT 0,
			cost DECIMAL(14,2) DEFAULT 0 COMMENT '整体消耗',
			pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '整体成交金额',
			roi DECIMAL(10,2) DEFAULT 0,
			cpm DECIMAL(10,2) DEFAULT 0 COMMENT '千次展现费用',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_douyin_ad_live (stat_date, shop_name, douyin_name),
			KEY idx_douyin_ad_live_date (stat_date)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音自营推广直播间画面'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_anchor_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			shop_name VARCHAR(100) NOT NULL,
			account VARCHAR(100) DEFAULT '' COMMENT '账号',
			anchor_name VARCHAR(100) NOT NULL,
			duration VARCHAR(50) DEFAULT '',
			pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '直播间用户支付金额',
			pay_per_hour DECIMAL(14,2) DEFAULT 0,
			max_online INT DEFAULT 0,
			avg_online INT DEFAULT 0,
			avg_item_price DECIMAL(10,2) DEFAULT 0 COMMENT '平均件单价',
			exposure_watch_rate VARCHAR(20) DEFAULT '' COMMENT '曝光-观看率',
			retention_rate VARCHAR(20) DEFAULT '' COMMENT '留存率',
			interact_rate VARCHAR(20) DEFAULT '' COMMENT '互动率',
			new_fans INT DEFAULT 0,
			fans_rate VARCHAR(20) DEFAULT '' COMMENT '关注率',
			new_group INT DEFAULT 0 COMMENT '新加团人数',
			uv_value DECIMAL(10,2) DEFAULT 0 COMMENT '单UV价值',
			pay_uv INT DEFAULT 0 COMMENT '成交人数',
			crowd_top3 VARCHAR(500) DEFAULT '' COMMENT '策略人群TOP3',
			gender VARCHAR(100) DEFAULT '' COMMENT '性别分布',
			age_top3 VARCHAR(500) DEFAULT '' COMMENT '年龄段TOP3',
			city_level_top3 VARCHAR(500) DEFAULT '' COMMENT '城市等级TOP3',
			region_top3 VARCHAR(500) DEFAULT '' COMMENT '地域分布TOP3',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_douyin_anchor (stat_date, shop_name, anchor_name),
			KEY idx_douyin_anchor_date (stat_date)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音自营主播分析'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_ad_material_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			shop_name VARCHAR(100) NOT NULL,
			material_name VARCHAR(500) DEFAULT '',
			material_id VARCHAR(100) DEFAULT '',
			material_eval VARCHAR(50) DEFAULT '' COMMENT '素材评估',
			material_duration VARCHAR(50) DEFAULT '',
			material_source VARCHAR(100) DEFAULT '',
			net_roi DECIMAL(10,2) DEFAULT 0,
			net_amount DECIMAL(14,2) DEFAULT 0,
			net_order_count INT DEFAULT 0,
			refund_1h_rate DECIMAL(8,4) DEFAULT 0,
			impressions INT DEFAULT 0,
			clicks INT DEFAULT 0,
			click_rate DECIMAL(8,4) DEFAULT 0,
			conv_rate DECIMAL(8,4) DEFAULT 0,
			cost DECIMAL(14,2) DEFAULT 0,
			pay_amount DECIMAL(14,2) DEFAULT 0,
			roi DECIMAL(10,2) DEFAULT 0,
			cpm DECIMAL(10,2) DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			KEY idx_douyin_material_date (stat_date),
			KEY idx_douyin_material_shop (shop_name)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音自营推广视频素材'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_dist_material_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			account_name VARCHAR(200) NOT NULL,
			material_id VARCHAR(100) DEFAULT '',
			material_name VARCHAR(500) DEFAULT '',
			impressions INT DEFAULT 0,
			clicks INT DEFAULT 0,
			click_rate DECIMAL(8,4) DEFAULT 0,
			conv_rate DECIMAL(8,4) DEFAULT 0,
			cost DECIMAL(14,2) DEFAULT 0,
			order_count INT DEFAULT 0,
			pay_amount DECIMAL(14,2) DEFAULT 0,
			roi DECIMAL(10,2) DEFAULT 0,
			order_cost DECIMAL(10,2) DEFAULT 0,
			user_pay_amount DECIMAL(14,2) DEFAULT 0,
			cpm DECIMAL(10,2) DEFAULT 0,
			cpc DECIMAL(10,2) DEFAULT 0,
			net_roi DECIMAL(10,2) DEFAULT 0,
			net_amount DECIMAL(14,2) DEFAULT 0,
			net_order_count INT DEFAULT 0,
			net_settle_rate DECIMAL(8,4) DEFAULT 0,
			refund_1h_rate DECIMAL(8,4) DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			KEY idx_dist_material_date (stat_date),
			KEY idx_dist_material_account (account_name)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音分销推素材'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_dist_promote_hourly (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			account_name VARCHAR(200) NOT NULL,
			stat_hour VARCHAR(20) NOT NULL COMMENT '小时(如23:00)',
			cost DECIMAL(14,2) DEFAULT 0 COMMENT '消耗',
			settle_amount DECIMAL(14,2) DEFAULT 0 COMMENT '净成交金额',
			settle_count INT DEFAULT 0 COMMENT '净成交订单数',
			roi DECIMAL(10,2) DEFAULT 0,
			refund_rate DECIMAL(8,4) DEFAULT 0 COMMENT '退款率',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_dist_promote (stat_date, account_name, stat_hour),
			KEY idx_dist_promote_date (stat_date)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音分销随心推(按小时)'`,
	}

	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}

	// 增量加列（忽略"列已存在"错误 MySQL 1060）
	addCols := []string{
		`ALTER TABLE users ADD COLUMN must_change_password TINYINT(1) NOT NULL DEFAULT 0 COMMENT '首次登录须改密码'`,
		`ALTER TABLE users ADD COLUMN department VARCHAR(100) DEFAULT '' COMMENT '所属部门'`,
		`ALTER TABLE users ADD COLUMN employee_id VARCHAR(50) DEFAULT '' COMMENT '工号'`,
		`ALTER TABLE users ADD COLUMN dingtalk_userid VARCHAR(64) DEFAULT '' COMMENT '钉钉用户ID'`,
		`ALTER TABLE users ADD COLUMN remark TEXT COMMENT '注册备注(权限申请说明)'`,
	}
	for _, alter := range addCols {
		if _, err := db.Exec(alter); err != nil && !strings.Contains(err.Error(), "1060") {
			return err
		}
	}

	if err := seedPermissions(db); err != nil {
		return err
	}
	if err := seedRoles(db); err != nil {
		return err
	}
	if err := seedRoleDefaultPermissions(db); err != nil {
		return err
	}
	if err := seedSuperAdminRolePermissions(db); err != nil {
		return err
	}
	if err := ensureDefaultAdmin(db); err != nil {
		return err
	}

	return nil
}

func seedPermissions(db *sql.DB) error {
	for _, seed := range permissionSeeds {
		if _, err := db.Exec(
			`INSERT INTO permissions (code, name, type) VALUES (?, ?, ?)
			 ON DUPLICATE KEY UPDATE name = VALUES(name), type = VALUES(type)`,
			seed.Code, seed.Name, seed.Type,
		); err != nil {
			return err
		}
	}
	return nil
}

func seedRoles(db *sql.DB) error {
	for _, seed := range roleSeeds {
		if _, err := db.Exec(
			`INSERT INTO roles (code, name, description) VALUES (?, ?, ?)
			 ON DUPLICATE KEY UPDATE name = VALUES(name), description = VALUES(description)`,
			seed.Code, seed.Name, seed.Description,
		); err != nil {
			return err
		}
	}
	return nil
}

func seedSuperAdminRolePermissions(db *sql.DB) error {
	var roleID int64
	if err := db.QueryRow(`SELECT id FROM roles WHERE code = 'super_admin'`).Scan(&roleID); err != nil {
		return err
	}

	for _, code := range allPermissionCodes() {
		var permissionID int64
		if err := db.QueryRow(`SELECT id FROM permissions WHERE code = ?`, code).Scan(&permissionID); err != nil {
			return err
		}
		if _, err := db.Exec(
			`INSERT IGNORE INTO role_permissions (role_id, permission_id) VALUES (?, ?)`,
			roleID, permissionID,
		); err != nil {
			return err
		}
	}
	return nil
}

func seedRoleDefaultPermissions(db *sql.DB) error {
	for roleCode, permissionCodes := range roleDefaultPermissions {
		var roleID int64
		if err := db.QueryRow(`SELECT id FROM roles WHERE code = ?`, roleCode).Scan(&roleID); err != nil {
			return err
		}
		for _, code := range permissionCodes {
			var permissionID int64
			if err := db.QueryRow(`SELECT id FROM permissions WHERE code = ?`, code).Scan(&permissionID); err != nil {
				return err
			}
			if _, err := db.Exec(
				`INSERT IGNORE INTO role_permissions (role_id, permission_id) VALUES (?, ?)`,
				roleID, permissionID,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureDefaultAdmin(db *sql.DB) error {
	var userCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&userCount); err != nil {
		return err
	}
	if userCount > 0 {
		return nil
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(defaultAdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	result, err := db.Exec(
		`INSERT INTO users (username, password_hash, real_name, status, must_change_password) VALUES (?, ?, ?, 'active', 1)`,
		defaultAdminUsername, string(passwordHash), "系统管理员",
	)
	if err != nil {
		return err
	}

	userID, err := result.LastInsertId()
	if err != nil {
		return err
	}

	var roleID int64
	if err := db.QueryRow(`SELECT id FROM roles WHERE code = 'super_admin'`).Scan(&roleID); err != nil {
		return err
	}

	if _, err := db.Exec(`INSERT IGNORE INTO user_roles (user_id, role_id) VALUES (?, ?)`, userID, roleID); err != nil {
		return err
	}

	log.Printf("default admin account created username=%s", defaultAdminUsername)
	return nil
}
