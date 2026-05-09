package handler

import (
	"database/sql"
	"net/http"
	"time"
)

// GetSpecialChannelAllotSummary 特殊渠道按调拨单算销售额-汇总+列单
// GET /api/special-channel-allot/summary?start=YYYY-MM-DD&end=YYYY-MM-DD
//
// 返回:
//   summary[]   各渠道 KPI(已入库完成 / 在途分别)
//   orders[]    所有调拨单(前端按 channel 过滤)
//   missing[]   价格表缺失的 SKU(给跑哥维护用)
func (h *DashboardHandler) GetSpecialChannelAllotSummary(w http.ResponseWriter, r *http.Request) {
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	dept := r.URL.Query().Get("dept") // ecommerce / instant_retail / 空=全部(向后兼容)
	if start == "" {
		// 默认拉最近 60 天
		start = time.Now().AddDate(0, 0, -60).Format("2006-01-02")
	}
	if end == "" {
		end = time.Now().Format("2006-01-02")
	}

	// 1) 各渠道 KPI
	type ChannelSummary struct {
		ChannelKey       string  `json:"channelKey"`
		ChannelName      string  `json:"channelName"`
		CompletedOrders  int     `json:"completedOrders"`
		CompletedSales   float64 `json:"completedSales"`
		PendingOrders    int     `json:"pendingOrders"`
		PendingSales     float64 `json:"pendingSales"`
		TotalOrders      int     `json:"totalOrders"`
		TotalSales       float64 `json:"totalSales"`
	}
	// 按部门拆分: 朴朴归即时零售, 京东+猫超归电商
	channelMapByDept := map[string]map[string]string{
		"ecommerce": {
			"京东": "ds-京东-清心湖自营",
			"猫超": "ds-天猫超市-寄售",
		},
		"instant_retail": {
			"朴朴": "js-即时零售事业一部（世创）-朴朴",
		},
	}
	// 按 dept 过滤 channelMap, 空 dept 返回全部(向后兼容老调用)
	channelMap := map[string]string{}
	channelOrder := []string{} // 保证返回顺序稳定
	if dept == "" || dept == "ecommerce" {
		for _, k := range []string{"京东", "猫超"} {
			channelMap[k] = channelMapByDept["ecommerce"][k]
			channelOrder = append(channelOrder, k)
		}
	}
	if dept == "" || dept == "instant_retail" {
		for _, k := range []string{"朴朴"} {
			channelMap[k] = channelMapByDept["instant_retail"][k]
			channelOrder = append(channelOrder, k)
		}
	}
	if len(channelMap) == 0 {
		writeError(w, 400, "dept 参数无效, 仅支持 ecommerce / instant_retail / 空")
		return
	}

	rows, err := h.DB.Query(`
		SELECT o.channel_key, o.in_status, COUNT(DISTINCT o.allocate_no) AS orders,
			ROUND(IFNULL(SUM(d.excel_amount), 0), 2) AS sales
		FROM allocate_orders o
		LEFT JOIN allocate_details d ON o.allocate_no = d.allocate_no
		WHERE DATE(o.gmt_modified) BETWEEN ? AND ?
		GROUP BY o.channel_key, o.in_status`, start, end)
	if writeDatabaseError(w, err) {
		return
	}
	defer rows.Close()

	summaryByCh := map[string]*ChannelSummary{}
	for k, name := range channelMap {
		summaryByCh[k] = &ChannelSummary{ChannelKey: k, ChannelName: name}
	}
	for rows.Next() {
		var ck string
		var inStatus, orders int
		var sales float64
		if writeDatabaseError(w, rows.Scan(&ck, &inStatus, &orders, &sales)) {
			return
		}
		s, ok := summaryByCh[ck]
		if !ok {
			continue
		}
		if inStatus == 3 {
			s.CompletedOrders += orders
			s.CompletedSales += sales
		} else {
			s.PendingOrders += orders
			s.PendingSales += sales
		}
		s.TotalOrders += orders
		s.TotalSales += sales
	}
	if writeDatabaseError(w, rows.Err()) {
		return
	}
	summary := make([]*ChannelSummary, 0, len(channelOrder))
	for _, k := range channelOrder {
		summary = append(summary, summaryByCh[k])
	}

	// 2) 全部调拨单(列表)
	type OrderRow struct {
		AllocateNo      string   `json:"allocateNo"`
		ChannelKey      string   `json:"channelKey"`
		InWarehouseName string   `json:"inWarehouseName"`
		InStatus        int      `json:"inStatus"`
		Status          int      `json:"status"`
		GmtCreate       string   `json:"gmtCreate"`
		GmtModified     string   `json:"gmtModified"`
		StatDate        string   `json:"statDate"`
		SkuCount        int      `json:"skuCount"`
		ExcelSales      float64  `json:"excelSales"`
		ApiSales        float64  `json:"apiSales"`
	}
	oRows, err := h.DB.Query(`
		SELECT o.allocate_no, o.channel_key, o.in_warehouse_name, o.in_status, o.status,
			DATE_FORMAT(o.gmt_create, '%Y-%m-%d %H:%i'),
			DATE_FORMAT(o.gmt_modified, '%Y-%m-%d %H:%i'),
			IFNULL(DATE_FORMAT(o.stat_date, '%Y-%m-%d'), ''),
			o.sku_count,
			ROUND(IFNULL((SELECT SUM(excel_amount) FROM allocate_details d WHERE d.allocate_no=o.allocate_no), 0), 2),
			ROUND(o.total_amount, 2)
		FROM allocate_orders o
		WHERE DATE(o.gmt_modified) BETWEEN ? AND ?
		ORDER BY o.gmt_modified DESC, o.allocate_no DESC`, start, end)
	if writeDatabaseError(w, err) {
		return
	}
	defer oRows.Close()
	orders := []OrderRow{}
	for oRows.Next() {
		var o OrderRow
		var gc, gm sql.NullString
		if writeDatabaseError(w, oRows.Scan(&o.AllocateNo, &o.ChannelKey, &o.InWarehouseName, &o.InStatus, &o.Status,
			&gc, &gm, &o.StatDate, &o.SkuCount, &o.ExcelSales, &o.ApiSales)) {
			return
		}
		// 按 dept 过滤: 不在当前 channelMap 的渠道跳过
		if _, ok := channelMap[o.ChannelKey]; !ok {
			continue
		}
		o.GmtCreate = gc.String
		o.GmtModified = gm.String
		orders = append(orders, o)
	}
	if writeDatabaseError(w, oRows.Err()) {
		return
	}

	// 3) 缺失 SKU 清单 (Excel 没维护的)
	type MissingRow struct {
		ChannelKey  string  `json:"channelKey"`
		GoodsNo     string  `json:"goodsNo"`
		Barcode     string  `json:"barcode"`
		GoodsName   string  `json:"goodsName"`
		AllocateCnt int     `json:"allocateCnt"`
		QtyTotal    float64 `json:"qtyTotal"`
	}
	mRows, err := h.DB.Query(`
		SELECT d.channel_key, d.goods_no, d.sku_barcode, d.goods_name,
			COUNT(DISTINCT d.allocate_no) AS cnt,
			ROUND(SUM(d.sku_count), 2) AS qty
		FROM allocate_details d
		JOIN allocate_orders o ON o.allocate_no = d.allocate_no
		WHERE d.price_source = 'missing'
		  AND DATE(o.gmt_modified) BETWEEN ? AND ?
		GROUP BY d.channel_key, d.goods_no, d.sku_barcode, d.goods_name
		ORDER BY qty DESC`, start, end)
	if writeDatabaseError(w, err) {
		return
	}
	defer mRows.Close()
	missing := []MissingRow{}
	for mRows.Next() {
		var m MissingRow
		if writeDatabaseError(w, mRows.Scan(&m.ChannelKey, &m.GoodsNo, &m.Barcode, &m.GoodsName, &m.AllocateCnt, &m.QtyTotal)) {
			return
		}
		// 按 dept 过滤
		if _, ok := channelMap[m.ChannelKey]; !ok {
			continue
		}
		missing = append(missing, m)
	}

	writeJSON(w, map[string]interface{}{
		"summary": summary,
		"orders":  orders,
		"missing": missing,
		"start":   start,
		"end":     end,
	})
}

// GetSpecialChannelAllotDetails 单个调拨单的 SKU 明细
// GET /api/special-channel-allot/details?allocate_no=XXX
func (h *DashboardHandler) GetSpecialChannelAllotDetails(w http.ResponseWriter, r *http.Request) {
	allotNo := r.URL.Query().Get("allocate_no")
	if allotNo == "" {
		writeError(w, 400, "allocate_no required")
		return
	}

	type DetailRow struct {
		GoodsNo     string  `json:"goodsNo"`
		SkuBarcode  string  `json:"skuBarcode"`
		GoodsName   string  `json:"goodsName"`
		SkuName     string  `json:"skuName"`
		SkuCount    float64 `json:"skuCount"`
		OutCount    float64 `json:"outCount"`
		InCount     float64 `json:"inCount"`
		ExcelPrice  float64 `json:"excelPrice"`
		ExcelAmount float64 `json:"excelAmount"`
		SkuPrice    float64 `json:"skuPrice"`
		ApiAmount   float64 `json:"apiAmount"`
		PriceSource string  `json:"priceSource"`
	}

	rows, err := h.DB.Query(`
		SELECT goods_no, sku_barcode, goods_name, sku_name,
			sku_count, out_count, in_count,
			excel_price, excel_amount, sku_price, total_amount, price_source
		FROM allocate_details
		WHERE allocate_no=?
		ORDER BY excel_amount DESC`, allotNo)
	if writeDatabaseError(w, err) {
		return
	}
	defer rows.Close()
	list := []DetailRow{}
	for rows.Next() {
		var d DetailRow
		if writeDatabaseError(w, rows.Scan(&d.GoodsNo, &d.SkuBarcode, &d.GoodsName, &d.SkuName,
			&d.SkuCount, &d.OutCount, &d.InCount,
			&d.ExcelPrice, &d.ExcelAmount, &d.SkuPrice, &d.ApiAmount, &d.PriceSource)) {
			return
		}
		list = append(list, d)
	}
	writeJSON(w, map[string]interface{}{"list": list})
}
