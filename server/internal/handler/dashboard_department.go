package handler

import (
	"bi-dashboard/internal/specialchannel"

	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

// excludeAllotShopsCond v1.04 跑哥要求: 电商店铺看板剔除走特殊渠道调拨对账的渠道
// (这两个渠道按调拨单算销售额 /api/special-channel-allot, 销售单不在电商部门重复计)
// 仅对 dept='ecommerce' 生效, 其他部门和综合看板 (overview) 不动
// 综合看板继续显示总数 + 前端文案标"电商部门含特殊渠道调拨金额"提示用户口径
// 店名清单来自 specialchannel 注册表 (单一来源), 值是白名单拼接无注入风险
var ecommerceExcludeAllotCond = ` AND shop_name NOT IN (` + allotChannelsInClause(specialchannel.ShopNamesByDept(specialchannel.DeptEcommerce)) + `)`
var ecommerceExcludeAllotCondAlias = ` AND s.shop_name NOT IN (` + allotChannelsInClause(specialchannel.ShopNamesByDept(specialchannel.DeptEcommerce)) + `)`

// ====== 即时零售"调拨当销售"渠道 (朴朴/小象/叮咚) ======
// 朴朴: 纯调拨, 无销售单 → 调拨即该店全部销售额。
// 小象/叮咚 (2026-06-05 跑哥追加): 有销售单, 销售额=销售单+调拨 (两批不同货, 不重复)。
// 渠道→店铺名 (店铺看板按店名合并调拨到对应 entry), 来自 specialchannel 注册表
var instantRetailAllotShop = specialchannel.ShopNameByKey(specialchannel.DeptInstantRetail)

// instantRetailAllotChannels 返回即时零售"调拨当销售"应纳入看板的渠道。
// 纯调拨渠道 (PureAllot, 朴朴): 无销售单, 是历史既有口径, **始终纳入**(不依赖价格表是否存在, 防价格行被删就静默丢 GMV)。
// 价格门控渠道 (小象/叮咚/未来七鲜): 有销售单, 只有"已配价格表"才纳入——没价格表时 excel_amount=0, 但调拨件数是真的,
// 贸然纳入会让销量/客单价失真; 故价格表到位前不进, 跑哥导入价格表后自动纳入(金额+件数一起生效), 无需再改代码部署。
// 渠道清单来自 specialchannel 注册表, 拼 IN 子句的值是白名单无注入风险。
func (h *DashboardHandler) instantRetailAllotChannels() []string {
	out := append([]string{}, specialchannel.PureAllotKeys(specialchannel.DeptInstantRetail)...)
	gated := specialchannel.PriceGatedKeys(specialchannel.DeptInstantRetail)
	if len(gated) == 0 {
		return out
	}
	has := map[string]bool{}
	rows, err := h.DB.Query(`SELECT DISTINCT channel_key FROM channel_special_price WHERE channel_key IN (` + allotChannelsInClause(gated) + `)`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var c string
			if rows.Scan(&c) == nil {
				has[c] = true
			}
		}
	}
	for _, c := range gated {
		if has[c] {
			out = append(out, c)
		}
	}
	return out
}

// allotChannelsInClause 把硬编码白名单渠道拼成 IN 子句 (值来自白名单, 无注入风险)
func allotChannelsInClause(chans []string) string {
	return "'" + strings.Join(chans, "','") + "'"
}

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
	//
	// v1.74.3 拓范 T6h: 电商部页"店铺数据概览"合并 2 调拨渠道
	// 保留 pureScopeCond (无电商部排除条件) 给 helper 调用 — helper 内 SQL 用 shop_id IN 锁定 2 渠道
	// 不能让 helper SQL 再加 shop_name NOT IN 冲突 (否则数据 0)
	pureScopeCond := scopeCond
	scopeCond += extraDeptCond(dept, false)
	_ = pureScopeCond // 仅 dept=ecommerce 时用, 防 unused warning

	// 1. 每日趋势（短范围自动扩展）
	trendStart, trendEnd := getTrendDateRange(start, end)
	trendArgs := append([]interface{}{dept, trendStart, trendEnd}, extraArgs...)
	trendRows, ok := queryRowsOrWriteError(w, r, h.DB, `
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

	// v1.74.6: 电商部页趋势漏算 2 调拨渠道 (清心湖自营+猫超寄售), 与同页 KPI 卡对不上
	// helper loadEcommerceDailyAllot 已就绪 (综合看板同款), 复用拿日级 allotAmt 加回 daily
	// 兜底: 失败 → log + 不阻塞, 趋势用原口径 (跟 KPI 对不上但页面不挂)
	if dept == "ecommerce" {
		if dailyAllot, dailyErr := h.loadEcommerceDailyAllot(
			r.Context(), trendStart, trendEnd, pureScopeCond, scopeArgs); dailyErr != nil {
			log.Printf("[department/ecommerce] 趋势日级调拨加载失败, 用原口径: %v", dailyErr)
		} else {
			for i := range daily {
				if d, ok := dailyAllot[daily[i].Date]; ok {
					daily[i].Sales += d.allotAmt
					daily[i].Qty += d.allotQty
				}
			}
		}
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
		GROUP BY shop_name ORDER BY sales DESC`
	}
	shopRows, ok := queryRowsOrWriteError(w, r, h.DB, shopListSQL, shopListArgs...)
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

	// v1.74.3 拓范 T6h: 电商部 "店铺数据概览" 合并 2 调拨渠道 (跑哥 5/25 追加)
	// shopListSQL 因 ecommerceExcludeAllotCond 排除了 2 渠道, 这里用 helper 加回 2 entry (调拨口径)
	// 兜底: helper 失败 → log + shop list 保持 v1.04 排除行为
	// 5/25 修 bug: 按 platform tab 过滤, 京东 tab 不能加猫超 entry
	addedAllotShops := 0
	hasAllotData := false // v1.74.3 拓范: 用来判断是否显示"调拨专区" tab
	if dept == "ecommerce" {
		// v1.74.3 拓范 (跑哥 5/25): 调拨专区独立 tab
		// - platform=""        全部 tab → 含 2 调拨 (跟综合看板总额一致)
		// - platform="allot"   调拨专区 tab → 仅 2 调拨店 (其它 SQL 因 AND 1=0 返 0)
		// - platform="jd"      京东 tab → 不含调拨店 (干净, 跟 v1.04 一致)
		// - platform="tmall_cs" 天猫超市 tab → 不含调拨店
		// - 其它 → 不含
		var allowedShops []string
		switch platform {
		case "":
			allowedShops = []string{jdShopName, tmcsShopNm}
		case "allot":
			allowedShops = []string{jdShopName, tmcsShopNm}
		}

		// 先 check 是否有 2 渠道调拨数据 (用于决定 allot tab 是否显示)
		// 即使当前 platform 不需要加 entry, 也要 check (用于 platforms 列表)
		if shopAllot, allotErr := h.loadEcommerceShopAllot(
			r.Context(), start, end, pureScopeCond, scopeArgs); allotErr != nil {
			log.Printf("[dept-detail] ecommerce shop 调拨加载失败, 沿用 v1.04 排除口径: %v", allotErr)
		} else {
			// 任一渠道 allotAmt > 0 即认为有调拨数据 (该时间段)
			for _, a := range shopAllot {
				if a.allotAmt > 0 {
					hasAllotData = true
					break
				}
			}
			// 按 allowedShops 加 entry
			for _, shopName := range allowedShops {
				allot, ok := shopAllot[shopName]
				if !ok {
					continue
				}
				// 非调拨专区 tab: allotAmt=0 跳过 ¥0 entry (避免空白行干扰)
				// 调拨专区 tab: 始终显示 2 entry, 即使 ¥0 (跑哥 5/25 决策: tab 定位是"看 2 家调拨店状态")
				if platform != "allot" && allot.allotAmt == 0 {
					continue
				}
				shops = append(shops, ShopData{
					ShopName: shopName,
					Sales:    allot.allotAmt,
					Qty:      allot.allotQty,
					Profit:   0,
				})
				addedAllotShops++
			}
			if addedAllotShops > 0 {
				sort.SliceStable(shops, func(i, j int) bool {
					return shops[i].Sales > shops[j].Sales
				})
			}
		}
	}

	// v1.74.3-2 (跑哥 5/25) + 2026-06-05 拓: 即时零售部 调拨当销售合并 (朴朴/小象/叮咚)
	// 朴朴无销售单→新增店铺 entry; 小象/叮咚有销售单→调拨金额/件数累加到现有 entry (不重复, 两批不同货)
	// 仅纳入已配价格表的渠道(instantRetailAllotChannels), 没价格表的暂不进, 价格表到位后自动生效
	// instantAllotChans 本请求只算一次, 店铺列表 + 下方 KPI 共用 (避免重复查 channel_special_price)
	var instantAllotChans []string
	if dept == "instant_retail" {
		instantAllotChans = h.instantRetailAllotChannels()
		changed := false
		for _, ck := range instantAllotChans {
			var amt, qty float64
			_ = h.DB.QueryRowContext(r.Context(), `SELECT IFNULL(SUM(d.excel_amount), 0), IFNULL(SUM(d.sku_count), 0)
				FROM allocate_orders o
				JOIN allocate_details d ON d.allocate_no = o.allocate_no
				WHERE o.channel_key = ? AND o.stat_date BETWEEN ? AND ?`, ck, start, end).Scan(&amt, &qty)
			if amt <= 0 {
				continue
			}
			shopName := instantRetailAllotShop[ck]
			found := false
			for i := range shops {
				if shops[i].ShopName == shopName {
					shops[i].Sales += amt
					shops[i].Qty += qty
					found = true
					break
				}
			}
			if !found {
				shops = append(shops, ShopData{ShopName: shopName, Sales: amt, Qty: qty, Profit: 0})
				addedAllotShops++
			}
			changed = true
		}
		if changed {
			sort.SliceStable(shops, func(i, j int) bool {
				return shops[i].Sales > shops[j].Sales
			})
		}
	}

	// 2.5 店铺总数（独立于 LIMIT 20 排行榜，给前端"全部 N 家"用真实总数）
	var totalShopCount int
	totalShopArgs := append([]interface{}{dept, start, end}, platArgs...)
	totalShopArgs = append(totalShopArgs, scopeArgs...)
	var totalShopSQL string
	if dept == "offline" {
		totalShopSQL = `SELECT COUNT(DISTINCT ` + offlineRegionExpr + `) FROM sales_goods_summary
			WHERE department = ? AND shop_name IS NOT NULL
			  AND stat_date BETWEEN ? AND ?` + offlineRegionPrefilter + scopeCond
	} else {
		totalShopSQL = `SELECT COUNT(DISTINCT shop_name) FROM sales_goods_summary
			WHERE department = ? AND shop_name IS NOT NULL
			  AND stat_date BETWEEN ? AND ?` + platCond + scopeCond
	}
	_ = h.DB.QueryRow(totalShopSQL, totalShopArgs...).Scan(&totalShopCount)
	if totalShopCount == 0 {
		// SQL 返 0 (e.g. AND 1=0): fallback 用 len(shops), 已含 addedAllotShops 加进去的 entry
		totalShopCount = len(shops)
	} else {
		// SQL 正常返数, 它因 ecommerceExcludeAllotCond 不含 2 调拨店, 这里加上
		totalShopCount += addedAllotShops
	}

	// 3. 商品排行
	goodsArgs := append([]interface{}{dept, start, end}, extraArgs...)
	goodsRows, ok := queryRowsOrWriteError(w, r, h.DB, `
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

	// v1.74.3 拓范 T6j (跑哥 5/25): 货品看板合并 2 调拨渠道 SKU
	// 加载调拨 SKU 详情 (LEFT JOIN goods 拿 brand/cate/grade) → 后续多 section 复用
	var allotGoods []GoodsAllotItem
	if dept == "ecommerce" && (platform == "" || platform == "allot") {
		if items, err := h.loadEcommerceGoodsAllotDetail(r.Context(), start, end); err != nil {
			log.Printf("[dept-detail] ecommerce goods 调拨加载失败: %v", err)
		} else {
			allotGoods = items
		}
	}

	// 合并调拨 SKU 进 topGoods (按 goods_no merge + 加和 sales/qty/profit)
	if len(allotGoods) > 0 {
		idx := make(map[string]int)
		for i, g := range goods {
			idx[g.GoodsNo] = i
		}
		for _, a := range allotGoods {
			if a.Sales == 0 && a.Qty == 0 {
				continue
			}
			if i, ok := idx[a.GoodsNo]; ok {
				// SKU 已在 topGoods (其它电商渠道也卖) → 加和
				goods[i].Sales += a.Sales
				goods[i].Qty += a.Qty
				// profit 不加 (调拨无 profit, 保留原销售单 profit)
			} else {
				// SKU 不在 topGoods (仅这 2 调拨渠道卖) → append entry
				goods = append(goods, GoodsData{
					GoodsNo:  a.GoodsNo,
					Name:     a.GoodsName,
					Brand:    a.BrandName,
					Category: a.CateName,
					Sales:    a.Sales,
					Qty:      a.Qty,
					Profit:   0, // 调拨无 profit
					Grade:    a.Grade,
				})
				idx[a.GoodsNo] = len(goods) - 1
			}
		}
		// 重新按 sales DESC 排 + LIMIT 15
		sort.SliceStable(goods, func(i, j int) bool {
			return goods[i].Sales > goods[j].Sales
		})
		if len(goods) > 15 {
			goods = goods[:15]
		}
	}

	// 3.1 商品维度总计 (KPI 卡用) — goods 数组只返回 TOP 15, 前端 reduce 会把"TOP 15 合计"当"全部"
	// 误算总销售额/总货品数/SKU数, 这里独立 SUM 全部商品给 KPI 准确口径. 修 跑哥 2026-05-20 报的
	// 线下 4 月总销售额 17,525,587.25 (TOP15) vs 综合看板 21,556,219.59 (全部) 差 18.7% bug
	var totalSales, totalQty float64
	var totalSku int
	totalArgs := append([]interface{}{dept, start, end}, extraArgs...)
	totalSQL := `SELECT IFNULL(ROUND(SUM(s.local_goods_amt), 2), 0), IFNULL(ROUND(SUM(s.goods_qty), 0), 0), COUNT(DISTINCT s.goods_no) FROM sales_goods_summary s WHERE s.department = ? AND s.goods_no IS NOT NULL AND s.stat_date BETWEEN ? AND ?` + strings.ReplaceAll(shopCond+platCond+scopeCond, "shop_name", "s.shop_name")
	_ = h.DB.QueryRow(totalSQL, totalArgs...).Scan(&totalSales, &totalQty, &totalSku)

	// v1.74.3 拓范 T6j: KPI 加调拨数据 (totalSales/totalQty/totalSku)
	// allotGoods 是 helper 数据 (5/1-5/24 样本 33 SKU), 加和到 KPI 让 ProductDashboard 总数对齐 StorePreview
	if len(allotGoods) > 0 {
		for _, a := range allotGoods {
			totalSales += a.Sales
			totalQty += a.Qty
		}
		// totalSku: 加 helper 中 distinct goods_no 数 (allotGoods 已经 GROUP BY goods_no)
		totalSku += len(allotGoods)
	}

	// v1.74.3-2 + 2026-06-05 拓: instant_retail 调拨当销售 KPI 加 (朴朴/小象/叮咚, 仅已配价格表的渠道)
	if dept == "instant_retail" {
		var puAmt, puQty float64
		var puSku int
		_ = h.DB.QueryRowContext(r.Context(), `SELECT IFNULL(SUM(d.excel_amount), 0),
			IFNULL(SUM(d.sku_count), 0),
			COUNT(DISTINCT d.goods_no)
			FROM allocate_orders o
			JOIN allocate_details d ON d.allocate_no = o.allocate_no
			WHERE o.channel_key IN (`+allotChannelsInClause(instantAllotChans)+`) AND o.stat_date BETWEEN ? AND ?`, start, end).Scan(&puAmt, &puQty, &puSku)
		totalSales += puAmt
		totalQty += puQty
		totalSku += puSku
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
		chRows, ok := queryRowsOrWriteError(w, r, h.DB, chSQL, chArgs...)
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
	brandRows, ok := queryRowsOrWriteError(w, r, h.DB, `
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

	// v1.74.3 拓范 T6j: 品牌分布合并调拨 (按 brand_name 聚合 allotGoods)
	if len(allotGoods) > 0 {
		brandIdx := make(map[string]int)
		for i, b := range brands {
			brandIdx[b.Brand] = i
		}
		brandAllotSum := make(map[string]float64)
		for _, a := range allotGoods {
			bk := a.BrandName
			if bk == "" {
				bk = "未知"
			}
			brandAllotSum[bk] += a.Sales
		}
		for brand, sales := range brandAllotSum {
			if sales == 0 {
				continue
			}
			if i, ok := brandIdx[brand]; ok {
				brands[i].Sales += sales
			} else {
				brands = append(brands, BrandData{Brand: brand, Sales: sales})
			}
		}
		sort.SliceStable(brands, func(i, j int) bool {
			return brands[i].Sales > brands[j].Sales
		})
		if len(brands) > 10 {
			brands = brands[:10]
		}
	}

	// 4.5 产品定位分布
	type GradeData struct {
		Grade string  `json:"grade"`
		Sales float64 `json:"sales"`
	}
	var grades []GradeData
	gradeArgs := append([]interface{}{dept, start, end}, extraArgs...)
	gradeRows, ok := queryRowsOrWriteError(w, r, h.DB, `
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

	// v1.74.3 拓范 T6j: Grade 分布合并调拨 (按 goods_field7 聚合 allotGoods)
	if len(allotGoods) > 0 {
		gradeIdx := make(map[string]int)
		for i, gd := range grades {
			gradeIdx[gd.Grade] = i
		}
		gradeAllotSum := make(map[string]float64)
		for _, a := range allotGoods {
			gk := a.Grade
			if gk == "" {
				gk = "未设置"
			}
			gradeAllotSum[gk] += a.Sales
		}
		for grade, sales := range gradeAllotSum {
			if sales == 0 {
				continue
			}
			if i, ok := gradeIdx[grade]; ok {
				grades[i].Sales += sales
			} else {
				grades = append(grades, GradeData{Grade: grade, Sales: sales})
			}
		}
		// 按 S/A/B/C/D 顺序保留 (没 sort 因为 grade 有特定顺序)
	}

	// 4.6 产品定位×平台销售分布（电商+社媒部门，平台维度通过 sales_channel.online_plat_name）
	type GradePlatItem struct {
		Grade    string  `json:"grade"`
		Platform string  `json:"platform"`
		Sales    float64 `json:"sales"`
	}
	var gradePlatSales []GradePlatItem
	if dept == "ecommerce" || dept == "social" || dept == "instant_retail" {
		gpRows, ok := queryRowsOrWriteError(w, r, h.DB, `
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
		gpRows, ok := queryRowsOrWriteError(w, r, h.DB, gpSQL, dept, start, end)
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
	platRows, ok := queryRowsOrWriteError(w, r, h.DB, `
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

	// v1.74.3 拓范 (跑哥 5/25): 调拨专区 tab — 仅 ecommerce + 该时段有调拨数据时显示
	if dept == "ecommerce" && hasAllotData {
		platforms = append(platforms, PlatTab{"allot", "调拨专区"})
	}

	// 6. 平台销售额分布
	type PlatSales struct {
		Platform string  `json:"platform"`
		Sales    float64 `json:"sales"`
		Qty      float64 `json:"qty"`
	}
	var platformSales []PlatSales
	platSalesRows, ok := queryRowsOrWriteError(w, r, h.DB, `
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

	// v1.74.3 拓范 T6j: platformSales 加 2 调拨渠道 (京东自营→京东, 猫超→天猫超市)
	if dept == "ecommerce" && (platform == "" || platform == "allot") {
		for _, ck := range []struct{ key, label string }{
			{jdChanKey, "京东"},
			{tmcsChanKey, "天猫超市"},
		} {
			var s, q float64
			_ = h.DB.QueryRow(`SELECT IFNULL(SUM(d.excel_amount), 0), IFNULL(SUM(d.sku_count), 0)
				FROM allocate_orders o
				JOIN allocate_details d ON d.allocate_no = o.allocate_no
				WHERE o.channel_key = ? AND o.stat_date BETWEEN ? AND ?`, ck.key, start, end).Scan(&s, &q)
			if s == 0 && q == 0 {
				continue
			}
			if ps, ok := platSalesMap[ck.label]; ok {
				ps.Sales += s
				ps.Qty += q
			} else {
				platSalesMap[ck.label] = &PlatSales{Platform: ck.label, Sales: s, Qty: q}
			}
		}
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
			tRows, tOk := queryRowsOrWriteError(w, r, h.DB, `
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
		gdRows, ok := queryRowsOrWriteError(w, r, h.DB, `
			SELECT IFNULL(g.goods_field7,'未设置') as grade, s.department,
				ROUND(SUM(s.local_goods_amt),2) as sales,
				ROUND(SUM(s.gross_profit),2) as profit
			FROM sales_goods_summary s
			LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods) g ON g.goods_no = s.goods_no
			WHERE s.stat_date BETWEEN ? AND ?
			  AND IFNULL(s.department,'') NOT IN ('excluded','other','')
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

		gsRows, ok := queryRowsOrWriteError(w, r, h.DB, `
			SELECT IFNULL(g.goods_field7,'未设置') as grade,
				IFNULL(s.department,'其他') as department,
				s.shop_name,
				ROUND(SUM(s.local_goods_amt),2) as sales,
				ROUND(SUM(s.gross_profit),2) as profit
			FROM sales_goods_summary s
			LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods) g ON g.goods_no = s.goods_no
			WHERE s.stat_date BETWEEN ? AND ?
			  AND IFNULL(s.department,'') NOT IN ('excluded','other','')
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
		"shopTotalCount":    totalShopCount,
		"goods":             goods,
		"totalSales":        totalSales,
		"totalQty":          totalQty,
		"totalSku":          totalSku,
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
