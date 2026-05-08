package handler

import (
	"net/http"
	"time"
)

// GetTmallOps 天猫运营数据（流量转化/推广投放/会员复购）
func (h *DashboardHandler) GetTmallOps(w http.ResponseWriter, r *http.Request) {
	shop := r.URL.Query().Get("shop")
	if shop == "" {
		writeError(w, 400, "shop is required")
		return
	}
	if writeScopeError(w, requireDomainAccess(r, "ops")) || writeScopeError(w, requirePlatformAccess(r, "tmall")) || writeScopeError(w, requireShopAccess(r, shop)) {
		return
	}
	start, end := getDateRange(r, h.DB)
	trendStart, trendEnd := getOverviewTrendRange(r, start, end)
	if trendStart == start && trendEnd == end {
		// 兼容未传 trendStart/trendEnd 的旧前端：仅单日查询时扩展趋势
		if start == end {
			if eDate, err := time.Parse("2006-01-02", end); err == nil {
				trendStart = eDate.AddDate(0, 0, -13).Format("2006-01-02")
				trendEnd = end
			}
		}
	}

	// 1. 店铺流量转化日趋势 (生意参谋)
	type TrafficDaily struct {
		Date        string  `json:"date"`
		Visitors    int     `json:"visitors"`
		PageViews   int     `json:"pageViews"`
		CartBuyers  int     `json:"cartBuyers"`
		PayBuyers   int     `json:"payBuyers"`
		PayAmount   float64 `json:"payAmount"`
		PayConvRate float64 `json:"payConvRate"`
		UvValue     float64 `json:"uvValue"`
		BounceRate  float64 `json:"bounceRate"`
	}
	var traffic []TrafficDaily
	tRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'), visitors, page_views,
			cart_buyers, pay_buyers, pay_amount, pay_conv_rate, uv_value, bounce_rate
		FROM op_tmall_shop_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer tRows.Close()
	for tRows.Next() {
		var t TrafficDaily
		if writeDatabaseError(w, tRows.Scan(&t.Date, &t.Visitors, &t.PageViews, &t.CartBuyers,
			&t.PayBuyers, &t.PayAmount, &t.PayConvRate, &t.UvValue, &t.BounceRate)) {
			return
		}
		traffic = append(traffic, t)
	}
	if writeDatabaseError(w, tRows.Err()) {
		return
	}

	// 2. CPC推广汇总(万象台) - 按天汇总所有场景
	type CampaignDaily struct {
		Date      string  `json:"date"`
		Cost      float64 `json:"cost"`
		PayAmount float64 `json:"payAmount"`
		ROI       float64 `json:"roi"`
		Clicks    int     `json:"clicks"`
		Impr      int     `json:"impressions"`
	}
	var campaigns []CampaignDaily
	cRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			ROUND(SUM(cost),2), ROUND(SUM(total_pay_amount),2),
			CASE WHEN SUM(cost)>0 THEN ROUND(SUM(total_pay_amount)/SUM(cost),2) ELSE 0 END,
			SUM(clicks), SUM(impressions)
		FROM op_tmall_campaign_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY stat_date ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer cRows.Close()
	for cRows.Next() {
		var c CampaignDaily
		if writeDatabaseError(w, cRows.Scan(&c.Date, &c.Cost, &c.PayAmount, &c.ROI, &c.Clicks, &c.Impr)) {
			return
		}
		campaigns = append(campaigns, c)
	}
	if writeDatabaseError(w, cRows.Err()) {
		return
	}

	// 3. CPC场景分布(万象台) - 按场景汇总(用原始范围)
	type SceneSummary struct {
		SceneName string  `json:"sceneName"`
		Cost      float64 `json:"cost"`
		PayAmount float64 `json:"payAmount"`
		ROI       float64 `json:"roi"`
		Clicks    int     `json:"clicks"`
	}
	var scenes []SceneSummary
	sRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT scene_name, ROUND(SUM(cost),2), ROUND(SUM(total_pay_amount),2),
			CASE WHEN SUM(cost)>0 THEN ROUND(SUM(total_pay_amount)/SUM(cost),2) ELSE 0 END,
			SUM(clicks)
		FROM op_tmall_campaign_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY scene_name ORDER BY SUM(cost) DESC`, shop, start, end)
	if !ok {
		return
	}
	defer sRows.Close()
	for sRows.Next() {
		var s SceneSummary
		if writeDatabaseError(w, sRows.Scan(&s.SceneName, &s.Cost, &s.PayAmount, &s.ROI, &s.Clicks)) {
			return
		}
		scenes = append(scenes, s)
	}
	if writeDatabaseError(w, sRows.Err()) {
		return
	}

	// 4. CPS推广(淘宝联盟) - 按天汇总
	type CPSDaily struct {
		Date          string  `json:"date"`
		PayAmount     float64 `json:"payAmount"`
		PayCommission float64 `json:"payCommission"`
		PayUsers      int     `json:"payUsers"`
	}
	var cps []CPSDaily
	cpsRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			ROUND(SUM(settle_amount),2), ROUND(SUM(settle_total_cost),2), SUM(pay_users)
		FROM op_tmall_cps_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY stat_date ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer cpsRows.Close()
	for cpsRows.Next() {
		var c CPSDaily
		if writeDatabaseError(w, cpsRows.Scan(&c.Date, &c.PayAmount, &c.PayCommission, &c.PayUsers)) {
			return
		}
		cps = append(cps, c)
	}
	if writeDatabaseError(w, cpsRows.Err()) {
		return
	}

	// 5. CPS按计划分布
	type CPSPlan struct {
		PlanName      string  `json:"planName"`
		PayAmount     float64 `json:"payAmount"`
		PayCommission float64 `json:"payCommission"`
		PayUsers      int     `json:"payUsers"`
	}
	var cpsPlans []CPSPlan
	cpRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT plan_name, ROUND(SUM(settle_amount),2), ROUND(SUM(settle_total_cost),2), SUM(pay_users)
		FROM op_tmall_cps_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY plan_name ORDER BY SUM(settle_amount) DESC`, shop, start, end)
	if !ok {
		return
	}
	defer cpRows.Close()
	for cpRows.Next() {
		var c CPSPlan
		if writeDatabaseError(w, cpRows.Scan(&c.PlanName, &c.PayAmount, &c.PayCommission, &c.PayUsers)) {
			return
		}
		cpsPlans = append(cpsPlans, c)
	}
	if writeDatabaseError(w, cpRows.Err()) {
		return
	}

	// 6. 会员数据
	type MemberDaily struct {
		Date            string  `json:"date"`
		PaidMemberCnt   int     `json:"paidMemberCnt"`
		MemberPayAmt    float64 `json:"memberPayAmt"`
		MemberUnitPrice float64 `json:"memberUnitPrice"`
		TotalMemberCnt  int     `json:"totalMemberCnt"`
		RepurchaseRate  float64 `json:"repurchaseRate"`
	}
	var members []MemberDaily
	mRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			paid_member_cnt, member_pay_amount, member_unit_price,
			total_member_cnt, repurchase_rate
		FROM op_tmall_member_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer mRows.Close()
	for mRows.Next() {
		var m MemberDaily
		if writeDatabaseError(w, mRows.Scan(&m.Date, &m.PaidMemberCnt, &m.MemberPayAmt,
			&m.MemberUnitPrice, &m.TotalMemberCnt, &m.RepurchaseRate)) {
			return
		}
		members = append(members, m)
	}
	if writeDatabaseError(w, mRows.Err()) {
		return
	}

	// 7. 商品TOP10 (生意参谋-商品销售)
	type GoodsItem struct {
		ProductName string  `json:"productName"`
		Visitors    int     `json:"visitors"`
		CartBuyers  int     `json:"cartBuyers"`
		PayQty      int     `json:"payQty"`
		PayAmount   float64 `json:"payAmount"`
		PayConvRate string  `json:"payConvRate"`
		PayBuyers   int     `json:"payBuyers"`
		RefundAmt   float64 `json:"refundAmount"`
	}
	var goodsTop []GoodsItem
	gRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT product_name, SUM(visitors), SUM(cart_buyers), SUM(pay_qty),
			ROUND(SUM(pay_amount),2),
			CASE WHEN SUM(visitors)>0 THEN CONCAT(ROUND(SUM(pay_buyers)/SUM(visitors)*100,2),'%%') ELSE '0%%' END,
			SUM(pay_buyers), ROUND(SUM(refund_amount),2)
		FROM op_tmall_goods_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY product_name ORDER BY SUM(pay_amount) DESC LIMIT 10`, shop, start, end)
	if !ok {
		return
	}
	defer gRows.Close()
	for gRows.Next() {
		var g GoodsItem
		if writeDatabaseError(w, gRows.Scan(&g.ProductName, &g.Visitors, &g.CartBuyers, &g.PayQty, &g.PayAmount, &g.PayConvRate, &g.PayBuyers, &g.RefundAmt)) {
			return
		}
		goodsTop = append(goodsTop, g)
	}

	// 8. 品牌数据趋势 (数据银行)
	type BrandDaily struct {
		Date           string  `json:"date"`
		MemberPayAmt   float64 `json:"memberPayAmt"`
		CustomerVolume int     `json:"customerVolume"`
		LoyalVolume    int     `json:"loyalVolume"`
		InterestVolume int     `json:"interestVolume"`
		PurchaseVolume int     `json:"purchaseVolume"`
		DeepenRatio    float64 `json:"deepenRatio"`
	}
	var brandDaily []BrandDaily
	bRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'), member_pay_amount,
			customer_volume, loyal_volume, interest_volume, purchase_volume, deepen_ratio
		FROM op_tmall_brand_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer bRows.Close()
	for bRows.Next() {
		var b BrandDaily
		if writeDatabaseError(w, bRows.Scan(&b.Date, &b.MemberPayAmt, &b.CustomerVolume, &b.LoyalVolume, &b.InterestVolume, &b.PurchaseVolume, &b.DeepenRatio)) {
			return
		}
		brandDaily = append(brandDaily, b)
	}

	// 9. 人群覆盖趋势 (达摩盘)
	type CrowdDaily struct {
		Date        string  `json:"date"`
		Coverage    int     `json:"coverage"`
		Concentrate float64 `json:"concentrate"`
		PayAmount   float64 `json:"payAmount"`
		PayUV       int     `json:"payUV"`
	}
	var crowdDaily []CrowdDaily
	crRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'), coverage,
			ta_concentrate_ratio, shop_alipay_amount, shop_alipay_uv
		FROM op_tmall_crowd_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer crRows.Close()
	for crRows.Next() {
		var c CrowdDaily
		if writeDatabaseError(w, crRows.Scan(&c.Date, &c.Coverage, &c.Concentrate, &c.PayAmount, &c.PayUV)) {
			return
		}
		crowdDaily = append(crowdDaily, c)
	}

	// 10. 行业月报 (集客)
	type IndustryMonthly struct {
		Month           string  `json:"month"`
		Category        string  `json:"category"`
		NewRatio        float64 `json:"newRatio"`
		NewSalesRatio   float64 `json:"newSalesRatio"`
		NewRepurchase30 float64 `json:"newRepurchase30d"`
		OldRatio        float64 `json:"oldRatio"`
		OldSalesRatio   float64 `json:"oldSalesRatio"`
		OldRepurchase30 float64 `json:"oldRepurchase30d"`
		UnitPrice       float64 `json:"unitPrice"`
	}
	var industry []IndustryMonthly
	iRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT stat_month, IFNULL(category,''), new_ratio, new_sales_ratio, new_repurchase_30d,
			old_ratio, old_sales_ratio, old_repurchase_30d, unit_price
		FROM op_tmall_industry_monthly
		WHERE shop_name = ? ORDER BY stat_month DESC LIMIT 12`, shop)
	if !ok {
		return
	}
	defer iRows.Close()
	for iRows.Next() {
		var i IndustryMonthly
		if writeDatabaseError(w, iRows.Scan(&i.Month, &i.Category, &i.NewRatio, &i.NewSalesRatio, &i.NewRepurchase30, &i.OldRatio, &i.OldSalesRatio, &i.OldRepurchase30, &i.UnitPrice)) {
			return
		}
		industry = append(industry, i)
	}

	// 11. 复购月报 (集客)
	type RepurchaseMonthly struct {
		Month              string  `json:"month"`
		Category           string  `json:"category"`
		ShopRepurchase30   float64 `json:"shopRepurchase30d"`
		ShopRepurchase180  float64 `json:"shopRepurchase180d"`
		ShopRepurchase360  float64 `json:"shopRepurchase360d"`
		LostRepurchase     float64 `json:"lostRepurchaseRate"`
		LastRepurchaseDays float64 `json:"lastRepurchaseDays"`
	}
	var repurchase []RepurchaseMonthly
	rRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT stat_month, IFNULL(category,''), shop_repurchase_30d, shop_repurchase_180d,
			shop_repurchase_360d, lost_repurchase_rate, last_repurchase_days
		FROM op_tmall_repurchase_monthly
		WHERE shop_name = ? ORDER BY stat_month DESC LIMIT 12`, shop)
	if !ok {
		return
	}
	defer rRows.Close()
	for rRows.Next() {
		var rp RepurchaseMonthly
		if writeDatabaseError(w, rRows.Scan(&rp.Month, &rp.Category, &rp.ShopRepurchase30, &rp.ShopRepurchase180, &rp.ShopRepurchase360, &rp.LostRepurchase, &rp.LastRepurchaseDays)) {
			return
		}
		repurchase = append(repurchase, rp)
	}

	writeJSON(w, map[string]interface{}{
		"traffic":    traffic,
		"campaigns":  campaigns,
		"scenes":     scenes,
		"cps":        cps,
		"cpsPlans":   cpsPlans,
		"members":    members,
		"goodsTop":   goodsTop,
		"brandDaily": brandDaily,
		"crowdDaily": crowdDaily,
		"industry":   industry,
		"repurchase": repurchase,
	})
}

// GetTmallcsOps 天猫超市运营数据
// 包含：经营概况(每日)、智多星推广(已在campaign_daily)、行业热词、市场品牌排名
func (h *DashboardHandler) GetTmallcsOps(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDomainAccess(r, "ops")) || writeScopeError(w, requirePlatformAccess(r, "tmall_cs")) {
		return
	}
	start, end := getDateRange(r, h.DB)
	trendStart, trendEnd := getTrendDateRange(start, end)

	// 1. 经营概况日趋势
	type BusinessDaily struct {
		Date             string  `json:"date"`
		PayAmount        float64 `json:"payAmount"`
		SubOrderAvgPrice float64 `json:"subOrderAvgPrice"`
		AvgPrice         float64 `json:"avgPrice"`
		IpvUv            int     `json:"ipvUv"`
		PaySubOrders     int     `json:"paySubOrders"`
		PayQty           int     `json:"payQty"`
		ConvRate         float64 `json:"convRate"`
		PayUsers         int     `json:"payUsers"`
	}
	var business []BusinessDaily
	bRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			pay_amount, sub_order_avg_price, avg_price, ipv_uv,
			pay_sub_orders, pay_qty, conv_rate, pay_users
		FROM op_tmall_cs_shop_daily
		WHERE stat_date BETWEEN ? AND ?
		ORDER BY stat_date`, trendStart, trendEnd)
	if !ok {
		return
	}
	defer bRows.Close()
	for bRows.Next() {
		var b BusinessDaily
		if writeDatabaseError(w, bRows.Scan(&b.Date, &b.PayAmount, &b.SubOrderAvgPrice, &b.AvgPrice, &b.IpvUv,
			&b.PaySubOrders, &b.PayQty, &b.ConvRate, &b.PayUsers)) {
			return
		}
		business = append(business, b)
	}

	// 2. 推广汇总(智多星/无界场景/淘客) - 按天
	type CampaignDaily struct {
		Date      string  `json:"date"`
		PromoType string  `json:"promoType"`
		Cost      float64 `json:"cost"`
		PayAmount float64 `json:"payAmount"`
		ROI       float64 `json:"roi"`
		Clicks    int     `json:"clicks"`
		Impr      int     `json:"impressions"`
	}
	var campaigns []CampaignDaily
	cRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'), promo_type,
			ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
			CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END,
			SUM(clicks), SUM(impressions)
		FROM op_tmall_cs_campaign_daily
		WHERE stat_date BETWEEN ? AND ?
		GROUP BY stat_date, promo_type ORDER BY stat_date`, trendStart, trendEnd)
	if !ok {
		return
	}
	defer cRows.Close()
	for cRows.Next() {
		var c CampaignDaily
		if writeDatabaseError(w, cRows.Scan(&c.Date, &c.PromoType, &c.Cost, &c.PayAmount, &c.ROI, &c.Clicks, &c.Impr)) {
			return
		}
		campaigns = append(campaigns, c)
	}

	// 3. 行业热词TOP30 (按搜索曝光热度)
	type IndustryKeyword struct {
		Keyword          string  `json:"keyword"`
		SearchImpression float64 `json:"searchImpression"`
		TradeHeat        float64 `json:"tradeHeat"`
		TradeScale       float64 `json:"tradeScale"`
		ConvIndex        float64 `json:"convIndex"`
		VisitHeat        float64 `json:"visitHeat"`
	}
	var keywords []IndustryKeyword
	kRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT keyword, ROUND(AVG(search_impression),2), ROUND(AVG(trade_heat),2),
			ROUND(AVG(trade_scale),2), ROUND(AVG(conv_index),2), ROUND(AVG(visit_heat),2)
		FROM op_tmall_cs_industry_keyword
		WHERE stat_date BETWEEN ? AND ? AND dimension='day' AND channel='整体'
		GROUP BY keyword
		ORDER BY AVG(search_impression) DESC LIMIT 30`, start, end)
	if !ok {
		return
	}
	defer kRows.Close()
	for kRows.Next() {
		var k IndustryKeyword
		if writeDatabaseError(w, kRows.Scan(&k.Keyword, &k.SearchImpression, &k.TradeHeat,
			&k.TradeScale, &k.ConvIndex, &k.VisitHeat)) {
			return
		}
		keywords = append(keywords, k)
	}

	// 4. 市场品牌排名 - 按品类返回
	type MarketRank struct {
		Category        string  `json:"category"`
		BrandName       string  `json:"brandName"`
		TradeHeat       float64 `json:"tradeHeat"`
		TradePopularity float64 `json:"tradePopularity"`
		VisitHeat       float64 `json:"visitHeat"`
		ConvIndex       float64 `json:"convIndex"`
		TradeIndex      float64 `json:"tradeIndex"`
	}
	var ranks []MarketRank
	rRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT category, brand_name,
			ROUND(AVG(trade_heat),2), ROUND(AVG(trade_popularity),2),
			ROUND(AVG(visit_heat),2), ROUND(AVG(conv_index),2), ROUND(AVG(trade_index),2)
		FROM op_tmall_cs_market_rank
		WHERE stat_date BETWEEN ? AND ?
		GROUP BY category, brand_name
		ORDER BY category, AVG(trade_heat) DESC`, start, end)
	if !ok {
		return
	}
	defer rRows.Close()
	for rRows.Next() {
		var r MarketRank
		if writeDatabaseError(w, rRows.Scan(&r.Category, &r.BrandName, &r.TradeHeat, &r.TradePopularity,
			&r.VisitHeat, &r.ConvIndex, &r.TradeIndex)) {
			return
		}
		ranks = append(ranks, r)
	}

	writeJSON(w, map[string]interface{}{
		"business":  business,
		"campaigns": campaigns,
		"keywords":  keywords,
		"ranks":     ranks,
	})
}
