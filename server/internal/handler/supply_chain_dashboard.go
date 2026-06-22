package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

func (h *DashboardHandler) GetSupplyChainMonthlyTrend(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDomainAccess(r, "supply_chain")) {
		return
	}
	salesScopeCond, salesScopeArgs, err := buildSalesDataScopeCond(r, "", "", "")
	if writeScopeError(w, err) {
		return
	}

	startMonth := r.URL.Query().Get("start_month")
	endMonth := r.URL.Query().Get("end_month")
	if startMonth == "" {
		startMonth = "2020-01"
	}
	if endMonth == "" {
		endMonth = "9999-12"
	}

	// 把 yyyy-MM 转成日期范围：start是月1号，end是下月1号-1天
	startT, err1 := time.Parse("2006-01", startMonth)
	endT, err2 := time.Parse("2006-01", endMonth)
	if err1 != nil || err2 != nil {
		writeError(w, 400, "invalid month format")
		return
	}
	startDate := startT.Format("2006-01-02")
	endDate := endT.AddDate(0, 1, 0).AddDate(0, 0, -1).Format("2006-01-02")

	// 查月表（从日表聚合生成的，数据一致且性能高）
	_ = startDate // 保留计算（未来可能复用）
	_ = endDate
	whCond, whArgs := buildPlanWarehouseFilter("warehouse_name")
	// v1.x: 趋势改按 10 核心品类(JOIN 品类派生表), 跟 GMV 卡/整页口径一致 —— 之前漏加品类过滤,
	//   趋势是全品类而 KPI 是 10 品类, 导致同页"趋势末月柱 ≠ 销售GMV卡"对不上(差非核心品类+组套产品)。
	catSub, catSubArgs := planCategoryGoodsSubquery()
	args := append([]interface{}{}, catSubArgs...)
	args = append(args, startMonth, endMonth)
	args = append(args, whArgs...)
	args = append(args, salesScopeArgs...)

	rows, err := h.DB.Query(`
		SELECT stat_month AS m, ROUND(SUM(local_goods_amt),2)
		FROM sales_goods_summary_monthly
		JOIN (`+catSub+`) gc ON gc.goods_no = sales_goods_summary_monthly.goods_no
		WHERE stat_month BETWEEN ? AND ?`+whCond+salesScopeCond+planExcludeAllotShopsCond+`
		GROUP BY stat_month ORDER BY stat_month`, args...)
	if err != nil {
		log.Printf("monthly trend query failed: %v", err)
		writeError(w, 500, "database query failed")
		return
	}
	defer rows.Close()

	type MonthData struct {
		Month string  `json:"month"`
		Value float64 `json:"value"`
	}
	list := []MonthData{}
	for rows.Next() {
		var d MonthData
		if err := rows.Scan(&d.Month, &d.Value); err != nil {
			writeServerError(w, 500, "扫描月度数据失败", err)
			return
		}
		list = append(list, d)
	}

	// v1.x: 月度趋势并入特殊渠道(京东/猫超/朴朴)调拨当销售(按月), 同样按 10 核心品类筛(跟销售侧+GMV卡一致)。
	//   调拨无仓维度(全公司), 这 3 渠道货从计划仓发故归入计划趋势。销售月份⊇调拨月份, 不补缺月。
	catSub2, catSub2Args := planCategoryGoodsSubquery()
	allotArgs := append([]interface{}{startMonth, endMonth}, catSub2Args...)
	allotByMonth := map[string]float64{}
	aRows, aErr := h.DB.Query(`
		SELECT DATE_FORMAT(o.stat_date,'%Y-%m') AS ym, IFNULL(SUM(d.excel_amount),0)
		FROM allocate_orders o JOIN allocate_details d ON d.allocate_no = o.allocate_no
		WHERE o.channel_key IN ('京东','猫超','朴朴')
		  AND DATE_FORMAT(o.stat_date,'%Y-%m') BETWEEN ? AND ?
		  AND d.goods_no IN (`+catSub2+`)
		GROUP BY ym`, allotArgs...)
	if aErr == nil {
		defer aRows.Close()
		for aRows.Next() {
			var ym string
			var amt float64
			if aRows.Scan(&ym, &amt) == nil {
				allotByMonth[ym] = amt
			}
		}
	}
	for i := range list {
		list[i].Value += allotByMonth[list[i].Month]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code": 200,
		"data": list,
	})
}

// GetSupplyChainDashboard 计划端BI看板
func (h *DashboardHandler) GetSupplyChainDashboard(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDomainAccess(r, "supply_chain")) {
		return
	}

	start, end := getDateRange(r, h.DB)
	warehouse := r.URL.Query().Get("warehouse")

	warehouseCond, warehouseArgs, err := buildWarehouseScopeCond(r, warehouse, "warehouse_name")
	if writeScopeError(w, err) {
		return
	}
	warehouseCondS, warehouseArgsS, err := buildWarehouseScopeCond(r, warehouse, "s.warehouse_name")
	if writeScopeError(w, err) {
		return
	}
	warehouseCondB, warehouseArgsB, err := buildWarehouseScopeCond(r, warehouse, "b.warehouse_name")
	if writeScopeError(w, err) {
		return
	}
	salesScopeCond, salesScopeArgs, err := buildSalesDataScopeCond(r, "", "", "")
	if writeScopeError(w, err) {
		return
	}

	// 库存快照日期：取小于等于 end 的最近一个快照日
	// 如果用户选的 end 当天没快照（历史数据或未来），自动回退到最近一次快照
	var stockSnapDate string
	if err := h.DB.QueryRow(`SELECT IFNULL(MAX(snapshot_date),'') FROM stock_quantity_daily WHERE snapshot_date <= ?`, end).Scan(&stockSnapDate); err != nil {
		log.Printf("查询库存快照日期失败: %v", err)
	}
	if stockSnapDate == "" {
		// end 早于任何快照（历史日期），使用最早的快照作为兜底
		h.DB.QueryRow(`SELECT IFNULL(MIN(snapshot_date),'') FROM stock_quantity_daily`).Scan(&stockSnapDate)
	}

	// 计划看板仓库白名单（采购需求：只展示这几个成品仓/委外仓/云仓的数据，名单在文件顶部 planWarehouses）
	// 动态构造 IN 子句, 跟 planWarehouses 数量自动对齐 (加/减仓只改名单, 不用再改问号个数)
	planWhCond, planWhArgs := buildPlanWarehouseFilter("warehouse_name")
	planWhCondS, _ := buildPlanWarehouseFilter("s.warehouse_name")
	planWhCondB, _ := buildPlanWarehouseFilter("b.warehouse_name")

	// v1.02: 6 KPI 全部加 10 品类白名单过滤, 跟"品类库存健康度"口径统一
	cateCond, cateArgs := planCategoryGoodsCond("")

	// 各 section 共用的作用域过滤条件收进 scQuery, 传给 loadSC* helper (见 supply_chain_dashboard_sections.go)
	q := scQuery{
		start: start, end: end, stockSnapDate: stockSnapDate,
		warehouseCond: warehouseCond, warehouseArgs: warehouseArgs,
		warehouseCondS: warehouseCondS, warehouseArgsS: warehouseArgsS,
		warehouseCondB: warehouseCondB, warehouseArgsB: warehouseArgsB,
		salesScopeCond: salesScopeCond, salesScopeArgs: salesScopeArgs,
		planWhCond: planWhCond, planWhArgs: planWhArgs,
		planWhCondS: planWhCondS, planWhCondB: planWhCondB,
		cateCond: cateCond, cateArgs: cateArgs,
	}

	// 所有 section 查询并发执行 (各写各的结果变量, 互不重叠 → 无数据竞争), wg.Wait 后顺序组装。
	// run() 收口 wg.Add(1)/go/defer wg.Done() 样板 + 错误传播 (任一 section 出错记第一个 err)。
	var wg sync.WaitGroup
	var queryErr error
	var errMu sync.Mutex
	setQueryErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		if queryErr == nil {
			queryErr = err
		}
		errMu.Unlock()
	}
	run := func(fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := fn(); e != nil {
				setQueryErr(e)
			}
		}()
	}

	// ===== 结果变量 (helper 并发写入, wg.Wait 后读) =====
	var salesGMV, stockCost, dailyCost, allotDailyCost, turnoverDays, agedStockValue float64
	var highStockValue, totalStockValue, highStockRate, stockoutRate float64
	var stockoutSKU, salesSKU int
	// v1.69.2: 单仓缺货率 (按 SKU×仓 单元) — 跟 stockoutRate 的"全仓汇总"区分
	var perWhStockoutUnits, perWhSalesUnits int
	var perWhStockoutRate float64
	// v1.x: 钱侧并入特殊渠道(京东/猫超/朴朴)调拨当销售。planAllot=本期, LM=上月同期, LY=去年同期。详见 loadPlanAllot。
	var planAllot, planAllotLM, planAllotLY planAllotAgg
	monthlySales := []scMonthData{}
	channelMap := &sync.Map{}
	channelOrder := []string{}
	channelSeen := map[string]bool{}
	lmMap := &sync.Map{}
	lyMap := &sync.Map{}
	categories := []scCateHealth{}
	highStockItems := []scStockDetail{}
	stockoutItems := []scStockoutDetail{}
	topProducts := []scTopProduct{}
	topQtyProducts := []scTopProduct{}
	cateSalesList := []scCateSales{}
	agedItems := []scAgedDetail{}
	whList := []string{}

	// ========== 1. KPI 卡片 ==========
	run(func() (e error) { planAllot, planAllotLM, planAllotLY, e = h.loadSCPlanAllotTriple(start, end); return })
	run(func() (e error) { salesGMV, e = h.loadSCSalesGMV(q); return })
	run(func() (e error) { stockCost, dailyCost, e = h.loadSCStockCost(q); return })
	run(func() (e error) { allotDailyCost, e = h.loadSCAllotDailyCost(q); return })
	run(func() (e error) { stockoutSKU, salesSKU, e = h.loadSCStockoutSKU(q); return })
	run(func() (e error) { perWhStockoutUnits, perWhSalesUnits, e = h.loadSCPerWhStockout(q); return })
	run(func() (e error) { highStockValue, totalStockValue, e = h.loadSCHighStockValue(q); return })
	run(func() (e error) { agedStockValue, e = h.loadSCAgedStockValue(q); return })

	// ========== 2. 月度销售趋势 ==========
	run(func() (e error) { monthlySales, e = h.loadSCMonthlySales(q); return })

	// ========== 3. 各渠道销售额环比 ==========
	run(func() error {
		list, e := h.loadSCChannels(q)
		if e != nil {
			return e
		}
		for i := range list {
			d := &list[i]
			channelMap.Store(d.Channel, d)
			if !channelSeen[d.Channel] {
				channelOrder = append(channelOrder, d.Channel)
				channelSeen[d.Channel] = true
			}
		}
		return nil
	})
	// 上月同期 / 去年同期 (SQL 仅差 INTERVAL 1 MONTH / 1 YEAR)
	run(func() error {
		m, e := h.loadSCDeptSales(q, "1 MONTH")
		if e != nil {
			return e
		}
		for k, v := range m {
			lmMap.Store(k, v)
		}
		return nil
	})
	run(func() error {
		m, e := h.loadSCDeptSales(q, "1 YEAR")
		if e != nil {
			return e
		}
		for k, v := range m {
			lyMap.Store(k, v)
		}
		return nil
	})

	// ========== 4. 品类库存健康度 ==========
	run(func() (e error) { categories, e = h.loadSCCategories(q); return })

	// ========== 5. 高库存 + 6. 缺货 (SKU 维度跨仓汇总) ==========
	run(func() (e error) { highStockItems, e = h.loadSCHighStockItems(q); return })
	run(func() (e error) { stockoutItems, e = h.loadSCStockoutItems(q); return })

	// ========== 7/8. 销售TOP20 ==========
	run(func() (e error) { topProducts, e = h.loadSCTopProducts(q); return })
	run(func() (e error) { topQtyProducts, e = h.loadSCTopQtyProducts(q); return })

	// ========== 9. 品类销售占比 ==========
	run(func() (e error) { cateSalesList, e = h.loadSCCateSales(q); return })

	// ========== 10. 库龄>90天 ==========
	run(func() (e error) { agedItems, e = h.loadSCAgedItems(q); return })

	// ========== 11. 仓库列表 ==========
	run(func() (e error) { whList, e = h.loadSCWarehouses(q); return })

	// 等待所有查询完成
	wg.Wait()
	if queryErr != nil {
		log.Printf("supply chain dashboard query failed: %v", queryErr)
		writeError(w, 500, "database query failed")
		return
	}

	// 计算KPI派生值
	if salesSKU > 0 {
		stockoutRate = float64(stockoutSKU) / float64(salesSKU) * 100
	}
	if perWhSalesUnits > 0 {
		perWhStockoutRate = float64(perWhStockoutUnits) / float64(perWhSalesUnits) * 100
	}
	if totalStockValue > 0 {
		highStockRate = highStockValue / totalStockValue * 100
	}
	// v1.x: 周转天数分母并入调拨日销成本(allotDailyCost), 与高库存/明细口径一致
	if dailyCost+allotDailyCost > 0 {
		turnoverDays = stockCost / (dailyCost + allotDailyCost)
	}

	// v1.x: 钱侧并入特殊渠道调拨当销售金额。GMV 纯加法; 品类饼/渠道 split 各自按品类/部门并 (保证同页 KPI/split/饼 合计一致)。
	//   月度趋势走独立端点 GetSupplyChainMonthlyTrend(前端 trendData 用那个), 在那边单独并, 不在此 embedded monthlySales。
	salesGMV += planAllot.Total
	cateSalesList = mergeSCAllotCateSales(cateSalesList, planAllot)
	channels := assembleSCChannels(start, end, channelOrder, channelMap, lmMap, lyMap, planAllot, planAllotLM, planAllotLY)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code": 200,
		"data": map[string]interface{}{
			"kpi": map[string]interface{}{
				"salesGMV":           salesGMV,        // 总销售额(销售出库 + 特殊渠道调拨当销售)
				"allotGMV":           planAllot.Total, // 其中: 特殊渠道(京东/猫超/朴朴)调拨当销售部分(前端拆分显示)
				"stockCost":          stockCost,
				"turnoverDays":       turnoverDays,
				"highStockRate":      highStockRate,
				"stockoutRate":       stockoutRate,
				"stockoutSKU":        stockoutSKU,
				"salesSKU":           salesSKU,
				"perWhStockoutRate":  perWhStockoutRate,
				"perWhStockoutUnits": perWhStockoutUnits,
				"perWhSalesUnits":    perWhSalesUnits,
				"agedStockValue":     agedStockValue,
			},
			"monthlySales":   monthlySales,
			"channels":       channels,
			"categories":     categories,
			"highStockItems": highStockItems,
			"stockoutItems":  stockoutItems,
			"topProducts":    topProducts,
			"topQtyProducts": topQtyProducts,
			"cateSales":      cateSalesList,
			"agedItems":      agedItems,
			"warehouses":     whList,
		},
	})
}

// GetPurchasePlan 采购计划仪表盘 (v0.48)
// 5 张表 JOIN: stock_quantity + sales (吉客云) + ys_purchase_orders + ys_subcontract_orders + ys_material_out
// 算法:
//   成品 (goods_attr=1) 目标 45 天, 日均 = month_qty/30 (吉客云销售)
//   包材 (goods_attr=4) 目标 90 天, 日均 = ys_material_out 近30天/30 (真实消耗)
//   建议采购量 = max(0, 目标天数 × 日均 - 库存 - 在途采购 - 在途委外)
//   状态: 紧急(可售<7) / 偏低(7-14) / 正常 / 积压(>90)
