package handler

import (
	"net/http"
)

// GetPddOps 拼多多运营数据（店铺经营+商品数据+短视频）
func (h *DashboardHandler) GetPddOps(w http.ResponseWriter, r *http.Request) {
	shop := r.URL.Query().Get("shop")
	if shop == "" {
		writeError(w, 400, "shop is required")
		return
	}
	if writeScopeError(w, requireDomainAccess(r, "ops")) || writeScopeError(w, requirePlatformAccess(r, "pdd")) || writeScopeError(w, requireShopAccess(r, shop)) {
		return
	}
	start, end := getDateRange(r, h.DB)
	trendStart, trendEnd := getTrendDateRange(start, end)

	// 店铺经营数据
	type ShopDaily struct {
		Date         string  `json:"date"`
		PayAmount    float64 `json:"payAmount"`
		PayCount     int     `json:"payCount"`
		PayOrders    int     `json:"payOrders"`
		ConvRate     float64 `json:"convRate"`
		UnitPrice    float64 `json:"unitPrice"`
		PayOrdersPct float64 `json:"payOrdersPct"`
	}
	var shopDaily []ShopDaily
	rows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			IFNULL(SUM(pay_amount),0), IFNULL(SUM(pay_count),0), IFNULL(SUM(pay_orders),0),
			IFNULL(AVG(conv_rate),0), IFNULL(AVG(unit_price),0), IFNULL(AVG(pay_orders_pct),0)
		FROM op_pdd_shop_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY stat_date ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var d ShopDaily
		if writeDatabaseError(w, rows.Scan(&d.Date, &d.PayAmount, &d.PayCount, &d.PayOrders, &d.ConvRate, &d.UnitPrice, &d.PayOrdersPct)) {
			return
		}
		shopDaily = append(shopDaily, d)
	}
	if writeDatabaseError(w, rows.Err()) {
		return
	}

	// 商品数据
	type GoodsDaily struct {
		Date           string  `json:"date"`
		GoodsVisitors  int     `json:"goodsVisitors"`
		GoodsViews     int     `json:"goodsViews"`
		GoodsCollect   int     `json:"goodsCollect"`
		SaleGoodsCount int     `json:"saleGoodsCount"`
		PayAmount      float64 `json:"payAmount"`
		PayCount       int     `json:"payCount"`
	}
	var goodsDaily []GoodsDaily
	rows2, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			IFNULL(SUM(goods_visitors),0), IFNULL(SUM(goods_views),0), IFNULL(SUM(goods_collect),0),
			IFNULL(SUM(sale_goods_count),0), IFNULL(SUM(pay_amount),0), IFNULL(SUM(pay_count),0)
		FROM op_pdd_goods_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY stat_date ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer rows2.Close()
	for rows2.Next() {
		var d GoodsDaily
		if writeDatabaseError(w, rows2.Scan(&d.Date, &d.GoodsVisitors, &d.GoodsViews, &d.GoodsCollect, &d.SaleGoodsCount, &d.PayAmount, &d.PayCount)) {
			return
		}
		goodsDaily = append(goodsDaily, d)
	}
	if writeDatabaseError(w, rows2.Err()) {
		return
	}

	// 短视频数据
	type VideoDaily struct {
		Date          string  `json:"date"`
		TotalGmv      float64 `json:"totalGmv"`
		OrderCount    int     `json:"orderCount"`
		OrderUv       int     `json:"orderUv"`
		FeedCount     int     `json:"feedCount"`
		VideoViewCnt  int     `json:"videoViewCnt"`
		GoodsClickCnt int     `json:"goodsClickCnt"`
	}
	var videoDaily []VideoDaily
	rows3, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			IFNULL(SUM(total_gmv),0), IFNULL(SUM(order_count),0), IFNULL(SUM(order_uv),0),
			IFNULL(SUM(feed_count),0), IFNULL(SUM(video_view_cnt),0), IFNULL(SUM(goods_click_cnt),0)
		FROM op_pdd_video_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY stat_date ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer rows3.Close()
	for rows3.Next() {
		var d VideoDaily
		if writeDatabaseError(w, rows3.Scan(&d.Date, &d.TotalGmv, &d.OrderCount, &d.OrderUv, &d.FeedCount, &d.VideoViewCnt, &d.GoodsClickCnt)) {
			return
		}
		videoDaily = append(videoDaily, d)
	}
	if writeDatabaseError(w, rows3.Err()) {
		return
	}

	writeJSON(w, map[string]interface{}{
		"shop":      shopDaily,
		"goods":     goodsDaily,
		"video":     videoDaily,
		"dateRange": map[string]string{"start": start, "end": end},
	})
}
