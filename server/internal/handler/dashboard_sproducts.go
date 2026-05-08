package handler

import (
	"net/http"
)

// GetSProducts S品渠道销售分析
// 参数: dept=ecommerce (按部门过滤)
func (h *DashboardHandler) GetSProducts(w http.ResponseWriter, r *http.Request) {
	start, end := getDateRange(r, h.DB)
	trendStart, trendEnd := getTrendDateRange(start, end)
	dept := r.URL.Query().Get("dept")
	platform := r.URL.Query().Get("platform")
	deptCond := ""
	var deptArgs []interface{}
	if dept != "" && dept != "all" {
		deptCond = " AND s.department = ?"
		deptArgs = append(deptArgs, dept)
	}
	// 店铺过滤（优先）
	shop := r.URL.Query().Get("shop")
	if shop != "" && shop != "all" {
		deptCond += " AND s.shop_name = ?"
		deptArgs = append(deptArgs, shop)
	} else if platform != "" && platform != "all" {
		// 平台过滤：根据平台code匹配shop_name前缀
		platPrefixMap := map[string]string{
			"tmall": "ds-天猫-", "tmall_cs": "ds-天猫超市-", "jd": "ds-京东-",
			"pdd": "ds-拼多多-", "vip": "ds-唯品会-",
		}
		if prefix, ok := platPrefixMap[platform]; ok {
			deptCond += " AND s.shop_name LIKE ?"
			deptArgs = append(deptArgs, prefix+"%")
		}
	}

	// 1. S品渠道销售排行（全部平台时按平台汇总，选了平台时按店铺）
	type ShopSales struct {
		ShopName   string  `json:"shopName"`
		Department string  `json:"department"`
		Sales      float64 `json:"sales"`
		Qty        float64 `json:"qty"`
	}
	var shopRank []ShopSales
	args1 := append([]interface{}{start, end}, deptArgs...)
	groupByPlatform := platform == "" || platform == "all"
	var shopSQL string
	if groupByPlatform {
		// 按平台汇总：从shop_name提取平台名（ds-天猫-xxx → 天猫）
		shopSQL = `
		SELECT
			CASE
				WHEN s.shop_name LIKE 'ds-天猫超市%' THEN '天猫超市'
				WHEN s.shop_name LIKE 'ds-天猫-%' THEN '天猫'
				WHEN s.shop_name LIKE 'ds-京东-%' THEN '京东'
				WHEN s.shop_name LIKE 'ds-拼多多-%' THEN '拼多多'
				WHEN s.shop_name LIKE 'ds-唯品会-%' THEN '唯品会'
				WHEN s.shop_name LIKE 'js-%' THEN '即时零售'
				ELSE '其他'
			END as platform_name,
			s.department,
			ROUND(SUM(s.local_goods_amt),2), ROUND(SUM(s.goods_qty),0)
		FROM sales_goods_summary s
		INNER JOIN (SELECT DISTINCT goods_no FROM goods WHERE goods_field7 = 'S') g ON g.goods_no = s.goods_no
		WHERE s.stat_date BETWEEN ? AND ?
		  AND s.shop_name IS NOT NULL AND s.shop_name != ''` + deptCond + `
		GROUP BY platform_name, s.department
		ORDER BY SUM(s.local_goods_amt) DESC`
	} else {
		shopSQL = `
		SELECT s.shop_name, s.department, ROUND(SUM(s.local_goods_amt),2), ROUND(SUM(s.goods_qty),0)
		FROM sales_goods_summary s
		INNER JOIN (SELECT DISTINCT goods_no FROM goods WHERE goods_field7 = 'S') g ON g.goods_no = s.goods_no
		WHERE s.stat_date BETWEEN ? AND ?
		  AND s.shop_name IS NOT NULL AND s.shop_name != ''` + deptCond + `
		GROUP BY s.shop_name, s.department
		ORDER BY SUM(s.local_goods_amt) DESC LIMIT 20`
	}
	sRows, ok := queryRowsOrWriteError(w, h.DB, shopSQL, args1...)
	if !ok {
		return
	}
	defer sRows.Close()
	for sRows.Next() {
		var s ShopSales
		if writeDatabaseError(w, sRows.Scan(&s.ShopName, &s.Department, &s.Sales, &s.Qty)) {
			return
		}
		shopRank = append(shopRank, s)
	}

	// 2. S品单品销售排行
	type GoodsSales struct {
		GoodsNo   string  `json:"goodsNo"`
		GoodsName string  `json:"goodsName"`
		Sales     float64 `json:"sales"`
		Qty       float64 `json:"qty"`
		ShopCount int     `json:"shopCount"`
	}
	var goodsRank []GoodsSales
	args2 := append([]interface{}{start, end}, deptArgs...)
	var goodsSQL string
	if groupByPlatform {
		goodsSQL = `
		SELECT s.goods_no, g.goods_name, ROUND(SUM(s.local_goods_amt),2), ROUND(SUM(s.goods_qty),0),
			COUNT(DISTINCT CASE
				WHEN s.shop_name LIKE 'ds-天猫超市%' THEN '天猫超市'
				WHEN s.shop_name LIKE 'ds-天猫-%' THEN '天猫'
				WHEN s.shop_name LIKE 'ds-京东-%' THEN '京东'
				WHEN s.shop_name LIKE 'ds-拼多多-%' THEN '拼多多'
				WHEN s.shop_name LIKE 'ds-唯品会-%' THEN '唯品会'
				WHEN s.shop_name LIKE 'js-%' THEN '即时零售'
				ELSE '其他'
			END)
		FROM sales_goods_summary s
		INNER JOIN (SELECT goods_no, MAX(goods_name) AS goods_name FROM goods WHERE goods_field7 = 'S' GROUP BY goods_no) g ON g.goods_no = s.goods_no
		WHERE s.stat_date BETWEEN ? AND ?` + deptCond + `
		GROUP BY s.goods_no, g.goods_name
		ORDER BY SUM(s.local_goods_amt) DESC`
	} else {
		goodsSQL = `
		SELECT s.goods_no, g.goods_name, ROUND(SUM(s.local_goods_amt),2), ROUND(SUM(s.goods_qty),0),
			COUNT(DISTINCT s.shop_name)
		FROM sales_goods_summary s
		INNER JOIN (SELECT goods_no, MAX(goods_name) AS goods_name FROM goods WHERE goods_field7 = 'S' GROUP BY goods_no) g ON g.goods_no = s.goods_no
		WHERE s.stat_date BETWEEN ? AND ?` + deptCond + `
		GROUP BY s.goods_no, g.goods_name
		ORDER BY SUM(s.local_goods_amt) DESC`
	}
	gRows, ok := queryRowsOrWriteError(w, h.DB, goodsSQL, args2...)
	if !ok {
		return
	}
	defer gRows.Close()
	for gRows.Next() {
		var g GoodsSales
		if writeDatabaseError(w, gRows.Scan(&g.GoodsNo, &g.GoodsName, &g.Sales, &g.Qty, &g.ShopCount)) {
			return
		}
		goodsRank = append(goodsRank, g)
	}

	// 3. S品每日销售趋势
	type DailyTrend struct {
		Date  string  `json:"date"`
		Sales float64 `json:"sales"`
		Qty   float64 `json:"qty"`
	}
	var trend []DailyTrend
	args3 := append([]interface{}{trendStart, trendEnd}, deptArgs...)
	tRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(s.stat_date,'%Y-%m-%d'), ROUND(SUM(s.local_goods_amt),2), ROUND(SUM(s.goods_qty),0)
		FROM sales_goods_summary s
		INNER JOIN (SELECT DISTINCT goods_no FROM goods WHERE goods_field7 = 'S') g ON g.goods_no = s.goods_no
		WHERE s.stat_date BETWEEN ? AND ?`+deptCond+`
		GROUP BY s.stat_date ORDER BY s.stat_date`, args3...)
	if !ok {
		return
	}
	defer tRows.Close()
	for tRows.Next() {
		var t DailyTrend
		if writeDatabaseError(w, tRows.Scan(&t.Date, &t.Sales, &t.Qty)) {
			return
		}
		trend = append(trend, t)
	}

	// 4. S品各单品明细（全部平台时按平台汇总，选了平台时按店铺）
	type GoodsShopDetail struct {
		GoodsName string  `json:"goodsName"`
		ShopName  string  `json:"shopName"`
		Sales     float64 `json:"sales"`
		Qty       float64 `json:"qty"`
	}
	var details []GoodsShopDetail
	args4 := append([]interface{}{start, end}, deptArgs...)
	var detailSQL string
	if groupByPlatform {
		detailSQL = `
		SELECT g.goods_name,
			CASE
				WHEN s.shop_name LIKE 'ds-天猫超市%' THEN '天猫超市'
				WHEN s.shop_name LIKE 'ds-天猫-%' THEN '天猫'
				WHEN s.shop_name LIKE 'ds-京东-%' THEN '京东'
				WHEN s.shop_name LIKE 'ds-拼多多-%' THEN '拼多多'
				WHEN s.shop_name LIKE 'ds-唯品会-%' THEN '唯品会'
				WHEN s.shop_name LIKE 'js-%' THEN '即时零售'
				ELSE '其他'
			END as platform_name,
			ROUND(SUM(s.local_goods_amt),2), ROUND(SUM(s.goods_qty),0)
		FROM sales_goods_summary s
		INNER JOIN (SELECT goods_no, MAX(goods_name) AS goods_name FROM goods WHERE goods_field7 = 'S' GROUP BY goods_no) g ON g.goods_no = s.goods_no
		WHERE s.stat_date BETWEEN ? AND ?
		  AND s.shop_name IS NOT NULL AND s.shop_name != ''` + deptCond + `
		GROUP BY g.goods_name, platform_name
		HAVING SUM(s.local_goods_amt) > 0
		ORDER BY g.goods_name, SUM(s.local_goods_amt) DESC`
	} else {
		detailSQL = `
		SELECT g.goods_name, s.shop_name, ROUND(SUM(s.local_goods_amt),2), ROUND(SUM(s.goods_qty),0)
		FROM sales_goods_summary s
		INNER JOIN (SELECT goods_no, MAX(goods_name) AS goods_name FROM goods WHERE goods_field7 = 'S' GROUP BY goods_no) g ON g.goods_no = s.goods_no
		WHERE s.stat_date BETWEEN ? AND ?
		  AND s.shop_name IS NOT NULL AND s.shop_name != ''` + deptCond + `
		GROUP BY g.goods_name, s.shop_name
		HAVING SUM(s.local_goods_amt) > 0
		ORDER BY g.goods_name, SUM(s.local_goods_amt) DESC`
	}
	dRows, ok := queryRowsOrWriteError(w, h.DB, detailSQL, args4...)
	if !ok {
		return
	}
	defer dRows.Close()
	for dRows.Next() {
		var d GoodsShopDetail
		if writeDatabaseError(w, dRows.Scan(&d.GoodsName, &d.ShopName, &d.Sales, &d.Qty)) {
			return
		}
		details = append(details, d)
	}

	writeJSON(w, map[string]interface{}{
		"shopRank":  shopRank,
		"goodsRank": goodsRank,
		"trend":     trend,
		"details":   details,
	})
}
