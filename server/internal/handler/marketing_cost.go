package handler

import (
	"database/sql"
	"net/http"
	"sort"
	"strings"
)

// GetMarketingCost 营销费用（多平台CPC+CPS跨店铺汇总）
// 参数: platform=all|tmall|jd|pdd|tmall_cs, shop=店铺名(可选)
func (h *DashboardHandler) GetMarketingCost(w http.ResponseWriter, r *http.Request) {
	start, end := getDateRange(r, h.DB)
	trendStart, trendEnd := getTrendDateRange(start, end)
	shop := r.URL.Query().Get("shop")
	platform := r.URL.Query().Get("platform")
	if platform == "" {
		platform = "all"
	}
	if writeScopeError(w, requireDomainAccess(r, "ops", "finance")) || writeScopeError(w, requireShopAccess(r, shop)) {
		return
	}

	payload, _ := authPayloadFromContext(r)
	allowedPlatforms := []string{}
	allowedShops := []string{}
	if payload != nil && !payload.IsSuperAdmin {
		allowedPlatforms = payload.DataScopes.Platforms
		allowedShops = payload.DataScopes.Shops
	}

	shopCond := ""
	var shopArgs []interface{}
	if shop != "" && shop != "all" {
		shopCond = " AND shop_name = ?"
		shopArgs = []interface{}{shop}
	}
	if shopCond == "" && len(allowedShops) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(allowedShops)), ",")
		shopCond = " AND shop_name IN (" + placeholders + ")"
		for _, allowedShop := range allowedShops {
			shopArgs = append(shopArgs, allowedShop)
		}
	}

	// 通用结构(CpcDaily/CpsDaily 用包级别定义)
	type ShopCost struct {
		ShopName  string  `json:"shopName"`
		Cost      float64 `json:"cost"`
		PayAmount float64 `json:"payAmount"`
		ROI       float64 `json:"roi"`
		Clicks    int     `json:"clicks"`
	}
	type DetailRow struct {
		Platform  string  `json:"platform"`
		Name      string  `json:"name"`
		Cost      float64 `json:"cost"`
		PayAmount float64 `json:"payAmount"`
		ROI       float64 `json:"roi"`
		Clicks    int     `json:"clicks"`
		AvgCpc    float64 `json:"avgCpc"`
		IsContent bool    `json:"isContent"` // true=内容推广(无投放费用，cost/roi/cpc 不参与对比)
	}
	type SkuTopRow struct {
		ShopName    string  `json:"shopName"`
		ProductID   string  `json:"productId"`
		ProductName string  `json:"productName"`
		Cost        float64 `json:"cost"`
		PayAmount   float64 `json:"payAmount"`
		ROI         float64 `json:"roi"`
		Clicks      int     `json:"clicks"`
	}
	var tmallSkuTop []SkuTopRow
	var pddSkuTop []SkuTopRow

	var cpcDaily []CpcDaily
	var cpsDaily []CpsDaily
	var shopCosts []ShopCost
	var details []DetailRow
	var shops []string
	hasCps := false

	// 根据平台查不同的表
	queryPlatforms := []string{platform}
	if platform == "all" {
		queryPlatforms = []string{"tmall", "jd", "pdd", "tmall_cs"}
	}
	if len(allowedPlatforms) > 0 {
		filtered := []string{}
		for _, plat := range queryPlatforms {
			if containsString(allowedPlatforms, plat) {
				filtered = append(filtered, plat)
			}
		}
		if platform != "all" && len(filtered) == 0 {
			writeError(w, http.StatusForbidden, "forbidden by data scope")
			return
		}
		queryPlatforms = filtered
	}
	if len(queryPlatforms) == 0 {
		writeError(w, http.StatusForbidden, "forbidden by data scope")
		return
	}

	// 趋势用扩展范围，汇总用原始范围
	trendShopArgs := append([]interface{}{trendStart, trendEnd}, shopArgs...)
	_ = trendShopArgs // 预定义，下面按需用

	for _, plat := range queryPlatforms {
		switch plat {
		case "tmall":
			// 天猫CPC(万象台)
			args := append([]interface{}{trendStart, trendEnd}, shopArgs...)
			rows, ok := queryRowsOrWriteError(w, h.DB, `SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
				ROUND(SUM(cost),2), ROUND(SUM(total_pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(total_pay_amount)/SUM(cost),2) ELSE 0 END,
				SUM(clicks), SUM(impressions)
				FROM op_tmall_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY stat_date ORDER BY stat_date`, args...)
			if !ok {
				return
			}
			if rows != nil {
				for rows.Next() {
					var c CpcDaily
					if writeDatabaseError(w, rows.Scan(&c.Date, &c.Cost, &c.PayAmount, &c.ROI, &c.Clicks, &c.Impr)) {
						rows.Close()
						return
					}
					cpcDaily = mergeCpcDaily(cpcDaily, c)
				}
				if writeDatabaseError(w, rows.Err()) {
					rows.Close()
					return
				}
				rows.Close()
			}
			// 天猫CPS(淘宝联盟)
			hasCps = true
			args2 := append([]interface{}{trendStart, trendEnd}, shopArgs...)
			rows2, ok := queryRowsOrWriteError(w, h.DB, `SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
				ROUND(SUM(settle_amount),2), ROUND(SUM(settle_total_cost),2), SUM(pay_users)
				FROM op_tmall_cps_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY stat_date ORDER BY stat_date`, args2...)
			if !ok {
				return
			}
			if rows2 != nil {
				for rows2.Next() {
					var c CpsDaily
					if writeDatabaseError(w, rows2.Scan(&c.Date, &c.PayAmount, &c.PayCommission, &c.PayUsers)) {
						rows2.Close()
						return
					}
					cpsDaily = mergeCpsDaily(cpsDaily, c)
				}
				if writeDatabaseError(w, rows2.Err()) {
					rows2.Close()
					return
				}
				rows2.Close()
			}
			// 天猫店铺CPC
			sArgs := append([]interface{}{start, end}, shopArgs...)
			sRows, ok := queryRowsOrWriteError(w, h.DB, `SELECT shop_name, ROUND(SUM(cost),2), ROUND(SUM(total_pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(total_pay_amount)/SUM(cost),2) ELSE 0 END, SUM(clicks)
				FROM op_tmall_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY shop_name ORDER BY SUM(cost) DESC`, sArgs...)
			if !ok {
				return
			}
			if sRows != nil {
				for sRows.Next() {
					var s ShopCost
					if writeDatabaseError(w, sRows.Scan(&s.ShopName, &s.Cost, &s.PayAmount, &s.ROI, &s.Clicks)) {
						sRows.Close()
						return
					}
					shopCosts = append(shopCosts, s)
				}
				if writeDatabaseError(w, sRows.Err()) {
					sRows.Close()
					return
				}
				sRows.Close()
			}
			// 天猫场景明细
			dArgs := append([]interface{}{start, end}, shopArgs...)
			dRows, ok := queryRowsOrWriteError(w, h.DB, `SELECT scene_name, ROUND(SUM(cost),2), ROUND(SUM(total_pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(total_pay_amount)/SUM(cost),2) ELSE 0 END,
				SUM(clicks),
				CASE WHEN SUM(clicks)>0 THEN ROUND(SUM(cost)/SUM(clicks),2) ELSE 0 END
				FROM op_tmall_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY scene_name ORDER BY SUM(cost) DESC`, dArgs...)
			if !ok {
				return
			}
			if dRows != nil {
				for dRows.Next() {
					var d DetailRow
					d.Platform = "天猫"
					if writeDatabaseError(w, dRows.Scan(&d.Name, &d.Cost, &d.PayAmount, &d.ROI, &d.Clicks, &d.AvgCpc)) {
						dRows.Close()
						return
					}
					details = append(details, d)
				}
				if writeDatabaseError(w, dRows.Err()) {
					dRows.Close()
					return
				}
				dRows.Close()
			}
			// 天猫万象台 SKU Top 20：仅在用户单选"天猫"平台时返回（all 时不返回避免视觉干扰）
			// 按 (店铺, 商品) 分组，避免同商品在不同店合并失真
			if platform == "tmall" {
				skuArgs := append([]interface{}{start, end}, shopArgs...)
				skuRows, skuErr := h.DB.Query(`SELECT shop_name, product_id,
					MAX(product_name) AS product_name,
					ROUND(SUM(cost),2) AS cost,
					ROUND(SUM(total_pay_amount),2) AS pay,
					CASE WHEN SUM(cost)>0 THEN ROUND(SUM(total_pay_amount)/SUM(cost),2) ELSE 0 END AS roi,
					SUM(clicks) AS clicks
					FROM op_tmall_campaign_detail_daily
					WHERE stat_date BETWEEN ? AND ?`+shopCond+`
					AND entity_type='商品'
					GROUP BY shop_name, product_id
					HAVING cost > 0
					ORDER BY cost DESC LIMIT 20`, skuArgs...)
				if skuErr == nil {
					for skuRows.Next() {
						var s SkuTopRow
						if err := skuRows.Scan(&s.ShopName, &s.ProductID, &s.ProductName, &s.Cost, &s.PayAmount, &s.ROI, &s.Clicks); err != nil {
							continue
						}
						tmallSkuTop = append(tmallSkuTop, s)
					}
					skuRows.Close()
				}
			}

		case "jd":
			// 京东CPC(京准通)
			args := append([]interface{}{trendStart, trendEnd}, shopArgs...)
			rows, ok := queryRowsOrWriteError(w, h.DB, `SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
				ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END,
				SUM(clicks), SUM(impressions)
				FROM op_jd_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY stat_date ORDER BY stat_date`, args...)
			if !ok {
				return
			}
			if rows != nil {
				for rows.Next() {
					var c CpcDaily
					if writeDatabaseError(w, rows.Scan(&c.Date, &c.Cost, &c.PayAmount, &c.ROI, &c.Clicks, &c.Impr)) {
						rows.Close()
						return
					}
					cpcDaily = mergeCpcDaily(cpcDaily, c)
				}
				if writeDatabaseError(w, rows.Err()) {
					rows.Close()
					return
				}
				rows.Close()
			}
			// 京东CPS(京东联盟)
			hasCps = true
			args2 := append([]interface{}{trendStart, trendEnd}, shopArgs...)
			rows2, ok := queryRowsOrWriteError(w, h.DB, `SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
				ROUND(SUM(complete_amount),2), ROUND(SUM(actual_commission),2), SUM(complete_buyers)
				FROM op_jd_affiliate_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY stat_date ORDER BY stat_date`, args2...)
			if !ok {
				return
			}
			if rows2 != nil {
				for rows2.Next() {
					var c CpsDaily
					if writeDatabaseError(w, rows2.Scan(&c.Date, &c.PayAmount, &c.PayCommission, &c.PayUsers)) {
						rows2.Close()
						return
					}
					cpsDaily = mergeCpsDaily(cpsDaily, c)
				}
				if writeDatabaseError(w, rows2.Err()) {
					rows2.Close()
					return
				}
				rows2.Close()
			}
			// 京东店铺CPC
			sArgs := append([]interface{}{start, end}, shopArgs...)
			sRows, ok := queryRowsOrWriteError(w, h.DB, `SELECT shop_name, ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END, SUM(clicks)
				FROM op_jd_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY shop_name ORDER BY SUM(cost) DESC`, sArgs...)
			if !ok {
				return
			}
			if sRows != nil {
				for sRows.Next() {
					var s ShopCost
					if writeDatabaseError(w, sRows.Scan(&s.ShopName, &s.Cost, &s.PayAmount, &s.ROI, &s.Clicks)) {
						sRows.Close()
						return
					}
					shopCosts = append(shopCosts, s)
				}
				if writeDatabaseError(w, sRows.Err()) {
					sRows.Close()
					return
				}
				sRows.Close()
			}
			// 京东推广类型明细
			dArgs := append([]interface{}{start, end}, shopArgs...)
			dRows, ok := queryRowsOrWriteError(w, h.DB, `SELECT promo_type, ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END,
				SUM(clicks),
				CASE WHEN SUM(clicks)>0 THEN ROUND(SUM(cost)/SUM(clicks),2) ELSE 0 END
				FROM op_jd_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY promo_type ORDER BY SUM(cost) DESC`, dArgs...)
			if !ok {
				return
			}
			if dRows != nil {
				for dRows.Next() {
					var d DetailRow
					d.Platform = "京东"
					if writeDatabaseError(w, dRows.Scan(&d.Name, &d.Cost, &d.PayAmount, &d.ROI, &d.Clicks, &d.AvgCpc)) {
						dRows.Close()
						return
					}
					details = append(details, d)
				}
				if writeDatabaseError(w, dRows.Err()) {
					dRows.Close()
					return
				}
				dRows.Close()
			}

		case "pdd":
			// 拼多多CPC
			args := append([]interface{}{trendStart, trendEnd}, shopArgs...)
			rows, ok := queryRowsOrWriteError(w, h.DB, `SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
				ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END,
				SUM(clicks), SUM(impressions)
				FROM op_pdd_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY stat_date ORDER BY stat_date`, args...)
			if !ok {
				return
			}
			if rows != nil {
				for rows.Next() {
					var c CpcDaily
					if writeDatabaseError(w, rows.Scan(&c.Date, &c.Cost, &c.PayAmount, &c.ROI, &c.Clicks, &c.Impr)) {
						rows.Close()
						return
					}
					cpcDaily = mergeCpcDaily(cpcDaily, c)
				}
				if writeDatabaseError(w, rows.Err()) {
					rows.Close()
					return
				}
				rows.Close()
			}
			// 拼多多店铺CPC
			sArgs := append([]interface{}{start, end}, shopArgs...)
			sRows, ok := queryRowsOrWriteError(w, h.DB, `SELECT shop_name, ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END, SUM(clicks)
				FROM op_pdd_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY shop_name ORDER BY SUM(cost) DESC`, sArgs...)
			if !ok {
				return
			}
			if sRows != nil {
				for sRows.Next() {
					var s ShopCost
					if writeDatabaseError(w, sRows.Scan(&s.ShopName, &s.Cost, &s.PayAmount, &s.ROI, &s.Clicks)) {
						sRows.Close()
						return
					}
					shopCosts = append(shopCosts, s)
				}
				if writeDatabaseError(w, sRows.Err()) {
					sRows.Close()
					return
				}
				sRows.Close()
			}
			// 拼多多推广类型明细
			dArgs := append([]interface{}{start, end}, shopArgs...)
			dRows, ok := queryRowsOrWriteError(w, h.DB, `SELECT promo_type, ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END,
				SUM(clicks),
				CASE WHEN SUM(clicks)>0 THEN ROUND(SUM(cost)/SUM(clicks),2) ELSE 0 END
				FROM op_pdd_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY promo_type ORDER BY SUM(cost) DESC`, dArgs...)
			if !ok {
				return
			}
			if dRows != nil {
				for dRows.Next() {
					var d DetailRow
					d.Platform = "拼多多"
					if writeDatabaseError(w, dRows.Scan(&d.Name, &d.Cost, &d.PayAmount, &d.ROI, &d.Clicks, &d.AvgCpc)) {
						dRows.Close()
						return
					}
					details = append(details, d)
				}
				if writeDatabaseError(w, dRows.Err()) {
					dRows.Close()
					return
				}
				dRows.Close()
			}

			// 拼多多 多多视频（内容推广，无投放费用）
			vArgs := append([]interface{}{start, end}, shopArgs...)
			var vGmv sql.NullFloat64
			var vClicks sql.NullInt64
			var vOrders sql.NullInt64
			vErr := h.DB.QueryRow(`SELECT ROUND(SUM(total_gmv),2), SUM(goods_click_cnt), SUM(order_count)
				FROM op_pdd_video_daily WHERE stat_date BETWEEN ? AND ?`+shopCond, vArgs...).
				Scan(&vGmv, &vClicks, &vOrders)
			if vErr == nil && (vGmv.Float64 > 0 || vOrders.Int64 > 0) {
				details = append(details, DetailRow{
					Platform:  "拼多多",
					Name:      "多多视频（内容）",
					Cost:      0,
					PayAmount: vGmv.Float64,
					ROI:       0,
					Clicks:    int(vClicks.Int64),
					AvgCpc:    0,
					IsContent: true,
				})
			}

			// 拼多多商品推广 SKU Top 20：仅在用户单选"拼多多"平台时返回
			if platform == "pdd" {
				skuArgs := append([]interface{}{start, end}, shopArgs...)
				skuRows, skuErr := h.DB.Query(`SELECT shop_name, goods_id,
					MAX(goods_name) AS goods_name,
					ROUND(SUM(cost_total),2) AS cost,
					ROUND(SUM(pay_amount),2) AS pay,
					CASE WHEN SUM(cost_total)>0 THEN ROUND(SUM(pay_amount)/SUM(cost_total),2) ELSE 0 END AS roi,
					SUM(clicks) AS clicks
					FROM op_pdd_campaign_goods_daily
					WHERE stat_date BETWEEN ? AND ?`+shopCond+`
					GROUP BY shop_name, goods_id
					HAVING cost > 0
					ORDER BY cost DESC LIMIT 20`, skuArgs...)
				if skuErr == nil {
					for skuRows.Next() {
						var s SkuTopRow
						if err := skuRows.Scan(&s.ShopName, &s.ProductID, &s.ProductName, &s.Cost, &s.PayAmount, &s.ROI, &s.Clicks); err != nil {
							continue
						}
						pddSkuTop = append(pddSkuTop, s)
					}
					skuRows.Close()
				}
			}

		case "tmall_cs":
			// 天猫超市CPC
			args := append([]interface{}{trendStart, trendEnd}, shopArgs...)
			rows, ok := queryRowsOrWriteError(w, h.DB, `SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
				ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END,
				SUM(clicks), SUM(impressions)
				FROM op_tmall_cs_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY stat_date ORDER BY stat_date`, args...)
			if !ok {
				return
			}
			if rows != nil {
				for rows.Next() {
					var c CpcDaily
					if writeDatabaseError(w, rows.Scan(&c.Date, &c.Cost, &c.PayAmount, &c.ROI, &c.Clicks, &c.Impr)) {
						rows.Close()
						return
					}
					cpcDaily = mergeCpcDaily(cpcDaily, c)
				}
				if writeDatabaseError(w, rows.Err()) {
					rows.Close()
					return
				}
				rows.Close()
			}
			// 天猫超市店铺CPC(一盘货/寄售双店对比)
			sArgs := append([]interface{}{start, end}, shopArgs...)
			sRows, ok := queryRowsOrWriteError(w, h.DB, `SELECT shop_name, ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END, SUM(clicks)
				FROM op_tmall_cs_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY shop_name ORDER BY SUM(cost) DESC`, sArgs...)
			if !ok {
				return
			}
			if sRows != nil {
				for sRows.Next() {
					var s ShopCost
					if writeDatabaseError(w, sRows.Scan(&s.ShopName, &s.Cost, &s.PayAmount, &s.ROI, &s.Clicks)) {
						sRows.Close()
						return
					}
					shopCosts = append(shopCosts, s)
				}
				if writeDatabaseError(w, sRows.Err()) {
					sRows.Close()
					return
				}
				sRows.Close()
			}
			// 天猫超市推广明细：无界/智多星 拆到场景级（每个场景独立行，name 带括号标父类），淘客单独一行
			// 无界场景：op_tmall_cs_wujie_scene_daily
			wujieArgs := append([]interface{}{start, end}, shopArgs...)
			wRows, wErr := h.DB.Query(`SELECT scene_name, ROUND(SUM(cost),2), ROUND(SUM(total_pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(total_pay_amount)/SUM(cost),2) ELSE 0 END,
				SUM(clicks),
				CASE WHEN SUM(clicks)>0 THEN ROUND(SUM(cost)/SUM(clicks),2) ELSE 0 END
				FROM op_tmall_cs_wujie_scene_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				AND scene_name IS NOT NULL AND scene_name != ''
				GROUP BY scene_name ORDER BY SUM(cost) DESC`, wujieArgs...)
			if wErr == nil {
				for wRows.Next() {
					var d DetailRow
					var sceneName string
					if err := wRows.Scan(&sceneName, &d.Cost, &d.PayAmount, &d.ROI, &d.Clicks, &d.AvgCpc); err != nil {
						continue
					}
					d.Platform = "天猫超市"
					d.Name = sceneName + "（无界）"
					details = append(details, d)
				}
				wRows.Close()
			}

			// 智多星场景：op_tmall_cs_smart_plan_daily
			smartArgs := append([]interface{}{start, end}, shopArgs...)
			spRows, spErr := h.DB.Query(`SELECT campaign_scene, ROUND(SUM(cost),2), ROUND(SUM(total_pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(total_pay_amount)/SUM(cost),2) ELSE 0 END,
				SUM(clicks),
				CASE WHEN SUM(clicks)>0 THEN ROUND(SUM(cost)/SUM(clicks),2) ELSE 0 END
				FROM op_tmall_cs_smart_plan_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				AND campaign_scene IS NOT NULL AND campaign_scene != ''
				GROUP BY campaign_scene ORDER BY SUM(cost) DESC`, smartArgs...)
			if spErr == nil {
				for spRows.Next() {
					var d DetailRow
					var sceneName string
					if err := spRows.Scan(&sceneName, &d.Cost, &d.PayAmount, &d.ROI, &d.Clicks, &d.AvgCpc); err != nil {
						continue
					}
					d.Platform = "天猫超市"
					d.Name = sceneName + "（智多星）"
					details = append(details, d)
				}
				spRows.Close()
			}

			// 淘客单独一行（无场景维度，从 op_tmall_cs_campaign_daily promo_type='淘客' 取）
			tkArgs := append([]interface{}{start, end}, shopArgs...)
			var tkCost, tkPay sql.NullFloat64
			var tkClicks sql.NullInt64
			tkErr := h.DB.QueryRow(`SELECT ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2), SUM(clicks)
				FROM op_tmall_cs_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				AND promo_type='淘客'`, tkArgs...).Scan(&tkCost, &tkPay, &tkClicks)
			if tkErr == nil && (tkCost.Float64 > 0 || tkPay.Float64 > 0) {
				roi := 0.0
				if tkCost.Float64 > 0 {
					roi = tkPay.Float64 / tkCost.Float64
				}
				cpc := 0.0
				if tkClicks.Int64 > 0 {
					cpc = tkCost.Float64 / float64(tkClicks.Int64)
				}
				details = append(details, DetailRow{
					Platform:  "天猫超市",
					Name:      "淘客",
					Cost:      tkCost.Float64,
					PayAmount: tkPay.Float64,
					ROI:       roi,
					Clicks:    int(tkClicks.Int64),
					AvgCpc:    cpc,
				})
			}

		}
	}

	// 店铺列表(用于筛选，不受shop参数影响，但保留权限过滤)
	shopListCond := ""
	var shopListArgs []interface{}
	if len(allowedShops) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(allowedShops)), ",")
		shopListCond = " AND shop_name IN (" + placeholders + ")"
		for _, s := range allowedShops {
			shopListArgs = append(shopListArgs, s)
		}
	}
	shopQueries := []string{}
	shopQueryArgs := []interface{}{}
	for _, plat := range queryPlatforms {
		switch plat {
		case "tmall":
			shopQueries = append(shopQueries, "SELECT DISTINCT shop_name FROM op_tmall_campaign_daily WHERE stat_date BETWEEN ? AND ?"+shopListCond)
			shopQueryArgs = append(shopQueryArgs, start, end)
			shopQueryArgs = append(shopQueryArgs, shopListArgs...)
			shopQueries = append(shopQueries, "SELECT DISTINCT shop_name FROM op_tmall_cps_daily WHERE stat_date BETWEEN ? AND ?"+shopListCond)
			shopQueryArgs = append(shopQueryArgs, start, end)
			shopQueryArgs = append(shopQueryArgs, shopListArgs...)
		case "jd":
			shopQueries = append(shopQueries, "SELECT DISTINCT shop_name FROM op_jd_campaign_daily WHERE stat_date BETWEEN ? AND ?"+shopListCond)
			shopQueryArgs = append(shopQueryArgs, start, end)
			shopQueryArgs = append(shopQueryArgs, shopListArgs...)
			shopQueries = append(shopQueries, "SELECT DISTINCT shop_name FROM op_jd_affiliate_daily WHERE stat_date BETWEEN ? AND ?"+shopListCond)
			shopQueryArgs = append(shopQueryArgs, start, end)
			shopQueryArgs = append(shopQueryArgs, shopListArgs...)
		case "pdd":
			shopQueries = append(shopQueries, "SELECT DISTINCT shop_name FROM op_pdd_campaign_daily WHERE stat_date BETWEEN ? AND ?"+shopListCond)
			shopQueryArgs = append(shopQueryArgs, start, end)
			shopQueryArgs = append(shopQueryArgs, shopListArgs...)
		case "tmall_cs":
			shopQueries = append(shopQueries, "SELECT DISTINCT shop_name FROM op_tmall_cs_campaign_daily WHERE stat_date BETWEEN ? AND ?"+shopListCond)
			shopQueryArgs = append(shopQueryArgs, start, end)
			shopQueryArgs = append(shopQueryArgs, shopListArgs...)
		}
	}
	if len(shopQueries) > 0 {
		fullQuery := joinStrings(shopQueries, " UNION ") + " ORDER BY 1"
		sRows, ok := queryRowsOrWriteError(w, h.DB, fullQuery, shopQueryArgs...)
		if !ok {
			return
		}
		if sRows != nil {
			for sRows.Next() {
				var s string
				if writeDatabaseError(w, sRows.Scan(&s)) {
					sRows.Close()
					return
				}
				shops = append(shops, s)
			}
			if writeDatabaseError(w, sRows.Err()) {
				sRows.Close()
				return
			}
			sRows.Close()
		}
	}

	sort.Slice(cpcDaily, func(i, j int) bool { return cpcDaily[i].Date < cpcDaily[j].Date })
	sort.Slice(cpsDaily, func(i, j int) bool { return cpsDaily[i].Date < cpsDaily[j].Date })

	writeJSON(w, map[string]interface{}{
		"cpcDaily":     cpcDaily,
		"cpsDaily":     cpsDaily,
		"shopCosts":    shopCosts,
		"details":      details,
		"tmallSkuTop":  tmallSkuTop,
		"pddSkuTop":    pddSkuTop,
		"shops":        shops,
		"hasCps":       hasCps,
		"dateRange":    map[string]string{"start": start, "end": end},
		"trendRange":   map[string]string{"start": trendStart, "end": trendEnd},
	})
}

// mergeCpcDaily 合并同日CPC数据(跨平台汇总时)
func mergeCpcDaily(arr []CpcDaily, item CpcDaily) []CpcDaily {
	for i, a := range arr {
		if a.Date == item.Date {
			arr[i].Cost = round2(arr[i].Cost + item.Cost)
			arr[i].PayAmount = round2(arr[i].PayAmount + item.PayAmount)
			arr[i].Clicks += item.Clicks
			arr[i].Impr += item.Impr
			if arr[i].Cost > 0 {
				arr[i].ROI = float64(int(arr[i].PayAmount/arr[i].Cost*100)) / 100
			}
			return arr
		}
	}
	item.Cost = round2(item.Cost)
	item.PayAmount = round2(item.PayAmount)
	return append(arr, item)
}

type CpcDaily struct {
	Date      string  `json:"date"`
	Cost      float64 `json:"cost"`
	PayAmount float64 `json:"payAmount"`
	ROI       float64 `json:"roi"`
	Clicks    int     `json:"clicks"`
	Impr      int64   `json:"impressions"`
}

// mergeCpsDaily 合并同日CPS数据
func mergeCpsDaily(arr []CpsDaily, item CpsDaily) []CpsDaily {
	for i, a := range arr {
		if a.Date == item.Date {
			arr[i].PayAmount = round2(arr[i].PayAmount + item.PayAmount)
			arr[i].PayCommission = round2(arr[i].PayCommission + item.PayCommission)
			arr[i].PayUsers += item.PayUsers
			return arr
		}
	}
	item.PayAmount = round2(item.PayAmount)
	item.PayCommission = round2(item.PayCommission)
	return append(arr, item)
}
