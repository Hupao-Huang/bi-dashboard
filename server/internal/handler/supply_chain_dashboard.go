package handler

import (
	"database/sql"
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
		WHERE stat_month BETWEEN ? AND ?`+whCond+salesScopeCond+`
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

	// 用WaitGroup并发执行所有查询
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
	queryRows := func(query string, args ...interface{}) (*sql.Rows, bool) {
		rows, err := h.DB.Query(query, args...)
		if err != nil {
			setQueryErr(err)
			return nil, false
		}
		return rows, true
	}

	// ========== 1. KPI 卡片 ==========
	var salesGMV, stockCost, dailyCost, allotDailyCost, turnoverDays, agedStockValue float64
	var highStockValue, totalStockValue, highStockRate, stockoutRate float64
	var stockoutSKU, salesSKU int
	// v1.69.2: 单仓缺货率 (按 SKU×仓 单元) — 跟 stockoutRate 的"全仓汇总"区分
	// 跑哥 2026-05-20 报: 全仓 0% 把 63 个 (SKU×仓) 单仓缺货掩盖了, 调拨成本/时效是真实采购预警信号
	var perWhStockoutUnits, perWhSalesUnits int
	var perWhStockoutRate float64

	// v1.02: 6 KPI 全部加 10 品类白名单过滤, 跟"品类库存健康度"口径统一
	cateCond, cateArgs := planCategoryGoodsCond("")

	// v1.x: 钱侧并入特殊渠道(京东/猫超/朴朴)调拨当销售金额(GMV/趋势/渠道split/品类饼 一致)。
	//   planAllot=本期[start,end], LM=上月同期, LY=去年同期(给渠道 MoM/YoY 公平对比), Trend=全历史按月(给趋势图)。
	//   计划看板当前 GMV 不含这 3 渠道(它们销售挂自己平台仓, 不在计划 8 仓)→ 纯加法。详见 loadPlanAllot。
	var planAllot, planAllotLM, planAllotLY planAllotAgg
	wg.Add(1)
	go func() {
		defer wg.Done()
		var e error
		if planAllot, e = h.loadPlanAllot(start, end); e != nil {
			setQueryErr(e)
			return
		}
		if sT, e1 := time.Parse("2006-01-02", start); e1 == nil {
			if eT, e2 := time.Parse("2006-01-02", end); e2 == nil {
				if planAllotLM, e = h.loadPlanAllot(sT.AddDate(0, -1, 0).Format("2006-01-02"), eT.AddDate(0, -1, 0).Format("2006-01-02")); e != nil {
					setQueryErr(e)
					return
				}
				if planAllotLY, e = h.loadPlanAllot(sT.AddDate(-1, 0, 0).Format("2006-01-02"), eT.AddDate(-1, 0, 0).Format("2006-01-02")); e != nil {
					setQueryErr(e)
					return
				}
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		salesGMVArgs := append([]interface{}{start, end}, planWhArgs...)
		salesGMVArgs = append(salesGMVArgs, salesScopeArgs...)
		salesGMVArgs = append(salesGMVArgs, cateArgs...)
		if err := h.DB.QueryRow(`SELECT IFNULL(SUM(local_goods_amt),0) FROM sales_goods_summary WHERE stat_date BETWEEN ? AND ?`+planWhCond+salesScopeCond+cateCond, salesGMVArgs...).Scan(&salesGMV); err != nil {
			setQueryErr(err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		stockCostArgs := append([]interface{}{stockSnapDate}, planWhArgs...)
		stockCostArgs = append(stockCostArgs, warehouseArgs...)
		stockCostArgs = append(stockCostArgs, cateArgs...)
		if err := h.DB.QueryRow(
			`SELECT IFNULL(SUM(current_qty * cost_price),0), IFNULL(SUM(month_qty * cost_price / 30),0)
			 FROM stock_quantity_daily WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+planWhCond+warehouseCond+cateCond,
			stockCostArgs...).Scan(&stockCost, &dailyCost); err != nil {
			setQueryErr(err)
		}
	}()

	// v1.x: 周转天数 KPI 的日销成本(dailyCost)并入特殊渠道调拨当销售。调拨是 goods 级件数无成本,
	//   用该货在计划仓的"库存加权平均成本"(SUM(qty*cost)/SUM(qty), 自动剔除0库存仓脏cost)配成本:
	//   allotDailyCost = SUM_over_goods( allot_qty × 加权avg成本 ) / 30, 加到 dailyCost(见下 turnoverDays 计算)。
	//   纯调拨0库存货取不到成本 → wcost=NULL → 该货记0(保守, 宁可周转略高不灌假成本)。日期锚 stockSnapDate 与库存快照一致。
	//   sca 的 2 个 ? 在 JOIN 文本最前, 外加 gc 的 snapshot_date ?, 故 stockSnapDate 出现 3 次。
	wg.Add(1)
	go func() {
		defer wg.Done()
		cateCondGC, _ := planCategoryGoodsCond("stock_quantity_daily")
		adcArgs := append([]interface{}{stockSnapDate, stockSnapDate, stockSnapDate}, planWhArgs...)
		adcArgs = append(adcArgs, warehouseArgs...)
		adcArgs = append(adcArgs, cateArgs...)
		if err := h.DB.QueryRow(`SELECT IFNULL(SUM(sca.allot_qty * gc.wcost)/30, 0)
			FROM `+planSpecialAllotQtySubSQL+` sca
			JOIN (
				SELECT goods_no, SUM(current_qty * cost_price)/NULLIF(SUM(current_qty),0) AS wcost
				FROM stock_quantity_daily
				WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+planWhCond+warehouseCond+cateCondGC+`
				GROUP BY goods_no
			) gc ON gc.goods_no = sca.goods_no`, adcArgs...).Scan(&allotDailyCost); err != nil {
			setQueryErr(err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		// v1.02: 缺货率改 SKU 维度跨仓汇总 (跟"缺货产品明细"口径一致)
		// + 排除非卖品/已下架/下架中/接单产/新品-接单产 标签的 SKU
		// v1.x 决策: 缺货门槛(sum_month>0)故意不并调拨当销售。缺货定义="计划仓没货可发但有人要买"的采购预警;
		//   京东/猫超/朴朴的货是从计划仓调拨出去的(货已发=已到位), 把调拨算进缺货分子会制造假缺货信号。
		//   与高库存(并调拨, 放宽)方向相反: 缺货并调拨是"收紧", 业务无对应需求。**勿好心改成 sum_month_sale**。
		flagExcludeCond, flagExcludeArgs := planStockoutExcludeFlagsCond("")
		stockoutArgs := append([]interface{}{stockSnapDate}, planWhArgs...)
		stockoutArgs = append(stockoutArgs, warehouseArgs...)
		stockoutArgs = append(stockoutArgs, cateArgs...)
		stockoutArgs = append(stockoutArgs, flagExcludeArgs...)
		if err := h.DB.QueryRow(`SELECT
			IFNULL(SUM(CASE WHEN sum_avail<=0 AND sum_month>0 THEN 1 ELSE 0 END),0),
			IFNULL(SUM(CASE WHEN sum_month>0 THEN 1 ELSE 0 END),0)
			FROM (
				SELECT goods_no,
					SUM(current_qty - locked_qty) AS sum_avail,
					SUM(month_qty) AS sum_month
				FROM stock_quantity_daily
				WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+planWhCond+warehouseCond+cateCond+flagExcludeCond+`
				GROUP BY goods_no
			) t`, stockoutArgs...).Scan(&stockoutSKU, &salesSKU); err != nil {
			setQueryErr(err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		// v1.69.2: 单仓缺货率 — 不 GROUP BY goods_no, 直接按 (SKU, 仓库) 单元算
		// 跟 stockoutRate "全仓汇总" 区分: 单仓缺货虽然其他仓有, 但调拨要时间+成本, 是采购预警
		// v1.x: 调拨无仓库维度(allocate_details 不含仓), 全公司合计无法拆到单仓单元 → 单仓缺货率保持纯 month_qty,
		//   不并调拨当销售(全仓视角的高库存/周转已并入)。缺货门槛口径见下方 stockoutRate 注释(决策: 缺货不并调拨)。
		perWhFlagCond, perWhFlagArgs := planStockoutExcludeFlagsCond("")
		perWhArgs := append([]interface{}{stockSnapDate}, planWhArgs...)
		perWhArgs = append(perWhArgs, warehouseArgs...)
		perWhArgs = append(perWhArgs, cateArgs...)
		perWhArgs = append(perWhArgs, perWhFlagArgs...)
		if err := h.DB.QueryRow(`SELECT
			IFNULL(SUM(CASE WHEN (current_qty - locked_qty) <= 0 AND month_qty > 0 THEN 1 ELSE 0 END), 0),
			IFNULL(SUM(CASE WHEN month_qty > 0 THEN 1 ELSE 0 END), 0)
			FROM stock_quantity_daily
			WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+planWhCond+warehouseCond+cateCond+perWhFlagCond, perWhArgs...).Scan(&perWhStockoutUnits, &perWhSalesUnits); err != nil {
			setQueryErr(err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		// v1.02: 高库存占比改 SKU 维度跨仓汇总 (跟"高库存产品明细"口径一致)
		// 子查询先按 SKU 汇总 → 外层判断"全仓周转>50天"才算高库存
		// v1.x: 高库存占比口径跟"高库存产品明细"一致, 周转分母并进特殊渠道(京东/猫超/朴朴)调拨当销售量。
		//   详见 planSpecialAllotQtySubSQL; 调拨 2 个 ? 在最前(JOIN 在 WHERE 之前), 故 stockSnapDate 出现 3 次。
		cateCondHR, _ := planCategoryGoodsCond("stock_quantity_daily")
		highStockArgs := append([]interface{}{stockSnapDate, stockSnapDate, stockSnapDate}, planWhArgs...)
		highStockArgs = append(highStockArgs, warehouseArgs...)
		highStockArgs = append(highStockArgs, cateArgs...)
		if err := h.DB.QueryRow(`SELECT
			IFNULL(SUM(CASE WHEN sum_month>0 AND sum_avail/(sum_month/30) > 50 THEN sku_stock_value ELSE 0 END),0),
			IFNULL(SUM(sku_stock_value),0)
			FROM (
				SELECT stock_quantity_daily.goods_no,
					SUM(current_qty - locked_qty) AS sum_avail,
					SUM(month_qty)+IFNULL(MAX(sca.allot_qty),0) AS sum_month,
					SUM(current_qty * cost_price) AS sku_stock_value
				FROM stock_quantity_daily
				LEFT JOIN `+planSpecialAllotQtySubSQL+` sca ON sca.goods_no = stock_quantity_daily.goods_no
				WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+planWhCond+warehouseCond+cateCondHR+`
				GROUP BY stock_quantity_daily.goods_no
			) t`, highStockArgs...).Scan(&highStockValue, &totalStockValue); err != nil {
			setQueryErr(err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		agedArgs := append([]interface{}{stockSnapDate, end}, planWhArgs...)
		agedArgs = append(agedArgs, warehouseArgsB...)
		// v1.02: 库龄>90天 KPI 加 10 品类过滤 (b.goods_no 走品类白名单子查询)
		cateCondB, cateArgsB := planCategoryGoodsCond("b")
		agedArgs = append(agedArgs, cateArgsB...)
		if err := h.DB.QueryRow(`SELECT IFNULL(SUM(b.current_qty * IFNULL(s.cost_price,0)),0)
			FROM stock_batch_daily b
			LEFT JOIN stock_quantity_daily s ON s.snapshot_date = b.snapshot_date AND b.sku_id = s.sku_id AND b.warehouse_id = s.warehouse_id
			WHERE b.snapshot_date=? AND b.production_date IS NOT NULL AND b.current_qty > 0
			AND b.production_date < DATE_SUB(?, INTERVAL 90 DAY)`+planWhCondB+warehouseCondB+cateCondB, agedArgs...).Scan(&agedStockValue); err != nil {
			setQueryErr(err)
		}
	}()

	// ========== 2. 月度销售趋势 ==========
	type MonthData struct {
		Month string  `json:"month"`
		Value float64 `json:"value"`
	}
	monthlySales := []MonthData{}
	var muMonthly sync.Mutex
	wg.Add(1)
	go func() {
		defer wg.Done()
		// v1.75.x: 改读月度物化表 sales_goods_summary_monthly (从日表聚合, 口径一致实测同值),
		// 恢复"全部历史月"趋势(原日表全表扫 173s 超时被临时砍成近 13 个月); 品类用 JOIN 派生表避开 9s 慢子查询(实测 1.3s)
		catSub, catSubArgs := planCategoryGoodsSubquery()
		monthlyArgs := append([]interface{}{}, catSubArgs...)
		monthlyArgs = append(monthlyArgs, planWhArgs...)
		monthlyArgs = append(monthlyArgs, salesScopeArgs...)
		rows, ok := queryRows(`
			SELECT stat_month AS m, ROUND(SUM(local_goods_amt),2)
			FROM sales_goods_summary_monthly
			JOIN (`+catSub+`) gc ON gc.goods_no = sales_goods_summary_monthly.goods_no
			WHERE 1=1`+planWhCond+salesScopeCond+`
			GROUP BY stat_month ORDER BY stat_month`, monthlyArgs...)
		if !ok {
			return
		}
		defer rows.Close()
		muMonthly.Lock()
		defer muMonthly.Unlock()
		for rows.Next() {
			var d MonthData
			if err := rows.Scan(&d.Month, &d.Value); err != nil {
				setQueryErr(err)
				return
			}
			monthlySales = append(monthlySales, d)
		}
		if err := rows.Err(); err != nil {
			setQueryErr(err)
		}
	}()

	// ========== 3. 各渠道销售额环比 ==========
	type ChannelData struct {
		Channel   string  `json:"channel"`
		DailyAvg  float64 `json:"dailyAvg"`
		Total     float64 `json:"total"`
		LastMonth float64 `json:"lastMonth"`
		MomRate   float64 `json:"momRate"`
		LastYear  float64 `json:"lastYear"`
		YoyRate   float64 `json:"yoyRate"`
	}
	channelMap := &sync.Map{}
	channelOrder := []string{}
	channelSeen := map[string]bool{}
	var muOrder sync.Mutex

	wg.Add(1)
	go func() {
		defer wg.Done()
		channelArgs := append([]interface{}{end, start, start, end}, planWhArgs...)
		channelArgs = append(channelArgs, salesScopeArgs...)
		channelArgs = append(channelArgs, cateArgs...)
		rows, ok := queryRows(`
			SELECT CASE WHEN department IS NULL OR department='' THEN 'other' ELSE department END,
				ROUND(SUM(local_goods_amt)/GREATEST(DATEDIFF(?,?)+1,1),2),
				ROUND(SUM(local_goods_amt),2)
			FROM sales_goods_summary WHERE stat_date BETWEEN ? AND ?`+planWhCond+salesScopeCond+cateCond+`
			GROUP BY CASE WHEN department IS NULL OR department='' THEN 'other' ELSE department END
			ORDER BY SUM(local_goods_amt) DESC`, channelArgs...)
		if !ok {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var d ChannelData
			if err := rows.Scan(&d.Channel, &d.DailyAvg, &d.Total); err != nil {
				setQueryErr(err)
				return
			}
			if d.Channel == "" {
				d.Channel = "other"
			}
			channelMap.Store(d.Channel, &d)
			muOrder.Lock()
			if !channelSeen[d.Channel] {
				channelOrder = append(channelOrder, d.Channel)
				channelSeen[d.Channel] = true
			}
			muOrder.Unlock()
		}
		if err := rows.Err(); err != nil {
			setQueryErr(err)
		}
	}()

	// 上月同期 + 去年同期
	lmMap := &sync.Map{}
	lyMap := &sync.Map{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		lmArgs := append([]interface{}{start, end}, planWhArgs...)
		lmArgs = append(lmArgs, salesScopeArgs...)
		lmArgs = append(lmArgs, cateArgs...)
		rows, ok := queryRows(`
			SELECT CASE WHEN department IS NULL OR department='' THEN 'other' ELSE department END, ROUND(SUM(local_goods_amt),2)
			FROM sales_goods_summary
			WHERE stat_date BETWEEN DATE_SUB(?, INTERVAL 1 MONTH) AND DATE_SUB(?, INTERVAL 1 MONTH)`+planWhCond+salesScopeCond+cateCond+`
			GROUP BY CASE WHEN department IS NULL OR department='' THEN 'other' ELSE department END`, lmArgs...)
		if !ok {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var dept string
			var val float64
			if err := rows.Scan(&dept, &val); err != nil {
				setQueryErr(err)
				return
			}
			if dept == "" {
				dept = "other"
			}
			lmMap.Store(dept, val)
		}
		if err := rows.Err(); err != nil {
			setQueryErr(err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		lyArgs := append([]interface{}{start, end}, planWhArgs...)
		lyArgs = append(lyArgs, salesScopeArgs...)
		lyArgs = append(lyArgs, cateArgs...)
		rows, ok := queryRows(`
			SELECT CASE WHEN department IS NULL OR department='' THEN 'other' ELSE department END, ROUND(SUM(local_goods_amt),2)
			FROM sales_goods_summary
			WHERE stat_date BETWEEN DATE_SUB(?, INTERVAL 1 YEAR) AND DATE_SUB(?, INTERVAL 1 YEAR)`+planWhCond+salesScopeCond+cateCond+`
			GROUP BY CASE WHEN department IS NULL OR department='' THEN 'other' ELSE department END`, lyArgs...)
		if !ok {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var dept string
			var val float64
			if err := rows.Scan(&dept, &val); err != nil {
				setQueryErr(err)
				return
			}
			if dept == "" {
				dept = "other"
			}
			lyMap.Store(dept, val)
		}
		if err := rows.Err(); err != nil {
			setQueryErr(err)
		}
	}()

	// ========== 4. 品类库存健康度 ==========
	type CateHealth struct {
		Category       string  `json:"category"`
		StockValue     float64 `json:"stockValue"`
		DailySalesCost float64 `json:"dailySalesCost"`
		Turnover       float64 `json:"turnover"`
		HighStockRate  float64 `json:"highStockRate"`
		StockoutRate   float64 `json:"stockoutRate"`
	}
	categories := []CateHealth{}
	var muCate sync.Mutex
	wg.Add(1)
	go func() {
		defer wg.Done()
		// v1.x: 高库存判定的周转分母并进特殊渠道(京东/猫超/朴朴)调拨当销售量, 跟"高库存产品明细"/"高库存占比KPI"口径一致。
		//   只动高库存判定(sum_month_sale), 缺货门槛(sum_month 原值)和金额周转(sku_daily_cost)不变。详见 planSpecialAllotQtySubSQL。
		//   调拨 2 个 ? 在最前(JOIN 在内层 WHERE 之前), 故 stockSnapDate 出现 3 次。
		cateArgs := append([]interface{}{stockSnapDate, stockSnapDate, stockSnapDate}, planWhArgs...)
		cateArgs = append(cateArgs, warehouseArgsS...)
		// v1.02: 品类库存健康度 高库存占比/缺货率 改 SKU 维度跨仓汇总 + 缺货统计排除非活跃标签
		// 子查询: 按 (品类, goods_no) 先聚合 → 外层按品类汇总
		// 高库存: SUM(可用)/SUM((月销+特殊渠道调拨)/30)>50 全仓视角; 缺货门槛仍用纯吉客云月销 sum_month
		// 缺货统计排除 flag_data IN ('非卖品','已下架','下架中','接单产','新品-接单产')
		rows, ok := queryRows(`
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
				WHERE s.snapshot_date=? AND s.goods_attr=1 AND s.warehouse_name!=''`+planWhCondS+warehouseCondS+`
				GROUP BY s.goods_no, category
			) t
			GROUP BY category
			HAVING SUM(sku_stock_value) > 0
				AND category IN ('调味料','酱油','调味汁','干制面','素蚝油','酱类','醋','汤底','番茄沙司','糖')
			ORDER BY SUM(sku_stock_value) DESC`, cateArgs...)
		if !ok {
			return
		}
		defer rows.Close()
		muCate.Lock()
		defer muCate.Unlock()
		for rows.Next() {
			var d CateHealth
			var highVal float64
			var soCnt, sCnt int
			if err := rows.Scan(&d.Category, &d.StockValue, &d.DailySalesCost, &highVal, &soCnt, &sCnt); err != nil {
				setQueryErr(err)
				return
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
			categories = append(categories, d)
		}
		if err := rows.Err(); err != nil {
			setQueryErr(err)
		}
	}()

	// ========== 5. 高库存 + 6. 缺货 (v1.02 SKU 维度跨仓汇总) ==========
	type StockDetail struct {
		GoodsNo    string  `json:"goodsNo"`
		GoodsName  string  `json:"goodsName"`
		UsableQty  float64 `json:"usableQty"`
		DailySales float64 `json:"dailySales"`
		Turnover   float64 `json:"turnover"`
		StockValue float64 `json:"stockValue"`
	}
	highStockItems := []StockDetail{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		// v1.x: 周转分母并进"特殊渠道(京东/猫超/朴朴)调拨当销售"量(吉客云 month_qty 不含调拨),
		//   否则靠调拨走量的畅销货会被误判高库存(实测素蚝油360g周转4624天→并进后49天)。详见 planSpecialAllotQtySubSQL。
		//   2 个调拨 ? 在最前(JOIN 在 WHERE 之前), 故 stockSnapDate 出现 3 次。
		cateCondHI, _ := planCategoryGoodsCond("stock_quantity_daily")
		highItemArgs := append([]interface{}{stockSnapDate, stockSnapDate, stockSnapDate}, planWhArgs...)
		highItemArgs = append(highItemArgs, warehouseArgs...)
		highItemArgs = append(highItemArgs, cateArgs...)
		// v1.02: 按 SKU 跨仓汇总, 一个 SKU 一行 (周转 = SUM 可用 / SUM(月销量+特殊渠道调拨)/30, 全仓视角)
		rows, ok := queryRows(`
			SELECT stock_quantity_daily.goods_no, MAX(goods_name),
				ROUND(SUM(current_qty - locked_qty),0),
				ROUND((SUM(month_qty)+IFNULL(MAX(sca.allot_qty),0))/30,1),
				ROUND(SUM(current_qty - locked_qty) / NULLIF((SUM(month_qty)+IFNULL(MAX(sca.allot_qty),0))/30,0),1),
				ROUND(SUM(current_qty * cost_price),2)
			FROM stock_quantity_daily
			LEFT JOIN `+planSpecialAllotQtySubSQL+` sca ON sca.goods_no = stock_quantity_daily.goods_no
			WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+planWhCond+warehouseCond+cateCondHI+`
			GROUP BY stock_quantity_daily.goods_no
			HAVING (SUM(month_qty)+IFNULL(MAX(sca.allot_qty),0)) > 0
				AND SUM(current_qty - locked_qty) / ((SUM(month_qty)+IFNULL(MAX(sca.allot_qty),0))/30) > 50
			ORDER BY SUM(current_qty * cost_price) DESC LIMIT 100`, highItemArgs...)
		if !ok {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var d StockDetail
			if err := rows.Scan(&d.GoodsNo, &d.GoodsName, &d.UsableQty, &d.DailySales, &d.Turnover, &d.StockValue); err != nil {
				setQueryErr(err)
				return
			}
			highStockItems = append(highStockItems, d)
		}
		if err := rows.Err(); err != nil {
			setQueryErr(err)
		}
	}()

	type StockoutDetail struct {
		GoodsNo    string  `json:"goodsNo"`
		GoodsName  string  `json:"goodsName"`
		DailySales float64 `json:"dailySales"`
		DailyValue float64 `json:"dailyValue"`
	}
	stockoutItems := []StockoutDetail{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		// v1.02: 排除非卖品/已下架/下架中/接单产/新品-接单产 标签的 SKU
		flagExcludeCondItem, flagExcludeArgsItem := planStockoutExcludeFlagsCond("")
		stockoutItemArgs := append([]interface{}{stockSnapDate}, planWhArgs...)
		stockoutItemArgs = append(stockoutItemArgs, warehouseArgs...)
		stockoutItemArgs = append(stockoutItemArgs, cateArgs...)
		stockoutItemArgs = append(stockoutItemArgs, flagExcludeArgsItem...)
		// 按 SKU 跨仓汇总缺货, 全仓没货且有销量才算缺货
		// 日均损失用 SUM(month_qty * cost_price)/30 加权计算, 不依赖 cost_price 跨仓一致
		rows, ok := queryRows(`
			SELECT goods_no, MAX(goods_name),
				ROUND(SUM(month_qty)/30,1),
				ROUND(SUM(month_qty * cost_price)/30,2)
			FROM stock_quantity_daily
			WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+planWhCond+warehouseCond+cateCond+flagExcludeCondItem+`
			GROUP BY goods_no
			HAVING SUM(current_qty - locked_qty) <= 0 AND SUM(month_qty) > 0
			ORDER BY SUM(month_qty) DESC LIMIT 100`, stockoutItemArgs...)
		if !ok {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var d StockoutDetail
			if err := rows.Scan(&d.GoodsNo, &d.GoodsName, &d.DailySales, &d.DailyValue); err != nil {
				setQueryErr(err)
				return
			}
			stockoutItems = append(stockoutItems, d)
		}
		if err := rows.Err(); err != nil {
			setQueryErr(err)
		}
	}()

	// ========== 7/8. 销售TOP20 ==========
	type TopProduct struct {
		GoodsNo   string  `json:"goodsNo"`
		GoodsName string  `json:"goodsName"`
		Category  string  `json:"category"`
		Grade     string  `json:"grade"`
		Sales     float64 `json:"sales"`
		Qty       float64 `json:"qty"`
	}
	topProducts := []TopProduct{}
	topQtyProducts := []TopProduct{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		topSalesArgs := append([]interface{}{start, end}, planWhArgs...)
		topSalesArgs = append(topSalesArgs, salesScopeArgs...)
		topSalesArgs = append(topSalesArgs, cateArgs...)
		rows, ok := queryRows(`
			SELECT t.goods_no, t.goods_name, IFNULL(g.cate_full_name,''), IFNULL(g.goods_field7,''), t.sales, t.qty
			FROM (SELECT goods_no, MAX(goods_name) AS goods_name,
				ROUND(SUM(local_goods_amt),2) AS sales, ROUND(SUM(goods_qty),0) AS qty
				FROM sales_goods_summary FORCE INDEX (idx_date_goods_amt) WHERE stat_date BETWEEN ? AND ?`+planWhCond+salesScopeCond+cateCond+`
				GROUP BY goods_no ORDER BY sales DESC LIMIT 20) t
			LEFT JOIN (SELECT goods_no, MAX(cate_full_name) AS cate_full_name, MAX(goods_field7) AS goods_field7
				FROM goods WHERE is_delete=0 GROUP BY goods_no) g ON t.goods_no = g.goods_no`, topSalesArgs...)
		if !ok {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var d TopProduct
			if err := rows.Scan(&d.GoodsNo, &d.GoodsName, &d.Category, &d.Grade, &d.Sales, &d.Qty); err != nil {
				setQueryErr(err)
				return
			}
			topProducts = append(topProducts, d)
		}
		if err := rows.Err(); err != nil {
			setQueryErr(err)
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		topQtyArgs := append([]interface{}{start, end}, planWhArgs...)
		topQtyArgs = append(topQtyArgs, salesScopeArgs...)
		topQtyArgs = append(topQtyArgs, cateArgs...)
		rows, ok := queryRows(`
			SELECT t.goods_no, t.goods_name, IFNULL(g.cate_full_name,''), IFNULL(g.goods_field7,''), t.qty, t.sales
			FROM (SELECT goods_no, MAX(goods_name) AS goods_name,
				ROUND(SUM(goods_qty),0) AS qty, ROUND(SUM(local_goods_amt),2) AS sales
				FROM sales_goods_summary FORCE INDEX (idx_date_goods_amt) WHERE stat_date BETWEEN ? AND ?`+planWhCond+salesScopeCond+cateCond+`
				GROUP BY goods_no ORDER BY qty DESC LIMIT 20) t
			LEFT JOIN (SELECT goods_no, MAX(cate_full_name) AS cate_full_name, MAX(goods_field7) AS goods_field7
				FROM goods WHERE is_delete=0 GROUP BY goods_no) g ON t.goods_no = g.goods_no`, topQtyArgs...)
		if !ok {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var d TopProduct
			if err := rows.Scan(&d.GoodsNo, &d.GoodsName, &d.Category, &d.Grade, &d.Qty, &d.Sales); err != nil {
				setQueryErr(err)
				return
			}
			topQtyProducts = append(topQtyProducts, d)
		}
		if err := rows.Err(); err != nil {
			setQueryErr(err)
		}
	}()

	// ========== 9. 品类销售占比 ==========
	type CateSales struct {
		Category string  `json:"category"`
		Sales    float64 `json:"sales"`
		Profit   float64 `json:"profit"`
	}
	cateSalesList := []CateSales{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		cateSalesArgs := append([]interface{}{start, end}, planWhArgs...)
		cateSalesArgs = append(cateSalesArgs, salesScopeArgs...)
		cateSalesArgs = append(cateSalesArgs, cateArgs...)
		rows, ok := queryRows(`
			SELECT
				CASE WHEN cate_name LIKE '成品/%' THEN SUBSTRING_INDEX(SUBSTRING_INDEX(cate_name,'/',2),'/',-1)
					WHEN cate_name IS NOT NULL AND cate_name != '' THEN cate_name ELSE '未分类' END AS category,
				ROUND(SUM(local_goods_amt),2), ROUND(SUM(gross_profit),2)
			FROM sales_goods_summary FORCE INDEX (idx_date_amt) WHERE stat_date BETWEEN ? AND ?`+planWhCond+salesScopeCond+cateCond+`
			GROUP BY category HAVING SUM(local_goods_amt) > 0
			ORDER BY SUM(local_goods_amt) DESC`, cateSalesArgs...)
		if !ok {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var d CateSales
			if err := rows.Scan(&d.Category, &d.Sales, &d.Profit); err != nil {
				setQueryErr(err)
				return
			}
			cateSalesList = append(cateSalesList, d)
		}
		if err := rows.Err(); err != nil {
			setQueryErr(err)
		}
	}()

	// ========== 10. 库龄>90天 ==========
	type AgedDetail struct {
		GoodsNo        string  `json:"goodsNo"`
		GoodsName      string  `json:"goodsName"`
		Warehouse      string  `json:"warehouse"`
		Qty            float64 `json:"qty"`
		StockValue     float64 `json:"stockValue"`
		BatchNo        string  `json:"batchNo"`
		ProductionDate string  `json:"productionDate"`
		AgeDays        int     `json:"ageDays"`
	}
	agedItems := []AgedDetail{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		agedItemArgs := append([]interface{}{end, stockSnapDate, end}, planWhArgs...)
		agedItemArgs = append(agedItemArgs, warehouseArgsB...)
		// v1.02: 库龄明细加 10 品类过滤 (b.goods_no)
		cateCondAgedB, cateArgsAgedB := planCategoryGoodsCond("b")
		agedItemArgs = append(agedItemArgs, cateArgsAgedB...)
		rows, ok := queryRows(`
			SELECT b.goods_no, b.goods_name, b.warehouse_name,
				ROUND(b.current_qty,0), ROUND(b.current_qty * IFNULL(s.cost_price,0),2),
				b.batch_no, b.production_date, DATEDIFF(?, b.production_date)
			FROM stock_batch_daily b
			LEFT JOIN stock_quantity_daily s ON s.snapshot_date = b.snapshot_date AND b.sku_id = s.sku_id AND b.warehouse_id = s.warehouse_id
			WHERE b.snapshot_date=? AND b.production_date IS NOT NULL AND b.current_qty > 0
				AND b.production_date < DATE_SUB(?, INTERVAL 90 DAY)`+planWhCondB+warehouseCondB+cateCondAgedB+`
			ORDER BY b.current_qty * IFNULL(s.cost_price,0) DESC LIMIT 100`, agedItemArgs...)
		if !ok {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var d AgedDetail
			if err := rows.Scan(&d.GoodsNo, &d.GoodsName, &d.Warehouse, &d.Qty, &d.StockValue, &d.BatchNo, &d.ProductionDate, &d.AgeDays); err != nil {
				setQueryErr(err)
				return
			}
			agedItems = append(agedItems, d)
		}
		if err := rows.Err(); err != nil {
			setQueryErr(err)
		}
	}()

	// ========== 11. 仓库列表 ==========
	whList := []string{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		whListArgs := append([]interface{}{stockSnapDate}, planWhArgs...)
		whListArgs = append(whListArgs, warehouseArgs...)
		whListArgs = append(whListArgs, cateArgs...)
		rows, ok := queryRows(`SELECT DISTINCT warehouse_name FROM stock_quantity_daily WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+planWhCond+warehouseCond+cateCond+` AND (current_qty>0 OR month_qty>0) ORDER BY warehouse_name`, whListArgs...)
		if !ok {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var wh string
			if err := rows.Scan(&wh); err != nil {
				setQueryErr(err)
				return
			}
			whList = append(whList, wh)
		}
		if err := rows.Err(); err != nil {
			setQueryErr(err)
		}
	}()

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

	// v1.x: 钱侧并入特殊渠道调拨当销售金额。GMV 纯加法; 品类饼按品类并(保证同页 KPI/渠道split/饼 合计一致)。
	//   月度趋势走独立端点 GetSupplyChainMonthlyTrend(前端 trendData 用那个), 在那边单独并, 不在此 embedded monthlySales。
	salesGMV += planAllot.Total
	for i := range cateSalesList {
		cateSalesList[i].Sales += planAllot.ByCategory[cateSalesList[i].Category]
	}
	// 补: 有调拨但本期基础销售=0 的品类(HAVING>0 滤掉了), 否则饼合计 < GMV
	seenCat := map[string]bool{}
	for _, c := range cateSalesList {
		seenCat[c.Category] = true
	}
	for cat, amt := range planAllot.ByCategory {
		if amt != 0 && !seenCat[cat] {
			cateSalesList = append(cateSalesList, CateSales{Category: cat, Sales: amt})
		}
	}
	// 区间天数(给渠道 split 的日均重算)
	allotDays := 1
	if sT, e1 := time.Parse("2006-01-02", start); e1 == nil {
		if eT, e2 := time.Parse("2006-01-02", end); e2 == nil {
			if dd := int(eT.Sub(sT).Hours()/24) + 1; dd > 1 {
				allotDays = dd
			}
		}
	}

	// 组装渠道环比数据
	channels := []ChannelData{}
	for _, key := range channelOrder {
		if v, ok := channelMap.Load(key); ok {
			d := v.(*ChannelData)
			if lm, ok := lmMap.Load(key); ok {
				d.LastMonth = lm.(float64)
			}
			if ly, ok := lyMap.Load(key); ok {
				d.LastYear = ly.(float64)
			}
			// v1.x: 并入该部门调拨当销售(本期/上月/去年同窗口), 保证 split 合计=GMV 且 MoM/YoY 公平对比
			if a := planAllot.ByDept[key]; a != 0 {
				d.Total += a
				d.DailyAvg = d.Total / float64(allotDays)
			}
			d.LastMonth += planAllotLM.ByDept[key]
			d.LastYear += planAllotLY.ByDept[key]
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
	for dept, amt := range planAllot.ByDept {
		if amt != 0 && !seenDept[dept] {
			nd := ChannelData{Channel: dept, Total: amt, DailyAvg: amt / float64(allotDays)}
			nd.LastMonth = planAllotLM.ByDept[dept]
			nd.LastYear = planAllotLY.ByDept[dept]
			if nd.LastMonth > 0 {
				nd.MomRate = (nd.Total/nd.LastMonth - 1) * 100
			}
			if nd.LastYear > 0 {
				nd.YoyRate = (nd.Total/nd.LastYear - 1) * 100
			}
			channels = append(channels, nd)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code": 200,
		"data": map[string]interface{}{
			"kpi": map[string]interface{}{
				"salesGMV":       salesGMV,                  // 总销售额(销售出库 + 特殊渠道调拨当销售)
				"allotGMV":       planAllot.Total,           // 其中: 特殊渠道(京东/猫超/朴朴)调拨当销售部分(前端拆分显示)
				"stockCost":      stockCost,
				"turnoverDays":   turnoverDays,
				"highStockRate":  highStockRate,
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
