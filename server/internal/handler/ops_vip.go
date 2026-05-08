package handler

import (
	"net/http"
)

// GetVipOps 唯品会运营数据
func (h *DashboardHandler) GetVipOps(w http.ResponseWriter, r *http.Request) {
	shop := r.URL.Query().Get("shop")
	if shop == "" {
		writeError(w, 400, "shop is required")
		return
	}
	if writeScopeError(w, requireDomainAccess(r, "ops")) || writeScopeError(w, requirePlatformAccess(r, "vip")) || writeScopeError(w, requireShopAccess(r, shop)) {
		return
	}
	start, end := getDateRange(r, h.DB)
	trendStart, trendEnd := getTrendDateRange(start, end)

	type VipDaily struct {
		Date          string  `json:"date"`
		Impressions   int     `json:"impressions"`
		PageViews     int     `json:"pageViews"`
		DetailUV      int     `json:"detailUv"`
		DetailUVValue float64 `json:"detailUvValue"`
		CartBuyers    int     `json:"cartBuyers"`
		CollectBuyers int     `json:"collectBuyers"`
		PayAmount     float64 `json:"payAmount"`
		PayCount      int     `json:"payCount"`
		PayOrders     int     `json:"payOrders"`
		Visitors      int     `json:"visitors"`
		ARPU          float64 `json:"arpu"`
		CartConvRate  string  `json:"cartConvRate"`
		PayConvRate   string  `json:"payConvRate"`
		CancelAmount  float64 `json:"cancelAmount"`
	}
	var daily []VipDaily
	rows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			impressions, page_views, detail_uv, detail_uv_value,
			cart_buyers, collect_buyers,
			pay_amount, pay_count, pay_orders, visitors, arpu,
			IFNULL(cart_conv_rate,''), IFNULL(pay_conv_rate,''), cancel_amount
		FROM op_vip_shop_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var d VipDaily
		if writeDatabaseError(w, rows.Scan(&d.Date, &d.Impressions, &d.PageViews, &d.DetailUV, &d.DetailUVValue,
			&d.CartBuyers, &d.CollectBuyers,
			&d.PayAmount, &d.PayCount, &d.PayOrders, &d.Visitors, &d.ARPU,
			&d.CartConvRate, &d.PayConvRate, &d.CancelAmount)) {
			return
		}
		daily = append(daily, d)
	}
	if writeDatabaseError(w, rows.Err()) {
		return
	}

	writeJSON(w, map[string]interface{}{
		"daily":     daily,
		"dateRange": map[string]string{"start": start, "end": end},
	})
}
