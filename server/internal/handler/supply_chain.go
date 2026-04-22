package handler

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

// GetSupplyChainMonthlyTrend 月度销售趋势(独立接口，支持月份范围筛选)
// 参数: start_month=2025-01  end_month=2026-04 (yyyy-MM)
// 默认: 最近15个月
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

	planWarehouses := []string{
		"南京委外成品仓-公司仓-委外",
		"天津委外仓-公司仓-外仓",
		"西安仓库成品-公司仓-外仓",
		"松鲜鲜&大地密码云仓",
		"长沙委外成品仓-公司仓-外仓",
		"安徽郎溪成品-公司仓-自营",
		"南京分销虚拟仓-公司仓-外仓",
	}
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
			writeError(w, 500, err.Error())
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
	planWarehouses := []string{
		"南京委外成品仓-公司仓-委外",
		"天津委外仓-公司仓-外仓",
		"西安仓库成品-公司仓-外仓",
		"松鲜鲜&大地密码云仓",
		"长沙委外成品仓-公司仓-外仓",
		"安徽郎溪成品-公司仓-自营",
		"南京分销虚拟仓-公司仓-外仓",
	}
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

	wg.Add(1)
	go func() {
		defer wg.Done()
		salesGMVArgs := append([]interface{}{start, end}, planWhArgs...)
		salesGMVArgs = append(salesGMVArgs, salesScopeArgs...)
		if err := h.DB.QueryRow(`SELECT IFNULL(SUM(local_goods_amt),0) FROM sales_goods_summary WHERE stat_date BETWEEN ? AND ?`+planWhCond+salesScopeCond, salesGMVArgs...).Scan(&salesGMV); err != nil {
			setQueryErr(err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		stockCostArgs := append([]interface{}{stockSnapDate}, planWhArgs...)
		stockCostArgs = append(stockCostArgs, warehouseArgs...)
		if err := h.DB.QueryRow(
			`SELECT IFNULL(SUM(current_qty * cost_price),0), IFNULL(SUM(month_qty * cost_price / 30),0)
			 FROM stock_quantity_daily WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+planWhCond+warehouseCond,
			stockCostArgs...).Scan(&stockCost, &dailyCost); err != nil {
			setQueryErr(err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		stockoutArgs := append([]interface{}{stockSnapDate}, planWhArgs...)
		stockoutArgs = append(stockoutArgs, warehouseArgs...)
		if err := h.DB.QueryRow(`SELECT
			IFNULL(SUM(CASE WHEN current_qty-locked_qty<=0 AND month_qty>0 THEN 1 ELSE 0 END),0),
			IFNULL(SUM(CASE WHEN month_qty>0 THEN 1 ELSE 0 END),0)
			FROM stock_quantity_daily WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+planWhCond+warehouseCond, stockoutArgs...).Scan(&stockoutSKU, &salesSKU); err != nil {
			setQueryErr(err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		highStockArgs := append([]interface{}{stockSnapDate}, planWhArgs...)
		highStockArgs = append(highStockArgs, warehouseArgs...)
		if err := h.DB.QueryRow(`SELECT
			IFNULL(SUM(CASE WHEN month_qty>0 AND (current_qty-locked_qty)/(month_qty/30)>50 THEN current_qty*cost_price ELSE 0 END),0),
			IFNULL(SUM(current_qty*cost_price),0)
			FROM stock_quantity_daily WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+planWhCond+warehouseCond, highStockArgs...).Scan(&highStockValue, &totalStockValue); err != nil {
			setQueryErr(err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		agedArgs := append([]interface{}{stockSnapDate, end}, planWhArgs...)
		agedArgs = append(agedArgs, warehouseArgsB...)
		if err := h.DB.QueryRow(`SELECT IFNULL(SUM(b.current_qty * IFNULL(s.cost_price,0)),0)
			FROM stock_batch_daily b
			LEFT JOIN stock_quantity_daily s ON s.snapshot_date = b.snapshot_date AND b.sku_id = s.sku_id AND b.warehouse_id = s.warehouse_id
			WHERE b.snapshot_date=? AND b.production_date IS NOT NULL AND b.current_qty > 0
			AND b.production_date < DATE_SUB(?, INTERVAL 90 DAY)`+planWhCondB+warehouseCondB, agedArgs...).Scan(&agedStockValue); err != nil {
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
		rows, ok := queryRows(`
			SELECT DATE_FORMAT(stat_date,'%Y-%m') AS m, ROUND(SUM(local_goods_amt),2)
			FROM sales_goods_summary
			WHERE 1=1`+planWhCond+salesScopeCond+`
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
		rows, ok := queryRows(`
			SELECT CASE WHEN department IS NULL OR department='' THEN 'other' ELSE department END,
				ROUND(SUM(local_goods_amt)/GREATEST(DATEDIFF(?,?)+1,1),2),
				ROUND(SUM(local_goods_amt),2)
			FROM sales_goods_summary WHERE stat_date BETWEEN ? AND ?`+planWhCond+salesScopeCond+`
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
		rows, ok := queryRows(`
			SELECT CASE WHEN department IS NULL OR department='' THEN 'other' ELSE department END, ROUND(SUM(local_goods_amt),2)
			FROM sales_goods_summary
			WHERE stat_date BETWEEN DATE_SUB(?, INTERVAL 1 MONTH) AND DATE_SUB(?, INTERVAL 1 MONTH)`+planWhCond+salesScopeCond+`
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
		rows, ok := queryRows(`
			SELECT CASE WHEN department IS NULL OR department='' THEN 'other' ELSE department END, ROUND(SUM(local_goods_amt),2)
			FROM sales_goods_summary
			WHERE stat_date BETWEEN DATE_SUB(?, INTERVAL 1 YEAR) AND DATE_SUB(?, INTERVAL 1 YEAR)`+planWhCond+salesScopeCond+`
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
		rows, ok := queryRows(`
			SELECT
				CASE
					WHEN g.cate_full_name LIKE '成品/%' THEN SUBSTRING_INDEX(SUBSTRING_INDEX(g.cate_full_name,'/',2),'/',-1)
					WHEN g.cate_full_name IS NOT NULL AND g.cate_full_name != '' THEN g.cate_full_name
					ELSE '未分类'
				END AS category,
				ROUND(SUM(s.current_qty * s.cost_price),2),
				ROUND(SUM(s.month_qty * s.cost_price / 30),2),
				SUM(CASE WHEN s.month_qty>0 AND (s.current_qty-s.locked_qty)/(s.month_qty/30)>50 THEN s.current_qty*s.cost_price ELSE 0 END),
				SUM(CASE WHEN s.current_qty-s.locked_qty<=0 AND s.month_qty>0 THEN 1 ELSE 0 END),
				SUM(CASE WHEN s.month_qty>0 THEN 1 ELSE 0 END)
			FROM stock_quantity_daily s
			LEFT JOIN goods g ON s.goods_no = g.goods_no AND g.is_delete=0
			WHERE s.snapshot_date=? AND s.goods_attr=1 AND s.warehouse_name!=''`+planWhCondS+warehouseCondS+`
			GROUP BY category
			HAVING SUM(s.current_qty * s.cost_price) > 0
				AND category IN ('调味料','酱油','调味汁','干制面','素蚝油','酱类','醋','汤底','番茄沙司','糖')
			ORDER BY SUM(s.current_qty * s.cost_price) DESC`, cateArgs...)
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

	// ========== 5. 高库存 + 6. 缺货 ==========
	type StockDetail struct {
		GoodsNo    string  `json:"goodsNo"`
		GoodsName  string  `json:"goodsName"`
		Warehouse  string  `json:"warehouse"`
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
		rows, ok := queryRows(`
			SELECT goods_no, goods_name, warehouse_name,
				ROUND(current_qty - locked_qty,0), ROUND(month_qty/30,1),
				ROUND((current_qty-locked_qty)/(month_qty/30),1),
				ROUND(current_qty * cost_price,2)
			FROM stock_quantity_daily
			WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+planWhCond+warehouseCond+` AND month_qty>0
				AND (current_qty-locked_qty)/(month_qty/30) > 50
			ORDER BY current_qty * cost_price DESC LIMIT 100`, highItemArgs...)
		if !ok {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var d StockDetail
			if err := rows.Scan(&d.GoodsNo, &d.GoodsName, &d.Warehouse, &d.UsableQty, &d.DailySales, &d.Turnover, &d.StockValue); err != nil {
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
		Warehouse  string  `json:"warehouse"`
		DailySales float64 `json:"dailySales"`
		DailyValue float64 `json:"dailyValue"`
	}
	stockoutItems := []StockoutDetail{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		stockoutItemArgs := append([]interface{}{stockSnapDate}, planWhArgs...)
		stockoutItemArgs = append(stockoutItemArgs, warehouseArgs...)
		rows, ok := queryRows(`
			SELECT goods_no, goods_name, warehouse_name,
				ROUND(month_qty/30,1), ROUND(month_qty/30 * cost_price,2)
			FROM stock_quantity_daily
			WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+planWhCond+warehouseCond+` AND current_qty - locked_qty <= 0 AND month_qty > 0
			ORDER BY month_qty DESC LIMIT 100`, stockoutItemArgs...)
		if !ok {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var d StockoutDetail
			if err := rows.Scan(&d.GoodsNo, &d.GoodsName, &d.Warehouse, &d.DailySales, &d.DailyValue); err != nil {
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
		rows, ok := queryRows(`
			SELECT t.goods_no, t.goods_name, IFNULL(g.cate_full_name,''), IFNULL(g.goods_field7,''), t.sales, t.qty
			FROM (SELECT goods_no, MAX(goods_name) AS goods_name,
				ROUND(SUM(local_goods_amt),2) AS sales, ROUND(SUM(goods_qty),0) AS qty
				FROM sales_goods_summary FORCE INDEX (idx_date_goods_amt) WHERE stat_date BETWEEN ? AND ?`+planWhCond+salesScopeCond+`
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
		rows, ok := queryRows(`
			SELECT t.goods_no, t.goods_name, IFNULL(g.cate_full_name,''), IFNULL(g.goods_field7,''), t.qty, t.sales
			FROM (SELECT goods_no, MAX(goods_name) AS goods_name,
				ROUND(SUM(goods_qty),0) AS qty, ROUND(SUM(local_goods_amt),2) AS sales
				FROM sales_goods_summary FORCE INDEX (idx_date_goods_amt) WHERE stat_date BETWEEN ? AND ?`+planWhCond+salesScopeCond+`
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
		rows, ok := queryRows(`
			SELECT
				CASE WHEN cate_name LIKE '成品/%' THEN SUBSTRING_INDEX(SUBSTRING_INDEX(cate_name,'/',2),'/',-1)
					WHEN cate_name IS NOT NULL AND cate_name != '' THEN cate_name ELSE '未分类' END AS category,
				ROUND(SUM(local_goods_amt),2), ROUND(SUM(gross_profit),2)
			FROM sales_goods_summary FORCE INDEX (idx_date_amt) WHERE stat_date BETWEEN ? AND ?`+planWhCond+salesScopeCond+`
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
		rows, ok := queryRows(`
			SELECT b.goods_no, b.goods_name, b.warehouse_name,
				ROUND(b.current_qty,0), ROUND(b.current_qty * IFNULL(s.cost_price,0),2),
				b.batch_no, b.production_date, DATEDIFF(?, b.production_date)
			FROM stock_batch_daily b
			LEFT JOIN stock_quantity_daily s ON s.snapshot_date = b.snapshot_date AND b.sku_id = s.sku_id AND b.warehouse_id = s.warehouse_id
			WHERE b.snapshot_date=? AND b.production_date IS NOT NULL AND b.current_qty > 0
				AND b.production_date < DATE_SUB(?, INTERVAL 90 DAY)`+planWhCondB+warehouseCondB+`
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
		rows, ok := queryRows(`SELECT DISTINCT warehouse_name FROM stock_quantity_daily WHERE snapshot_date=? AND goods_attr=1 AND warehouse_name!=''`+planWhCond+warehouseCond+` AND (current_qty>0 OR month_qty>0) ORDER BY warehouse_name`, whListArgs...)
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
