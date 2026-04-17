package handler

import (
	"net/http"
	"regexp"
	"strings"
)

// RPAMappingItem 表示一条RPA文件映射记录
type RPAMappingItem struct {
	Platform       string `json:"platform"`
	TableName      string `json:"table_name"`
	RPAFileKeyword string `json:"rpa_file_keyword"`
	FileFormat     string `json:"file_format"`
	ImportTool     string `json:"import_tool"`
}

// GetRPAMapping 返回RPA文件映射列表
func (h *DashboardHandler) GetRPAMapping(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	items := []RPAMappingItem{
		// 天猫
		{"天猫", "op_tmall_shop_daily", "生意参谋_店铺销售数据", "xlsx", "import-tmall"},
		{"天猫", "op_tmall_goods_daily", "生意参谋_商品销售数据", "xlsx", "import-tmall"},
		{"天猫", "op_tmall_campaign_daily", "万象台_营销场景数据", "xlsx", "import-tmall"},
		{"天猫", "op_tmall_cps_daily", "淘宝联盟_营销场景数据", "xlsx", "import-tmall"},
		{"天猫", "op_tmall_service_inquiry", "生意参谋_业绩询单", "xlsx", "import-tmall"},
		{"天猫", "op_tmall_service_consult", "生意参谋_咨询接待", "xlsx", "import-tmall"},
		{"天猫", "op_tmall_service_avgprice", "生意参谋_客单价客服", "xlsx", "import-tmall"},
		{"天猫", "op_tmall_service_evaluation", "生意参谋_接待评价", "xlsx", "import-tmall"},
		{"天猫", "op_tmall_member_daily", "生意参谋_会员数据", "json", "import-tmall"},
		{"天猫", "op_tmall_brand_daily", "数据银行_品牌数据", "json", "import-tmall"},
		{"天猫", "op_tmall_crowd_daily", "达摩盘_人群数据", "json", "import-tmall"},
		{"天猫", "op_tmall_repurchase_monthly", "集客_复购数据", "xlsx", "import-tmall"},
		{"天猫", "op_tmall_industry_monthly", "集客_行业数据", "xlsx", "import-tmall"},
		// 天猫超市
		{"天猫超市", "op_tmall_cs_shop_daily", "经营概况", "xlsx", "import-tmallcs"},
		{"天猫超市", "op_tmall_cs_campaign_daily", "无界场景_智多星_淘客", "xlsx", "import-tmallcs"},
		{"天猫超市", "op_tmall_cs_industry_keyword", "市场_行业热词", "xlsx", "import-tmallcs"},
		{"天猫超市", "op_tmall_cs_market_rank", "市场排名数据", "xlsx", "import-tmallcs"},
		// 京东
		{"京东", "op_jd_shop_daily", "销售数据", "xlsx", "import-jd"},
		{"京东", "op_jd_affiliate_daily", "推广_京东联盟", "xlsx", "import-jd"},
		{"京东", "op_jd_customer_daily", "客户数据_洞察", "xlsx", "import-jd"},
		{"京东", "op_jd_customer_type_daily", "客户数据_新老客", "xlsx", "import-jd"},
		{"京东", "op_jd_promo_sku_daily", "营销数据_便宜包邮", "xlsx", "import-jd"},
		{"京东", "op_jd_promo_daily", "营销数据_百亿补贴_秒杀活动", "xlsx", "import-jd"},
		{"京东", "op_jd_industry_keyword", "交易榜", "xlsx", "import-jd"},
		{"京东", "op_jd_industry_rank", "热搜榜_飙升榜", "xlsx", "import-jd"},
		{"京东", "op_jd_campaign_daily", "推广_京准通全站_非全站", "xlsx", "import-promo"},
		// 拼多多
		{"拼多多", "op_pdd_shop_daily", "销售数据_交易概况", "xlsx", "import-pdd"},
		{"拼多多", "op_pdd_goods_daily", "销售数据_商品概况", "xlsx", "import-pdd"},
		{"拼多多", "op_pdd_goods_detail", "销售数据_商品数据", "json", "import-pdd"},
		{"拼多多", "op_pdd_campaign_daily", "推广数据_商品推广_明星店铺_直播推广", "xlsx", "import-pdd"},
		{"拼多多", "op_pdd_video_daily", "推广数据_多多视频", "json", "import-pdd"},
		{"拼多多", "op_pdd_service_overview", "销售数据_服务概况", "xlsx", "import-pdd"},
		{"拼多多", "op_pdd_cs_service_daily", "客服_服务数据", "xlsx", "import-pdd"},
		{"拼多多", "op_pdd_cs_sales_daily", "客服_销售数据", "xlsx", "import-pdd"},
		// 抖音
		{"抖音", "op_douyin_live_daily", "自营_直播数据", "xlsx", "import-douyin"},
		{"抖音", "op_douyin_goods_daily", "自营_商品数据", "xlsx", "import-douyin"},
		{"抖音", "op_douyin_ad_live_daily", "自营_推广直播间画面", "xlsx", "import-douyin"},
		{"抖音", "op_douyin_ad_material_daily", "自营_推广视频素材", "xlsx", "import-douyin"},
		{"抖音", "op_douyin_channel_daily", "自营_渠道分析", "json", "import-douyin"},
		{"抖音", "op_douyin_funnel_daily", "自营_转化漏斗", "json", "import-douyin"},
		{"抖音", "op_douyin_anchor_daily", "自营_主播分析", "xlsx", "import-douyin"},
		// 抖音分销
		{"抖音分销", "op_douyin_dist_product_daily", "推商品", "xlsx", "import-douyin-dist"},
		{"抖音分销", "op_douyin_dist_account_daily", "推抖音号", "xlsx", "import-douyin-dist"},
		{"抖音分销", "op_douyin_dist_material_daily", "推素材", "xlsx", "import-douyin-dist"},
		{"抖音分销", "op_douyin_dist_promote_hourly", "随心推", "json", "import-douyin-dist"},
		// 唯品会
		{"唯品会", "op_vip_shop_daily", "销售数据_经营", "xlsx", "import-vip"},
		{"唯品会", "op_vip_cancel", "销售数据_取消金额", "json", "import-vip"},
		{"唯品会", "op_vip_targetmax", "推广_TargetMax", "json", "import-vip"},
		{"唯品会", "op_vip_weixiangke", "推广_唯享客", "json", "import-vip"},
		// 京东自营（客服）
		{"京东自营", "op_jd_cs_workload_daily", "客服_服务工作量", "xlsx", "import-customer"},
		{"京东自营", "op_jd_cs_sales_perf_daily", "客服_销售绩效", "xlsx", "import-customer"},
		// 抖音（客服）
		{"抖音(客服)", "op_douyin_cs_feige_daily", "飞鸽_客服表现", "xlsx", "import-customer"},
		// 快手（客服）
		{"快手", "op_kuaishou_cs_assessment_daily", "客服_考核数据", "xlsx", "import-customer"},
		// 小红书（客服）
		{"小红书", "op_xhs_cs_analysis_daily", "客服数据_客服分析", "json", "import-customer"},
		{"小红书", "op_xhs_cs_trend_daily", "从客服分析json解析", "json", "import-customer"},
		{"小红书", "op_xhs_cs_excellent_trend_daily", "从客服分析json解析", "json", "import-customer"},
		// 飞瓜
		{"飞瓜", "fg_creator_roster", "飞瓜_达人数据_达人归属", "xlsx", "import-feigua"},
	}

	writeJSON(w, items)
}

// DBColumnInfo 表示数据库列信息
type DBColumnInfo struct {
	ColumnName    string `json:"column_name"`
	ColumnType    string `json:"column_type"`
	IsNullable    string `json:"is_nullable"`
	ColumnKey     string `json:"column_key"`
	ColumnDefault string `json:"column_default"`
	ColumnComment string `json:"column_comment"`
}

// DBTableInfo 表示数据库表信息（含列）
type DBTableInfo struct {
	TableName    string         `json:"table_name"`
	TableComment string         `json:"table_comment"`
	Columns      []DBColumnInfo `json:"columns"`
}

// yyyymmRe 用于识别按月份分表的表名后缀（如 _202401）
var yyyymmRe = regexp.MustCompile(`^(.+)_(\d{6})$`)

// GetDBDictionary 查询 bi_dashboard 数据库的表结构并返回数据字典
func (h *DashboardHandler) GetDBDictionary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 查询所有基础表（排除模板表）
	tableRows, err := h.DB.QueryContext(r.Context(),
		`SELECT TABLE_NAME, TABLE_COMMENT
		 FROM information_schema.TABLES
		 WHERE TABLE_SCHEMA='bi_dashboard' AND TABLE_TYPE='BASE TABLE'
		 ORDER BY TABLE_NAME`)
	if err != nil {
		http.Error(w, "db query error", http.StatusInternalServerError)
		return
	}
	defer tableRows.Close()

	type rawTable struct {
		name    string
		comment string
	}

	var allTables []rawTable
	for tableRows.Next() {
		var t rawTable
		if err := tableRows.Scan(&t.name, &t.comment); err != nil {
			http.Error(w, "db scan error", http.StatusInternalServerError)
			return
		}
		allTables = append(allTables, t)
	}
	if err := tableRows.Err(); err != nil {
		http.Error(w, "db rows error", http.StatusInternalServerError)
		return
	}

	// 过滤：排除 _template 结尾的表；对 YYYYMM 分表只保留第一个（最早月份）
	seenPattern := map[string]bool{}
	var filteredTables []rawTable
	for _, t := range allTables {
		// 排除模板表
		if strings.HasSuffix(t.name, "_template") {
			continue
		}
		// 检查是否是 YYYYMM 分表
		if m := yyyymmRe.FindStringSubmatch(t.name); m != nil {
			base := m[1]
			if seenPattern[base] {
				// 已见过该基础表名，跳过后续月份
				continue
			}
			seenPattern[base] = true
		}
		filteredTables = append(filteredTables, t)
	}

	// 为每个表查询列信息
	result := make([]DBTableInfo, 0, len(filteredTables))
	for _, t := range filteredTables {
		colRows, err := h.DB.QueryContext(r.Context(),
			`SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY,
			        IFNULL(COLUMN_DEFAULT,''), COLUMN_COMMENT
			 FROM information_schema.COLUMNS
			 WHERE TABLE_SCHEMA='bi_dashboard' AND TABLE_NAME=?
			 ORDER BY ORDINAL_POSITION`, t.name)
		if err != nil {
			http.Error(w, "db column query error", http.StatusInternalServerError)
			return
		}

		var cols []DBColumnInfo
		for colRows.Next() {
			var c DBColumnInfo
			if err := colRows.Scan(&c.ColumnName, &c.ColumnType, &c.IsNullable,
				&c.ColumnKey, &c.ColumnDefault, &c.ColumnComment); err != nil {
				colRows.Close()
				http.Error(w, "db column scan error", http.StatusInternalServerError)
				return
			}
			cols = append(cols, c)
		}
		colRows.Close()
		if err := colRows.Err(); err != nil {
			http.Error(w, "db column rows error", http.StatusInternalServerError)
			return
		}

		result = append(result, DBTableInfo{
			TableName:    t.name,
			TableComment: t.comment,
			Columns:      cols,
		})
	}

	writeJSON(w, result)
}
