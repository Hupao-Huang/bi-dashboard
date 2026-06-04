package handler

// 计划看板"钱侧"特殊渠道调拨当销售金额聚合。
//
// 背景: 京东(清心湖自营)/猫超(寄售)/朴朴这 3 个特殊渠道靠调拨单发货当销售(口径见 special_channel.go),
//   它们在 sales_goods_summary 的销售行挂在自己的平台仓(京东自营仓/天猫超市仓), 不在计划 8 仓白名单,
//   所以计划看板当前 GMV 里这 3 渠道贡献≈0 → 钱侧是干净相加(不像综合看板 helper 要减 salesExcluded)。
// 跟综合看板 loadEcommerceAllotAdjustment 的区别: 这里(1)按计划看板 10 核心品类筛 (2)含朴朴共 3 渠道 (3)纯加法。
//   故不复用那个 helper, 单独建一个 plan-scoped 的。
//
// 渠道→部门映射(跟 special_channel.go / 计划看板 department 口径一致):
//   京东 + 猫超 → ecommerce(电商部); 朴朴 → instant_retail(即时零售部)。

type planAllotAgg struct {
	Total      float64            // 区间内 10 品类 3 渠道调拨金额合计(喂 GMV KPI)
	ByCategory map[string]float64 // 品类 → 金额(喂品类饼; 品类名同计划看板 CASE 口径)
	ByDept     map[string]float64 // 部门 → 金额(喂渠道 split; ecommerce / instant_retail)
	ByMonth    map[string]float64 // YYYY-MM → 金额(喂月度趋势)
}

// loadPlanAllot 拉取 [start,end] 区间、10 核心品类、3 特殊渠道的调拨金额, 一次查询 Go 侧聚合成 4 个口径。
// 金额可比性: excel_amount(Excel 价格表算的销售额) 与 sales_goods_summary.local_goods_amt 沿用综合看板既定口径直接相加。
func (h *DashboardHandler) loadPlanAllot(start, end string) (planAllotAgg, error) {
	agg := planAllotAgg{
		ByCategory: map[string]float64{},
		ByDept:     map[string]float64{},
		ByMonth:    map[string]float64{},
	}
	catSub, catArgs := planCategoryGoodsSubquery()
	args := append([]interface{}{start, end}, catArgs...)
	rows, err := h.DB.Query(`
		SELECT d.channel_key,
			DATE_FORMAT(o.stat_date, '%Y-%m') AS ym,
			CASE
				WHEN g.cate_full_name LIKE '成品/%' THEN SUBSTRING_INDEX(SUBSTRING_INDEX(g.cate_full_name,'/',2),'/',-1)
				WHEN g.cate_full_name IS NOT NULL AND g.cate_full_name != '' THEN g.cate_full_name
				ELSE '未分类'
			END AS category,
			IFNULL(SUM(d.excel_amount), 0) AS amt
		FROM allocate_orders o
		JOIN allocate_details d ON d.allocate_no = o.allocate_no
		LEFT JOIN (SELECT goods_no, MAX(cate_full_name) AS cate_full_name FROM goods WHERE is_delete=0 GROUP BY goods_no) g
			ON g.goods_no = d.goods_no
		WHERE o.channel_key IN ('京东','猫超','朴朴')
		  AND o.stat_date BETWEEN ? AND ?
		  AND d.goods_no IN (`+catSub+`)
		GROUP BY d.channel_key, ym, category`, args...)
	if err != nil {
		return agg, err
	}
	defer rows.Close()
	for rows.Next() {
		var channel, ym, category string
		var amt float64
		if err := rows.Scan(&channel, &ym, &category, &amt); err != nil {
			return agg, err
		}
		agg.Total += amt
		agg.ByCategory[category] += amt
		agg.ByMonth[ym] += amt
		if channel == "朴朴" {
			agg.ByDept["instant_retail"] += amt
		} else { // 京东 / 猫超
			agg.ByDept["ecommerce"] += amt
		}
	}
	return agg, rows.Err()
}
