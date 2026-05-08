package handler

import (
	"net/http"
)

// GetJdOps 京东运营数据（店铺经营+客户分析）
func (h *DashboardHandler) GetJdOps(w http.ResponseWriter, r *http.Request) {
	shop := r.URL.Query().Get("shop")
	if shop == "" {
		writeError(w, 400, "shop is required")
		return
	}
	if writeScopeError(w, requireDomainAccess(r, "ops")) || writeScopeError(w, requirePlatformAccess(r, "jd")) || writeScopeError(w, requireShopAccess(r, shop)) {
		return
	}
	start, end := getDateRange(r, h.DB)
	trendStart, trendEnd := getTrendDateRange(start, end)

	// 店铺经营数据
	type ShopDaily struct {
		Date         string  `json:"date"`
		Visitors     int     `json:"visitors"`
		PageViews    int     `json:"pageViews"`
		PayCustomers int     `json:"payCustomers"`
		PayAmount    float64 `json:"payAmount"`
		PayCount     int     `json:"payCount"`
		PayOrders    int     `json:"payOrders"`
		UnitPrice    float64 `json:"unitPrice"`
		ConvRate     float64 `json:"convRate"`
		UvValue      float64 `json:"uvValue"`
		BounceRate   float64 `json:"bounceRate"`
		RefundAmount float64 `json:"refundAmount"`
	}
	var shopDaily []ShopDaily
	rows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			IFNULL(SUM(visitors),0), IFNULL(SUM(page_views),0),
			IFNULL(SUM(pay_customers),0), IFNULL(SUM(pay_amount),0),
			IFNULL(SUM(pay_count),0), IFNULL(SUM(pay_orders),0),
			IFNULL(AVG(unit_price),0), IFNULL(AVG(conv_rate),0),
			IFNULL(AVG(uv_value),0), IFNULL(AVG(bounce_rate),0),
			IFNULL(SUM(refund_amount),0)
		FROM op_jd_shop_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY stat_date ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var d ShopDaily
		if writeDatabaseError(w, rows.Scan(&d.Date, &d.Visitors, &d.PageViews, &d.PayCustomers, &d.PayAmount,
			&d.PayCount, &d.PayOrders, &d.UnitPrice, &d.ConvRate, &d.UvValue, &d.BounceRate, &d.RefundAmount)) {
			return
		}
		shopDaily = append(shopDaily, d)
	}
	if writeDatabaseError(w, rows.Err()) {
		return
	}

	// 客户分析数据
	type CustomerDaily struct {
		Date                string `json:"date"`
		BrowseCustomers     int    `json:"browseCustomers"`
		CartCustomers       int    `json:"cartCustomers"`
		OrderCustomers      int    `json:"orderCustomers"`
		PayCustomers        int    `json:"payCustomers"`
		RepurchaseCustomers int    `json:"repurchaseCustomers"`
		LostCustomers       int    `json:"lostCustomers"`
	}
	var customerDaily []CustomerDaily
	rows2, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			IFNULL(SUM(browse_customers),0), IFNULL(SUM(cart_customers),0),
			IFNULL(SUM(order_customers),0), IFNULL(SUM(pay_customers),0),
			IFNULL(SUM(repurchase_customers),0), IFNULL(SUM(lost_customers),0)
		FROM op_jd_customer_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY stat_date ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer rows2.Close()
	for rows2.Next() {
		var d CustomerDaily
		if writeDatabaseError(w, rows2.Scan(&d.Date, &d.BrowseCustomers, &d.CartCustomers, &d.OrderCustomers,
			&d.PayCustomers, &d.RepurchaseCustomers, &d.LostCustomers)) {
			return
		}
		customerDaily = append(customerDaily, d)
	}
	if writeDatabaseError(w, rows2.Err()) {
		return
	}

	// 3. 新老客分析
	type CustomerType struct {
		Date         string  `json:"date"`
		CustomerType string  `json:"customerType"`
		PayCustomers int     `json:"payCustomers"`
		PayPct       float64 `json:"payPct"`
		ConvRate     float64 `json:"convRate"`
		UnitPrice    float64 `json:"unitPrice"`
	}
	var customerTypes []CustomerType
	ctRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'), customer_type, pay_customers,
			pay_pct, conv_rate, unit_price
		FROM op_jd_customer_type_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		ORDER BY stat_date, customer_type`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer ctRows.Close()
	for ctRows.Next() {
		var c CustomerType
		if writeDatabaseError(w, ctRows.Scan(&c.Date, &c.CustomerType, &c.PayCustomers, &c.PayPct, &c.ConvRate, &c.UnitPrice)) {
			return
		}
		customerTypes = append(customerTypes, c)
	}

	// 4. 行业热词TOP20
	type IndustryKeyword struct {
		Keyword        string `json:"keyword"`
		SearchRank     string `json:"searchRank"`
		CompeteRank    string `json:"competeRank"`
		ClickRank      string `json:"clickRank"`
		PayAmountRange string `json:"payAmountRange"`
		TopBrand       string `json:"topBrand"`
	}
	var keywords []IndustryKeyword
	kwRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT keyword, search_rank, compete_rank, click_rank,
			IFNULL(pay_amount_range,''), IFNULL(top_brand,'')
		FROM op_jd_industry_keyword
		WHERE shop_name = ? AND stat_date = (
			SELECT MAX(stat_date) FROM op_jd_industry_keyword WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		) LIMIT 20`, shop, shop, start, end)
	if !ok {
		return
	}
	defer kwRows.Close()
	for kwRows.Next() {
		var k IndustryKeyword
		if writeDatabaseError(w, kwRows.Scan(&k.Keyword, &k.SearchRank, &k.CompeteRank, &k.ClickRank, &k.PayAmountRange, &k.TopBrand)) {
			return
		}
		keywords = append(keywords, k)
	}

	// 5. 促销活动汇总
	type PromoSummary struct {
		PromoType string  `json:"promoType"`
		PayAmount float64 `json:"payAmount"`
		PayUsers  int     `json:"payUsers"`
		PayCount  int     `json:"payCount"`
		ConvRate  float64 `json:"convRate"`
		UV        int     `json:"uv"`
	}
	var promos []PromoSummary
	pRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT promo_type, ROUND(SUM(pay_amount),2), SUM(pay_users), SUM(pay_count),
			CASE WHEN SUM(uv)>0 THEN ROUND(SUM(pay_users)/SUM(uv)*100,2) ELSE 0 END, SUM(uv)
		FROM op_jd_promo_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY promo_type ORDER BY SUM(pay_amount) DESC`, shop, start, end)
	if !ok {
		return
	}
	defer pRows.Close()
	for pRows.Next() {
		var p PromoSummary
		if writeDatabaseError(w, pRows.Scan(&p.PromoType, &p.PayAmount, &p.PayUsers, &p.PayCount, &p.ConvRate, &p.UV)) {
			return
		}
		promos = append(promos, p)
	}

	// 6. 促销商品TOP10
	type PromoSku struct {
		GoodsName string  `json:"goodsName"`
		PromoType string  `json:"promoType"`
		UV        int     `json:"uv"`
		PayAmount float64 `json:"payAmount"`
		PayUsers  int     `json:"payUsers"`
		PayCount  int     `json:"payCount"`
	}
	var promoSkus []PromoSku
	psRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT goods_name, promo_type, SUM(uv), ROUND(SUM(pay_amount),2), SUM(pay_users), SUM(pay_count)
		FROM op_jd_promo_sku_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY goods_name, promo_type ORDER BY SUM(pay_amount) DESC LIMIT 10`, shop, start, end)
	if !ok {
		return
	}
	defer psRows.Close()
	for psRows.Next() {
		var p PromoSku
		if writeDatabaseError(w, psRows.Scan(&p.GoodsName, &p.PromoType, &p.UV, &p.PayAmount, &p.PayUsers, &p.PayCount)) {
			return
		}
		promoSkus = append(promoSkus, p)
	}

	writeJSON(w, map[string]interface{}{
		"shop":          shopDaily,
		"customer":      customerDaily,
		"customerTypes": customerTypes,
		"keywords":      keywords,
		"promos":        promos,
		"promoSkus":     promoSkus,
		"dateRange":     map[string]string{"start": start, "end": end},
	})
}
