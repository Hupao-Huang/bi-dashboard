package handler

// supply_chain_dashboard_sections.go — GetSupplyChainDashboard 的 section 查询 helper。
// 把原 895 行巨函数里 20 段并发查询逐段抽成 loadSC* 方法 (各返回 (结果, error)),
// 主函数只保留 scQuery 组装 + 并发编排 + 组装。行为保持: 见 supply_chain_dashboard_golden_test.go 逐字节对拍。
//
// 命名: 原函数内局部类型 (MonthData/ChannelData/…) 提升到包级并加 sc 前缀 (避免与 douyin.go 等局部类型混淆),
//   JSON 标签一字不改 → 前端无感。

import (
	"sync"
	"time"
)

// scQuery 收口 GetSupplyChainDashboard 各 section 共用的作用域过滤条件 + 参数,
// 避免每个 helper 拖一长串形参。cond 是拼进 SQL 的 WHERE 片段, args 是对应的占位符参数。
type scQuery struct {
	start, end    string
	stockSnapDate string

	warehouseCond  string        // 别名 warehouse_name
	warehouseArgs  []interface{} //
	warehouseCondS string        // 别名 s.warehouse_name
	warehouseArgsS []interface{} //
	warehouseCondB string        // 别名 b.warehouse_name
	warehouseArgsB []interface{} //

	salesScopeCond string
	salesScopeArgs []interface{}

	planWhCond  string // 计划仓白名单 别名 warehouse_name
	planWhArgs  []interface{}
	planWhCondS string // 别名 s.warehouse_name
	planWhCondB string // 别名 b.warehouse_name

	cateCond string // 10 核心品类白名单 (别名 "")
	cateArgs []interface{}
}

// ===== 包级结果类型 (原为函数内局部类型, JSON 标签保持不变) =====

type scMonthData struct {
	Month string  `json:"month"`
	Value float64 `json:"value"`
}

type scChannelData struct {
	Channel   string  `json:"channel"`
	DailyAvg  float64 `json:"dailyAvg"`
	Total     float64 `json:"total"`
	LastMonth float64 `json:"lastMonth"`
	MomRate   float64 `json:"momRate"`
	LastYear  float64 `json:"lastYear"`
	YoyRate   float64 `json:"yoyRate"`
}

type scCateHealth struct {
	Category       string  `json:"category"`
	StockValue     float64 `json:"stockValue"`
	DailySalesCost float64 `json:"dailySalesCost"`
	Turnover       float64 `json:"turnover"`
	HighStockRate  float64 `json:"highStockRate"`
	StockoutRate   float64 `json:"stockoutRate"`
}

type scStockDetail struct {
	GoodsNo    string  `json:"goodsNo"`
	GoodsName  string  `json:"goodsName"`
	UsableQty  float64 `json:"usableQty"`
	DailySales float64 `json:"dailySales"`
	Turnover   float64 `json:"turnover"`
	StockValue float64 `json:"stockValue"`
}

type scStockoutDetail struct {
	GoodsNo    string  `json:"goodsNo"`
	GoodsName  string  `json:"goodsName"`
	DailySales float64 `json:"dailySales"`
	DailyValue float64 `json:"dailyValue"`
}

type scTopProduct struct {
	GoodsNo   string  `json:"goodsNo"`
	GoodsName string  `json:"goodsName"`
	Category  string  `json:"category"`
	Grade     string  `json:"grade"`
	Sales     float64 `json:"sales"`
	Qty       float64 `json:"qty"`
}

type scCateSales struct {
	Category string  `json:"category"`
	Sales    float64 `json:"sales"`
	Profit   float64 `json:"profit"`
}

type scAgedDetail struct {
	GoodsNo        string  `json:"goodsNo"`
	GoodsName      string  `json:"goodsName"`
	Warehouse      string  `json:"warehouse"`
	Qty            float64 `json:"qty"`
	StockValue     float64 `json:"stockValue"`
	BatchNo        string  `json:"batchNo"`
	ProductionDate string  `json:"productionDate"`
	AgeDays        int     `json:"ageDays"`
}

// ===== KPI 卡片 (1) =====

// loadSCPlanAllotTriple 取特殊渠道(京东/猫超/朴朴)调拨当销售的本期/上月同期/去年同期聚合。
// 解析失败 (start/end 非法) 时 lm/ly 留零值 — 与原 inline 逻辑一致 (只在 parse 成功时才算 lm/ly)。
func (h *DashboardHandler) loadSCPlanAllotTriple(start, end string) (cur, lm, ly planAllotAgg, err error) {
	if cur, err = h.loadPlanAllot(start, end); err != nil {
		return
	}
	sT, e1 := time.Parse("2006-01-02", start)
	eT, e2 := time.Parse("2006-01-02", end)
	if e1 != nil || e2 != nil {
		return
	}
	if lm, err = h.loadPlanAllot(sT.AddDate(0, -1, 0).Format("2006-01-02"), eT.AddDate(0, -1, 0).Format("2006-01-02")); err != nil {
		return
	}
	ly, err = h.loadPlanAllot(sT.AddDate(-1, 0, 0).Format("2006-01-02"), eT.AddDate(-1, 0, 0).Format("2006-01-02"))
	return
}

// loadSCSalesGMV 计划 8 仓 + 10 品类的销售出库 GMV (不含特殊渠道调拨, 调拨在主函数纯加法并入)。
func (h *DashboardHandler) loadSCSalesGMV(q scQuery) (float64, error) {
	args := append([]interface{}{q.start, q.end}, q.planWhArgs...)
	args = append(args, q.salesScopeArgs...)
	args = append(args, q.cateArgs...)
	var v float64
	err := h.DB.QueryRow(`SELECT IFNULL(SUM(local_goods_amt),0) FROM sales_goods_summary WHERE stat_date BETWEEN ? AND ?`+q.planWhCond+q.salesScopeCond+q.cateCond+planExcludeAllotShopsCond, args...).Scan(&v)
	return v, err
}

// loadSCStockCost 库存金额 (current_qty*成本) + 日销成本 (month_qty*成本/30), 库存快照日。
func (h *DashboardHandler) loadSCStockCost(q scQuery) (stockCost, dailyCost float64, err error) {
	args := append([]interface{}{q.stockSnapDate}, q.planWhArgs...)
	args = append(args, q.warehouseArgs...)
	args = append(args, q.cateArgs...)
	err = h.DB.QueryRow(
		`SELECT IFNULL(SUM(current_qty * cost_price),0), IFNULL(SUM(month_qty * cost_price / 30),0)
		 FROM stock_quantity_daily WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+q.planWhCond+q.warehouseCond+q.cateCond,
		args...).Scan(&stockCost, &dailyCost)
	return
}

// loadSCAllotDailyCost 周转天数 KPI 的调拨日销成本: SUM(调拨量 × 该货库存加权平均成本)/30。
// stockSnapDate 出现 3 次 (planSpecial 子句 2 个 ? + gc 子查询 snapshot_date 1 个)。
func (h *DashboardHandler) loadSCAllotDailyCost(q scQuery) (float64, error) {
	cateCondGC, _ := planCategoryGoodsCond("stock_quantity_daily")
	args := append([]interface{}{q.stockSnapDate, q.stockSnapDate, q.stockSnapDate}, q.planWhArgs...)
	args = append(args, q.warehouseArgs...)
	args = append(args, q.cateArgs...)
	var v float64
	err := h.DB.QueryRow(`SELECT IFNULL(SUM(sca.allot_qty * gc.wcost)/30, 0)
		FROM `+planSpecialAllotQtySubSQL+` sca
		JOIN (
			SELECT goods_no, SUM(current_qty * cost_price)/NULLIF(SUM(current_qty),0) AS wcost
			FROM stock_quantity_daily
			WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+q.planWhCond+q.warehouseCond+cateCondGC+`
			GROUP BY goods_no
		) gc ON gc.goods_no = sca.goods_no`, args...).Scan(&v)
	return v, err
}

// loadSCStockoutSKU 缺货 SKU 数 / 有销量 SKU 数 (SKU 维度跨仓汇总, 缺货门槛用纯吉客云月销不并调拨)。
func (h *DashboardHandler) loadSCStockoutSKU(q scQuery) (stockoutSKU, salesSKU int, err error) {
	flagCond, flagArgs := planStockoutExcludeFlagsCond("")
	args := append([]interface{}{q.stockSnapDate}, q.planWhArgs...)
	args = append(args, q.warehouseArgs...)
	args = append(args, q.cateArgs...)
	args = append(args, flagArgs...)
	err = h.DB.QueryRow(`SELECT
		IFNULL(SUM(CASE WHEN sum_avail<=0 AND sum_month>0 THEN 1 ELSE 0 END),0),
		IFNULL(SUM(CASE WHEN sum_month>0 THEN 1 ELSE 0 END),0)
		FROM (
			SELECT goods_no,
				SUM(current_qty - locked_qty) AS sum_avail,
				SUM(month_qty) AS sum_month
			FROM stock_quantity_daily
			WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+q.planWhCond+q.warehouseCond+q.cateCond+flagCond+`
			GROUP BY goods_no
		) t`, args...).Scan(&stockoutSKU, &salesSKU)
	return
}

// loadSCPerWhStockout 单仓缺货单元数 / 有销量单元数 (按 SKU×仓 单元, 不 GROUP BY goods_no)。
func (h *DashboardHandler) loadSCPerWhStockout(q scQuery) (units, sales int, err error) {
	flagCond, flagArgs := planStockoutExcludeFlagsCond("")
	args := append([]interface{}{q.stockSnapDate}, q.planWhArgs...)
	args = append(args, q.warehouseArgs...)
	args = append(args, q.cateArgs...)
	args = append(args, flagArgs...)
	err = h.DB.QueryRow(`SELECT
		IFNULL(SUM(CASE WHEN (current_qty - locked_qty) <= 0 AND month_qty > 0 THEN 1 ELSE 0 END), 0),
		IFNULL(SUM(CASE WHEN month_qty > 0 THEN 1 ELSE 0 END), 0)
		FROM stock_quantity_daily
		WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+q.planWhCond+q.warehouseCond+q.cateCond+flagCond, args...).Scan(&units, &sales)
	return
}

// loadSCHighStockValue 高库存金额 (全仓周转>50天) / 总库存金额, SKU 维度跨仓汇总。
// 周转分母并进特殊渠道调拨当销售量。stockSnapDate 3 次 (planSpecial 2 + WHERE 1)。
func (h *DashboardHandler) loadSCHighStockValue(q scQuery) (highStockValue, totalStockValue float64, err error) {
	cateCondHR, _ := planCategoryGoodsCond("stock_quantity_daily")
	args := append([]interface{}{q.stockSnapDate, q.stockSnapDate, q.stockSnapDate}, q.planWhArgs...)
	args = append(args, q.warehouseArgs...)
	args = append(args, q.cateArgs...)
	err = h.DB.QueryRow(`SELECT
		IFNULL(SUM(CASE WHEN sum_month>0 AND sum_avail/(sum_month/30) > 50 THEN sku_stock_value ELSE 0 END),0),
		IFNULL(SUM(sku_stock_value),0)
		FROM (
			SELECT stock_quantity_daily.goods_no,
				SUM(current_qty - locked_qty) AS sum_avail,
				SUM(month_qty)+IFNULL(MAX(sca.allot_qty),0) AS sum_month,
				SUM(current_qty * cost_price) AS sku_stock_value
			FROM stock_quantity_daily
			LEFT JOIN `+planSpecialAllotQtySubSQL+` sca ON sca.goods_no = stock_quantity_daily.goods_no
			WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+q.planWhCond+q.warehouseCond+cateCondHR+`
			GROUP BY stock_quantity_daily.goods_no
		) t`, args...).Scan(&highStockValue, &totalStockValue)
	return
}

// loadSCAgedStockValue 库龄>90天 的库存金额。cateCondB/cateArgsB 用别名 b。
func (h *DashboardHandler) loadSCAgedStockValue(q scQuery) (float64, error) {
	cateCondB, cateArgsB := planCategoryGoodsCond("b")
	args := append([]interface{}{q.stockSnapDate, q.end}, q.planWhArgs...)
	args = append(args, q.warehouseArgsB...)
	args = append(args, cateArgsB...)
	var v float64
	err := h.DB.QueryRow(`SELECT IFNULL(SUM(b.current_qty * IFNULL(s.cost_price,0)),0)
		FROM stock_batch_daily b
		LEFT JOIN stock_quantity_daily s ON s.snapshot_date = b.snapshot_date AND b.sku_id = s.sku_id AND b.warehouse_id = s.warehouse_id
		WHERE b.snapshot_date=? AND b.production_date IS NOT NULL AND b.current_qty > 0
		AND b.production_date < DATE_SUB(?, INTERVAL 90 DAY)`+q.planWhCondB+q.warehouseCondB+cateCondB, args...).Scan(&v)
	return v, err
}

// ===== 月度销售趋势 (2) =====

// loadSCMonthlySales 全部历史月的销售趋势 (读月度物化表, JOIN 10 品类派生表)。
func (h *DashboardHandler) loadSCMonthlySales(q scQuery) ([]scMonthData, error) {
	catSub, catSubArgs := planCategoryGoodsSubquery()
	args := append([]interface{}{}, catSubArgs...)
	args = append(args, q.planWhArgs...)
	args = append(args, q.salesScopeArgs...)
	rows, err := h.DB.Query(`
		SELECT stat_month AS m, ROUND(SUM(local_goods_amt),2)
		FROM sales_goods_summary_monthly
		JOIN (`+catSub+`) gc ON gc.goods_no = sales_goods_summary_monthly.goods_no
		WHERE 1=1`+q.planWhCond+q.salesScopeCond+planExcludeAllotShopsCond+`
		GROUP BY stat_month ORDER BY stat_month`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []scMonthData{}
	for rows.Next() {
		var d scMonthData
		if err := rows.Scan(&d.Month, &d.Value); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

// ===== 各渠道销售额环比 (3) =====

// loadSCChannels 按部门(渠道)聚合本期销售额 + 日均, 已按销售额降序 (顺序即返回切片顺序)。
// 上月/去年同期 + 调拨并入在主函数 channels 组装阶段做。
func (h *DashboardHandler) loadSCChannels(q scQuery) ([]scChannelData, error) {
	args := append([]interface{}{q.end, q.start, q.start, q.end}, q.planWhArgs...)
	args = append(args, q.salesScopeArgs...)
	args = append(args, q.cateArgs...)
	rows, err := h.DB.Query(`
		SELECT CASE WHEN department IS NULL OR department='' THEN 'other' ELSE department END,
			ROUND(SUM(local_goods_amt)/GREATEST(DATEDIFF(?,?)+1,1),2),
			ROUND(SUM(local_goods_amt),2)
		FROM sales_goods_summary WHERE stat_date BETWEEN ? AND ?`+q.planWhCond+q.salesScopeCond+q.cateCond+planExcludeAllotShopsCond+`
		GROUP BY CASE WHEN department IS NULL OR department='' THEN 'other' ELSE department END
		ORDER BY SUM(local_goods_amt) DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []scChannelData{}
	for rows.Next() {
		var d scChannelData
		if err := rows.Scan(&d.Channel, &d.DailyAvg, &d.Total); err != nil {
			return nil, err
		}
		if d.Channel == "" {
			d.Channel = "other"
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

// loadSCDeptSales 按部门聚合 [start,end] 往前推 interval 的销售额, 返回 dept→金额。
// interval 由主函数传 "1 MONTH"(上月同期) 或 "1 YEAR"(去年同期) — 字面常量, 非用户输入, 无注入。
func (h *DashboardHandler) loadSCDeptSales(q scQuery, interval string) (map[string]float64, error) {
	args := append([]interface{}{q.start, q.end}, q.planWhArgs...)
	args = append(args, q.salesScopeArgs...)
	args = append(args, q.cateArgs...)
	rows, err := h.DB.Query(`
		SELECT CASE WHEN department IS NULL OR department='' THEN 'other' ELSE department END, ROUND(SUM(local_goods_amt),2)
		FROM sales_goods_summary
		WHERE stat_date BETWEEN DATE_SUB(?, INTERVAL `+interval+`) AND DATE_SUB(?, INTERVAL `+interval+`)`+q.planWhCond+q.salesScopeCond+q.cateCond+planExcludeAllotShopsCond+`
		GROUP BY CASE WHEN department IS NULL OR department='' THEN 'other' ELSE department END`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]float64{}
	for rows.Next() {
		var dept string
		var val float64
		if err := rows.Scan(&dept, &val); err != nil {
			return nil, err
		}
		if dept == "" {
			dept = "other"
		}
		m[dept] = val
	}
	return m, rows.Err()
}

// ===== 品类库存健康度 (4) =====

// loadSCCategories 10 核心品类的库存金额/日销成本/周转/高库存占比/缺货率 (SKU 维度跨仓汇总)。
// 注意: 用别名 s, 参数只到 warehouseArgsS (品类靠 HAVING 硬编码白名单, 不走 cateArgs)。
func (h *DashboardHandler) loadSCCategories(q scQuery) ([]scCateHealth, error) {
	args := append([]interface{}{q.stockSnapDate, q.stockSnapDate, q.stockSnapDate}, q.planWhArgs...)
	args = append(args, q.warehouseArgsS...)
	rows, err := h.DB.Query(`
		SELECT
			category,
			ROUND(SUM(sku_stock_value),2),
			ROUND(SUM(sku_daily_cost),2),
			SUM(CASE WHEN sum_month_sale>0 AND sum_avail/(sum_month_sale/30)>50 THEN sku_stock_value ELSE 0 END),
			SUM(CASE WHEN sum_avail<=0 AND sum_month>0 AND is_active_sku=1 THEN 1 ELSE 0 END),
			SUM(CASE WHEN sum_month>0 AND is_active_sku=1 THEN 1 ELSE 0 END)
		FROM (
			SELECT
				s.goods_no,
				CASE
					WHEN g.cate_full_name LIKE '成品/%' THEN SUBSTRING_INDEX(SUBSTRING_INDEX(g.cate_full_name,'/',2),'/',-1)
					WHEN g.cate_full_name IS NOT NULL AND g.cate_full_name != '' THEN g.cate_full_name
					ELSE '未分类'
				END AS category,
				SUM(s.current_qty - s.locked_qty) AS sum_avail,
				SUM(s.month_qty) AS sum_month,
				SUM(s.month_qty)+IFNULL(MAX(sca.allot_qty),0) AS sum_month_sale,
				SUM(s.current_qty * s.cost_price) AS sku_stock_value,
				SUM(s.month_qty * s.cost_price / 30) + IFNULL(MAX(sca.allot_qty),0) * IFNULL(SUM(s.current_qty*s.cost_price)/NULLIF(SUM(s.current_qty),0), 0) / 30 AS sku_daily_cost,
				CASE WHEN MAX(g.flag_data) IN ('非卖品','已下架','下架中','接单产','新品-接单产') THEN 0 ELSE 1 END AS is_active_sku
			FROM stock_quantity_daily s
			LEFT JOIN `+planSpecialAllotQtySubSQL+` sca ON sca.goods_no = s.goods_no
			LEFT JOIN (SELECT goods_no, MAX(cate_full_name) AS cate_full_name, MAX(flag_data) AS flag_data FROM goods WHERE is_delete=0 GROUP BY goods_no) g ON g.goods_no = s.goods_no
			WHERE s.snapshot_date=? AND s.goods_attr=1 AND s.warehouse_name!=''`+q.planWhCondS+q.warehouseCondS+`
			GROUP BY s.goods_no, category
		) t
		GROUP BY category
		HAVING SUM(sku_stock_value) > 0
			AND category IN ('调味料','酱油','调味汁','干制面','素蚝油','酱类','醋','汤底','番茄沙司','糖')
		ORDER BY SUM(sku_stock_value) DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []scCateHealth{}
	for rows.Next() {
		var d scCateHealth
		var highVal float64
		var soCnt, sCnt int
		if err := rows.Scan(&d.Category, &d.StockValue, &d.DailySalesCost, &highVal, &soCnt, &sCnt); err != nil {
			return nil, err
		}
		if d.DailySalesCost > 0 {
			d.Turnover = d.StockValue / d.DailySalesCost
		}
		if d.StockValue > 0 {
			d.HighStockRate = highVal / d.StockValue * 100
		}
		if sCnt > 0 {
			d.StockoutRate = float64(soCnt) / float64(sCnt) * 100
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

// ===== 高库存明细 (5) =====

// loadSCHighStockItems 高库存产品明细 (全仓周转>50天), SKU 维度跨仓汇总, 按库存金额降序 TOP100。
func (h *DashboardHandler) loadSCHighStockItems(q scQuery) ([]scStockDetail, error) {
	cateCondHI, _ := planCategoryGoodsCond("stock_quantity_daily")
	args := append([]interface{}{q.stockSnapDate, q.stockSnapDate, q.stockSnapDate}, q.planWhArgs...)
	args = append(args, q.warehouseArgs...)
	args = append(args, q.cateArgs...)
	rows, err := h.DB.Query(`
		SELECT stock_quantity_daily.goods_no, MAX(goods_name),
			ROUND(SUM(current_qty - locked_qty),0),
			ROUND((SUM(month_qty)+IFNULL(MAX(sca.allot_qty),0))/30,1),
			ROUND(SUM(current_qty - locked_qty) / NULLIF((SUM(month_qty)+IFNULL(MAX(sca.allot_qty),0))/30,0),1),
			ROUND(SUM(current_qty * cost_price),2)
		FROM stock_quantity_daily
		LEFT JOIN `+planSpecialAllotQtySubSQL+` sca ON sca.goods_no = stock_quantity_daily.goods_no
		WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+q.planWhCond+q.warehouseCond+cateCondHI+`
		GROUP BY stock_quantity_daily.goods_no
		HAVING (SUM(month_qty)+IFNULL(MAX(sca.allot_qty),0)) > 0
			AND SUM(current_qty - locked_qty) / ((SUM(month_qty)+IFNULL(MAX(sca.allot_qty),0))/30) > 50
		ORDER BY SUM(current_qty * cost_price) DESC LIMIT 100`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []scStockDetail{}
	for rows.Next() {
		var d scStockDetail
		if err := rows.Scan(&d.GoodsNo, &d.GoodsName, &d.UsableQty, &d.DailySales, &d.Turnover, &d.StockValue); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

// ===== 缺货明细 (6) =====

// loadSCStockoutItems 缺货产品明细 (全仓没货且有销量), SKU 维度跨仓汇总, 按月销降序 TOP100。
func (h *DashboardHandler) loadSCStockoutItems(q scQuery) ([]scStockoutDetail, error) {
	flagCond, flagArgs := planStockoutExcludeFlagsCond("")
	args := append([]interface{}{q.stockSnapDate}, q.planWhArgs...)
	args = append(args, q.warehouseArgs...)
	args = append(args, q.cateArgs...)
	args = append(args, flagArgs...)
	rows, err := h.DB.Query(`
		SELECT goods_no, MAX(goods_name),
			ROUND(SUM(month_qty)/30,1),
			ROUND(SUM(month_qty * cost_price)/30,2)
		FROM stock_quantity_daily
		WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+q.planWhCond+q.warehouseCond+q.cateCond+flagCond+`
		GROUP BY goods_no
		HAVING SUM(current_qty - locked_qty) <= 0 AND SUM(month_qty) > 0
		ORDER BY SUM(month_qty) DESC LIMIT 100`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []scStockoutDetail{}
	for rows.Next() {
		var d scStockoutDetail
		if err := rows.Scan(&d.GoodsNo, &d.GoodsName, &d.DailySales, &d.DailyValue); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

// ===== 销售 TOP20 (7/8) =====

// loadSCTopProducts 销售额 TOP20 (Scan: …Sales, Qty)。
func (h *DashboardHandler) loadSCTopProducts(q scQuery) ([]scTopProduct, error) {
	args := append([]interface{}{q.start, q.end}, q.planWhArgs...)
	args = append(args, q.salesScopeArgs...)
	args = append(args, q.cateArgs...)
	rows, err := h.DB.Query(`
		SELECT t.goods_no, t.goods_name, IFNULL(g.cate_full_name,''), IFNULL(g.goods_field7,''), t.sales, t.qty
		FROM (SELECT goods_no, MAX(goods_name) AS goods_name,
			ROUND(SUM(local_goods_amt),2) AS sales, ROUND(SUM(goods_qty),0) AS qty
			FROM sales_goods_summary FORCE INDEX (idx_date_goods_amt) WHERE stat_date BETWEEN ? AND ?`+q.planWhCond+q.salesScopeCond+q.cateCond+planExcludeAllotShopsCond+`
			GROUP BY goods_no ORDER BY sales DESC LIMIT 20) t
		LEFT JOIN (SELECT goods_no, MAX(cate_full_name) AS cate_full_name, MAX(goods_field7) AS goods_field7
			FROM goods WHERE is_delete=0 GROUP BY goods_no) g ON t.goods_no = g.goods_no`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []scTopProduct{}
	for rows.Next() {
		var d scTopProduct
		if err := rows.Scan(&d.GoodsNo, &d.GoodsName, &d.Category, &d.Grade, &d.Sales, &d.Qty); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

// loadSCTopQtyProducts 销量 TOP20 (Scan: …Qty, Sales — 与销售额榜列序相反)。
func (h *DashboardHandler) loadSCTopQtyProducts(q scQuery) ([]scTopProduct, error) {
	args := append([]interface{}{q.start, q.end}, q.planWhArgs...)
	args = append(args, q.salesScopeArgs...)
	args = append(args, q.cateArgs...)
	rows, err := h.DB.Query(`
		SELECT t.goods_no, t.goods_name, IFNULL(g.cate_full_name,''), IFNULL(g.goods_field7,''), t.qty, t.sales
		FROM (SELECT goods_no, MAX(goods_name) AS goods_name,
			ROUND(SUM(goods_qty),0) AS qty, ROUND(SUM(local_goods_amt),2) AS sales
			FROM sales_goods_summary FORCE INDEX (idx_date_goods_amt) WHERE stat_date BETWEEN ? AND ?`+q.planWhCond+q.salesScopeCond+q.cateCond+planExcludeAllotShopsCond+`
			GROUP BY goods_no ORDER BY qty DESC LIMIT 20) t
		LEFT JOIN (SELECT goods_no, MAX(cate_full_name) AS cate_full_name, MAX(goods_field7) AS goods_field7
			FROM goods WHERE is_delete=0 GROUP BY goods_no) g ON t.goods_no = g.goods_no`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []scTopProduct{}
	for rows.Next() {
		var d scTopProduct
		if err := rows.Scan(&d.GoodsNo, &d.GoodsName, &d.Category, &d.Grade, &d.Qty, &d.Sales); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

// ===== 品类销售占比 (9) =====

// loadSCCateSales 品类销售额 + 毛利 (成品按二级品类聚合)。
func (h *DashboardHandler) loadSCCateSales(q scQuery) ([]scCateSales, error) {
	args := append([]interface{}{q.start, q.end}, q.planWhArgs...)
	args = append(args, q.salesScopeArgs...)
	args = append(args, q.cateArgs...)
	rows, err := h.DB.Query(`
		SELECT
			CASE WHEN cate_name LIKE '成品/%' THEN SUBSTRING_INDEX(SUBSTRING_INDEX(cate_name,'/',2),'/',-1)
				WHEN cate_name IS NOT NULL AND cate_name != '' THEN cate_name ELSE '未分类' END AS category,
			ROUND(SUM(local_goods_amt),2), ROUND(SUM(gross_profit),2)
		FROM sales_goods_summary FORCE INDEX (idx_date_amt) WHERE stat_date BETWEEN ? AND ?`+q.planWhCond+q.salesScopeCond+q.cateCond+planExcludeAllotShopsCond+`
		GROUP BY category HAVING SUM(local_goods_amt) > 0
		ORDER BY SUM(local_goods_amt) DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []scCateSales{}
	for rows.Next() {
		var d scCateSales
		if err := rows.Scan(&d.Category, &d.Sales, &d.Profit); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

// ===== 库龄>90天明细 (10) =====

// loadSCAgedItems 库龄>90天批次明细, 按金额降序 TOP100。args 顺序: end, stockSnapDate, end, …。
func (h *DashboardHandler) loadSCAgedItems(q scQuery) ([]scAgedDetail, error) {
	cateCondAgedB, cateArgsAgedB := planCategoryGoodsCond("b")
	args := append([]interface{}{q.end, q.stockSnapDate, q.end}, q.planWhArgs...)
	args = append(args, q.warehouseArgsB...)
	args = append(args, cateArgsAgedB...)
	rows, err := h.DB.Query(`
		SELECT b.goods_no, b.goods_name, b.warehouse_name,
			ROUND(b.current_qty,0), ROUND(b.current_qty * IFNULL(s.cost_price,0),2),
			b.batch_no, b.production_date, DATEDIFF(?, b.production_date)
		FROM stock_batch_daily b
		LEFT JOIN stock_quantity_daily s ON s.snapshot_date = b.snapshot_date AND b.sku_id = s.sku_id AND b.warehouse_id = s.warehouse_id
		WHERE b.snapshot_date=? AND b.production_date IS NOT NULL AND b.current_qty > 0
			AND b.production_date < DATE_SUB(?, INTERVAL 90 DAY)`+q.planWhCondB+q.warehouseCondB+cateCondAgedB+`
		ORDER BY b.current_qty * IFNULL(s.cost_price,0) DESC LIMIT 100`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []scAgedDetail{}
	for rows.Next() {
		var d scAgedDetail
		if err := rows.Scan(&d.GoodsNo, &d.GoodsName, &d.Warehouse, &d.Qty, &d.StockValue, &d.BatchNo, &d.ProductionDate, &d.AgeDays); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

// ===== 仓库列表 (11) =====

// loadSCWarehouses 当前快照下有库存或有销量的计划仓名称列表。
func (h *DashboardHandler) loadSCWarehouses(q scQuery) ([]string, error) {
	args := append([]interface{}{q.stockSnapDate}, q.planWhArgs...)
	args = append(args, q.warehouseArgs...)
	args = append(args, q.cateArgs...)
	rows, err := h.DB.Query(`SELECT DISTINCT warehouse_name FROM stock_quantity_daily WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+q.planWhCond+q.warehouseCond+q.cateCond+` AND (current_qty>0 OR month_qty>0) ORDER BY warehouse_name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []string{}
	for rows.Next() {
		var wh string
		if err := rows.Scan(&wh); err != nil {
			return nil, err
		}
		list = append(list, wh)
	}
	return list, rows.Err()
}

// ===== wg.Wait 后的组装 (把特殊渠道调拨当销售并入渠道/品类口径) =====

// scAllotDays 区间天数 (给渠道 split 的日均重算); start/end 非法时退化为 1。
func scAllotDays(start, end string) int {
	allotDays := 1
	if sT, e1 := time.Parse("2006-01-02", start); e1 == nil {
		if eT, e2 := time.Parse("2006-01-02", end); e2 == nil {
			if dd := int(eT.Sub(sT).Hours()/24) + 1; dd > 1 {
				allotDays = dd
			}
		}
	}
	return allotDays
}

// mergeSCAllotCateSales 把特殊渠道调拨当销售按品类并进品类销售饼: 已有品类累加, 仅有调拨的品类补行。
func mergeSCAllotCateSales(list []scCateSales, allot planAllotAgg) []scCateSales {
	for i := range list {
		list[i].Sales += allot.ByCategory[list[i].Category]
	}
	// 补: 有调拨但本期基础销售=0 的品类(HAVING>0 滤掉了), 否则饼合计 < GMV
	seenCat := map[string]bool{}
	for _, c := range list {
		seenCat[c.Category] = true
	}
	for cat, amt := range allot.ByCategory {
		if amt != 0 && !seenCat[cat] {
			list = append(list, scCateSales{Category: cat, Sales: amt})
		}
	}
	return list
}

// assembleSCChannels 组装渠道环比: 基础销售(channelMap, 按 channelOrder 顺序)并入上月/去年同期 + 特殊渠道调拨当销售,
// 算 MoM/YoY; 再补"只有调拨没有基础销售"的部门。channelMap 的 value 是 *scChannelData。
func assembleSCChannels(start, end string, channelOrder []string, channelMap, lmMap, lyMap *sync.Map, allot, allotLM, allotLY planAllotAgg) []scChannelData {
	allotDays := scAllotDays(start, end)
	channels := []scChannelData{}
	for _, key := range channelOrder {
		if v, ok := channelMap.Load(key); ok {
			d := v.(*scChannelData)
			if lm, ok := lmMap.Load(key); ok {
				d.LastMonth = lm.(float64)
			}
			if ly, ok := lyMap.Load(key); ok {
				d.LastYear = ly.(float64)
			}
			// 并入该部门调拨当销售(本期/上月/去年同窗口), 保证 split 合计=GMV 且 MoM/YoY 公平对比
			if a := allot.ByDept[key]; a != 0 {
				d.Total += a
				d.DailyAvg = d.Total / float64(allotDays)
			}
			d.LastMonth += allotLM.ByDept[key]
			d.LastYear += allotLY.ByDept[key]
			if d.LastMonth > 0 {
				d.MomRate = (d.Total/d.LastMonth - 1) * 100
			}
			if d.LastYear > 0 {
				d.YoyRate = (d.Total/d.LastYear - 1) * 100
			}
			channels = append(channels, *d)
		}
	}
	// 补: 有调拨但本期8仓+10品类基础销售=0 的部门(不在 channelOrder), 否则渠道 split 合计 < GMV
	seenDept := map[string]bool{}
	for _, c := range channels {
		seenDept[c.Channel] = true
	}
	for dept, amt := range allot.ByDept {
		if amt != 0 && !seenDept[dept] {
			nd := scChannelData{Channel: dept, Total: amt, DailyAvg: amt / float64(allotDays)}
			nd.LastMonth = allotLM.ByDept[dept]
			nd.LastYear = allotLY.ByDept[dept]
			if nd.LastMonth > 0 {
				nd.MomRate = (nd.Total/nd.LastMonth - 1) * 100
			}
			if nd.LastYear > 0 {
				nd.YoyRate = (nd.Total/nd.LastYear - 1) * 100
			}
			channels = append(channels, nd)
		}
	}
	return channels
}
