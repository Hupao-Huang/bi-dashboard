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

// ====== 部门页大区/平台常量 + 返回结构 ======
// 下列常量/类型/平台 Tab 表原本声明在 GetDepartmentDetail 内, v1.77.7 拆函数时提到包级,
// 供拆出来的各 section helper 共用 (无任何 SQL/口径变化, 纯结构整理)。

// offlineRegionExpr 线下大区合并: shop_name → 大区名映射 (多个 SQL section 共用)
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

// offlineRegionPrefilter 只取大区/省区/重客的行 (排掉非大区门店)
const offlineRegionPrefilter = ` AND (shop_name LIKE '%大区%' OR shop_name LIKE '%省区%' OR shop_name LIKE '%重客系统%')`

// deptDailyData 每日趋势单元
type deptDailyData struct {
	Date   string  `json:"date"`
	Sales  float64 `json:"sales"`
	Qty    float64 `json:"qty"`
	Profit float64 `json:"profit"`
	Cost   float64 `json:"cost"`
}

// deptShopData 店铺/大区排行单元
type deptShopData struct {
	ShopName string  `json:"shopName"`
	Sales    float64 `json:"sales"`
	Qty      float64 `json:"qty"`
	Profit   float64 `json:"profit"`
}

// deptGoodsData 商品排行单元
type deptGoodsData struct {
	GoodsNo  string  `json:"goodsNo"`
	Name     string  `json:"goodsName"`
	Brand    string  `json:"brand"`
	Category string  `json:"category"`
	Sales    float64 `json:"sales"`
	Qty      float64 `json:"qty"`
	Profit   float64 `json:"profit"`
	Grade    string  `json:"grade"`
}

// deptChannelSales 单商品在某渠道的销售
type deptChannelSales struct {
	ShopName string  `json:"shopName"`
	Sales    float64 `json:"sales"`
	Qty      float64 `json:"qty"`
}

// deptBrandData 品牌分布单元
type deptBrandData struct {
	Brand string  `json:"brand"`
	Sales float64 `json:"sales"`
}

// deptGradeData 产品定位分布单元
type deptGradeData struct {
	Grade string  `json:"grade"`
	Sales float64 `json:"sales"`
}

// deptGradePlatItem 产品定位×平台(渠道) 销售
type deptGradePlatItem struct {
	Grade    string  `json:"grade"`
	Platform string  `json:"platform"`
	Sales    float64 `json:"sales"`
}

// deptPlatTab 平台 Tab (key=前端筛选键, label=展示名)
type deptPlatTab struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// deptPlatSales 平台销售额分布单元
type deptPlatSales struct {
	Platform string  `json:"platform"`
	Sales    float64 `json:"sales"`
	Qty      float64 `json:"qty"`
}

// deptGradeDeptItem crossDept=1: 产品定位×部门 全口径聚合(含毛利)
type deptGradeDeptItem struct {
	Grade      string  `json:"grade"`
	Department string  `json:"department"`
	Sales      float64 `json:"sales"`
	Profit     float64 `json:"profit"`
}

// deptGradeShopItem crossDept=1: 产品定位×店铺 全口径聚合(含毛利)
type deptGradeShopItem struct {
	Grade      string  `json:"grade"`
	Department string  `json:"department"`
	ShopName   string  `json:"shopName"`
	Sales      float64 `json:"sales"`
	Profit     float64 `json:"profit"`
}

// platTabDefs 平台 Tab 定义 (顺序即前端展示顺序); platforms 列表 + platformSales 标签复用同一份
var platTabDefs = []deptPlatTab{
	{"tmall", "天猫"}, {"tmall_cs", "天猫超市"}, {"jd", "京东"}, {"pdd", "拼多多"},
	{"vip", "唯品会"}, {"taobao", "淘宝"}, {"instant", "即时零售"},
	{"douyin", "抖音"}, {"kuaishou", "快手"}, {"xiaohongshu", "小红书"},
	{"youzan", "有赞"}, {"weidian", "微店"}, {"shipinhao", "视频号"},
}

// deptShopRankResult loadDeptShopRanking 的多值返回 (店铺排行 + 销售/调拨口径拆分 + 即时零售调拨渠道)
type deptShopRankResult struct {
	shops             []deptShopData
	salesAmt          float64  // 店铺概览 KPI: 纯销售单口径 (调拨加入前的常规店铺合计)
	allotAmt          float64  // 店铺概览 KPI: 调拨当销售口径 (电商 2 渠道 / 即时零售朴朴等)
	addedAllotShops   int      // 调拨补进来的 entry 数 (给 totalShopCount 加回)
	hasAllotData      bool     // 电商该时段是否有调拨数据 (决定是否显示"调拨专区" tab)
	instantAllotChans []string // 即时零售本请求的调拨渠道清单 (店铺列表 + KPI 共用, 只算一次)
}

// deptQuery 部门页一次请求的公共查询上下文 (GetDepartmentDetail 解析一次, 透传给各 section helper)。
// 收口原先散在 13 个 helper 签名里的 9-11 个同型位置参数 (dept/日期/各 cond/各 args),
// 消除"相邻同型参数传错位置 → 绑错 SQL 槽"的隐患, 也收敛 fuck-u-code "参数数量" 维度。
type deptQuery struct {
	dept          string
	start         string
	end           string
	platform      string
	shopCond      string
	platCond      string
	scopeCond     string
	pureScopeCond string
	extraArgs     []interface{}
	platArgs      []interface{}
	scopeArgs     []interface{}
}

// GetDepartmentDetail 部门页详情接口 (/api/department)。
// 编排器: 解析参数 → 按原 DB 调用顺序逐个 section helper → 汇总 writeJSON。
// 各 helper 顺序不可乱: sqlmock 单测按 SQL 出现顺序匹配 (见 dashboard_department_test.go)。
func (h *DashboardHandler) GetDepartmentDetail(w http.ResponseWriter, r *http.Request) {
	dept := r.URL.Query().Get("dept")
	if dept == "" {
		writeError(w, 400, "dept is required")
		return
	}
	start, end := getDateRange(r, h.DB)
	shop := r.URL.Query().Get("shop")         // 可选：按店铺过滤
	platform := r.URL.Query().Get("platform") // 可选：按平台过滤

	// 线下大区 shop 过滤: shop=大区名 → 对应 LIKE 条件 (仅 offline shop 过滤用)
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
	// 拼到 scopeCond 上, 所有 sales_goods_summary SQL 自动生效
	// pureScopeCond (无电商部排除条件) 给调拨 helper 用 — helper 内 SQL 用 shop_id IN 锁定 2 渠道,
	// 不能再加 shop_name NOT IN 冲突 (否则数据 0)
	pureScopeCond := scopeCond
	scopeCond += extraDeptCond(dept, false)

	// 3.5 商品渠道分布 / crossDept: 跨 4 部门聚合 (财务·产品利润页用)
	crossDept := r.URL.Query().Get("crossDept") == "1"

	// q 收口本次请求的公共查询上下文, 透传给各 section helper (替代原 9-11 个同型位置参数)
	q := deptQuery{
		dept: dept, start: start, end: end, platform: platform,
		shopCond: shopCond, platCond: platCond, scopeCond: scopeCond, pureScopeCond: pureScopeCond,
		extraArgs: extraArgs, platArgs: platArgs, scopeArgs: scopeArgs,
	}

	// ===== 各 section: 顺序 = 原 DB 调用顺序, 不可乱 (sqlmock 单测按序匹配) =====

	// 1. 每日趋势
	daily, trendStart, trendEnd, ok := h.loadDeptDailyTrend(w, r, q)
	if !ok {
		return
	}

	// 2. 店铺/大区排行 (+ 电商/即时零售调拨合并)
	shopRank, ok := h.loadDeptShopRanking(w, r, q)
	if !ok {
		return
	}

	// 2.5 店铺总数 (独立于 LIMIT 排行榜, 给前端"全部 N 家"真实总数)
	totalShopCount := h.loadDeptShopTotalCount(q, len(shopRank.shops), shopRank.addedAllotShops)

	// 3. 商品排行 (+ 电商调拨 SKU 合并, allotGoods 后续多 section 复用)
	goods, allotGoods, ok := h.loadDeptGoodsRanking(w, r, q)
	if !ok {
		return
	}

	// 3.1 商品维度总计 (KPI 卡用, 独立 SUM 全部商品避免 TOP15 reduce 偏小)
	totalSales, totalQty, totalSku := h.loadDeptGoodsTotals(r, q, allotGoods, shopRank.instantAllotChans)

	// 3.5 商品渠道分布
	goodsChannels, ok := h.loadDeptGoodsChannels(w, r, q, goods, crossDept)
	if !ok {
		return
	}

	// 4. 品牌分布
	brands, ok := h.loadDeptBrands(w, r, q, allotGoods)
	if !ok {
		return
	}

	// 4.5 产品定位分布
	grades, ok := h.loadDeptGrades(w, r, q, allotGoods)
	if !ok {
		return
	}

	// 4.6 产品定位×平台 销售分布
	gradePlatSales, ok := h.loadDeptGradePlatSales(w, r, q)
	if !ok {
		return
	}

	// 5. 平台 Tab 列表 (只返回有销售数据的)
	platforms, ok := h.loadDeptPlatformTabs(w, r, q, shopRank.hasAllotData)
	if !ok {
		return
	}

	// 6. 平台销售额分布
	platformSales, ok := h.loadDeptPlatformSales(w, r, q)
	if !ok {
		return
	}

	// offline 补充: 日期范围内各月目标累加
	regionTargets, ok := h.loadDeptRegionTargets(w, r, q)
	if !ok {
		return
	}

	// crossDept=1 额外返回: 产品定位×部门 + 产品定位×店铺 全口径聚合(含毛利)
	var gradeDeptSalesAll []deptGradeDeptItem
	var gradeShopSalesAll []deptGradeShopItem
	if crossDept {
		gradeDeptSalesAll, gradeShopSalesAll, ok = h.loadDeptCrossGrades(w, r, q)
		if !ok {
			return
		}
	}

	writeJSON(w, map[string]interface{}{
		"daily":             daily,
		"shops":             shopRank.shops,
		"salesAmt":          shopRank.salesAmt, // 店铺概览 KPI: 纯销售单口径 (调拨加入前的常规店铺合计)
		"allotAmt":          shopRank.allotAmt, // 店铺概览 KPI: 调拨当销售口径 (电商 2 渠道 / 即时零售朴朴等)
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

// loadDeptDailyTrend 每日趋势 (短范围自动扩展) + 电商部日级调拨加回。
// 返回 daily 数据 + 实际趋势区间 trendStart/trendEnd (给前端 trendRange)。
func (h *DashboardHandler) loadDeptDailyTrend(w http.ResponseWriter, r *http.Request, q deptQuery) ([]deptDailyData, string, string, bool) {
	dept, start, end, shopCond, platCond, scopeCond, pureScopeCond, extraArgs, scopeArgs := q.dept, q.start, q.end, q.shopCond, q.platCond, q.scopeCond, q.pureScopeCond, q.extraArgs, q.scopeArgs
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
		return nil, "", "", false
	}
	defer trendRows.Close()

	var daily []deptDailyData
	for trendRows.Next() {
		var d deptDailyData
		if writeDatabaseError(w, trendRows.Scan(&d.Date, &d.Sales, &d.Qty, &d.Profit, &d.Cost)) {
			return nil, "", "", false
		}
		daily = append(daily, d)
	}
	if writeDatabaseError(w, trendRows.Err()) {
		return nil, "", "", false
	}

	// v1.74.6: 电商部页趋势漏算 2 调拨渠道 (清心湖自营+猫超寄售), 与同页 KPI 卡对不上
	// helper loadEcommerceDailyAllot 已就绪 (综合看板同款), 复用拿日级 allotAmt 加回 daily
	// 兜底: 失败 → log + 不阻塞, 趋势用原口径 (跟 KPI 对不上但页面不挂)
	if dept == "ecommerce" {
		if dailyAllot, dailyErr := h.loadEcommerceDailyAllot(
			r.Context(), trendStart, trendEnd, pureScopeCond, scopeArgs); dailyErr != nil {
			log.Printf("[department/ecommerce] 趋势日级调拨加载失败, 用原口径: %v", dailyErr)
		} else {
			daily = applyDeptEcommerceDailyAllot(daily, dailyAllot)
		}
	} else if dept == "instant_retail" {
		// 2026-06-26: 即时零售部门页趋势也加日级调拨(朴朴/小象/叮咚), 跟本页 KPI/货品口径对齐
		// (此前只电商加, 即时零售趋势只算销售单, 跟含调拨的总销售额对不上)。复用综合看板同款 helper。
		// 兜底: 失败 → log + 不阻塞, 趋势用原口径(显瘦但页面不挂)。
		if puDailyAllot, puErr := h.loadInstantRetailDailyAllot(
			r.Context(), trendStart, trendEnd, h.instantRetailAllotChannels()); puErr != nil {
			log.Printf("[department/instant_retail] 趋势日级调拨加载失败, 用原口径: %v", puErr)
		} else {
			// 即时零售调拨日不一定有销售单(朴朴是纯调拨), 不能只更新已有销售日:
			// 已有销售日→加和; 纯调拨日→补一个点。否则趋势漏掉这些天的调拨, 总和跟含调拨的总销售额对不上
			// (实测漏 8 天纯调拨日共 203 万)。allot-only 日的 Profit/Cost 留 0(调拨无利润成本)。
			idx := make(map[string]int, len(daily))
			for i := range daily {
				idx[daily[i].Date] = i
			}
			for date, a := range puDailyAllot {
				if i, ok := idx[date]; ok {
					daily[i].Sales += a.allotAmt
					daily[i].Qty += a.allotQty
				} else {
					daily = append(daily, deptDailyData{Date: date, Sales: a.allotAmt, Qty: a.allotQty})
				}
			}
			sort.Slice(daily, func(i, j int) bool { return daily[i].Date < daily[j].Date })
		}
	}
	return daily, trendStart, trendEnd, true
}

func applyDeptEcommerceDailyAllot(daily []deptDailyData, dailyAllot map[string]ecomDailyAllot) []deptDailyData {
	idx := make(map[string]int, len(daily))
	for i := range daily {
		idx[daily[i].Date] = i
	}

	added := false
	for date, a := range dailyAllot {
		if i, ok := idx[date]; ok {
			daily[i].Sales += a.allotAmt
			daily[i].Qty += a.allotQty
		} else if a.allotAmt != 0 || a.allotQty != 0 {
			daily = append(daily, deptDailyData{Date: date, Sales: a.allotAmt, Qty: a.allotQty})
			added = true
		}
	}
	if added {
		sort.Slice(daily, func(i, j int) bool { return daily[i].Date < daily[j].Date })
	}
	return daily
}

// loadDeptShopRanking 店铺/大区排行 (offline 按大区合并, 其余按 shop_name) + 电商/即时零售调拨合并。
// 同时算出店铺概览 KPI 的销售/调拨拆分, 以及即时零售调拨渠道清单 (后续 KPI 复用)。
func (h *DashboardHandler) loadDeptShopRanking(w http.ResponseWriter, r *http.Request, q deptQuery) (deptShopRankResult, bool) {
	dept, start, end, platform, platCond, scopeCond, pureScopeCond, platArgs, scopeArgs := q.dept, q.start, q.end, q.platform, q.platCond, q.scopeCond, q.pureScopeCond, q.platArgs, q.scopeArgs
	var res deptShopRankResult

	shopListArgs := append([]interface{}{dept, start, end}, platArgs...)
	shopListArgs = append(shopListArgs, scopeArgs...)
	var shopListSQL string
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
		return res, false
	}
	defer shopRows.Close()

	var shops []deptShopData
	for shopRows.Next() {
		var s deptShopData
		if writeDatabaseError(w, shopRows.Scan(&s.ShopName, &s.Sales, &s.Qty, &s.Profit)) {
			return res, false
		}
		shops = append(shops, s)
	}
	if writeDatabaseError(w, shopRows.Err()) {
		return res, false
	}

	// 店铺数据概览 KPI 销售/调拨拆分: 先记录纯销售单口径 (allot 加入前的常规店铺合计)
	// 电商部 shopListSQL 已剔除 2 调拨渠道, 故 shopSalesAmt = 销售口径; 下方 allot 块累加 shopAllotAmt = 调拨口径
	// 与综合看板 applyEcommerceAllotAdjustment 同口径 (SalesAmt=Sales-salesExcluded, AllotAmt=调拨), 两页数字对齐
	var shopSalesAmt, shopAllotAmt float64
	for _, s := range shops {
		shopSalesAmt += s.Sales
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
				shops = append(shops, deptShopData{
					ShopName: shopName,
					Sales:    allot.allotAmt,
					Qty:      allot.allotQty,
					Profit:   0,
				})
				addedAllotShops++
				shopAllotAmt += allot.allotAmt
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
			shopAllotAmt += amt
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
				shops = append(shops, deptShopData{ShopName: shopName, Sales: amt, Qty: qty, Profit: 0})
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

	res.shops = shops
	res.salesAmt = shopSalesAmt
	res.allotAmt = shopAllotAmt
	res.addedAllotShops = addedAllotShops
	res.hasAllotData = hasAllotData
	res.instantAllotChans = instantAllotChans
	return res, true
}

// loadDeptShopTotalCount 店铺总数 (独立于 LIMIT 排行榜)。
// SQL 错误按原逻辑忽略 (返 0 时用 len(shops) 兜底), 故不写错误响应。
func (h *DashboardHandler) loadDeptShopTotalCount(q deptQuery, shopsLen, addedAllotShops int) int {
	dept, start, end, platCond, scopeCond, platArgs, scopeArgs := q.dept, q.start, q.end, q.platCond, q.scopeCond, q.platArgs, q.scopeArgs
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
		totalShopCount = shopsLen
	} else {
		// SQL 正常返数, 它因 ecommerceExcludeAllotCond 不含 2 调拨店, 这里加上
		totalShopCount += addedAllotShops
	}
	return totalShopCount
}

// loadDeptGoodsRanking 商品 TOP15 排行 + 电商调拨 SKU 合并。
// 同时返回 allotGoods (调拨 SKU 详情) 给后续品牌/定位/KPI 多 section 复用。
func (h *DashboardHandler) loadDeptGoodsRanking(w http.ResponseWriter, r *http.Request, q deptQuery) ([]deptGoodsData, []GoodsAllotItem, bool) {
	dept, start, end, platform, shopCond, platCond, scopeCond, extraArgs := q.dept, q.start, q.end, q.platform, q.shopCond, q.platCond, q.scopeCond, q.extraArgs
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
		return nil, nil, false
	}
	defer goodsRows.Close()

	var goods []deptGoodsData
	for goodsRows.Next() {
		var g deptGoodsData
		if writeDatabaseError(w, goodsRows.Scan(&g.GoodsNo, &g.Name, &g.Brand, &g.Category, &g.Sales, &g.Qty, &g.Profit, &g.Grade)) {
			return nil, nil, false
		}
		goods = append(goods, g)
	}
	if writeDatabaseError(w, goodsRows.Err()) {
		return nil, nil, false
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
	} else if dept == "instant_retail" && platform == "" {
		// 2026-06-26: 即时零售货品明细/TOP15 也合并调拨 SKU(朴朴/小象/叮咚), 跟本页"总销售额"口径对齐
		// (此前只把调拨加进 KPI 总数, 明细仍只算销售单, 大数字含调拨/明细不含, 自相矛盾)。
		// 平台 Tab 视图(platform!="")不并, 避免单平台下重复展示全渠道调拨(沿用电商按视图门控的做法);
		// 此时 allotGoods 仍空, loadDeptGoodsTotals 用 len==0 守卫走旧路补 KPI, 口径不变。
		if items, err := h.loadInstantRetailGoodsAllotDetail(r.Context(), start, end, h.instantRetailAllotChannels()); err != nil {
			log.Printf("[dept-detail] instant_retail goods 调拨加载失败: %v", err)
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
				goods = append(goods, deptGoodsData{
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
	return goods, allotGoods, true
}

// loadDeptGoodsTotals 商品维度总计 (KPI 卡用) — 独立 SUM 全部商品, 避免前端把 TOP15 reduce 当全部。
// 修 跑哥 2026-05-20 报的 线下 TOP15 vs 全部差 18.7% bug。SQL 错误按原逻辑忽略, 不写错误响应。
func (h *DashboardHandler) loadDeptGoodsTotals(r *http.Request, q deptQuery, allotGoods []GoodsAllotItem, instantAllotChans []string) (float64, float64, int) {
	dept, start, end, shopCond, platCond, scopeCond, extraArgs := q.dept, q.start, q.end, q.shopCond, q.platCond, q.scopeCond, q.extraArgs
	var totalSales, totalQty float64
	var totalSku int
	totalArgs := append([]interface{}{dept, start, end}, extraArgs...)
	totalSQL := `SELECT IFNULL(ROUND(SUM(s.local_goods_amt), 2), 0), IFNULL(ROUND(SUM(s.goods_qty), 0), 0), COUNT(DISTINCT s.goods_no) FROM sales_goods_summary s WHERE s.department = ? AND s.goods_no IS NOT NULL AND s.stat_date BETWEEN ? AND ?` + strings.ReplaceAll(shopCond+platCond+scopeCond, "shop_name", "s.shop_name")
	_ = h.DB.QueryRow(totalSQL, totalArgs...).Scan(&totalSales, &totalQty, &totalSku)

	// v1.74.3 拓范 T6j: KPI 加调拨数据 (totalSales/totalQty/totalSku)
	// allotGoods 是 helper 数据, 加和到 KPI 让 ProductDashboard 总数对齐 StorePreview
	if len(allotGoods) > 0 {
		for _, a := range allotGoods {
			totalSales += a.Sales
			totalQty += a.Qty
		}
		// totalSku: 加 helper 中 distinct goods_no 数 (allotGoods 已经 GROUP BY goods_no)
		totalSku += len(allotGoods)
	}

	// v1.74.3-2 + 2026-06-05 拓: instant_retail 调拨当销售 KPI 加 (朴朴/小象/叮咚, 仅已配价格表的渠道)
	// 2026-06-26 守卫 len(allotGoods)==0: 默认视图 loadDeptGoodsRanking 已把即时零售调拨塞进 allotGoods,
	//   上面 len(allotGoods)>0 那段已加和到 KPI, 这里就别再加一遍(否则总销售额 = 销售单+调拨+调拨 翻倍)。
	//   仅平台 Tab 视图(allotGoods 为空)才走这条旧路补 KPI, 口径与之前完全一致。
	if dept == "instant_retail" && len(allotGoods) == 0 {
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
	return totalSales, totalQty, totalSku
}

// loadDeptGoodsChannels 商品渠道分布 (为 TOP15 每个商品查各渠道销售额)。
// crossDept=1: 跨 4 部门聚合渠道分布 (财务·产品利润页看商品全口径分布)。
func (h *DashboardHandler) loadDeptGoodsChannels(w http.ResponseWriter, r *http.Request, q deptQuery, goods []deptGoodsData, crossDept bool) (map[string][]deptChannelSales, bool) {
	dept, start, end, shopCond, platCond, scopeCond, extraArgs := q.dept, q.start, q.end, q.shopCond, q.platCond, q.scopeCond, q.extraArgs
	goodsChannels := map[string][]deptChannelSales{}
	if len(goods) == 0 {
		return goodsChannels, true
	}
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
		return nil, false
	}
	defer chRows.Close()
	for chRows.Next() {
		var goodsNo, shopName string
		var sales, qty float64
		if writeDatabaseError(w, chRows.Scan(&goodsNo, &shopName, &sales, &qty)) {
			return nil, false
		}
		goodsChannels[goodsNo] = append(goodsChannels[goodsNo], deptChannelSales{ShopName: shopName, Sales: sales, Qty: qty})
	}
	if writeDatabaseError(w, chRows.Err()) {
		return nil, false
	}
	return goodsChannels, true
}

// loadDeptBrands 品牌分布 TOP10 (+ 电商调拨按 brand_name 合并)。
func (h *DashboardHandler) loadDeptBrands(w http.ResponseWriter, r *http.Request, q deptQuery, allotGoods []GoodsAllotItem) ([]deptBrandData, bool) {
	dept, start, end, shopCond, platCond, scopeCond, extraArgs := q.dept, q.start, q.end, q.shopCond, q.platCond, q.scopeCond, q.extraArgs
	brandArgs := append([]interface{}{dept, start, end}, extraArgs...)
	brandRows, ok := queryRowsOrWriteError(w, r, h.DB, `
		SELECT IFNULL(brand_name,'未知') as brand,
			ROUND(SUM(local_goods_amt), 2) as sales
		FROM sales_goods_summary
		WHERE department = ? AND stat_date BETWEEN ? AND ?`+shopCond+platCond+scopeCond+`
		GROUP BY brand_name ORDER BY sales DESC LIMIT 10`, brandArgs...)
	if !ok {
		return nil, false
	}
	defer brandRows.Close()

	var brands []deptBrandData
	for brandRows.Next() {
		var b deptBrandData
		if writeDatabaseError(w, brandRows.Scan(&b.Brand, &b.Sales)) {
			return nil, false
		}
		brands = append(brands, b)
	}
	if writeDatabaseError(w, brandRows.Err()) {
		return nil, false
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
				brands = append(brands, deptBrandData{Brand: brand, Sales: sales})
			}
		}
		sort.SliceStable(brands, func(i, j int) bool {
			return brands[i].Sales > brands[j].Sales
		})
		if len(brands) > 10 {
			brands = brands[:10]
		}
	}
	return brands, true
}

// loadDeptGrades 产品定位(S/A/B/C/D) 分布 (+ 电商调拨按 goods_field7 合并)。
func (h *DashboardHandler) loadDeptGrades(w http.ResponseWriter, r *http.Request, q deptQuery, allotGoods []GoodsAllotItem) ([]deptGradeData, bool) {
	dept, start, end, shopCond, platCond, scopeCond, extraArgs := q.dept, q.start, q.end, q.shopCond, q.platCond, q.scopeCond, q.extraArgs
	var grades []deptGradeData
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
		return nil, false
	}
	defer gradeRows.Close()
	for gradeRows.Next() {
		var gd deptGradeData
		if writeDatabaseError(w, gradeRows.Scan(&gd.Grade, &gd.Sales)) {
			return nil, false
		}
		grades = append(grades, gd)
	}
	if writeDatabaseError(w, gradeRows.Err()) {
		return nil, false
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
				grades = append(grades, deptGradeData{Grade: grade, Sales: sales})
			}
		}
		// allot append 可能打乱 grade 顺序 (尤其"调拨专区" tab: 主查询被 platCond AND 1=0 清空,
		// grades 全靠 map 迭代 append, Go map 迭代随机 → 输出顺序不确定)。
		// 按主查询同款 ORDER BY FIELD(grade,'S','A','B','C','D') 重排, 保证输出确定。
		// (对齐 brands 段: 它合并后也 sort; 原 grades 漏了这步)
		gradeRank := map[string]int{"S": 1, "A": 2, "B": 3, "C": 4, "D": 5} // 不在表(未设置/空)→0, 同 SQL FIELD 语义
		sort.SliceStable(grades, func(i, j int) bool {
			ri, rj := gradeRank[grades[i].Grade], gradeRank[grades[j].Grade]
			if ri != rj {
				return ri < rj
			}
			return grades[i].Sales > grades[j].Sales
		})
	}
	return grades, true
}

// loadDeptGradePlatSales 产品定位×平台(渠道) 销售分布。
// 电商/社媒/即时零售: 平台维度走 sales_channel.online_plat_name; 线下: 按大区; 分销: 按 shop_name。
func (h *DashboardHandler) loadDeptGradePlatSales(w http.ResponseWriter, r *http.Request, q deptQuery) ([]deptGradePlatItem, bool) {
	dept, start, end, scopeCond, scopeArgs := q.dept, q.start, q.end, q.scopeCond, q.scopeArgs
	var gradePlatSales []deptGradePlatItem
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
			ORDER BY FIELD(grade,'S','A','B','C','D'), sales DESC`, append([]interface{}{dept, start, end}, scopeArgs...)...)
		if !ok {
			return nil, false
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
		gpMap := map[string]*deptGradePlatItem{}
		for gpRows.Next() {
			var grade, rawPlat string
			var sales float64
			if writeDatabaseError(w, gpRows.Scan(&grade, &rawPlat, &sales)) {
				return nil, false
			}
			label := rawPlat
			if l, ok := platLabelMap[rawPlat]; ok {
				label = l
			}
			key := gpKey(grade, label)
			if ps, ok := gpMap[key]; ok {
				ps.Sales += sales
			} else {
				gpMap[key] = &deptGradePlatItem{Grade: grade, Platform: label, Sales: sales}
			}
		}
		if writeDatabaseError(w, gpRows.Err()) {
			return nil, false
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
		gpRows, ok := queryRowsOrWriteError(w, r, h.DB, gpSQL, append([]interface{}{dept, start, end}, scopeArgs...)...)
		if !ok {
			return nil, false
		}
		defer gpRows.Close()
		for gpRows.Next() {
			var grade, channel string
			var sales float64
			if writeDatabaseError(w, gpRows.Scan(&grade, &channel, &sales)) {
				return nil, false
			}
			gradePlatSales = append(gradePlatSales, deptGradePlatItem{Grade: grade, Platform: channel, Sales: sales})
		}
		if writeDatabaseError(w, gpRows.Err()) {
			return nil, false
		}
	}
	return gradePlatSales, true
}

// loadDeptPlatformTabs 平台 Tab 列表 (只返回该部门该时段有销售数据的平台 + 电商调拨专区)。
func (h *DashboardHandler) loadDeptPlatformTabs(w http.ResponseWriter, r *http.Request, q deptQuery, hasAllotData bool) ([]deptPlatTab, bool) {
	dept, start, end := q.dept, q.start, q.end
	// 先查有数据的原始平台名
	platRows, ok := queryRowsOrWriteError(w, r, h.DB, `
		SELECT DISTINCT sc.online_plat_name
		FROM sales_channel sc
		INNER JOIN sales_goods_summary s ON s.shop_name = sc.channel_name
		WHERE sc.department = ? AND sc.online_plat_name IS NOT NULL AND sc.online_plat_name != ''
		  AND s.stat_date BETWEEN ? AND ? AND s.department = ?`, dept, start, end, dept)
	if !ok {
		return nil, false
	}
	defer platRows.Close()
	rawPlats := map[string]bool{}
	for platRows.Next() {
		var p string
		if writeDatabaseError(w, platRows.Scan(&p)) {
			return nil, false
		}
		rawPlats[p] = true
	}
	if writeDatabaseError(w, platRows.Err()) {
		return nil, false
	}

	// 即时零售特殊检查：按店铺名匹配
	var instantCount int
	if writeDatabaseError(w, h.DB.QueryRow(`SELECT COUNT(DISTINCT shop_name) FROM sales_goods_summary
		WHERE shop_name LIKE '%即时零售%' AND stat_date BETWEEN ? AND ?
		AND shop_name IN (SELECT channel_name FROM sales_channel WHERE department = ?)
		AND department = ?`, start, end, dept, dept).Scan(&instantCount)) {
		return nil, false
	}

	// 按合并规则生成平台Tab列表
	var platforms []deptPlatTab
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
		platforms = append(platforms, deptPlatTab{"allot", "调拨专区"})
	}
	return platforms, true
}

// loadDeptPlatformSales 平台销售额分布 (合并平台名 + 电商 2 调拨渠道加回)。
func (h *DashboardHandler) loadDeptPlatformSales(w http.ResponseWriter, r *http.Request, q deptQuery) ([]deptPlatSales, bool) {
	dept, start, end, scopeCond, platform, scopeArgs := q.dept, q.start, q.end, q.scopeCond, q.platform, q.scopeArgs
	var platformSales []deptPlatSales
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
		ORDER BY SUM(s.local_goods_amt) DESC`, append([]interface{}{dept, start, end}, scopeArgs...)...)
	if !ok {
		return nil, false
	}
	defer platSalesRows.Close()
	platSalesMap := map[string]*deptPlatSales{}
	for platSalesRows.Next() {
		var rawPlat string
		var sales, qty float64
		if writeDatabaseError(w, platSalesRows.Scan(&rawPlat, &sales, &qty)) {
			return nil, false
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
			platSalesMap[label] = &deptPlatSales{Platform: label, Sales: sales, Qty: qty}
		}
	}
	if writeDatabaseError(w, platSalesRows.Err()) {
		return nil, false
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
				platSalesMap[ck.label] = &deptPlatSales{Platform: ck.label, Sales: s, Qty: q}
			}
		}
	} else if dept == "instant_retail" && platform == "" {
		// 2026-06-26: 平台销售额饼图加即时零售调拨(朴朴/小象/叮咚各成一个平台块), 跟本页总销售额口径对齐。
		// 渠道动态(价格门控决定纳入), 标签=channel_key。base 已含 小象/叮咚 销售单, 这里加它们的调拨
		// (两批不同货, 不重复); 朴朴纯调拨无销售单, 新增一块。channel_key 是白名单, 但用 ? 参数化更稳。
		for _, ck := range h.instantRetailAllotChannels() {
			var s, q float64
			_ = h.DB.QueryRow(`SELECT IFNULL(SUM(d.excel_amount), 0), IFNULL(SUM(d.sku_count), 0)
				FROM allocate_orders o
				JOIN allocate_details d ON d.allocate_no = o.allocate_no
				WHERE o.channel_key = ? AND o.stat_date BETWEEN ? AND ?`, ck, start, end).Scan(&s, &q)
			if s == 0 && q == 0 {
				continue
			}
			if ps, ok := platSalesMap[ck]; ok {
				ps.Sales += s
				ps.Qty += q
			} else {
				platSalesMap[ck] = &deptPlatSales{Platform: ck, Sales: s, Qty: q}
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
	return platformSales, true
}

// loadDeptRegionTargets offline 部门各大区目标 (日期范围内各月累加); 非 offline 返回空 map。
func (h *DashboardHandler) loadDeptRegionTargets(w http.ResponseWriter, r *http.Request, q deptQuery) (map[string]float64, bool) {
	dept, start, end := q.dept, q.start, q.end
	regionTargets := map[string]float64{}
	if dept != "offline" {
		return regionTargets, true
	}
	// 解析 start/end 年月
	startTime, _ := time.Parse("2006-01-02", start)
	endTime, _ := time.Parse("2006-01-02", end)
	if startTime.IsZero() || endTime.IsZero() {
		return regionTargets, true
	}
	tRows, tOk := queryRowsOrWriteError(w, r, h.DB, `
		SELECT region, SUM(target)
		FROM offline_region_target
		WHERE (year*100+month) BETWEEN ? AND ?
		GROUP BY region`,
		startTime.Year()*100+int(startTime.Month()),
		endTime.Year()*100+int(endTime.Month()),
	)
	if !tOk {
		// 查询打开失败: queryRowsOrWriteError 已写 500, 必须 abort, 不能 fall through 到编排器 writeJSON (修原双写)
		return nil, false
	}
	defer tRows.Close()
	for tRows.Next() {
		var reg string
		var tgt float64
		if writeDatabaseError(w, tRows.Scan(&reg, &tgt)) {
			return nil, false
		}
		regionTargets[reg] = tgt
	}
	return regionTargets, true
}

// loadDeptCrossGrades crossDept=1 额外聚合: 产品定位×部门 + 产品定位×店铺 全口径(含毛利)。
func (h *DashboardHandler) loadDeptCrossGrades(w http.ResponseWriter, r *http.Request, q deptQuery) ([]deptGradeDeptItem, []deptGradeShopItem, bool) {
	start, end := q.start, q.end
	var gradeDeptSalesAll []deptGradeDeptItem
	var gradeShopSalesAll []deptGradeShopItem

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
		return nil, nil, false
	}
	defer gdRows.Close()
	for gdRows.Next() {
		var it deptGradeDeptItem
		if writeDatabaseError(w, gdRows.Scan(&it.Grade, &it.Department, &it.Sales, &it.Profit)) {
			return nil, nil, false
		}
		gradeDeptSalesAll = append(gradeDeptSalesAll, it)
	}
	if writeDatabaseError(w, gdRows.Err()) {
		return nil, nil, false
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
		return nil, nil, false
	}
	defer gsRows.Close()
	for gsRows.Next() {
		var it deptGradeShopItem
		if writeDatabaseError(w, gsRows.Scan(&it.Grade, &it.Department, &it.ShopName, &it.Sales, &it.Profit)) {
			return nil, nil, false
		}
		gradeShopSalesAll = append(gradeShopSalesAll, it)
	}
	if writeDatabaseError(w, gsRows.Err()) {
		return nil, nil, false
	}
	return gradeDeptSalesAll, gradeShopSalesAll, true
}
