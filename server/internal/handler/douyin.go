package handler

import (
	"net/http"
)

// GetDouyinOps 抖音自营运营数据
// GET /api/douyin/ops?start=&end=
func (h *DashboardHandler) GetDouyinOps(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeError(w, 405, "method not allowed")
		return
	}
	if writeScopeError(w, requireDomainAccess(r, "ops")) ||
		writeScopeError(w, requireDeptAccess(r, "social")) ||
		writeScopeError(w, requirePlatformAccess(r, "douyin")) {
		return
	}
	start, end := getDateRange(r, h.DB)
	trendStart, trendEnd := getTrendDateRange(start, end)

	// 1. 每日直播汇总趋势
	type LiveTrend struct {
		Date       string  `json:"date"`
		Sessions   int     `json:"sessions"`
		WatchUV    int     `json:"watchUV"`
		PayAmount  float64 `json:"payAmount"`
		AvgOnline  int     `json:"avgOnline"`
		RefundRate float64 `json:"refundRate"`
	}
	var liveTrend []LiveTrend
	rows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			COUNT(DISTINCT anchor_id, start_time), SUM(watch_uv), ROUND(SUM(pay_amount),2),
			ROUND(AVG(avg_online),0),
			CASE WHEN SUM(order_count)>0 THEN ROUND(SUM(refund_count)/SUM(order_count)*100,2) ELSE 0 END
		FROM op_douyin_live_daily
		WHERE stat_date BETWEEN ? AND ?
		GROUP BY stat_date ORDER BY stat_date`, trendStart, trendEnd)
	if !ok {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var d LiveTrend
		if writeDatabaseError(w, rows.Scan(&d.Date, &d.Sessions, &d.WatchUV, &d.PayAmount, &d.AvgOnline, &d.RefundRate)) {
			return
		}
		liveTrend = append(liveTrend, d)
	}

	// 2. 商品TOP10
	type GoodsTop struct {
		ProductName string  `json:"productName"`
		PayAmount   float64 `json:"payAmount"`
		PayQty      int     `json:"payQty"`
		ClickUV     int     `json:"clickUV"`
		ConvRate    float64 `json:"convRate"`
	}
	var goodsTop []GoodsTop
	gRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT product_name, ROUND(SUM(pay_amount),2), SUM(pay_qty), SUM(click_uv),
			CASE WHEN SUM(click_uv)>0 THEN ROUND(SUM(pay_qty)/SUM(click_uv)*100,2) ELSE 0 END
		FROM op_douyin_goods_daily
		WHERE stat_date BETWEEN ? AND ?
		GROUP BY product_name ORDER BY SUM(pay_amount) DESC LIMIT 10`, start, end)
	if !ok {
		return
	}
	defer gRows.Close()
	for gRows.Next() {
		var g GoodsTop
		if writeDatabaseError(w, gRows.Scan(&g.ProductName, &g.PayAmount, &g.PayQty, &g.ClickUV, &g.ConvRate)) {
			return
		}
		goodsTop = append(goodsTop, g)
	}

	// 3. 主播排行
	type AnchorRank struct {
		AnchorName string  `json:"anchorName"`
		Sessions   int     `json:"sessions"`
		TotalMin   float64 `json:"totalMin"`
		PayAmount  float64 `json:"payAmount"`
		WatchUV    int     `json:"watchUV"`
		MaxOnline  int     `json:"maxOnline"`
	}
	var anchors []AnchorRank
	aRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT anchor_name, COUNT(DISTINCT stat_date, start_time), ROUND(SUM(duration_min),0),
			ROUND(SUM(pay_amount),2), SUM(watch_uv), MAX(max_online)
		FROM op_douyin_live_daily
		WHERE stat_date BETWEEN ? AND ?
		GROUP BY anchor_name ORDER BY SUM(pay_amount) DESC LIMIT 20`, start, end)
	if !ok {
		return
	}
	defer aRows.Close()
	for aRows.Next() {
		var a AnchorRank
		if writeDatabaseError(w, aRows.Scan(&a.AnchorName, &a.Sessions, &a.TotalMin, &a.PayAmount, &a.WatchUV, &a.MaxOnline)) {
			return
		}
		anchors = append(anchors, a)
	}

	// 4. 渠道分析（按渠道汇总）
	type ChannelData struct {
		ChannelName string  `json:"channelName"`
		WatchUcnt   int     `json:"watchUcnt"`
		PayAmt      float64 `json:"payAmt"`
		PayCnt      int     `json:"payCnt"`
		AdCost      float64 `json:"adCost"`
	}
	var channels []ChannelData
	chRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT channel_name, SUM(watch_ucnt), ROUND(SUM(pay_amt),2), SUM(pay_cnt), ROUND(SUM(ad_costed_amt),2)
		FROM op_douyin_channel_daily
		WHERE stat_date BETWEEN ? AND ?
		GROUP BY channel_name ORDER BY SUM(pay_amt) DESC`, start, end)
	if !ok {
		return
	}
	defer chRows.Close()
	for chRows.Next() {
		var c ChannelData
		if writeDatabaseError(w, chRows.Scan(&c.ChannelName, &c.WatchUcnt, &c.PayAmt, &c.PayCnt, &c.AdCost)) {
			return
		}
		channels = append(channels, c)
	}

	// 5. 转化漏斗（取最新一天数据）
	type FunnelStep struct {
		StepName  string `json:"stepName"`
		StepValue int64  `json:"stepValue"`
	}
	var funnel []FunnelStep
	fRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT step_name, step_value FROM op_douyin_funnel_daily
		WHERE stat_date = (SELECT MAX(stat_date) FROM op_douyin_funnel_daily WHERE stat_date BETWEEN ? AND ?)
		ORDER BY step_order LIMIT 7`, start, end)
	if !ok {
		return
	}
	defer fRows.Close()
	for fRows.Next() {
		var f FunnelStep
		if writeDatabaseError(w, fRows.Scan(&f.StepName, &f.StepValue)) {
			return
		}
		funnel = append(funnel, f)
	}

	// 6. 推广直播间每日趋势
	type AdTrend struct {
		Date      string  `json:"date"`
		Cost      float64 `json:"cost"`
		PayAmount float64 `json:"payAmount"`
		ROI       float64 `json:"roi"`
		NetAmount float64 `json:"netAmount"`
		NetROI    float64 `json:"netROI"`
	}
	var adTrend []AdTrend
	adTRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
			CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END,
			ROUND(SUM(net_amount),2),
			CASE WHEN SUM(cost)>0 THEN ROUND(SUM(net_amount)/SUM(cost),2) ELSE 0 END
		FROM op_douyin_ad_live_daily
		WHERE stat_date BETWEEN ? AND ?
		GROUP BY stat_date ORDER BY stat_date`, trendStart, trendEnd)
	if !ok {
		return
	}
	defer adTRows.Close()
	for adTRows.Next() {
		var a AdTrend
		if writeDatabaseError(w, adTRows.Scan(&a.Date, &a.Cost, &a.PayAmount, &a.ROI, &a.NetAmount, &a.NetROI)) {
			return
		}
		adTrend = append(adTrend, a)
	}

	// 7. 推广直播间明细（按直播间汇总）
	type AdDetail struct {
		DouyinName  string  `json:"douyinName"`
		Cost        float64 `json:"cost"`
		PayAmount   float64 `json:"payAmount"`
		ROI         float64 `json:"roi"`
		NetAmount   float64 `json:"netAmount"`
		NetROI      float64 `json:"netROI"`
		Impressions int     `json:"impressions"`
		Clicks      int     `json:"clicks"`
		Refund1h    float64 `json:"refund1hRate"`
	}
	var adDetails []AdDetail
	adDRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT douyin_name, ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
			CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END,
			ROUND(SUM(net_amount),2),
			CASE WHEN SUM(cost)>0 THEN ROUND(SUM(net_amount)/SUM(cost),2) ELSE 0 END,
			SUM(impressions), SUM(clicks),
			CASE WHEN SUM(net_order_count)>0 THEN ROUND(SUM(CASE WHEN refund_1h_rate>0 THEN net_order_count*refund_1h_rate ELSE 0 END)/SUM(net_order_count)*100,2) ELSE 0 END
		FROM op_douyin_ad_live_daily
		WHERE stat_date BETWEEN ? AND ?
		GROUP BY douyin_name ORDER BY SUM(cost) DESC`, start, end)
	if !ok {
		return
	}
	defer adDRows.Close()
	for adDRows.Next() {
		var a AdDetail
		if writeDatabaseError(w, adDRows.Scan(&a.DouyinName, &a.Cost, &a.PayAmount, &a.ROI, &a.NetAmount, &a.NetROI, &a.Impressions, &a.Clicks, &a.Refund1h)) {
			return
		}
		adDetails = append(adDetails, a)
	}

	writeJSON(w, map[string]interface{}{
		"liveTrend": liveTrend,
		"goodsTop":  goodsTop,
		"anchors":   anchors,
		"channels":  channels,
		"funnel":    funnel,
		"adTrend":   adTrend,
		"adDetails": adDetails,
		"dateRange": map[string]string{"start": start, "end": end},
	})
}

// GetDouyinDistOps 抖音分销运营数据
// GET /api/douyin-dist/ops?start=&end=
func (h *DashboardHandler) GetDouyinDistOps(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeError(w, 405, "method not allowed")
		return
	}
	if writeScopeError(w, requireDomainAccess(r, "ops")) ||
		writeScopeError(w, requireDeptAccess(r, "social")) ||
		writeScopeError(w, requirePlatformAccess(r, "douyin")) {
		return
	}
	start, end := getDateRange(r, h.DB)
	trendStart, trendEnd := getTrendDateRange(start, end)
	account := r.URL.Query().Get("account")

	acctCond := ""
	var acctArgs []interface{}
	if account != "" && account != "all" {
		acctCond = " AND account_name = ?"
		acctArgs = []interface{}{account}
	}

	// 0. 投放计划列表（用于前端筛选）
	type PlanItem struct {
		AccountName string  `json:"accountName"`
		Cost        float64 `json:"cost"`
		PayAmount   float64 `json:"payAmount"`
		Talents     int     `json:"talents"`
	}
	var plans []PlanItem
	plRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT account_name, ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2), COUNT(DISTINCT douyin_name)
		FROM op_douyin_dist_account_daily
		WHERE stat_date BETWEEN ? AND ?
		GROUP BY account_name ORDER BY SUM(pay_amount) DESC`, start, end)
	if !ok {
		return
	}
	defer plRows.Close()
	for plRows.Next() {
		var p PlanItem
		if writeDatabaseError(w, plRows.Scan(&p.AccountName, &p.Cost, &p.PayAmount, &p.Talents)) {
			return
		}
		plans = append(plans, p)
	}

	// 1. 每日投放趋势
	type DistTrend struct {
		Date      string  `json:"date"`
		Cost      float64 `json:"cost"`
		PayAmount float64 `json:"payAmount"`
		ROI       float64 `json:"roi"`
	}
	var distTrend []DistTrend
	trendArgs := append([]interface{}{trendStart, trendEnd}, acctArgs...)
	rows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
			CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END
		FROM op_douyin_dist_account_daily
		WHERE stat_date BETWEEN ? AND ?`+acctCond+`
		GROUP BY stat_date ORDER BY stat_date`, trendArgs...)
	if !ok {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var d DistTrend
		if writeDatabaseError(w, rows.Scan(&d.Date, &d.Cost, &d.PayAmount, &d.ROI)) {
			return
		}
		distTrend = append(distTrend, d)
	}

	// 2. 达人排行TOP20
	type AccountRank struct {
		DouyinName  string  `json:"douyinName"`
		AccountName string  `json:"accountName"`
		Cost        float64 `json:"cost"`
		PayAmount   float64 `json:"payAmount"`
		ROI         float64 `json:"roi"`
		NetAmount   float64 `json:"netAmount"`
	}
	var accountRank []AccountRank
	rankArgs := append([]interface{}{start, end}, acctArgs...)
	aRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT douyin_name, account_name, ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
			CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END,
			ROUND(SUM(net_amount),2)
		FROM op_douyin_dist_account_daily
		WHERE stat_date BETWEEN ? AND ?`+acctCond+`
		GROUP BY douyin_name, account_name ORDER BY SUM(pay_amount) DESC LIMIT 20`, rankArgs...)
	if !ok {
		return
	}
	defer aRows.Close()
	for aRows.Next() {
		var a AccountRank
		if writeDatabaseError(w, aRows.Scan(&a.DouyinName, &a.AccountName, &a.Cost, &a.PayAmount, &a.ROI, &a.NetAmount)) {
			return
		}
		accountRank = append(accountRank, a)
	}

	// 3. 商品排行TOP10
	type ProductRank struct {
		ProductName string  `json:"productName"`
		AccountName string  `json:"accountName"`
		Cost        float64 `json:"cost"`
		PayAmount   float64 `json:"payAmount"`
		ROI         float64 `json:"roi"`
		Clicks      int     `json:"clicks"`
	}
	var productRank []ProductRank
	prodArgs := append([]interface{}{start, end}, acctArgs...)
	pRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT product_name, account_name, ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
			CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END,
			SUM(clicks)
		FROM op_douyin_dist_product_daily
		WHERE stat_date BETWEEN ? AND ?`+acctCond+`
		GROUP BY product_name, account_name ORDER BY SUM(pay_amount) DESC LIMIT 10`, prodArgs...)
	if !ok {
		return
	}
	defer pRows.Close()
	for pRows.Next() {
		var p ProductRank
		if writeDatabaseError(w, pRows.Scan(&p.ProductName, &p.AccountName, &p.Cost, &p.PayAmount, &p.ROI, &p.Clicks)) {
			return
		}
		productRank = append(productRank, p)
	}

	writeJSON(w, map[string]interface{}{
		"plans":       plans,
		"distTrend":   distTrend,
		"accountRank": accountRank,
		"productRank": productRank,
		"dateRange":   map[string]string{"start": start, "end": end},
	})
}
