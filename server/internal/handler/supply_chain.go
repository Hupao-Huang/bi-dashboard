package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// excludeAnhuiOrg v0.52: 跑哥要求所有 YS 数据屏蔽"安徽香松自然调味品有限公司" 组织
// 改这一处即可同步影响所有 YS 表 (ys_stock / ys_material_out / ys_purchase_orders / ys_subcontract_orders) 查询
const excludeAnhuiOrg = "安徽香松自然调味品有限公司"
const excludeAnhuiOrgWHERE = " AND org_name != '安徽香松自然调味品有限公司'"
const excludeAnhuiOrgYsWHERE = " AND ys.org_name != '安徽香松自然调味品有限公司'"

// planWarehouses 计划/采购看板 + 库存预警共用的 7 仓白名单
// 改这一处即可同步影响：计划看板、库存预警等所有"按仓库白名单"过滤的查询
var planWarehouses = []string{
	"南京委外成品仓-公司仓-委外",
	"天津委外仓-公司仓-外仓",
	"西安仓库成品-公司仓-外仓",
	"松鲜鲜&大地密码云仓",
	"长沙委外成品仓-公司仓-外仓",
	"安徽郎溪成品-公司仓-自营",
	"南京分销虚拟仓-公司仓-外仓",
}

// planExcludeGoods 计划/采购看板排除 SKU 名单 (虚拟商品/邮费/差价补拍 等)
// 通用 goods_no 黑名单, 影响 prodSQL + matSQL + otherSQL
// v0.64: 删除 05010493 (广宣品已分流到"其他"Tab, 不再需要全黑名单)
var planExcludeGoods = []string{
	"yflj", // 运费说明及差价补拍链接 邮费
}

// buildExcludeGoodsFilter 返回 " AND <column> NOT IN (?,?,...)" 子句和参数
func buildExcludeGoodsFilter(column string) (string, []interface{}) {
	if len(planExcludeGoods) == 0 {
		return "", nil
	}
	args := make([]interface{}, len(planExcludeGoods))
	placeholders := ""
	for i, g := range planExcludeGoods {
		args[i] = g
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
	}
	return " AND " + column + " NOT IN (" + placeholders + ")", args
}

// buildPlanWarehouseFilter 返回 " AND <column> IN (?,?,...)" 子句和对应参数
func buildPlanWarehouseFilter(column string) (string, []interface{}) {
	args := make([]interface{}, len(planWarehouses))
	placeholders := ""
	for i, w := range planWarehouses {
		args[i] = w
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
	}
	return " AND " + column + " IN (" + placeholders + ")", args
}

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

// GetPurchasePlan 采购计划仪表盘 (v0.48)
// 5 张表 JOIN: stock_quantity + sales (吉客云) + ys_purchase_orders + ys_subcontract_orders + ys_material_out
// 算法:
//   成品 (goods_attr=1) 目标 45 天, 日均 = month_qty/30 (吉客云销售)
//   包材 (goods_attr=4) 目标 90 天, 日均 = ys_material_out 近30天/30 (真实消耗)
//   建议采购量 = max(0, 目标天数 × 日均 - 库存 - 在途采购 - 在途委外)
//   状态: 紧急(可售<7) / 偏低(7-14) / 正常 / 积压(>90)
func (h *DashboardHandler) GetPurchasePlan(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDomainAccess(r, "supply_chain")) {
		return
	}

	// === 1. KPI 4 个 ===
	type kpiData struct {
		UrgentSKU            int     `json:"urgentSku"`
		InTransitOrders      int     `json:"inTransitOrders"`
		InTransitSubcontract int     `json:"inTransitSubcontract"`
		Recent30Amount       float64 `json:"recent30Amount"`
	}
	var kpi kpiData

	// 紧急 SKU 数 (成品可售天数 < 7) — 必须按 SKU 聚合再算, 不能 row-level
	// row-level 会因为同 SKU 散在多仓而把虚高计数, 实际全公司库存充裕也会算紧急
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM (
		SELECT goods_no, SUM(current_qty - locked_qty) AS stock_total, SUM(month_qty)/30 AS daily_avg
		FROM stock_quantity
		WHERE goods_attr=1 AND month_qty > 0
		GROUP BY goods_no
		HAVING stock_total > 0 AND stock_total / daily_avg < 7
	) t`).Scan(&kpi.UrgentSKU); err != nil {
		log.Printf("kpi urgent: %v", err)
	}

	// 在途采购订单数 (v0.52: 排除安徽香松组织)
	h.DB.QueryRow(`SELECT COUNT(DISTINCT id) FROM ys_purchase_orders
		WHERE purchase_orders_in_wh_status IN (2,3)` + excludeAnhuiOrgWHERE).Scan(&kpi.InTransitOrders)

	// 在途委外订单数 (v0.52: 排除安徽香松组织)
	h.DB.QueryRow(`SELECT COUNT(DISTINCT id) FROM ys_subcontract_orders
		WHERE status NOT IN (2)` + excludeAnhuiOrgWHERE).Scan(&kpi.InTransitSubcontract)

	// 最近 30 天采购金额 (相对 DB 内 MAX(vouchdate) 滚动) (v0.52: 排除安徽香松)
	var amt sql.NullFloat64
	h.DB.QueryRow(`SELECT SUM(ori_sum) FROM ys_purchase_orders
		WHERE vouchdate >= DATE_SUB((SELECT MAX(vouchdate) FROM ys_purchase_orders), INTERVAL 30 DAY)` + excludeAnhuiOrgWHERE).Scan(&amt)
	kpi.Recent30Amount = amt.Float64

	// === 2. 月度趋势 (近 6 个月采购金额) ===
	type monthRow struct {
		Month  string  `json:"month"`
		Amount float64 `json:"amount"`
	}
	monthlyTrend := []monthRow{}
	mRows, _ := h.DB.Query(`SELECT DATE_FORMAT(vouchdate, '%Y-%m') AS month,
		ROUND(SUM(ori_sum), 0) AS amount
		FROM ys_purchase_orders
		WHERE vouchdate >= DATE_SUB((SELECT MAX(vouchdate) FROM ys_purchase_orders), INTERVAL 6 MONTH)` + excludeAnhuiOrgWHERE + `
		GROUP BY DATE_FORMAT(vouchdate, '%Y-%m') ORDER BY month`)
	if mRows != nil {
		for mRows.Next() {
			var m monthRow
			if err := mRows.Scan(&m.Month, &m.Amount); err == nil {
				monthlyTrend = append(monthlyTrend, m)
			}
		}
		mRows.Close()
	}

	// === 3. TOP 10 供应商 (按采购金额) ===
	type vendorRow struct {
		VendorName string  `json:"vendorName"`
		Amount     float64 `json:"amount"`
		OrderCount int     `json:"orderCount"`
	}
	topVendors := []vendorRow{}
	vRows, _ := h.DB.Query(`SELECT vendor_name, ROUND(SUM(ori_sum), 0) AS amount,
		COUNT(DISTINCT id) AS order_count
		FROM ys_purchase_orders WHERE vendor_name IS NOT NULL` + excludeAnhuiOrgWHERE + `
		GROUP BY vendor_name ORDER BY amount DESC LIMIT 10`)
	if vRows != nil {
		for vRows.Next() {
			var v vendorRow
			if err := vRows.Scan(&v.VendorName, &v.Amount, &v.OrderCount); err == nil {
				topVendors = append(topVendors, v)
			}
		}
		vRows.Close()
	}

	// === 4. 建议采购清单 (UNION 成品 + 包材, 按建议量倒序) ===
	// v0.51: 在途量按 recieve_date <= today+90天 过滤 (远期/超期排除); 加 nextArriveDate 显示最近到货
	// 编码两套并存: jkyCode + ysCode 通过 goods.sku_code 映射
	// v0.62 改: 成品段限定 7 仓白名单(planWarehouses), 不含京东/天猫超市/朴朴外仓+采购外仓+不合格仓
	//          展示全部 SKU(去掉 HAVING > 0 过滤), 跑哥要核对
	planSqWhCond, planSqWhArgs := buildPlanWarehouseFilter("sq.warehouse_name")
	prodExclCond, prodExclArgs := buildExcludeGoodsFilter("sq.goods_no")
	type suggestRow struct {
		Type                 string  `json:"type"`    // 成品 / 包材
		JkyCode              string  `json:"jkyCode"` // 吉客云编码
		YsCode               string  `json:"ysCode"`  // 用友编码
		GoodsName            string  `json:"goodsName"`
		Stock                float64 `json:"stock"`
		DailyAvg             float64 `json:"dailyAvg"`
		InTransit            float64 `json:"inTransit"`            // 在途采购量
		InTransitSubcontract float64 `json:"inTransitSubcontract"` // v0.54: 在途委外量 (委外加工未完工)
		SuggestedQty         float64 `json:"suggestedQty"`
		Status               string  `json:"status"`         // 紧急 / 偏低 / 正常 / 积压
		SellableDays         float64 `json:"sellableDays"`   // 可售天数
		NextArriveDate       string  `json:"nextArriveDate"` // 最近一笔在途(采购+委外)到货日期
		NextArriveDays       int     `json:"nextArriveDays"` // 距今天数 (负=已逾期, NULL→999)
		YsClassName          string  `json:"ysClassName"`    // YS 分类(固态/液态/标签/纸箱 等)
	}
	suggested := []suggestRow{}

	// 4a. 成品 (goods_attr=1, 目标 45 天) — 主表 stock_quantity, 通过 goods.sku_code (吉客云外部编码) 映射 YS 编码
	// v0.54: 加在途委外量 (sc 子查询) + next_arrive 综合采购+委外两种到货
	// 公式: max(0, 45 × 吉客云日均 - 吉客云库存 - YS 在途采购 - YS 在途委外)
	prodSQL := `SELECT '成品/半成品' AS t,
		sq.goods_no AS jky_code,
		IFNULL(MAX(gm.ys_code), '') AS ys_code,
		sq.goods_name,
		ROUND(SUM(sq.current_qty - sq.locked_qty), 0) AS stock,
		ROUND(SUM(sq.month_qty)/30, 1) AS daily_avg,
		IFNULL(ROUND(MAX(po.in_transit_qty), 0), 0) AS in_transit,
		IFNULL(ROUND(MAX(sc.in_transit_qty), 0), 0) AS in_transit_subcontract,
		COALESCE(NULLIF(MAX(gm.ys_class_name), ''), MAX(ys_direct.direct_class_name), '') AS ys_class_name,
		GREATEST(0, ROUND(45 * SUM(sq.month_qty)/30 - SUM(sq.current_qty - sq.locked_qty) - IFNULL(MAX(po.in_transit_qty), 0) - IFNULL(MAX(sc.in_transit_qty), 0), 0)) AS suggested,
		CASE
		  WHEN SUM(sq.month_qty) > 0 AND (SUM(sq.current_qty - sq.locked_qty)) <= 0 THEN -1
		  WHEN SUM(sq.month_qty) > 0 THEN ROUND(SUM(sq.current_qty - sq.locked_qty) / (SUM(sq.month_qty)/30), 1)
		  ELSE 9999 END AS sellable_days,
		CASE WHEN LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')) = '9999-12-31' THEN ''
		     ELSE DATE_FORMAT(LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')), '%Y-%m-%d') END AS next_arrive_date,
		CASE WHEN LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')) = '9999-12-31' THEN 999
		     ELSE DATEDIFF(LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')), CURDATE()) END AS next_arrive_days
		FROM stock_quantity sq
		LEFT JOIN (
		  -- v0.54 fix: ys_purchase_orders.product_c_code 是 YS 编码, 必须通过 goods.sku_code 桥接到吉客云 goods_no
		  SELECT g.goods_no AS jky_no, SUM(p.qty - IFNULL(p.total_in_qty, 0)) AS in_transit_qty
		  FROM ys_purchase_orders p JOIN goods g ON g.sku_code = p.product_c_code
		  WHERE p.purchase_orders_in_wh_status IN (2,3) AND p.qty > IFNULL(p.total_in_qty, 0)
		    AND (p.recieve_date IS NULL OR p.recieve_date <= DATE_ADD(CURDATE(), INTERVAL 90 DAY))
		    AND p.org_name != '安徽香松自然调味品有限公司'
		  GROUP BY g.goods_no
		) po ON po.jky_no = sq.goods_no
		LEFT JOIN (
		  SELECT g.goods_no AS jky_no, MIN(IFNULL(p.recieve_date, DATE_ADD(p.vouchdate, INTERVAL 30 DAY))) AS next_arrive
		  FROM ys_purchase_orders p JOIN goods g ON g.sku_code = p.product_c_code
		  WHERE p.purchase_orders_in_wh_status IN (2,3) AND p.qty > IFNULL(p.total_in_qty, 0)
		    AND p.org_name != '安徽香松自然调味品有限公司'
		  GROUP BY g.goods_no
		) po_arr ON po_arr.jky_no = sq.goods_no
		LEFT JOIN (
		  SELECT g.goods_no AS jky_no,
		    SUM(s.order_product_subcontract_quantity_mu - IFNULL(s.order_product_incoming_quantity, 0)) AS in_transit_qty
		  FROM ys_subcontract_orders s JOIN goods g ON g.sku_code = s.order_product_material_code
		  WHERE s.status NOT IN (2)
		    AND s.order_product_subcontract_quantity_mu > IFNULL(s.order_product_incoming_quantity, 0)
		    AND (s.order_product_delivery_date IS NULL OR s.order_product_delivery_date <= DATE_ADD(CURDATE(), INTERVAL 90 DAY))
		    AND s.org_name != '安徽香松自然调味品有限公司'
		  GROUP BY g.goods_no
		) sc ON sc.jky_no = sq.goods_no
		LEFT JOIN (
		  SELECT g.goods_no AS jky_no,
		    MIN(IFNULL(s.order_product_delivery_date, DATE_ADD(s.vouchdate, INTERVAL 30 DAY))) AS next_arrive
		  FROM ys_subcontract_orders s JOIN goods g ON g.sku_code = s.order_product_material_code
		  WHERE s.status NOT IN (2)
		    AND s.order_product_subcontract_quantity_mu > IFNULL(s.order_product_incoming_quantity, 0)
		    AND s.org_name != '安徽香松自然调味品有限公司'
		  GROUP BY g.goods_no
		) sc_arr ON sc_arr.jky_no = sq.goods_no
		LEFT JOIN (
		  SELECT g.goods_no,
		    MAX(NULLIF(g.sku_code,'')) AS ys_code,
		    MAX(yc.manage_class_name) AS ys_class_name,
		    MAX(yc.manage_class_code) AS ys_class_code
		  FROM goods g
		  LEFT JOIN (SELECT product_code,
		                    MAX(manage_class_name) AS manage_class_name,
		                    MAX(manage_class_code) AS manage_class_code
		             FROM ys_stock GROUP BY product_code) yc ON yc.product_code = g.sku_code
		  WHERE g.sku_code IS NOT NULL AND g.sku_code != '' GROUP BY g.goods_no
		) gm ON gm.goods_no = sq.goods_no
		LEFT JOIN (
		  -- v0.65 直连兜底: 当 goods.sku_code 缺失时, 用 sq.goods_no 直接对 ys_stock.product_code
		  SELECT product_code,
		    MAX(manage_class_name) AS direct_class_name,
		    MAX(manage_class_code) AS direct_class_code
		  FROM ys_stock GROUP BY product_code
		) ys_direct ON ys_direct.product_code = sq.goods_no
		WHERE sq.goods_attr = 1 AND sq.month_qty > 0
		  AND IFNULL(gm.ys_class_code, '') NOT LIKE '05%'
		  AND IFNULL(ys_direct.direct_class_code, '') NOT LIKE '05%'` + planSqWhCond + prodExclCond + `
		GROUP BY sq.goods_no, sq.goods_name`

	// 4b. 包材/原料 (v0.49 改用 YS 现存量) — 主表 ys_stock, 反向通过 goods.sku_code 映射回吉客云 goods_no
	// v0.54: 加在途委外 (sc) 子查询 + next_arrive 综合采购+委外
	// 公式: max(0, 90 × YS日均 - YS库存 - YS在途采购 - YS在途委外)
	matSQL := `SELECT '原材料/包材' AS t,
		IFNULL(MAX(gm.goods_no), '') AS jky_code,
		ys.product_code AS ys_code,
		MAX(ys.product_name) AS goods_name,
		ROUND(SUM(ys.currentqty), 0) AS stock,
		ROUND(IFNULL(MAX(mo.daily_avg), 0), 1) AS daily_avg,
		IFNULL(ROUND(MAX(po.in_transit_qty), 0), 0) AS in_transit,
		IFNULL(ROUND(MAX(sc.in_transit_qty), 0), 0) AS in_transit_subcontract,
		IFNULL(MAX(ys.manage_class_name), '') AS ys_class_name,
		GREATEST(0, ROUND(90 * IFNULL(MAX(mo.daily_avg), 0) - SUM(ys.currentqty) - IFNULL(MAX(po.in_transit_qty), 0) - IFNULL(MAX(sc.in_transit_qty), 0), 0)) AS suggested,
		CASE
		  WHEN IFNULL(MAX(mo.daily_avg), 0) > 0 AND SUM(ys.currentqty) <= 0 THEN -1
		  WHEN IFNULL(MAX(mo.daily_avg), 0) > 0 THEN ROUND(SUM(ys.currentqty) / MAX(mo.daily_avg), 1)
		  ELSE 9999 END AS sellable_days,
		CASE WHEN LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')) = '9999-12-31' THEN ''
		     ELSE DATE_FORMAT(LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')), '%Y-%m-%d') END AS next_arrive_date,
		CASE WHEN LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')) = '9999-12-31' THEN 999
		     ELSE DATEDIFF(LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')), CURDATE()) END AS next_arrive_days
		FROM ys_stock ys
		LEFT JOIN (
		  SELECT product_c_code, SUM(qty)/30 AS daily_avg FROM ys_material_out
		  WHERE vouchdate >= DATE_SUB((SELECT MAX(vouchdate) FROM ys_material_out), INTERVAL 30 DAY)` + excludeAnhuiOrgWHERE + `
		  GROUP BY product_c_code
		) mo ON mo.product_c_code = ys.product_code
		LEFT JOIN (
		  SELECT product_c_code, SUM(qty - IFNULL(total_in_qty, 0)) AS in_transit_qty
		  FROM ys_purchase_orders
		  WHERE purchase_orders_in_wh_status IN (2,3) AND qty > IFNULL(total_in_qty, 0)
		    AND (recieve_date IS NULL OR recieve_date <= DATE_ADD(CURDATE(), INTERVAL 90 DAY))` + excludeAnhuiOrgWHERE + `
		  GROUP BY product_c_code
		) po ON po.product_c_code = ys.product_code
		LEFT JOIN (
		  SELECT product_c_code, MIN(IFNULL(recieve_date, DATE_ADD(vouchdate, INTERVAL 30 DAY))) AS next_arrive
		  FROM ys_purchase_orders
		  WHERE purchase_orders_in_wh_status IN (2,3) AND qty > IFNULL(total_in_qty, 0)` + excludeAnhuiOrgWHERE + `
		  GROUP BY product_c_code
		) po_arr ON po_arr.product_c_code = ys.product_code
		LEFT JOIN (
		  SELECT order_product_material_code AS pcode,
		    SUM(order_product_subcontract_quantity_mu - IFNULL(order_product_incoming_quantity, 0)) AS in_transit_qty
		  FROM ys_subcontract_orders
		  WHERE status NOT IN (2)
		    AND order_product_subcontract_quantity_mu > IFNULL(order_product_incoming_quantity, 0)
		    AND (order_product_delivery_date IS NULL OR order_product_delivery_date <= DATE_ADD(CURDATE(), INTERVAL 90 DAY))` + excludeAnhuiOrgWHERE + `
		  GROUP BY order_product_material_code
		) sc ON sc.pcode = ys.product_code
		LEFT JOIN (
		  SELECT order_product_material_code AS pcode,
		    MIN(IFNULL(order_product_delivery_date, DATE_ADD(vouchdate, INTERVAL 30 DAY))) AS next_arrive
		  FROM ys_subcontract_orders
		  WHERE status NOT IN (2)
		    AND order_product_subcontract_quantity_mu > IFNULL(order_product_incoming_quantity, 0)` + excludeAnhuiOrgWHERE + `
		  GROUP BY order_product_material_code
		) sc_arr ON sc_arr.pcode = ys.product_code
		LEFT JOIN (
		  SELECT sku_code, MAX(goods_no) AS goods_no FROM goods
		  WHERE sku_code IS NOT NULL AND sku_code != '' GROUP BY sku_code
		) gm ON gm.sku_code = ys.product_code
		WHERE (ys.manage_class_code LIKE '01%' OR ys.manage_class_code LIKE '02%')` + excludeAnhuiOrgYsWHERE + `
		GROUP BY ys.product_code`

	// 4c. 其他 (含广宣品/周边品/物流易耗品/其它) — v0.64 新增
	// 跑哥指示: 用吉客云的库存和销量 (业务对广宣品的"消耗"走销售出库, 不走YS生产领料)
	// 公式: max(0, 45 × 吉客云日均 - 吉客云库存 - YS在途采购 - YS在途委外)
	// 范围: ys_stock manage_class_code LIKE '05%' 圈定 SKU, stock_quantity 取 7 仓白名单, 有月销
	otherPlanSqWhCond, otherPlanSqWhArgs := buildPlanWarehouseFilter("sq.warehouse_name")
	otherProdExclCond, otherProdExclArgs := buildExcludeGoodsFilter("sq.goods_no")
	otherSQL := `SELECT '其他' AS t,
		sq.goods_no AS jky_code,
		IFNULL(MAX(gm.ys_code), '') AS ys_code,
		sq.goods_name,
		ROUND(SUM(sq.current_qty - sq.locked_qty), 0) AS stock,
		ROUND(SUM(sq.month_qty)/30, 1) AS daily_avg,
		IFNULL(ROUND(MAX(po.in_transit_qty), 0), 0) AS in_transit,
		IFNULL(ROUND(MAX(sc.in_transit_qty), 0), 0) AS in_transit_subcontract,
		COALESCE(NULLIF(MAX(gm.ys_class_name), ''), MAX(ys_direct.direct_class_name), '') AS ys_class_name,
		GREATEST(0, ROUND(45 * SUM(sq.month_qty)/30 - SUM(sq.current_qty - sq.locked_qty) - IFNULL(MAX(po.in_transit_qty), 0) - IFNULL(MAX(sc.in_transit_qty), 0), 0)) AS suggested,
		CASE
		  WHEN SUM(sq.month_qty) > 0 AND (SUM(sq.current_qty - sq.locked_qty)) <= 0 THEN -1
		  WHEN SUM(sq.month_qty) > 0 THEN ROUND(SUM(sq.current_qty - sq.locked_qty) / (SUM(sq.month_qty)/30), 1)
		  ELSE 9999 END AS sellable_days,
		CASE WHEN LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')) = '9999-12-31' THEN ''
		     ELSE DATE_FORMAT(LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')), '%Y-%m-%d') END AS next_arrive_date,
		CASE WHEN LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')) = '9999-12-31' THEN 999
		     ELSE DATEDIFF(LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')), CURDATE()) END AS next_arrive_days
		FROM stock_quantity sq
		LEFT JOIN (
		  SELECT g.goods_no AS jky_no, SUM(p.qty - IFNULL(p.total_in_qty, 0)) AS in_transit_qty
		  FROM ys_purchase_orders p JOIN goods g ON g.sku_code = p.product_c_code
		  WHERE p.purchase_orders_in_wh_status IN (2,3) AND p.qty > IFNULL(p.total_in_qty, 0)
		    AND (p.recieve_date IS NULL OR p.recieve_date <= DATE_ADD(CURDATE(), INTERVAL 90 DAY))
		    AND p.org_name != '安徽香松自然调味品有限公司'
		  GROUP BY g.goods_no
		) po ON po.jky_no = sq.goods_no
		LEFT JOIN (
		  SELECT g.goods_no AS jky_no, MIN(IFNULL(p.recieve_date, DATE_ADD(p.vouchdate, INTERVAL 30 DAY))) AS next_arrive
		  FROM ys_purchase_orders p JOIN goods g ON g.sku_code = p.product_c_code
		  WHERE p.purchase_orders_in_wh_status IN (2,3) AND p.qty > IFNULL(p.total_in_qty, 0)
		    AND p.org_name != '安徽香松自然调味品有限公司'
		  GROUP BY g.goods_no
		) po_arr ON po_arr.jky_no = sq.goods_no
		LEFT JOIN (
		  SELECT g.goods_no AS jky_no,
		    SUM(s.order_product_subcontract_quantity_mu - IFNULL(s.order_product_incoming_quantity, 0)) AS in_transit_qty
		  FROM ys_subcontract_orders s JOIN goods g ON g.sku_code = s.order_product_material_code
		  WHERE s.status NOT IN (2)
		    AND s.order_product_subcontract_quantity_mu > IFNULL(s.order_product_incoming_quantity, 0)
		    AND (s.order_product_delivery_date IS NULL OR s.order_product_delivery_date <= DATE_ADD(CURDATE(), INTERVAL 90 DAY))
		    AND s.org_name != '安徽香松自然调味品有限公司'
		  GROUP BY g.goods_no
		) sc ON sc.jky_no = sq.goods_no
		LEFT JOIN (
		  SELECT g.goods_no AS jky_no,
		    MIN(IFNULL(s.order_product_delivery_date, DATE_ADD(s.vouchdate, INTERVAL 30 DAY))) AS next_arrive
		  FROM ys_subcontract_orders s JOIN goods g ON g.sku_code = s.order_product_material_code
		  WHERE s.status NOT IN (2)
		    AND s.order_product_subcontract_quantity_mu > IFNULL(s.order_product_incoming_quantity, 0)
		    AND s.org_name != '安徽香松自然调味品有限公司'
		  GROUP BY g.goods_no
		) sc_arr ON sc_arr.jky_no = sq.goods_no
		LEFT JOIN (
		  SELECT g.goods_no,
		    MAX(NULLIF(g.sku_code,'')) AS ys_code,
		    MAX(yc.manage_class_name) AS ys_class_name,
		    MAX(yc.manage_class_code) AS ys_class_code
		  FROM goods g
		  LEFT JOIN (SELECT product_code,
		                    MAX(manage_class_name) AS manage_class_name,
		                    MAX(manage_class_code) AS manage_class_code
		             FROM ys_stock GROUP BY product_code) yc ON yc.product_code = g.sku_code
		  WHERE g.sku_code IS NOT NULL AND g.sku_code != '' GROUP BY g.goods_no
		) gm ON gm.goods_no = sq.goods_no
		LEFT JOIN (
		  -- v0.65 直连兜底: 当 goods.sku_code 缺失时, 用 sq.goods_no 直接对 ys_stock.product_code
		  SELECT product_code,
		    MAX(manage_class_name) AS direct_class_name,
		    MAX(manage_class_code) AS direct_class_code
		  FROM ys_stock GROUP BY product_code
		) ys_direct ON ys_direct.product_code = sq.goods_no
		WHERE sq.month_qty > 0
		  AND (gm.ys_class_code LIKE '05%' OR ys_direct.direct_class_code LIKE '05%')` + otherPlanSqWhCond + otherProdExclCond + `
		GROUP BY sq.goods_no, sq.goods_name`

	type queryWithArgs struct {
		sql  string
		args []interface{}
	}
	prodArgs := append([]interface{}{}, planSqWhArgs...)
	prodArgs = append(prodArgs, prodExclArgs...)
	otherArgs := append([]interface{}{}, otherPlanSqWhArgs...)
	otherArgs = append(otherArgs, otherProdExclArgs...)
	for _, qa := range []queryWithArgs{
		{prodSQL, prodArgs},   // 成品/半成品 7 仓白名单 + 虚拟品排除 + 排除广宣品(05%)
		{matSQL, nil},         // 原材料/包材 YS 全仓 (限定 01%/02%)
		{otherSQL, otherArgs}, // 其他 7 仓白名单 + 限定广宣品(05%)
	} {
		sRows, err := h.DB.Query(qa.sql, qa.args...)
		if err != nil {
			log.Printf("suggest query err: %v", err)
			continue
		}
		for sRows.Next() {
			var s suggestRow
			if err := sRows.Scan(&s.Type, &s.JkyCode, &s.YsCode, &s.GoodsName, &s.Stock, &s.DailyAvg,
				&s.InTransit, &s.InTransitSubcontract, &s.YsClassName, &s.SuggestedQty, &s.SellableDays,
				&s.NextArriveDate, &s.NextArriveDays); err != nil {
				log.Printf("[suggest] scan err: %v", err)
				continue
			}
			// 判断 status
			switch {
			case s.SellableDays < 0:
				s.Status = "断货"
			case s.SellableDays < 7:
				s.Status = "紧急"
			case s.SellableDays < 14:
				s.Status = "偏低"
			case s.SellableDays > 90:
				s.Status = "积压"
			default:
				s.Status = "正常"
			}
			suggested = append(suggested, s)
		}
		sRows.Close()
	}

	// 按 suggestedQty 倒序
	for i := 0; i < len(suggested); i++ {
		for j := i + 1; j < len(suggested); j++ {
			if suggested[j].SuggestedQty > suggested[i].SuggestedQty {
				suggested[i], suggested[j] = suggested[j], suggested[i]
			}
		}
	}

	writeJSON(w, map[string]interface{}{
		"kpis":         kpi,
		"monthlyTrend": monthlyTrend,
		"topVendors":   topVendors,
		"suggested":    suggested,
		"params": map[string]interface{}{
			"finishedGoodsTargetDays":  45,
			"materialTargetDays":       90,
			"urgentThresholdDays":      7,
			"lowThresholdDays":         14,
			"overstockThresholdDays":   90,
		},
	})
}

// syncYSStockMu 防止并发触发同步 (一次只允许一个跑)
var syncYSStockMu sync.Mutex
var syncYSStockRunning bool
var syncYSLastEndTime time.Time // v0.74: 全局后端 cooldown, 防止任何来源(包括前端 bug) 60s 内重复触发

// syncYSProgress v0.71: 同步进度状态 (前端轮询用)
type syncStepProgress struct {
	Name        string `json:"name"`
	Ins         int    `json:"ins"`
	Upd         int    `json:"upd"`
	Err         int    `json:"err"`
	DurationSec int    `json:"durationSec"`
	Failed      bool   `json:"failed"`
	Message     string `json:"message,omitempty"`
}
type syncProgressState struct {
	Running     bool               `json:"running"`
	Done        bool               `json:"done"`
	StartTime   time.Time          `json:"-"`
	StartedAt   string             `json:"startedAt"`
	TotalSteps  int                `json:"totalSteps"`
	CurrentStep int                `json:"currentStep"` // 1-indexed, 0=未开始
	CurrentName string             `json:"currentName"`
	Results     []syncStepProgress `json:"results"`
	ElapsedSec  int                `json:"elapsedSec"`
	Err         string             `json:"err,omitempty"`
}

var syncYSProgress syncProgressState

// SyncYSStock v0.68: 一键全量同步 YS 4 类数据 (现存量+采购订单+委外订单+材料出库)
// POST /api/supply-chain/sync-ys-stock (路由名保留以保持兼容, 内部行为已升级)
// 串行执行避免 YS API 限流, 总耗时约 60-120s
func (h *DashboardHandler) SyncYSStock(w http.ResponseWriter, r *http.Request) {
	// v0.73 诊断日志: 记录每次 sync 请求的来源, 排查"自动重新同步"问题
	log.Printf("[sync-ys-stock] 收到请求 method=%s remote=%s referer=%q ua=%q origin=%q",
		r.Method, r.RemoteAddr, r.Header.Get("Referer"), r.Header.Get("User-Agent"), r.Header.Get("Origin"))

	if r.Method != "POST" {
		log.Printf("[sync-ys-stock] 拒绝: 非 POST")
		writeError(w, 405, "method not allowed")
		return
	}

	syncYSStockMu.Lock()
	if syncYSStockRunning {
		syncYSStockMu.Unlock()
		log.Printf("[sync-ys-stock] 拒绝: 已有同步在执行")
		writeError(w, 429, "已有同步任务正在执行, 请稍后再试")
		return
	}
	// v0.74: 全局后端 cooldown 60s — 防止任何来源(浏览器 bug/扩展/双 tab/手抖)在上次同步完成后立即触发新一轮
	if !syncYSLastEndTime.IsZero() {
		since := time.Since(syncYSLastEndTime)
		if since < 60*time.Second {
			wait := int((60*time.Second - since).Seconds())
			syncYSStockMu.Unlock()
			log.Printf("[sync-ys-stock] 拒绝: 上次同步 %.0fs 前结束, cooldown 还需 %ds (来源 %s referer=%s)",
				since.Seconds(), wait, r.RemoteAddr, r.Header.Get("Referer"))
			writeError(w, 429, fmt.Sprintf("上次同步刚完成, 请 %d 秒后再试", wait))
			return
		}
	}
	syncYSStockRunning = true
	log.Printf("[sync-ys-stock] ★ 启动新一轮同步, 锁已获取")
	// v0.71: 重置进度状态
	syncYSProgress = syncProgressState{
		Running:   true,
		StartTime: time.Now(),
		StartedAt: time.Now().Format("2006-01-02 15:04:05"),
		Results:   []syncStepProgress{},
	}
	syncYSStockMu.Unlock()
	defer func() {
		syncYSStockMu.Lock()
		syncYSStockRunning = false
		syncYSLastEndTime = time.Now() // v0.74: 记录完成时间, 触发 60s 全局 cooldown
		syncYSProgress.Running = false
		syncYSProgress.Done = true
		syncYSProgress.ElapsedSec = int(time.Since(syncYSProgress.StartTime).Seconds())
		syncYSStockMu.Unlock()
	}()

	start := time.Now()
	exeDir, _ := filepath.Abs(filepath.Dir(os.Args[0]))

	type stepResult = syncStepProgress

	// v0.70: 立即同步按钮动态算范围 — 按本地"未关闭"单的最早 vouchdate 算起点
	// 业务规则: 已关闭单不会再变化, 只要覆盖未关闭单的 vouchdate 范围即可
	// 兜底 30 天 (防止本地暂时无未结单, 仍保留近期窗口)
	rangeEnd := time.Now().Format("2006-01-02")
	defaultStart := time.Now().AddDate(0, 0, -30).Format("2006-01-02")

	// 采购未结单 = status IN (2,3,4) AND qty > total_in_qty
	purchaseStart := defaultStart
	var minPurchase sql.NullString
	if err := h.DB.QueryRow(`SELECT DATE_FORMAT(MIN(vouchdate), '%Y-%m-%d') FROM ys_purchase_orders
		WHERE purchase_orders_in_wh_status IN (2,3,4) AND qty > IFNULL(total_in_qty, 0)`).Scan(&minPurchase); err == nil {
		if minPurchase.Valid && minPurchase.String != "" && minPurchase.String < purchaseStart {
			purchaseStart = minPurchase.String
		}
	}

	// 委外未结单 = status NOT IN (2) AND quantity > incoming
	subcontractStart := defaultStart
	var minSubcontract sql.NullString
	if err := h.DB.QueryRow(`SELECT DATE_FORMAT(MIN(vouchdate), '%Y-%m-%d') FROM ys_subcontract_orders
		WHERE status NOT IN (2)
		  AND order_product_subcontract_quantity_mu > IFNULL(order_product_incoming_quantity, 0)`).Scan(&minSubcontract); err == nil {
		if minSubcontract.Valid && minSubcontract.String != "" && minSubcontract.String < subcontractStart {
			subcontractStart = minSubcontract.String
		}
	}

	purchaseLabel := "采购订单 (" + purchaseStart + " ~ " + rangeEnd + ")"
	subcontractLabel := "委外订单 (" + subcontractStart + " ~ " + rangeEnd + ")"

	exes := []struct {
		name, exe string
		args      []string
	}{
		{"吉客云库存", "sync-stock.exe", nil}, // v0.76: 成品 Tab 数据源
		{"YS 现存量", "sync-yonsuite-stock.exe", nil},
		{purchaseLabel, "sync-yonsuite-purchase.exe", []string{purchaseStart, rangeEnd}},
		{subcontractLabel, "sync-yonsuite-subcontract.exe", []string{subcontractStart, rangeEnd}},
		{"YS 材料出库", "sync-yonsuite-materialout.exe", nil},
	}

	re := regexp.MustCompile(`新增 (\d+) / 更新 (\d+) / 失败 (\d+)`)
	results := make([]stepResult, 0, len(exes))
	var totalIns, totalUpd, totalErr int

	// v0.71: 进度推送 — 每步开始时更新 currentStep + currentName
	syncYSStockMu.Lock()
	syncYSProgress.TotalSteps = len(exes)
	syncYSStockMu.Unlock()

	for i, item := range exes {
		// 步骤开始
		syncYSStockMu.Lock()
		syncYSProgress.CurrentStep = i + 1
		syncYSProgress.CurrentName = item.name
		syncYSProgress.ElapsedSec = int(time.Since(syncYSProgress.StartTime).Seconds())
		syncYSStockMu.Unlock()

		stepStart := time.Now()
		exePath := filepath.Join(exeDir, item.exe)
		if _, err := os.Stat(exePath); err != nil {
			r := stepResult{
				Name: item.name, Failed: true, Message: "exe 文件缺失: " + item.exe,
				DurationSec: 0,
			}
			results = append(results, r)
			syncYSStockMu.Lock()
			syncYSProgress.Results = append(syncYSProgress.Results, r)
			syncYSStockMu.Unlock()
			continue
		}
		cmd := exec.Command(exePath, item.args...)
		cmd.Dir = exeDir
		out, err := cmd.CombinedOutput()
		output := string(out)
		var ins, upd, errN int
		if m := re.FindStringSubmatch(output); len(m) == 4 {
			ins, _ = strconv.Atoi(m[1])
			upd, _ = strconv.Atoi(m[2])
			errN, _ = strconv.Atoi(m[3])
		}
		failed := err != nil
		msg := ""
		if failed {
			msg = err.Error()
			log.Printf("[sync-ys-all] %s 失败: err=%v output=%s", item.name, err, output)
		}
		r := stepResult{
			Name: item.name, Ins: ins, Upd: upd, Err: errN,
			DurationSec: int(time.Since(stepStart).Seconds()),
			Failed:      failed, Message: msg,
		}
		results = append(results, r)
		// 步骤结束 — 推送到 progress
		syncYSStockMu.Lock()
		syncYSProgress.Results = append(syncYSProgress.Results, r)
		syncYSProgress.ElapsedSec = int(time.Since(syncYSProgress.StartTime).Seconds())
		syncYSStockMu.Unlock()
		totalIns += ins
		totalUpd += upd
		totalErr += errN
	}

	// 清缓存 — 计划采购 + 库存 + 供应链整段
	cleared := ClearCacheByPrefix("api|/api/supply-chain")
	cleared += ClearCacheByPrefix("api|/api/stock/")

	log.Printf("[sync-ys-all] 完成 总 ins=%d upd=%d err=%d cache=%d 耗时=%.1fs",
		totalIns, totalUpd, totalErr, cleared, time.Since(start).Seconds())

	writeJSON(w, map[string]interface{}{
		"ok":           true,
		"steps":        results,
		"ins":          totalIns,
		"upd":          totalUpd,
		"err":          totalErr,
		"cacheCleared": cleared,
		"durationSec":  int(time.Since(start).Seconds()),
	})
}

// GetSyncYSProgress v0.71: 同步进度查询 (前端轮询)
// GET /api/supply-chain/sync-ys-progress
// 返回当前同步状态: running/done/totalSteps/currentStep/currentName/results/elapsedSec
func (h *DashboardHandler) GetSyncYSProgress(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDomainAccess(r, "supply_chain")) {
		return
	}
	syncYSStockMu.Lock()
	defer syncYSStockMu.Unlock()
	// 复制一份当前 progress (避免共享指针在 JSON 序列化时被并发改写)
	snapshot := syncProgressState{
		Running:     syncYSProgress.Running,
		Done:        syncYSProgress.Done,
		StartedAt:   syncYSProgress.StartedAt,
		TotalSteps:  syncYSProgress.TotalSteps,
		CurrentStep: syncYSProgress.CurrentStep,
		CurrentName: syncYSProgress.CurrentName,
		Results:     append([]syncStepProgress{}, syncYSProgress.Results...),
		Err:         syncYSProgress.Err,
	}
	if syncYSProgress.Running {
		snapshot.ElapsedSec = int(time.Since(syncYSProgress.StartTime).Seconds())
	} else {
		snapshot.ElapsedSec = syncYSProgress.ElapsedSec
	}
	writeJSON(w, snapshot)
}

// GetInTransitDetail v0.67: 在途采购/委外订单明细 (按 SKU 下钻)
// 参数: goodsNo (吉客云 goods_no, 必填)
// 返回: { purchaseOrders: [...], subcontractOrders: [...] }
//
// 桥接策略 (兼容 v0.65 双路径):
//  1. 优先 goods.sku_code → YS product_c_code 桥接
//  2. 兜底 sq.goods_no 直接 = ys_*.product_c_code
func (h *DashboardHandler) GetInTransitDetail(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDomainAccess(r, "supply_chain")) {
		return
	}
	goodsNo := strings.TrimSpace(r.URL.Query().Get("goodsNo"))
	if goodsNo == "" {
		writeError(w, http.StatusBadRequest, "goodsNo required")
		return
	}

	type purchaseOrder struct {
		Code         string  `json:"code"`         // YS 订单号
		VendorName   string  `json:"vendorName"`   // 供应商
		OrgName      string  `json:"orgName"`      // 组织
		VouchDate    string  `json:"vouchDate"`    // 开单日期
		ArriveDate   string  `json:"arriveDate"`   // 计划到货日期
		TotalQty     float64 `json:"totalQty"`     // 订单总量
		IncomingQty  float64 `json:"incomingQty"`  // 已入库
		InTransitQty float64 `json:"inTransitQty"` // 未入库(在途)
		StatusText   string  `json:"statusText"`   // 业务大白话状态
	}

	type subcontractOrder struct {
		Code         string  `json:"code"`
		VendorName   string  `json:"vendorName"`
		OrgName      string  `json:"orgName"`
		VouchDate    string  `json:"vouchDate"`
		ArriveDate   string  `json:"arriveDate"`
		TotalQty     float64 `json:"totalQty"`
		IncomingQty  float64 `json:"incomingQty"`
		InTransitQty float64 `json:"inTransitQty"`
		StatusText   string  `json:"statusText"`
	}

	purchaseOrders := []purchaseOrder{}
	subcontractOrders := []subcontractOrder{}

	// 1. 在途采购订单 (双路径桥接)
	purchaseSQL := `SELECT
		p.code,
		IFNULL(p.vendor_name, '') AS vendor_name,
		IFNULL(p.org_name, '') AS org_name,
		IFNULL(DATE_FORMAT(p.vouchdate, '%Y-%m-%d'), '') AS vouch_date,
		IFNULL(DATE_FORMAT(p.recieve_date, '%Y-%m-%d'), '') AS arrive_date,
		IFNULL(p.qty, 0) AS total_qty,
		IFNULL(p.total_in_qty, 0) AS incoming_qty,
		IFNULL(p.qty, 0) - IFNULL(p.total_in_qty, 0) AS in_transit_qty,
		CASE p.purchase_orders_in_wh_status
		  WHEN 2 THEN '已审核未入库'
		  WHEN 3 THEN '部分入库'
		  ELSE CONCAT('状态码', p.purchase_orders_in_wh_status)
		END AS status_text
		FROM ys_purchase_orders p
		WHERE p.purchase_orders_in_wh_status IN (2,3)
		  AND p.qty > IFNULL(p.total_in_qty, 0)
		  AND p.org_name != '安徽香松自然调味品有限公司'
		  AND (
		    p.product_c_code IN (SELECT sku_code FROM goods WHERE goods_no = ? AND sku_code IS NOT NULL AND sku_code != '')
		    OR p.product_c_code = ?
		  )
		ORDER BY p.vouchdate DESC, p.code`
	if rows, err := h.DB.Query(purchaseSQL, goodsNo, goodsNo); err == nil {
		defer rows.Close()
		for rows.Next() {
			var o purchaseOrder
			if err := rows.Scan(&o.Code, &o.VendorName, &o.OrgName, &o.VouchDate, &o.ArriveDate,
				&o.TotalQty, &o.IncomingQty, &o.InTransitQty, &o.StatusText); err == nil {
				purchaseOrders = append(purchaseOrders, o)
			}
		}
	} else {
		log.Printf("in-transit purchase query err: %v", err)
	}

	// 2. 在途委外订单 (双路径桥接)
	subcontractSQL := `SELECT
		s.code,
		IFNULL(s.subcontract_vendor_name, '') AS vendor_name,
		IFNULL(s.org_name, '') AS org_name,
		IFNULL(DATE_FORMAT(s.vouchdate, '%Y-%m-%d'), '') AS vouch_date,
		IFNULL(DATE_FORMAT(s.order_product_delivery_date, '%Y-%m-%d'), '') AS arrive_date,
		IFNULL(s.order_product_subcontract_quantity_mu, 0) AS total_qty,
		IFNULL(s.order_product_incoming_quantity, 0) AS incoming_qty,
		IFNULL(s.order_product_subcontract_quantity_mu, 0) - IFNULL(s.order_product_incoming_quantity, 0) AS in_transit_qty,
		CASE s.status
		  WHEN 0 THEN '草稿'
		  WHEN 1 THEN '已审核未入库'
		  WHEN 3 THEN '部分入库'
		  WHEN 4 THEN '已完成'
		  ELSE CONCAT('状态码', s.status)
		END AS status_text
		FROM ys_subcontract_orders s
		WHERE s.status NOT IN (2)
		  AND s.order_product_subcontract_quantity_mu > IFNULL(s.order_product_incoming_quantity, 0)
		  AND s.org_name != '安徽香松自然调味品有限公司'
		  AND (
		    s.order_product_material_code IN (SELECT sku_code FROM goods WHERE goods_no = ? AND sku_code IS NOT NULL AND sku_code != '')
		    OR s.order_product_material_code = ?
		  )
		ORDER BY s.vouchdate DESC, s.code`
	if rows, err := h.DB.Query(subcontractSQL, goodsNo, goodsNo); err == nil {
		defer rows.Close()
		for rows.Next() {
			var o subcontractOrder
			if err := rows.Scan(&o.Code, &o.VendorName, &o.OrgName, &o.VouchDate, &o.ArriveDate,
				&o.TotalQty, &o.IncomingQty, &o.InTransitQty, &o.StatusText); err == nil {
				subcontractOrders = append(subcontractOrders, o)
			}
		}
	} else {
		log.Printf("in-transit subcontract query err: %v", err)
	}

	writeJSON(w, map[string]interface{}{
		"goodsNo":           goodsNo,
		"purchaseOrders":    purchaseOrders,
		"subcontractOrders": subcontractOrders,
	})
}
