package handler

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sync"
)

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
		salesGMVArgs := append([]interface{}{start, end}, salesScopeArgs...)
		if err := h.DB.QueryRow(`SELECT IFNULL(SUM(local_goods_amt),0) FROM sales_goods_summary WHERE stat_date BETWEEN ? AND ?`+salesScopeCond, salesGMVArgs...).Scan(&salesGMV); err != nil {
			setQueryErr(err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		stockCostArgs := append([]interface{}{}, warehouseArgs...)
		if err := h.DB.QueryRow(
			`SELECT IFNULL(SUM(current_qty * cost_price),0), IFNULL(SUM(month_qty * cost_price / 30),0)
			 FROM stock_quantity WHERE goods_attr=1 AND warehouse_name!=''`+warehouseCond,
			stockCostArgs...).Scan(&stockCost, &dailyCost); err != nil {
			setQueryErr(err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := h.DB.QueryRow(`SELECT
			IFNULL(SUM(CASE WHEN current_qty-locked_qty<=0 AND month_qty>0 THEN 1 ELSE 0 END),0),
			IFNULL(SUM(CASE WHEN month_qty>0 THEN 1 ELSE 0 END),0)
			FROM stock_quantity WHERE goods_attr=1 AND warehouse_name!=''`+warehouseCond, warehouseArgs...).Scan(&stockoutSKU, &salesSKU); err != nil {
			setQueryErr(err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := h.DB.QueryRow(`SELECT
			IFNULL(SUM(CASE WHEN month_qty>0 AND (current_qty-locked_qty)/(month_qty/30)>50 THEN current_qty*cost_price ELSE 0 END),0),
			IFNULL(SUM(current_qty*cost_price),0)
			FROM stock_quantity WHERE goods_attr=1 AND warehouse_name!=''`+warehouseCond, warehouseArgs...).Scan(&highStockValue, &totalStockValue); err != nil {
			setQueryErr(err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := h.DB.QueryRow(`SELECT IFNULL(SUM(b.current_qty * IFNULL(s.cost_price,0)),0)
			FROM stock_batch b
			LEFT JOIN stock_quantity s ON b.sku_id = s.sku_id AND b.warehouse_id = s.warehouse_id
			WHERE b.production_date IS NOT NULL AND b.current_qty > 0
			AND b.production_date < DATE_SUB(CURDATE(), INTERVAL 90 DAY)`+warehouseCondB, warehouseArgsB...).Scan(&agedStockValue); err != nil {
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
		monthlyArgs := append([]interface{}{}, salesScopeArgs...)
		rows, ok := queryRows(`
			SELECT DATE_FORMAT(stat_date,'%Y-%m') AS m, ROUND(SUM(local_goods_amt),2)
			FROM sales_goods_summary
			WHERE stat_date >= DATE_FORMAT(DATE_SUB(CURDATE(), INTERVAL 15 MONTH),'%%Y-%%m-01')`+salesScopeCond+`
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
		channelArgs := append([]interface{}{end, start, start, end}, salesScopeArgs...)
		rows, ok := queryRows(`
			SELECT CASE WHEN department IS NULL OR department='' THEN 'other' ELSE department END,
				ROUND(SUM(local_goods_amt)/GREATEST(DATEDIFF(?,?),1),2),
				ROUND(SUM(local_goods_amt),2)
			FROM sales_goods_summary WHERE stat_date BETWEEN ? AND ?`+salesScopeCond+`
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
		lmArgs := append([]interface{}{start, end}, salesScopeArgs...)
		rows, ok := queryRows(`
			SELECT CASE WHEN department IS NULL OR department='' THEN 'other' ELSE department END, ROUND(SUM(local_goods_amt),2)
			FROM sales_goods_summary
			WHERE stat_date BETWEEN DATE_SUB(?, INTERVAL 1 MONTH) AND DATE_SUB(?, INTERVAL 1 MONTH)`+salesScopeCond+`
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
		lyArgs := append([]interface{}{start, end}, salesScopeArgs...)
		rows, ok := queryRows(`
			SELECT CASE WHEN department IS NULL OR department='' THEN 'other' ELSE department END, ROUND(SUM(local_goods_amt),2)
			FROM sales_goods_summary
			WHERE stat_date BETWEEN DATE_SUB(?, INTERVAL 1 YEAR) AND DATE_SUB(?, INTERVAL 1 YEAR)`+salesScopeCond+`
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
			FROM stock_quantity s
			LEFT JOIN goods g ON s.goods_no = g.goods_no AND g.is_delete=0
			WHERE s.goods_attr=1 AND s.warehouse_name!=''`+warehouseCondS+`
			GROUP BY category
			HAVING SUM(s.current_qty * s.cost_price) > 0
				AND category IN ('调味料','酱油','调味汁','干制面','素蚝油','酱类','醋','汤底','番茄沙司','糖')
			ORDER BY SUM(s.current_qty * s.cost_price) DESC`, warehouseArgsS...)
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
		rows, ok := queryRows(`
			SELECT goods_no, goods_name, warehouse_name,
				ROUND(current_qty - locked_qty,0), ROUND(month_qty/30,1),
				ROUND((current_qty-locked_qty)/(month_qty/30),1),
				ROUND(current_qty * cost_price,2)
			FROM stock_quantity
			WHERE goods_attr=1 AND warehouse_name!=''`+warehouseCond+` AND month_qty>0
				AND (current_qty-locked_qty)/(month_qty/30) > 50
			ORDER BY current_qty * cost_price DESC LIMIT 100`, warehouseArgs...)
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
		rows, ok := queryRows(`
			SELECT goods_no, goods_name, warehouse_name,
				ROUND(month_qty/30,1), ROUND(month_qty/30 * cost_price,2)
			FROM stock_quantity
			WHERE goods_attr=1 AND warehouse_name!=''`+warehouseCond+` AND current_qty - locked_qty <= 0 AND month_qty > 0
			ORDER BY month_qty DESC LIMIT 100`, warehouseArgs...)
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
		topSalesArgs := append([]interface{}{start, end}, salesScopeArgs...)
		rows, ok := queryRows(`
			SELECT t.goods_no, t.goods_name, IFNULL(g.cate_full_name,''), IFNULL(g.goods_field7,''), t.sales, t.qty
			FROM (SELECT goods_no, MAX(goods_name) AS goods_name,
				ROUND(SUM(local_goods_amt),2) AS sales, ROUND(SUM(goods_qty),0) AS qty
				FROM sales_goods_summary WHERE stat_date BETWEEN ? AND ?`+salesScopeCond+`
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
		topQtyArgs := append([]interface{}{start, end}, salesScopeArgs...)
		rows, ok := queryRows(`
			SELECT t.goods_no, t.goods_name, IFNULL(g.cate_full_name,''), IFNULL(g.goods_field7,''), t.qty, t.sales
			FROM (SELECT goods_no, MAX(goods_name) AS goods_name,
				ROUND(SUM(goods_qty),0) AS qty, ROUND(SUM(local_goods_amt),2) AS sales
				FROM sales_goods_summary WHERE stat_date BETWEEN ? AND ?`+salesScopeCond+`
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
		cateSalesArgs := append([]interface{}{start, end}, salesScopeArgs...)
		rows, ok := queryRows(`
			SELECT
				CASE WHEN cate_name LIKE '成品/%' THEN SUBSTRING_INDEX(SUBSTRING_INDEX(cate_name,'/',2),'/',-1)
					WHEN cate_name IS NOT NULL AND cate_name != '' THEN cate_name ELSE '未分类' END AS category,
				ROUND(SUM(local_goods_amt),2), ROUND(SUM(gross_profit),2)
			FROM sales_goods_summary WHERE stat_date BETWEEN ? AND ?`+salesScopeCond+`
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
		rows, ok := queryRows(`
			SELECT b.goods_no, b.goods_name, b.warehouse_name,
				ROUND(b.current_qty,0), ROUND(b.current_qty * IFNULL(s.cost_price,0),2),
				b.batch_no, b.production_date, DATEDIFF(CURDATE(), b.production_date)
			FROM stock_batch b
			LEFT JOIN stock_quantity s ON b.sku_id = s.sku_id AND b.warehouse_id = s.warehouse_id
			WHERE b.production_date IS NOT NULL AND b.current_qty > 0
				AND b.production_date < DATE_SUB(CURDATE(), INTERVAL 90 DAY)`+warehouseCondB+`
			ORDER BY b.current_qty * IFNULL(s.cost_price,0) DESC LIMIT 100`, warehouseArgsB...)
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
		rows, ok := queryRows(`SELECT DISTINCT warehouse_name FROM stock_quantity WHERE goods_attr=1 AND warehouse_name!=''`+warehouseCond+` AND (current_qty>0 OR month_qty>0) ORDER BY warehouse_name`, warehouseArgs...)
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
