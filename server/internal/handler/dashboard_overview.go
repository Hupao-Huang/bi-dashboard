package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// DeptSummary 综合看板各部门汇总 (v1.74.3 提到包级别, 让 applyEcommerceAllotAdjustment 能引用)
type DeptSummary struct {
	Department string  `json:"department"`
	Sales      float64 `json:"sales"`
	Qty        float64 `json:"qty"`
	Profit     float64 `json:"profit"`
	Cost       float64 `json:"cost"`
	SkuCount   int     `json:"skuCount"`
	SalesAmt   float64 `json:"salesAmt,omitempty"` // v1.74.3: 电商部排除 2 调拨渠道后的销售口径
	AllotAmt   float64 `json:"allotAmt,omitempty"` // v1.74.3: 电商部 2 调拨渠道的调拨口径
}

// GetOverview 综合看板
func (h *DashboardHandler) GetOverview(w http.ResponseWriter, r *http.Request) {
	start, end := getDateRange(r, h.DB)
	trendStart, trendEnd := getOverviewTrendRange(r, start, end)
	scopeCond, scopeArgs, err := buildSalesDataScopeCond(r, "", "", "")
	if writeScopeError(w, err) {
		return
	}

	cacheKey := buildOverviewCacheKey(r, start, end, trendStart, trendEnd)
	if cached, ok := getOverviewCache(cacheKey); ok {
		writeJSON(w, cached)
		return
	}

	// 1. 各部门汇总（含未映射部门，归入other）
	deptArgs := append([]interface{}{start, end}, scopeArgs...)
	deptRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT CASE WHEN department IS NULL OR department = '' THEN 'other' ELSE department END as dept,
			ROUND(SUM(IFNULL(local_goods_amt, goods_amt)), 2) as sales,
			ROUND(SUM(goods_qty), 0) as qty,
			ROUND(SUM(gross_profit), 2) as profit,
			ROUND(SUM(goods_cost), 2) as cost,
			COUNT(DISTINCT goods_id) as sku_count
		FROM sales_goods_summary
		WHERE stat_date BETWEEN ? AND ?
		  AND IFNULL(department,'') NOT IN ('excluded','other','')`+scopeCond+`
		GROUP BY dept
		ORDER BY sales DESC`, deptArgs...)
	if !ok {
		return
	}
	defer deptRows.Close()

	deptMap := map[string]DeptSummary{}
	for deptRows.Next() {
		var d DeptSummary
		if writeDatabaseError(w, deptRows.Scan(&d.Department, &d.Sales, &d.Qty, &d.Profit, &d.Cost, &d.SkuCount)) {
			return
		}
		deptMap[d.Department] = d
	}
	if writeDatabaseError(w, deptRows.Err()) {
		return
	}
	// 确保 5 个部门都返回（没数据的补0）
	// v1.02: 加 instant_retail 即时零售部 (5 个 js- 渠道)
	allDepts := []string{"ecommerce", "social", "offline", "distribution", "instant_retail"}
	var deptList []DeptSummary
	for _, dept := range allDepts {
		if d, ok := deptMap[dept]; ok {
			deptList = append(deptList, d)
		} else {
			deptList = append(deptList, DeptSummary{Department: dept})
		}
	}
	// 加上其他未知部门
	for dept, d := range deptMap {
		found := false
		for _, ad := range allDepts {
			if dept == ad {
				found = true
				break
			}
		}
		if !found {
			deptList = append(deptList, d)
		}
	}

	// v1.74.3: 电商部门 KPI 合并 2 调拨渠道金额 (排除销售单口径 + 加调拨口径)
	// 设计文档 docs/specs/2026-05-25-overview-ecommerce-allot-merge-design.md
	// helper 失败 → log + 不阻塞主流程, 用原口径 (回落到 v1.74.2 之前行为)
	if salesExcluded, allotAmt, allotErr := h.loadEcommerceAllotAdjustment(
		r.Context(), start, end, scopeCond, scopeArgs); allotErr != nil {
		log.Printf("[overview] 调拨口径加载失败, 用原口径: %v", allotErr)
	} else {
		applyEcommerceAllotAdjustment(deptList, salesExcluded, allotAmt)
	}

	// 2. 每日销售趋势（含未映射部门，归入other）
	trendArgs := append([]interface{}{trendStart, trendEnd}, scopeArgs...)
	trendRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date, '%Y-%m-%d') as d,
			CASE WHEN department IS NULL OR department = '' THEN 'other' ELSE department END as dept,
			ROUND(SUM(IFNULL(local_goods_amt, goods_amt)), 2) as sales,
			ROUND(SUM(goods_qty), 0) as qty
		FROM sales_goods_summary
		WHERE stat_date BETWEEN ? AND ?
		  AND IFNULL(department,'') NOT IN ('excluded','other','')`+scopeCond+`
		GROUP BY stat_date, dept
		ORDER BY stat_date`, trendArgs...)
	if !ok {
		return
	}
	defer trendRows.Close()

	type TrendPoint struct {
		Date       string  `json:"date"`
		Department string  `json:"department"`
		Sales      float64 `json:"sales"`
		Qty        float64 `json:"qty"`
	}
	var trend []TrendPoint
	for trendRows.Next() {
		var t TrendPoint
		if writeDatabaseError(w, trendRows.Scan(&t.Date, &t.Department, &t.Sales, &t.Qty)) {
			return
		}
		trend = append(trend, t)
	}
	if writeDatabaseError(w, trendRows.Err()) {
		return
	}

	// 3. 商品销售排行 TOP15
	goodsArgs := append([]interface{}{start, end}, scopeArgs...)
	goodsRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT s.goods_no, s.goods_name, s.brand_name, s.cate_name,
			IFNULL(g.goods_field7,'') as grade,
			ROUND(SUM(s.goods_amt), 2) as sales,
			ROUND(SUM(s.goods_qty), 0) as qty,
			ROUND(SUM(s.gross_profit), 2) as profit
		FROM sales_goods_summary s
		LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods) g ON g.goods_no = s.goods_no
		WHERE s.goods_no IS NOT NULL AND s.goods_no != ''
		  AND s.stat_date BETWEEN ? AND ?
		  AND IFNULL(s.department,'') NOT IN ('excluded','other','')`+withAlias(scopeCond, "s")+`
		GROUP BY s.goods_no, s.goods_name, s.brand_name, s.cate_name, g.goods_field7
		ORDER BY sales DESC LIMIT 15`, goodsArgs...)
	if !ok {
		return
	}
	defer goodsRows.Close()

	type GoodsRank struct {
		GoodsNo  string  `json:"goodsNo"`
		Name     string  `json:"goodsName"`
		Brand    string  `json:"brand"`
		Category string  `json:"category"`
		Grade    string  `json:"grade"`
		Sales    float64 `json:"sales"`
		Qty      float64 `json:"qty"`
		Profit   float64 `json:"profit"`
	}
	var topGoods []GoodsRank
	for goodsRows.Next() {
		var g GoodsRank
		if writeDatabaseError(w, goodsRows.Scan(&g.GoodsNo, &g.Name, &g.Brand, &g.Category, &g.Grade, &g.Sales, &g.Qty, &g.Profit)) {
			return
		}
		topGoods = append(topGoods, g)
	}
	if writeDatabaseError(w, goodsRows.Err()) {
		return
	}

	// 3.5 商品渠道分布
	type OverviewChannelSales struct {
		ShopName string  `json:"shopName"`
		Sales    float64 `json:"sales"`
		Qty      float64 `json:"qty"`
	}
	overviewGoodsChannels := map[string][]OverviewChannelSales{}
	if len(topGoods) > 0 {
		ph := make([]string, len(topGoods))
		chArgs := []interface{}{start, end}
		for i, g := range topGoods {
			ph[i] = "?"
			chArgs = append(chArgs, g.GoodsNo)
		}
		chArgs = append(chArgs, scopeArgs...)
		chRows, ok := queryRowsOrWriteError(w, h.DB, `
			SELECT goods_no, shop_name,
				ROUND(SUM(IFNULL(local_goods_amt, goods_amt)), 2) as sales,
				ROUND(SUM(goods_qty), 0) as qty
			FROM sales_goods_summary
			WHERE stat_date BETWEEN ? AND ?
			  AND goods_no IN (`+joinStrings(ph, ",")+`)
			  AND IFNULL(department,'') NOT IN ('excluded','other','')`+scopeCond+`
			GROUP BY goods_no, shop_name
			ORDER BY goods_no, sales DESC`, chArgs...)
		if !ok {
			return
		}
		defer chRows.Close()
		for chRows.Next() {
			var goodsNo, shopName string
			var sales, qty float64
			if writeDatabaseError(w, chRows.Scan(&goodsNo, &shopName, &sales, &qty)) {
				return
			}
			overviewGoodsChannels[goodsNo] = append(overviewGoodsChannels[goodsNo], OverviewChannelSales{ShopName: shopName, Sales: sales, Qty: qty})
		}
		if writeDatabaseError(w, chRows.Err()) {
			return
		}
	}

	// 4. 店铺/渠道排行 TOP15
	shopArgs := append([]interface{}{start, end}, scopeArgs...)
	shopRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT shop_name, department,
			ROUND(SUM(IFNULL(local_goods_amt, goods_amt)), 2) as sales,
			ROUND(SUM(goods_qty), 0) as qty
		FROM sales_goods_summary
		WHERE shop_name IS NOT NULL AND shop_name != ''
		  AND stat_date BETWEEN ? AND ?
		  AND IFNULL(department,'') NOT IN ('excluded','other','')`+scopeCond+`
		GROUP BY shop_name, department
		ORDER BY sales DESC LIMIT 15`, shopArgs...)
	if !ok {
		return
	}
	defer shopRows.Close()

	type ShopRank struct {
		ShopName   string  `json:"shopName"`
		Department string  `json:"department"`
		Sales      float64 `json:"sales"`
		Qty        float64 `json:"qty"`
	}
	var topShops []ShopRank
	for shopRows.Next() {
		var s ShopRank
		if writeDatabaseError(w, shopRows.Scan(&s.ShopName, &s.Department, &s.Sales, &s.Qty)) {
			return
		}
		topShops = append(topShops, s)
	}
	if writeDatabaseError(w, shopRows.Err()) {
		return
	}

	// 4.5 店铺销售明细 (Top15 店铺各 Top 5 SKU + Top 5 分类, 给 hover tooltip 用)
	type ShopBreakdownGoodsItem struct {
		GoodsNo   string  `json:"goodsNo"`
		GoodsName string  `json:"goodsName"`
		Grade     string  `json:"grade"`
		Sales     float64 `json:"sales"`
	}
	type ShopBreakdownCateItem struct {
		CateName string  `json:"cateName"`
		Sales    float64 `json:"sales"`
	}
	type ShopBreakdownEntry struct {
		TopGoods   []ShopBreakdownGoodsItem `json:"topGoods"`
		TopCates   []ShopBreakdownCateItem  `json:"topCates"`
		TotalSales float64                  `json:"totalSales"`
	}
	shopBreakdown := map[string]*ShopBreakdownEntry{}
	if len(topShops) > 0 {
		shopList := make([]string, 0, len(topShops))
		ph := make([]string, 0, len(topShops))
		breakdownArgs := make([]interface{}, 0, len(topShops)+2+len(scopeArgs))
		for _, s := range topShops {
			shopList = append(shopList, s.ShopName)
			ph = append(ph, "?")
			breakdownArgs = append(breakdownArgs, s.ShopName)
			shopBreakdown[s.ShopName] = &ShopBreakdownEntry{TotalSales: s.Sales}
		}
		breakdownArgs = append(breakdownArgs, start, end)
		breakdownArgs = append(breakdownArgs, scopeArgs...)
		phJoined := strings.Join(ph, ",")

		// Top 5 SKU per shop
		gRows, gok := queryRowsOrWriteError(w, h.DB, `
			WITH RankedGoods AS (
				SELECT s.shop_name, s.goods_no, s.goods_name,
					IFNULL(g.goods_field7,'') as grade,
					ROUND(SUM(IFNULL(s.local_goods_amt, s.goods_amt)),2) as sales,
					ROW_NUMBER() OVER (PARTITION BY s.shop_name ORDER BY SUM(IFNULL(s.local_goods_amt, s.goods_amt)) DESC) as rn
				FROM sales_goods_summary s
				LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods) g ON g.goods_no = s.goods_no
				WHERE s.shop_name IN (`+phJoined+`)
				  AND s.stat_date BETWEEN ? AND ?
				  AND IFNULL(s.department,'') NOT IN ('excluded','other','')`+withAlias(scopeCond, "s")+`
				GROUP BY s.shop_name, s.goods_no, s.goods_name, g.goods_field7
			)
			SELECT shop_name, goods_no, goods_name, grade, sales FROM RankedGoods WHERE rn <= 5
			ORDER BY shop_name, sales DESC`, breakdownArgs...)
		if gok {
			defer gRows.Close()
			for gRows.Next() {
				var sn, gno, gname, grade string
				var sales float64
				if err := gRows.Scan(&sn, &gno, &gname, &grade, &sales); err == nil {
					if sb, exists := shopBreakdown[sn]; exists {
						sb.TopGoods = append(sb.TopGoods, ShopBreakdownGoodsItem{
							GoodsNo: gno, GoodsName: gname, Grade: grade, Sales: sales,
						})
					}
				}
			}
		}

		// Top 5 分类 per shop
		cRows, cok := queryRowsOrWriteError(w, h.DB, `
			WITH RankedCates AS (
				SELECT s.shop_name, IFNULL(NULLIF(s.cate_name,''),'未分类') as cate_name,
					ROUND(SUM(IFNULL(s.local_goods_amt, s.goods_amt)),2) as sales,
					ROW_NUMBER() OVER (PARTITION BY s.shop_name ORDER BY SUM(IFNULL(s.local_goods_amt, s.goods_amt)) DESC) as rn
				FROM sales_goods_summary s
				WHERE s.shop_name IN (`+phJoined+`)
				  AND s.stat_date BETWEEN ? AND ?
				  AND IFNULL(s.department,'') NOT IN ('excluded','other','')`+withAlias(scopeCond, "s")+`
				GROUP BY s.shop_name, s.cate_name
			)
			SELECT shop_name, cate_name, sales FROM RankedCates WHERE rn <= 5
			ORDER BY shop_name, sales DESC`, breakdownArgs...)
		if cok {
			defer cRows.Close()
			for cRows.Next() {
				var sn, cate string
				var sales float64
				if err := cRows.Scan(&sn, &cate, &sales); err == nil {
					if sb, exists := shopBreakdown[sn]; exists {
						sb.TopCates = append(sb.TopCates, ShopBreakdownCateItem{
							CateName: cate, Sales: sales,
						})
					}
				}
			}
		}
	}

	// 5. 产品定位分布
	type GradeDist struct {
		Grade string  `json:"grade"`
		Sales float64 `json:"sales"`
	}
	var grades []GradeDist
	gradeArgs := append([]interface{}{start, end}, scopeArgs...)
	gradeRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT IFNULL(g.goods_field7,'未设置') as grade,
			ROUND(SUM(s.goods_amt), 2) as sales
		FROM sales_goods_summary s
		LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods) g ON g.goods_no = s.goods_no
		WHERE s.stat_date BETWEEN ? AND ?
		  AND IFNULL(s.department,'') NOT IN ('excluded','other','')`+withAlias(scopeCond, "s")+`
		GROUP BY g.goods_field7
		ORDER BY FIELD(g.goods_field7,'S','A','B','C','D'), sales DESC`, gradeArgs...)
	if !ok {
		return
	}
	defer gradeRows.Close()
	for gradeRows.Next() {
		var gd GradeDist
		if writeDatabaseError(w, gradeRows.Scan(&gd.Grade, &gd.Sales)) {
			return
		}
		grades = append(grades, gd)
	}
	if writeDatabaseError(w, gradeRows.Err()) {
		return
	}

	// 6. 产品定位 × 部门明细（含毛利，总览矩阵表用）
	type GradeDeptSales struct {
		Grade      string  `json:"grade"`
		Department string  `json:"department"`
		Sales      float64 `json:"sales"`
		Profit     float64 `json:"profit"`
	}
	var gradeDeptSales []GradeDeptSales
	gdArgs := append([]interface{}{start, end}, scopeArgs...)
	gdRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT IFNULL(g.goods_field7,'未设置') as grade,
			IFNULL(s.department,'其他') as department,
			ROUND(SUM(s.goods_amt), 2) as sales,
			ROUND(SUM(s.gross_profit), 2) as profit
		FROM sales_goods_summary s
		LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods) g ON g.goods_no = s.goods_no
		WHERE s.stat_date BETWEEN ? AND ?
		  AND IFNULL(s.department,'') NOT IN ('excluded','other','')`+withAlias(scopeCond, "s")+`
		GROUP BY g.goods_field7, s.department
		ORDER BY FIELD(g.goods_field7,'S','A','B','C','D'), sales DESC`, gdArgs...)
	if !ok {
		return
	}
	defer gdRows.Close()
	for gdRows.Next() {
		var gd GradeDeptSales
		if writeDatabaseError(w, gdRows.Scan(&gd.Grade, &gd.Department, &gd.Sales, &gd.Profit)) {
			return
		}
		gradeDeptSales = append(gradeDeptSales, gd)
	}
	if writeDatabaseError(w, gdRows.Err()) {
		return
	}

	// 7. 可选日期范围
	var minDate, maxDate string
	_ = h.DB.QueryRow("SELECT IFNULL(MIN(stat_date),''), IFNULL(MAX(stat_date),'') FROM sales_goods_summary").Scan(&minDate, &maxDate)

	response := map[string]interface{}{
		"departments":    deptList,
		"trend":          trend,
		"topGoods":       topGoods,
		"goodsChannels":  overviewGoodsChannels,
		"topShops":       topShops,
		"shopBreakdown":  shopBreakdown,
		"grades":         grades,
		"gradeDeptSales": gradeDeptSales,
		"dateRange":      map[string]string{"start": start, "end": end, "min": minDate, "max": maxDate},
		"trendRange":     map[string]string{"start": trendStart, "end": trendEnd},
	}
	setOverviewCache(cacheKey, response)
	writeJSON(w, response)
}

// loadEcommerceAllotAdjustment v1.74.3: 加载电商部 2 调拨渠道的双口径金额
// 业务背景: ds-京东-清心湖自营 / ds-天猫超市-寄售 这 2 渠道业务上不算销售单, 按调拨入库统计.
// 综合看板长期用销售单口径 → 跟业务对账不一致. 本 helper 单独查 2 渠道双口径.
//
// 返回:
//   salesExcluded: 这 2 渠道在 sales_goods_summary 的销售单口径 SUM (要从 dept.sales 减掉)
//   allotAmt: 这 2 渠道在 allocate_details 的 excel_amount SUM (要加到 dept.sales)
//   err: 任一 query 失败返 err, 调用方决定 fallback 还是 fail
func (h *DashboardHandler) loadEcommerceAllotAdjustment(
	ctx context.Context,
	start, end string,
	scopeCond string, scopeArgs []interface{},
) (salesExcluded, allotAmt float64, err error) {
	// 固定 2 渠道 ID (跟 special_channel.go 一致, channel_id 即 shop_id)
	const jdShopID = "1819610592561398400"   // ds-京东-清心湖自营
	const tmcsShopID = "1819610591915475584" // ds-天猫超市-寄售

	// query 1: 这 2 渠道销售单口径 (要从 dept.sales 减)
	salesArgs := append([]interface{}{start, end, jdShopID, tmcsShopID}, scopeArgs...)
	err = h.DB.QueryRowContext(ctx, `
		SELECT IFNULL(SUM(IFNULL(local_goods_amt, goods_amt)), 0)
		FROM sales_goods_summary
		WHERE stat_date BETWEEN ? AND ?
		  AND shop_id IN (?, ?)`+scopeCond, salesArgs...).Scan(&salesExcluded)
	if err != nil {
		return 0, 0, fmt.Errorf("查 2 渠道销售单口径失败: %w", err)
	}

	// query 2: 这 2 渠道调拨口径 (要加进 dept.sales)
	// channel_key 在 allocate_orders 是 '京东' / '猫超' (跟 special_channel.go 一致)
	err = h.DB.QueryRowContext(ctx, `
		SELECT IFNULL(SUM(d.excel_amount), 0)
		FROM allocate_orders o
		JOIN allocate_details d ON d.allocate_no = o.allocate_no
		WHERE o.stat_date BETWEEN ? AND ?
		  AND o.channel_key IN ('京东', '猫超')`, start, end).Scan(&allotAmt)
	if err != nil {
		return 0, 0, fmt.Errorf("查 2 渠道调拨口径失败: %w", err)
	}

	return salesExcluded, allotAmt, nil
}

// applyEcommerceAllotAdjustment v1.74.3: 把 2 调拨渠道的口径换到 dept.Sales
// 找到 ecommerce dept, 计算:
//   SalesAmt = Sales - salesExcluded (其它电商渠道销售口径)
//   AllotAmt = allotAmt              (这 2 调拨渠道)
//   Sales    = SalesAmt + AllotAmt   (新总和, 给顶部 totalSales / 右上角 tag 用)
// 兜底: SalesAmt < 0 钳到 0 (理论不应发生, 防数据异常)
//
// 提取为独立函数便于单测 (不依赖 DB).
func applyEcommerceAllotAdjustment(deptList []DeptSummary, salesExcluded, allotAmt float64) {
	for i, d := range deptList {
		if d.Department != "ecommerce" {
			continue
		}
		salesAmt := d.Sales - salesExcluded
		if salesAmt < 0 {
			salesAmt = 0
		}
		deptList[i].SalesAmt = salesAmt
		deptList[i].AllotAmt = allotAmt
		deptList[i].Sales = salesAmt + allotAmt
		break
	}
}

