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
	args := []interface{}{startMonth, endMonth}
	for _, wh := range planWarehouses {
		args = append(args, wh)
	}
	args = append(args, salesScopeArgs...)

	rows, err := h.DB.Query(`
		SELECT stat_month AS m, ROUND(SUM(local_goods_amt),2)
		FROM sales_goods_summary_monthly
		WHERE stat_month BETWEEN ? AND ?
		AND warehouse_name IN (?,?,?,?,?,?,?)`+salesScopeCond+`
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

	// 计划看板仓库白名单（采购需求：只展示这7个成品仓/委外仓/云仓的数据）
	// 白名单定义在文件顶部 planWarehouses
	planWhArgs := make([]interface{}, len(planWarehouses))
	for i, w := range planWarehouses {
		planWhArgs[i] = w
	}
	planWhCond := " AND warehouse_name IN (?,?,?,?,?,?,?)"
	planWhCondS := " AND s.warehouse_name IN (?,?,?,?,?,?,?)"
	planWhCondB := " AND b.warehouse_name IN (?,?,?,?,?,?,?)"

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
	var salesGMV, stockCost, dailyCost, turnoverDays, agedStockValue float64
	var highStockValue, totalStockValue, highStockRate, stockoutRate float64
	var stockoutSKU, salesSKU int

	// v1.02: 6 KPI 全部加 10 品类白名单过滤, 跟"品类库存健康度"口径统一
	cateCond, cateArgs := planCategoryGoodsCond("")

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

	wg.Add(1)
	go func() {
		defer wg.Done()
		// v1.02: 缺货率改 SKU 维度跨仓汇总 (跟"缺货产品明细"口径一致)
		// + 排除非卖品/已下架/下架中/接单产/新品-接单产 标签的 SKU
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
		// v1.02: 高库存占比改 SKU 维度跨仓汇总 (跟"高库存产品明细"口径一致)
		// 子查询先按 SKU 汇总 → 外层判断"全仓周转>50天"才算高库存
		highStockArgs := append([]interface{}{stockSnapDate}, planWhArgs...)
		highStockArgs = append(highStockArgs, warehouseArgs...)
		highStockArgs = append(highStockArgs, cateArgs...)
		if err := h.DB.QueryRow(`SELECT
			IFNULL(SUM(CASE WHEN sum_month>0 AND sum_avail/(sum_month/30) > 50 THEN sku_stock_value ELSE 0 END),0),
			IFNULL(SUM(sku_stock_value),0)
			FROM (
				SELECT goods_no,
					SUM(current_qty - locked_qty) AS sum_avail,
					SUM(month_qty) AS sum_month,
					SUM(current_qty * cost_price) AS sku_stock_value
				FROM stock_quantity_daily
				WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+planWhCond+warehouseCond+cateCond+`
				GROUP BY goods_no
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
		monthlyArgs := append([]interface{}{}, planWhArgs...)
		monthlyArgs = append(monthlyArgs, salesScopeArgs...)
		monthlyArgs = append(monthlyArgs, cateArgs...)
		rows, ok := queryRows(`
			SELECT DATE_FORMAT(stat_date,'%Y-%m') AS m, ROUND(SUM(local_goods_amt),2)
			FROM sales_goods_summary
			WHERE 1=1`+planWhCond+salesScopeCond+cateCond+`
			GROUP BY m ORDER BY m`, monthlyArgs...)
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
		cateArgs := append([]interface{}{stockSnapDate}, planWhArgs...)
		cateArgs = append(cateArgs, warehouseArgsS...)
		// v1.02: 品类库存健康度 高库存占比/缺货率 改 SKU 维度跨仓汇总 + 缺货统计排除非活跃标签
		// 子查询: 按 (品类, goods_no) 先聚合 → 外层按品类汇总
		// 高库存: SUM(可用)/SUM(月销/30)>50 全仓视角
		// 缺货统计排除 flag_data IN ('非卖品','已下架','下架中','接单产','新品-接单产')
		rows, ok := queryRows(`
			SELECT
				category,
				ROUND(SUM(sku_stock_value),2),
				ROUND(SUM(sku_daily_cost),2),
				SUM(CASE WHEN sum_month>0 AND sum_avail/(sum_month/30)>50 THEN sku_stock_value ELSE 0 END),
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
					SUM(s.current_qty * s.cost_price) AS sku_stock_value,
					SUM(s.month_qty * s.cost_price / 30) AS sku_daily_cost,
					CASE WHEN MAX(g.flag_data) IN ('非卖品','已下架','下架中','接单产','新品-接单产') THEN 0 ELSE 1 END AS is_active_sku
				FROM stock_quantity_daily s
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
		highItemArgs := append([]interface{}{stockSnapDate}, planWhArgs...)
		highItemArgs = append(highItemArgs, warehouseArgs...)
		highItemArgs = append(highItemArgs, cateArgs...)
		// v1.02: 按 SKU 跨仓汇总, 一个 SKU 一行 (周转 = SUM 可用 / SUM 月销量/30, 全仓视角)
		rows, ok := queryRows(`
			SELECT goods_no, MAX(goods_name),
				ROUND(SUM(current_qty - locked_qty),0),
				ROUND(SUM(month_qty)/30,1),
				ROUND(SUM(current_qty - locked_qty) / NULLIF(SUM(month_qty)/30,0),1),
				ROUND(SUM(current_qty * cost_price),2)
			FROM stock_quantity_daily
			WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+planWhCond+warehouseCond+cateCond+`
			GROUP BY goods_no
			HAVING SUM(month_qty) > 0
				AND SUM(current_qty - locked_qty) / (SUM(month_qty)/30) > 50
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
	if totalStockValue > 0 {
		highStockRate = highStockValue / totalStockValue * 100
	}
	if dailyCost > 0 {
		turnoverDays = stockCost / dailyCost
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
			if d.LastMonth > 0 {
				d.MomRate = (d.Total/d.LastMonth - 1) * 100
			}
			if d.LastYear > 0 {
				d.YoyRate = (d.Total/d.LastYear - 1) * 100
			}
			channels = append(channels, *d)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code": 200,
		"data": map[string]interface{}{
			"kpi": map[string]interface{}{
				"salesGMV":       salesGMV,
				"stockCost":      stockCost,
				"turnoverDays":   turnoverDays,
				"highStockRate":  highStockRate,
				"stockoutRate":   stockoutRate,
				"stockoutSKU":    stockoutSKU,
				"salesSKU":       salesSKU,
				"agedStockValue": agedStockValue,
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
