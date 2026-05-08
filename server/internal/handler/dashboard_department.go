package handler

import (
	"net/http"
	"sort"
	"strings"
	"time"
)

// excludeAllotShopsCond v1.04 跑哥要求: 电商店铺看板剔除走特殊渠道调拨对账的渠道
// (这两个渠道按调拨单算销售额 /api/special-channel-allot, 销售单不在电商部门重复计)
// 仅对 dept='ecommerce' 生效, 其他部门和综合看板 (overview) 不动
// 综合看板继续显示总数 + 前端文案标"电商部门含特殊渠道调拨金额"提示用户口径
const ecommerceExcludeAllotCond = ` AND shop_name NOT IN ('ds-京东-清心湖自营','ds-天猫超市-寄售')`
const ecommerceExcludeAllotCondAlias = ` AND s.shop_name NOT IN ('ds-京东-清心湖自营','ds-天猫超市-寄售')`

// extraDeptCond 返回特定部门的额外过滤 SQL (alias=true 用 s.shop_name 别名)
func extraDeptCond(dept string, alias bool) string {
	if dept == "ecommerce" {
		if alias {
			return ecommerceExcludeAllotCondAlias
		}
		return ecommerceExcludeAllotCond
	}
	return ""
}

func (h *DashboardHandler) GetDepartmentDetail(w http.ResponseWriter, r *http.Request) {
	dept := r.URL.Query().Get("dept")
	if dept == "" {
		writeError(w, 400, "dept is required")
		return
	}
	start, end := getDateRange(r, h.DB)
	shop := r.URL.Query().Get("shop")         // 可选：按店铺过滤
	platform := r.URL.Query().Get("platform") // 可选：按平台过滤

	// 线下大区合并：shop_name → 大区名映射
	const offlineRegionExpr = `CASE
		WHEN shop_name LIKE '%华东大区%' THEN '华东大区'
		WHEN shop_name LIKE '%华北大区%' THEN '华北大区'
		WHEN shop_name LIKE '%华南大区%' THEN '华南大区'
		WHEN shop_name LIKE '%华中大区%' THEN '华中大区'
		WHEN shop_name LIKE '%西北大区%' THEN '西北大区'
		WHEN shop_name LIKE '%西南大区%' THEN '西南大区'
		WHEN shop_name LIKE '%东北大区%' THEN '东北大区'
		WHEN shop_name LIKE '%山东大区%' OR shop_name LIKE '%山东省区%' THEN '山东大区'
		WHEN shop_name LIKE '%重客系统%' THEN '重客'
		ELSE NULL END`
	offlineRegionCond := map[string]string{
		"华东大区": "shop_name LIKE '%华东大区%'",
		"华北大区": "shop_name LIKE '%华北大区%'",
		"华南大区": "shop_name LIKE '%华南大区%'",
		"华中大区": "shop_name LIKE '%华中大区%'",
		"西北大区": "shop_name LIKE '%西北大区%'",
		"西南大区": "shop_name LIKE '%西南大区%'",
		"东北大区": "shop_name LIKE '%东北大区%'",
		"山东大区": "(shop_name LIKE '%山东大区%' OR shop_name LIKE '%山东省区%')",
		"重客":   "shop_name LIKE '%重客系统%'",
	}

	// 构建额外条件
	shopCond := ""
	extraArgs := []interface{}{}
	if shop != "" {
		if dept == "offline" {
			if cond, ok := offlineRegionCond[shop]; ok {
				shopCond = " AND " + cond
			}
		} else {
			shopCond = " AND shop_name = ?"
			extraArgs = append(extraArgs, shop)
		}
	}
	platCond, platArgs := buildPlatformCond(dept, platform)
	extraArgs = append(extraArgs, platArgs...)
	scopeCond, scopeArgs, err := buildSalesDataScopeCond(r, dept, platform, shop)
	if writeScopeError(w, err) {
		return
	}
	extraArgs = append(extraArgs, scopeArgs...)

	// v1.04: 电商部门剔除走特殊渠道调拨对账的渠道 (跑哥要求)
	// 拼到 scopeCond 上, 所有 17 处 sales_goods_summary SQL 自动生效
	// 用 strings.ReplaceAll(_, "shop_name", "s.shop_name") 的 SQL 也同步替换别名
	scopeCond += extraDeptCond(dept, false)

	// 1. 每日趋势（短范围自动扩展）
	trendStart, trendEnd := getTrendDateRange(start, end)
	trendArgs := append([]interface{}{dept, trendStart, trendEnd}, extraArgs...)
	trendRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date, '%Y-%m-%d'),
			ROUND(SUM(local_goods_amt), 2) as sales,
			ROUND(SUM(goods_qty), 0) as qty,
			ROUND(SUM(gross_profit), 2) as profit,
			ROUND(SUM(IFNULL(goods_cost,0) + IFNULL(fixed_cost,0)), 2) as cost
		FROM sales_goods_summary
		WHERE department = ? AND stat_date BETWEEN ? AND ?`+shopCond+platCond+`
		`+scopeCond+`
		GROUP BY stat_date ORDER BY stat_date`, trendArgs...)
	if !ok {
		return
	}
	defer trendRows.Close()

	type DailyData struct {
		Date   string  `json:"date"`
		Sales  float64 `json:"sales"`
		Qty    float64 `json:"qty"`
		Profit float64 `json:"profit"`
		Cost   float64 `json:"cost"`
	}
	var daily []DailyData
	for trendRows.Next() {
		var d DailyData
		if writeDatabaseError(w, trendRows.Scan(&d.Date, &d.Sales, &d.Qty, &d.Profit, &d.Cost)) {
			return
		}
		daily = append(daily, d)
	}
	if writeDatabaseError(w, trendRows.Err()) {
		return
	}

	// 2. 店铺/大区排行（offline 按大区合并，其余按 shop_name）
	shopListArgs := append([]interface{}{dept, start, end}, platArgs...)
	shopListArgs = append(shopListArgs, scopeArgs...)
	var shopListSQL string
	const offlineRegionPrefilter = ` AND (shop_name LIKE '%大区%' OR shop_name LIKE '%省区%' OR shop_name LIKE '%重客系统%')`
	if dept == "offline" {
		shopListSQL = `SELECT ` + offlineRegionExpr + ` as shop_name,
			ROUND(SUM(local_goods_amt), 2) as sales,
			ROUND(SUM(goods_qty), 0) as qty,
			ROUND(SUM(gross_profit), 2) as profit
		FROM sales_goods_summary
		WHERE department = ? AND shop_name IS NOT NULL
		  AND stat_date BETWEEN ? AND ?` + offlineRegionPrefilter + scopeCond + `
		GROUP BY 1 ORDER BY sales DESC`
	} else {
		shopListSQL = `SELECT shop_name,
			ROUND(SUM(local_goods_amt), 2) as sales,
			ROUND(SUM(goods_qty), 0) as qty,
			ROUND(SUM(gross_profit), 2) as profit
		FROM sales_goods_summary
		WHERE department = ? AND shop_name IS NOT NULL
		  AND stat_date BETWEEN ? AND ?` + platCond + scopeCond + `
		GROUP BY shop_name ORDER BY sales DESC LIMIT 20`
	}
	shopRows, ok := queryRowsOrWriteError(w, h.DB, shopListSQL, shopListArgs...)
	if !ok {
		return
	}
	defer shopRows.Close()

	type ShopData struct {
		ShopName string  `json:"shopName"`
		Sales    float64 `json:"sales"`
		Qty      float64 `json:"qty"`
		Profit   float64 `json:"profit"`
	}
	var shops []ShopData
	for shopRows.Next() {
		var s ShopData
		if writeDatabaseError(w, shopRows.Scan(&s.ShopName, &s.Sales, &s.Qty, &s.Profit)) {
			return
		}
		shops = append(shops, s)
	}
	if writeDatabaseError(w, shopRows.Err()) {
		return
	}

	// 3. 商品排行
	goodsArgs := append([]interface{}{dept, start, end}, extraArgs...)
	goodsRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT s.goods_no, s.goods_name, s.brand_name, IFNULL(s.cate_name,''),
			ROUND(SUM(s.local_goods_amt), 2) as sales,
			ROUND(SUM(s.goods_qty), 0) as qty,
			ROUND(SUM(s.gross_profit), 2) as profit,
			IFNULL(g.goods_field7,'') as grade
		FROM sales_goods_summary s
		LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods WHERE goods_field7 IS NOT NULL AND goods_field7 != '') g ON g.goods_no = s.goods_no
		WHERE s.department = ? AND s.goods_no IS NOT NULL
		  AND s.stat_date BETWEEN ? AND ?`+strings.ReplaceAll(shopCond+platCond+scopeCond, "shop_name", "s.shop_name")+`
		GROUP BY s.goods_no, s.goods_name, s.brand_name, s.cate_name, g.goods_field7
		ORDER BY sales DESC LIMIT 15`, goodsArgs...)
	if !ok {
		return
	}
	defer goodsRows.Close()

	type GoodsData struct {
		GoodsNo  string  `json:"goodsNo"`
		Name     string  `json:"goodsName"`
		Brand    string  `json:"brand"`
		Category string  `json:"category"`
		Sales    float64 `json:"sales"`
		Qty      float64 `json:"qty"`
		Profit   float64 `json:"profit"`
		Grade    string  `json:"grade"`
	}
	var goods []GoodsData
	for goodsRows.Next() {
		var g GoodsData
		if writeDatabaseError(w, goodsRows.Scan(&g.GoodsNo, &g.Name, &g.Brand, &g.Category, &g.Sales, &g.Qty, &g.Profit, &g.Grade)) {
			return
		}
		goods = append(goods, g)
	}
	if writeDatabaseError(w, goodsRows.Err()) {
		return
	}

	// 3.5 商品渠道分布（为TOP15每个商品查各渠道销售额）
	// crossDept=1: 跨 4 部门聚合渠道分布（财务·产品利润页用于看商品在各部门/各渠道的全口径分布）
	crossDept := r.URL.Query().Get("crossDept") == "1"
	type ChannelSales struct {
		ShopName string  `json:"shopName"`
		Sales    float64 `json:"sales"`
		Qty      float64 `json:"qty"`
	}
	goodsChannels := map[string][]ChannelSales{}
	if len(goods) > 0 {
		placeholders := make([]string, len(goods))
		for i := range goods {
			placeholders[i] = "?"
		}
		var chSQL string
		var chArgs []interface{}
		if crossDept {
			// 跨部门：忽略 dept/shop/platform/scope 过滤，按 stat_date+goods_no 全口径聚合
			chArgs = []interface{}{start, end}
			for _, g := range goods {
				chArgs = append(chArgs, g.GoodsNo)
			}
			chSQL = `SELECT goods_no, shop_name,
				ROUND(SUM(local_goods_amt), 2) as sales,
				ROUND(SUM(goods_qty), 0) as qty
			FROM sales_goods_summary
			WHERE stat_date BETWEEN ? AND ?
			  AND goods_no IN (` + joinStrings(placeholders, ",") + `)
			GROUP BY goods_no, shop_name
			ORDER BY goods_no, sales DESC`
		} else {
			chArgs = []interface{}{dept, start, end}
			for _, g := range goods {
				chArgs = append(chArgs, g.GoodsNo)
			}
			chArgs = append(chArgs, extraArgs...)
			if dept == "offline" {
				chSQL = `SELECT goods_no, ` + offlineRegionExpr + ` as shop_name,
					ROUND(SUM(local_goods_amt), 2) as sales,
					ROUND(SUM(goods_qty), 0) as qty
				FROM sales_goods_summary
				WHERE department = ? AND stat_date BETWEEN ? AND ?
				  AND goods_no IN (` + joinStrings(placeholders, ",") + `)` +
					offlineRegionPrefilter + shopCond + scopeCond + `
				GROUP BY goods_no, 2
				ORDER BY goods_no, sales DESC`
			} else {
				chSQL = `SELECT goods_no, shop_name,
					ROUND(SUM(local_goods_amt), 2) as sales,
					ROUND(SUM(goods_qty), 0) as qty
				FROM sales_goods_summary
				WHERE department = ? AND stat_date BETWEEN ? AND ?
				  AND goods_no IN (` + joinStrings(placeholders, ",") + `)` +
					shopCond + platCond + scopeCond + `
				GROUP BY goods_no, shop_name
				ORDER BY goods_no, sales DESC`
			}
		}
		chRows, ok := queryRowsOrWriteError(w, h.DB, chSQL, chArgs...)
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
			goodsChannels[goodsNo] = append(goodsChannels[goodsNo], ChannelSales{ShopName: shopName, Sales: sales, Qty: qty})
		}
		if writeDatabaseError(w, chRows.Err()) {
			return
		}
	}

	// 4. 品牌分布
	brandArgs := append([]interface{}{dept, start, end}, extraArgs...)
	brandRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT IFNULL(brand_name,'未知') as brand,
			ROUND(SUM(local_goods_amt), 2) as sales
		FROM sales_goods_summary
		WHERE department = ? AND stat_date BETWEEN ? AND ?`+shopCond+platCond+scopeCond+`
		GROUP BY brand_name ORDER BY sales DESC LIMIT 10`, brandArgs...)
	if !ok {
		return
	}
	defer brandRows.Close()

	type BrandData struct {
		Brand string  `json:"brand"`
		Sales float64 `json:"sales"`
	}
	var brands []BrandData
	for brandRows.Next() {
		var b BrandData
		if writeDatabaseError(w, brandRows.Scan(&b.Brand, &b.Sales)) {
			return
		}
		brands = append(brands, b)
	}
	if writeDatabaseError(w, brandRows.Err()) {
		return
	}

	// 4.5 产品定位分布
	type GradeData struct {
		Grade string  `json:"grade"`
		Sales float64 `json:"sales"`
	}
	var grades []GradeData
	gradeArgs := append([]interface{}{dept, start, end}, extraArgs...)
	gradeRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT IFNULL(g.goods_field7,'未设置') as grade,
			ROUND(SUM(s.local_goods_amt), 2) as sales
		FROM sales_goods_summary s
		LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods) g ON g.goods_no = s.goods_no
		WHERE s.department = ? AND s.stat_date BETWEEN ? AND ?`+strings.ReplaceAll(shopCond+platCond+scopeCond, "shop_name", "s.shop_name")+`
		GROUP BY g.goods_field7
		ORDER BY FIELD(g.goods_field7,'S','A','B','C','D'), sales DESC`, gradeArgs...)
	if !ok {
		return
	}
	defer gradeRows.Close()
	for gradeRows.Next() {
		var gd GradeData
		if writeDatabaseError(w, gradeRows.Scan(&gd.Grade, &gd.Sales)) {
			return
		}
		grades = append(grades, gd)
	}
	if writeDatabaseError(w, gradeRows.Err()) {
		return
	}

	// 4.6 产品定位×平台销售分布（电商+社媒部门，平台维度通过 sales_channel.online_plat_name）
	type GradePlatItem struct {
		Grade    string  `json:"grade"`
		Platform string  `json:"platform"`
		Sales    float64 `json:"sales"`
	}
	var gradePlatSales []GradePlatItem
	if dept == "ecommerce" || dept == "social" || dept == "instant_retail" {
		gpRows, ok := queryRowsOrWriteError(w, h.DB, `
			SELECT IFNULL(g.goods_field7,'未设置') as grade,
			CASE
				WHEN s.shop_name LIKE '%即时零售%' THEN '即时零售'
				WHEN sc.online_plat_name IS NULL OR sc.online_plat_name = '' THEN '其他'
				ELSE sc.online_plat_name
			END AS plat,
			ROUND(SUM(s.local_goods_amt),2) as sales
			FROM sales_goods_summary s
			LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods) g ON g.goods_no = s.goods_no
			LEFT JOIN (SELECT channel_name, department, MAX(online_plat_name) AS online_plat_name FROM sales_channel GROUP BY channel_name, department) sc ON sc.channel_name = s.shop_name AND sc.department = s.department
			WHERE s.department = ? AND s.stat_date BETWEEN ? AND ?`+scopeCond+`
			GROUP BY grade, plat
			ORDER BY FIELD(grade,'S','A','B','C','D'), sales DESC`, dept, start, end)
		if !ok {
			return
		}
		defer gpRows.Close()
		platLabelMap := map[string]string{
			"天猫商城": "天猫", "天猫超市": "天猫超市", "京东": "京东",
			"拼多多": "拼多多", "唯品会MP": "唯品会", "唯品会JIT": "唯品会",
			"抖音超市": "即时零售", "淘宝": "淘宝",
			"放心购（抖音小店）": "抖音", "快手小店": "快手",
			"小红书": "小红书", "有赞": "有赞", "微店": "微店",
			"微信视频号小店": "视频号",
		}
		gpKey := func(grade, plat string) string { return grade + "|" + plat }
		gpMap := map[string]*GradePlatItem{}
		for gpRows.Next() {
			var grade, rawPlat string
			var sales float64
			if writeDatabaseError(w, gpRows.Scan(&grade, &rawPlat, &sales)) {
				return
			}
			label := rawPlat
			if l, ok := platLabelMap[rawPlat]; ok {
				label = l
			}
			key := gpKey(grade, label)
			if ps, ok := gpMap[key]; ok {
				ps.Sales += sales
			} else {
				gpMap[key] = &GradePlatItem{Grade: grade, Platform: label, Sales: sales}
			}
		}
		if writeDatabaseError(w, gpRows.Err()) {
			return
		}
		for _, ps := range gpMap {
			gradePlatSales = append(gradePlatSales, *ps)
		}
		sort.Slice(gradePlatSales, func(i, j int) bool {
			gradeOrder := map[string]int{"S": 0, "A": 1, "B": 2, "C": 3, "D": 4, "未设置": 5}
			gi, gj := gradeOrder[gradePlatSales[i].Grade], gradeOrder[gradePlatSales[j].Grade]
			if gi != gj {
				return gi < gj
			}
			return gradePlatSales[i].Sales > gradePlatSales[j].Sales
		})
	} else if dept == "offline" || dept == "distribution" {
		// 线下：按大区合并；分销：保留原 shop_name
		var gpSQL string
		if dept == "offline" {
			gpSQL = `SELECT IFNULL(g.goods_field7,'未设置') as grade,
				` + offlineRegionExpr + ` as channel,
				ROUND(SUM(s.local_goods_amt),2) as sales
				FROM sales_goods_summary s
				LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods) g ON g.goods_no = s.goods_no
				WHERE s.department = ? AND s.stat_date BETWEEN ? AND ?` + offlineRegionPrefilter + scopeCond + `
				GROUP BY 1, 2
				ORDER BY FIELD(grade,'S','A','B','C','D'), sales DESC`
		} else {
			gpSQL = `SELECT IFNULL(g.goods_field7,'未设置') as grade, s.shop_name as channel,
				ROUND(SUM(s.local_goods_amt),2) as sales
				FROM sales_goods_summary s
				LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods) g ON g.goods_no = s.goods_no
				WHERE s.department = ? AND s.stat_date BETWEEN ? AND ?` + scopeCond + `
				GROUP BY grade, s.shop_name
				ORDER BY FIELD(grade,'S','A','B','C','D'), sales DESC`
		}
		gpRows, ok := queryRowsOrWriteError(w, h.DB, gpSQL, dept, start, end)
		if !ok {
			return
		}
		defer gpRows.Close()
		for gpRows.Next() {
			var grade, channel string
			var sales float64
			if writeDatabaseError(w, gpRows.Scan(&grade, &channel, &sales)) {
				return
			}
			gradePlatSales = append(gradePlatSales, GradePlatItem{Grade: grade, Platform: channel, Sales: sales})
		}
		if writeDatabaseError(w, gpRows.Err()) {
			return
		}
	}

	// 5. 平台列表（合并后，只返回有销售数据的）
	// 先查有数据的原始平台名
	platRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DISTINCT sc.online_plat_name
		FROM sales_channel sc
		INNER JOIN sales_goods_summary s ON s.shop_name = sc.channel_name
		WHERE sc.department = ? AND sc.online_plat_name IS NOT NULL AND sc.online_plat_name != ''
		  AND s.stat_date BETWEEN ? AND ? AND s.department = ?`, dept, start, end, dept)
	if !ok {
		return
	}
	defer platRows.Close()
	rawPlats := map[string]bool{}
	for platRows.Next() {
		var p string
		if writeDatabaseError(w, platRows.Scan(&p)) {
			return
		}
		rawPlats[p] = true
	}
	if writeDatabaseError(w, platRows.Err()) {
		return
	}

	// 即时零售特殊检查：按店铺名匹配
	var instantCount int
	if writeDatabaseError(w, h.DB.QueryRow(`SELECT COUNT(DISTINCT shop_name) FROM sales_goods_summary
		WHERE shop_name LIKE '%即时零售%' AND stat_date BETWEEN ? AND ?
		AND shop_name IN (SELECT channel_name FROM sales_channel WHERE department = ?)
		AND department = ?`, start, end, dept, dept).Scan(&instantCount)) {
		return
	}

	// 按合并规则生成平台Tab列表
	type PlatTab struct {
		Key   string `json:"key"`
		Label string `json:"label"`
	}
	platTabDefs := []PlatTab{
		{"tmall", "天猫"}, {"tmall_cs", "天猫超市"}, {"jd", "京东"}, {"pdd", "拼多多"},
		{"vip", "唯品会"}, {"taobao", "淘宝"}, {"instant", "即时零售"},
		{"douyin", "抖音"}, {"kuaishou", "快手"}, {"xiaohongshu", "小红书"},
		{"youzan", "有赞"}, {"weidian", "微店"}, {"shipinhao", "视频号"},
	}
	var platforms []PlatTab
	for _, pt := range platTabDefs {
		if !isPlatformAllowedForDept(dept, pt.Key) {
			continue
		}
		if pt.Key == "instant" {
			if instantCount > 0 {
				platforms = append(platforms, pt)
			}
			continue
		}
		plats := platformToPlats[pt.Key]
		hasData := false
		for _, p := range plats {
			if rawPlats[p] {
				hasData = true
				break
			}
		}
		if hasData {
			platforms = append(platforms, pt)
		}
	}

	// 6. 平台销售额分布
	type PlatSales struct {
		Platform string  `json:"platform"`
		Sales    float64 `json:"sales"`
		Qty      float64 `json:"qty"`
	}
	var platformSales []PlatSales
	platSalesRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT CASE
			WHEN s.shop_name LIKE '%即时零售%' THEN '即时零售'
			WHEN sc.online_plat_name IS NULL OR sc.online_plat_name = '' THEN '其他'
			ELSE sc.online_plat_name
		END AS plat,
		ROUND(SUM(s.local_goods_amt),2), ROUND(SUM(s.goods_qty),0)
		FROM sales_goods_summary s
		LEFT JOIN (SELECT channel_name, department, MAX(online_plat_name) AS online_plat_name FROM sales_channel GROUP BY channel_name, department) sc ON sc.channel_name = s.shop_name AND sc.department = s.department
		WHERE s.department = ? AND s.stat_date BETWEEN ? AND ?`+scopeCond+`
		GROUP BY plat
		ORDER BY SUM(s.local_goods_amt) DESC`, dept, start, end)
	if !ok {
		return
	}
	defer platSalesRows.Close()
	platSalesMap := map[string]*PlatSales{}
	for platSalesRows.Next() {
		var rawPlat string
		var sales, qty float64
		if writeDatabaseError(w, platSalesRows.Scan(&rawPlat, &sales, &qty)) {
			return
		}
		// 合并平台：通过 platformToPlats 反查分组
		label := rawPlat
		for key, plats := range platformToPlats {
			for _, p := range plats {
				if p == rawPlat {
					for _, pt := range platTabDefs {
						if pt.Key == key {
							label = pt.Label
							break
						}
					}
					break
				}
			}
		}
		if ps, ok := platSalesMap[label]; ok {
			ps.Sales += sales
			ps.Qty += qty
		} else {
			platSalesMap[label] = &PlatSales{Platform: label, Sales: sales, Qty: qty}
		}
	}
	if writeDatabaseError(w, platSalesRows.Err()) {
		return
	}
	for _, ps := range platSalesMap {
		platformSales = append(platformSales, *ps)
	}
	// 按销售额降序
	sort.Slice(platformSales, func(i, j int) bool {
		return platformSales[i].Sales > platformSales[j].Sales
	})

	// offline 补充：查询日期范围内各月目标累加
	regionTargets := map[string]float64{}
	if dept == "offline" {
		// 解析 start/end 年月
		startTime, _ := time.Parse("2006-01-02", start)
		endTime, _ := time.Parse("2006-01-02", end)
		if !startTime.IsZero() && !endTime.IsZero() {
			tRows, tOk := queryRowsOrWriteError(w, h.DB, `
				SELECT region, SUM(target)
				FROM offline_region_target
				WHERE (year*100+month) BETWEEN ? AND ?
				GROUP BY region`,
				startTime.Year()*100+int(startTime.Month()),
				endTime.Year()*100+int(endTime.Month()),
			)
			if tOk {
				defer tRows.Close()
				for tRows.Next() {
					var reg string
					var tgt float64
					if writeDatabaseError(w, tRows.Scan(&reg, &tgt)) {
						return
					}
					regionTargets[reg] = tgt
				}
			}
		}
	}

	// crossDept=1 额外返回：产品定位×部门 + 产品定位×店铺 全口径聚合（含毛利）
	type GradeDeptItem struct {
		Grade      string  `json:"grade"`
		Department string  `json:"department"`
		Sales      float64 `json:"sales"`
		Profit     float64 `json:"profit"`
	}
	type GradeShopItem struct {
		Grade      string  `json:"grade"`
		Department string  `json:"department"`
		ShopName   string  `json:"shopName"`
		Sales      float64 `json:"sales"`
		Profit     float64 `json:"profit"`
	}
	var gradeDeptSalesAll []GradeDeptItem
	var gradeShopSalesAll []GradeShopItem
	if crossDept {
		gdRows, ok := queryRowsOrWriteError(w, h.DB, `
			SELECT IFNULL(g.goods_field7,'未设置') as grade, s.department,
				ROUND(SUM(s.local_goods_amt),2) as sales,
				ROUND(SUM(s.gross_profit),2) as profit
			FROM sales_goods_summary s
			LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods) g ON g.goods_no = s.goods_no
			WHERE s.stat_date BETWEEN ? AND ?
			GROUP BY grade, s.department
			ORDER BY FIELD(grade,'S','A','B','C','D','未设置'), sales DESC`, start, end)
		if !ok {
			return
		}
		defer gdRows.Close()
		for gdRows.Next() {
			var it GradeDeptItem
			if writeDatabaseError(w, gdRows.Scan(&it.Grade, &it.Department, &it.Sales, &it.Profit)) {
				return
			}
			gradeDeptSalesAll = append(gradeDeptSalesAll, it)
		}
		if writeDatabaseError(w, gdRows.Err()) {
			return
		}

		gsRows, ok := queryRowsOrWriteError(w, h.DB, `
			SELECT IFNULL(g.goods_field7,'未设置') as grade,
				IFNULL(s.department,'其他') as department,
				s.shop_name,
				ROUND(SUM(s.local_goods_amt),2) as sales,
				ROUND(SUM(s.gross_profit),2) as profit
			FROM sales_goods_summary s
			LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods) g ON g.goods_no = s.goods_no
			WHERE s.stat_date BETWEEN ? AND ?
			GROUP BY grade, s.department, s.shop_name
			ORDER BY FIELD(grade,'S','A','B','C','D','未设置'), sales DESC`, start, end)
		if !ok {
			return
		}
		defer gsRows.Close()
		for gsRows.Next() {
			var it GradeShopItem
			if writeDatabaseError(w, gsRows.Scan(&it.Grade, &it.Department, &it.ShopName, &it.Sales, &it.Profit)) {
				return
			}
			gradeShopSalesAll = append(gradeShopSalesAll, it)
		}
		if writeDatabaseError(w, gsRows.Err()) {
			return
		}
	}

	writeJSON(w, map[string]interface{}{
		"daily":             daily,
		"shops":             shops,
		"goods":             goods,
		"goodsChannels":     goodsChannels,
		"brands":            brands,
		"grades":            grades,
		"platforms":         platforms,
		"platformSales":     platformSales,
		"gradePlatSales":    gradePlatSales,
		"gradeDeptSalesAll": gradeDeptSalesAll,
		"gradeShopSalesAll": gradeShopSalesAll,
		"regionTargets":     regionTargets,
		"dateRange":         map[string]string{"start": start, "end": end},
		"trendRange":        map[string]string{"start": trendStart, "end": trendEnd},
	})
}
