package handler

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// syncStockMu 防止 /api/stock/sync-now 并发执行
var syncStockMu sync.Mutex

// syncStockState 记录上一次/当前同步任务的状态，供 /api/stock/sync-status 查询
var syncStockState = struct {
	sync.Mutex
	Running        bool
	StartedAt      time.Time
	LastFinishedAt time.Time
	LastElapsed    time.Duration
	LastError      string
}{}

func setSyncStockStart() {
	syncStockState.Lock()
	syncStockState.Running = true
	syncStockState.StartedAt = time.Now()
	syncStockState.Unlock()
}

func setSyncStockFinish(elapsed time.Duration, errMsg string) {
	syncStockState.Lock()
	syncStockState.Running = false
	syncStockState.LastFinishedAt = time.Now()
	syncStockState.LastElapsed = elapsed
	syncStockState.LastError = errMsg
	syncStockState.Unlock()
}

// SyncStockNow POST /api/stock/sync-now
// 用户主动触发：拉取吉客云全量库存到 stock_quantity（约 2-3 分钟）
func (h *DashboardHandler) SyncStockNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	if writeScopeError(w, requireDomainAccess(r, "supply_chain")) {
		return
	}

	elapsed, output, locked, err := RunSyncStockOnce()
	if locked {
		writeError(w, 409, "已有库存同步任务进行中，请稍后再试")
		return
	}
	tail := tailLines(output, 12)
	if err != nil {
		writeError(w, 500, fmt.Sprintf("同步失败（耗时 %s）: %v\n最后输出:\n%s", elapsed.Round(time.Second), err, tail))
		return
	}
	cacheCleared := ClearCacheByPrefix("api|/api/stock/")
	writeJSON(w, map[string]interface{}{
		"success":      true,
		"elapsed":      elapsed.Round(time.Second).String(),
		"cacheCleared": cacheCleared,
		"tail":         tail,
	})
}

// RunSyncStockOnce v0.79: 公共入口 - 跑 sync-stock.exe 同步吉客云库存
// 任何 handler 都用这个, 共享 syncStockMu 锁防并发, 共享 syncStockState 让两个页面看到一致状态
// 返回: 耗时, exe 输出, 是否被锁拒绝(true=已有任务在跑), 执行错误
func RunSyncStockOnce() (elapsed time.Duration, output string, locked bool, err error) {
	if !syncStockMu.TryLock() {
		return 0, "", true, nil
	}
	defer syncStockMu.Unlock()
	setSyncStockStart()

	exePath := `C:\Users\Administrator\bi-dashboard\server\sync-stock.exe`
	workDir := `C:\Users\Administrator\bi-dashboard\server`

	start := time.Now()
	cmd := exec.Command(exePath)
	cmd.Dir = workDir
	out, runErr := cmd.CombinedOutput()
	elapsed = time.Since(start)
	output = string(out)
	errMsg := ""
	if runErr != nil {
		errMsg = runErr.Error()
	}
	setSyncStockFinish(elapsed, errMsg)
	return elapsed, output, false, runErr
}

// SyncStockStatus GET /api/stock/sync-status
// 返回当前/上次同步状态，前端进页面 + 轮询用
func (h *DashboardHandler) SyncStockStatus(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDomainAccess(r, "supply_chain")) {
		return
	}
	syncStockState.Lock()
	defer syncStockState.Unlock()

	resp := map[string]interface{}{
		"running": syncStockState.Running,
	}
	if syncStockState.Running {
		resp["startedAt"] = syncStockState.StartedAt.Format(time.RFC3339)
		resp["elapsedSec"] = int(time.Since(syncStockState.StartedAt).Seconds())
	}
	if !syncStockState.LastFinishedAt.IsZero() {
		resp["lastFinishedAt"] = syncStockState.LastFinishedAt.Format(time.RFC3339)
		resp["lastElapsedSec"] = int(syncStockState.LastElapsed.Seconds())
		resp["lastError"] = syncStockState.LastError
	}
	writeJSON(w, resp)
}

// tailLines 取字符串最后 n 行
func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\r\n"), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// GetStockWarning 库存预警数据
func (h *DashboardHandler) GetStockWarning(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDomainAccess(r, "supply_chain")) {
		return
	}

	warehouse := r.URL.Query().Get("warehouse")
	warning := r.URL.Query().Get("warning")
	keyword := r.URL.Query().Get("keyword")
	warehouseCond, warehouseArgs, err := buildWarehouseScopeCond(r, warehouse, "warehouse_name")
	if writeScopeError(w, err) {
		return
	}
	warehouseScopeCond, warehouseScopeArgs, err := buildWarehouseScopeCond(r, "", "warehouse_name")
	if writeScopeError(w, err) {
		return
	}

	// 库存预警仓库白名单（与计划看板共用，定义在 supply_chain.go 顶部 planWarehouses）
	planCond, planArgs := buildPlanWarehouseFilter("warehouse_name")

	// 1. 预警统计卡片（按SKU+仓库维度）
	var stockout, urgent, low, overstock, dead, total int
	summaryArgs := append([]interface{}{}, warehouseArgs...)
	summaryArgs = append(summaryArgs, planArgs...)
	if err := h.DB.QueryRow(`
		SELECT
			COUNT(*) AS total,
			SUM(CASE WHEN current_qty - locked_qty <= 0 AND month_qty > 0 THEN 1 ELSE 0 END),
			SUM(CASE WHEN (current_qty - locked_qty) > 0 AND month_qty > 0
				AND (current_qty - locked_qty) / (month_qty/30) < 7 THEN 1 ELSE 0 END),
			SUM(CASE WHEN (current_qty - locked_qty) > 0 AND month_qty > 0
				AND (current_qty - locked_qty) / (month_qty/30) BETWEEN 7 AND 14 THEN 1 ELSE 0 END),
			SUM(CASE WHEN (current_qty - locked_qty) > 0 AND month_qty > 0
				AND (current_qty - locked_qty) / (month_qty/30) > 90 THEN 1 ELSE 0 END),
			SUM(CASE WHEN month_qty = 0 AND current_qty > 0 AND IFNULL(sca.allot_qty,0) = 0 THEN 1 ELSE 0 END)
		FROM stock_quantity
		LEFT JOIN `+planSpecialAllotQtyLiveSubSQL+` sca ON sca.goods_no = stock_quantity.goods_no
		WHERE goods_attr = 1 AND warehouse_name != ''`+warehouseCond+planCond,
		summaryArgs...,
	).Scan(&total, &stockout, &urgent, &low, &overstock, &dead); err != nil {
		writeError(w, 500, "database query failed")
		return
	}

	summary := map[string]int{
		"total": total, "stockout": stockout, "urgent": urgent,
		"low": low, "overstock": overstock, "dead": dead,
	}

	// 2. 查明细数据（LEFT JOIN goods 拿商品分类 + 产品定位）
	query := `
		SELECT sq.goods_no, sq.goods_name, sq.unit_name,
			sq.warehouse_name,
			ROUND(sq.current_qty - sq.locked_qty, 2) AS usable_qty,
			sq.month_qty,
			ROUND(sq.month_qty / 30, 1) AS daily_avg,
			CASE
				WHEN sq.month_qty > 0 AND (sq.current_qty - sq.locked_qty) <= 0 THEN -1
				WHEN sq.month_qty > 0 THEN ROUND((sq.current_qty - sq.locked_qty) / (sq.month_qty/30), 1)
				WHEN sq.current_qty > 0 THEN 9999
				ELSE 0
			END AS sellable_days,
			sq.current_qty,
			IFNULL(g.cate_name,'') AS category,
			IFNULL(g.goods_field7,'') AS position
		FROM stock_quantity sq
		LEFT JOIN (SELECT DISTINCT goods_no, cate_name, goods_field7 FROM goods) g ON g.goods_no = sq.goods_no
		WHERE sq.goods_attr = 1 AND sq.warehouse_name != ''
	`
	query += strings.ReplaceAll(warehouseCond, "warehouse_name", "sq.warehouse_name")
	query += strings.ReplaceAll(planCond, "warehouse_name", "sq.warehouse_name")
	args := append([]interface{}{}, warehouseArgs...)
	args = append(args, planArgs...)
	if keyword != "" {
		query += " AND (sq.goods_no LIKE ? OR sq.goods_name LIKE ?)"
		kw := "%" + keyword + "%"
		args = append(args, kw, kw)
	}

	switch warning {
	case "stockout":
		query += " AND (sq.current_qty - sq.locked_qty) <= 0 AND sq.month_qty > 0"
	case "urgent":
		query += " AND (sq.current_qty - sq.locked_qty) > 0 AND sq.month_qty > 0 AND (sq.current_qty - sq.locked_qty) / (sq.month_qty/30) < 7"
	case "low":
		query += " AND (sq.current_qty - sq.locked_qty) > 0 AND sq.month_qty > 0 AND (sq.current_qty - sq.locked_qty) / (sq.month_qty/30) BETWEEN 7 AND 14"
	case "overstock":
		query += " AND (sq.current_qty - sq.locked_qty) > 0 AND sq.month_qty > 0 AND (sq.current_qty - sq.locked_qty) / (sq.month_qty/30) > 90"
	case "dead":
		// v1.x: 与卡片 dead 计数口径一致 —— 排除靠特殊渠道(京东/猫超/朴朴)调拨走量的货(它们不是真滞销)
		query += ` AND sq.month_qty = 0 AND sq.current_qty > 0
			AND sq.goods_no NOT IN (SELECT goods_no FROM allocate_details
				WHERE channel_key IN ('京东','猫超','朴朴')
				  AND stat_date > DATE_SUB(CURDATE(), INTERVAL 30 DAY) AND stat_date <= CURDATE())`
	default:
		query += " AND (sq.current_qty > 0 OR sq.month_qty > 0)"
	}

	query += " ORDER BY CASE WHEN sq.month_qty > 0 AND (sq.current_qty - sq.locked_qty) <= 0 THEN 0 WHEN sq.month_qty > 0 THEN (sq.current_qty - sq.locked_qty) / (sq.month_qty/30) ELSE 99999 END ASC LIMIT 2000"

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 500, "msg": err.Error()})
		return
	}
	defer rows.Close()

	type RawItem struct {
		GoodsNo       string
		GoodsName     string
		UnitName      string
		WarehouseName string
		UsableQty     float64
		MonthQty      float64
		DailyAvg      float64
		SellableDays  float64
		CurrentQty    float64
		Category      string
		Position      string
	}

	rawItems := []RawItem{}
	for rows.Next() {
		var item RawItem
		if writeDatabaseError(w, rows.Scan(&item.GoodsNo, &item.GoodsName, &item.UnitName,
			&item.WarehouseName, &item.UsableQty,
			&item.MonthQty, &item.DailyAvg, &item.SellableDays, &item.CurrentQty,
			&item.Category, &item.Position)) {
			return
		}
		rawItems = append(rawItems, item)
	}
	if writeDatabaseError(w, rows.Err()) {
		return
	}

	// 如果选了具体仓库，直接平铺返回
	if warehouse != "" {
		type FlatItem struct {
			GoodsNo      string  `json:"goodsNo"`
			GoodsName    string  `json:"goodsName"`
			Category     string  `json:"category"`
			Position     string  `json:"position"`
			Warehouse    string  `json:"warehouse"`
			UsableQty    float64 `json:"usableQty"`
			SellableDays float64 `json:"sellableDays"`
			DailyAvg     float64 `json:"dailyAvg"`
			MonthQty     float64 `json:"monthQty"`
			CurrentQty   float64 `json:"currentQty"`
		}
		flat := []FlatItem{}
		for _, r := range rawItems {
			flat = append(flat, FlatItem{
				GoodsNo: r.GoodsNo, GoodsName: r.GoodsName,
				Category: r.Category, Position: r.Position,
				Warehouse: r.WarehouseName, UsableQty: r.UsableQty,
				SellableDays: r.SellableDays, DailyAvg: r.DailyAvg,
				MonthQty: r.MonthQty, CurrentQty: r.CurrentQty,
			})
		}
		writeStockResponse(w, r, summary, flat, h, warehouseScopeCond, warehouseScopeArgs)
		return
	}

	// v1.x: 取特殊渠道(京东/猫超/朴朴)近30天调拨件数(goods 级), 用于 goods 聚合行判断"是否在卖"。
	//   per-(SKU×仓)明细/预警桶仍用纯 month_qty —— 调拨无仓库维度不可拆到单仓单元(全仓视角的"调拨当销售"见计划看板)。
	//   这里仅 goods 聚合行: ① DailyAvg 并入调拨 ② "全仓零销量"滞销判定排除靠调拨走量的货(避免与计划看板/dead桶口径打架)。
	allotMap := map[string]float64{}
	if aRows, aErr := h.DB.Query(`SELECT goods_no, SUM(sku_count) FROM allocate_details
		WHERE channel_key IN ('京东','猫超','朴朴')
		  AND stat_date > DATE_SUB(CURDATE(), INTERVAL 30 DAY) AND stat_date <= CURDATE()
		GROUP BY goods_no`); aErr == nil {
		defer aRows.Close()
		for aRows.Next() {
			var gn string
			var q float64
			if aRows.Scan(&gn, &q) == nil {
				allotMap[gn] = q
			}
		}
	}

	// 按 goods_no 聚合
	type ChildItem struct {
		Warehouse    string  `json:"warehouse"`
		UsableQty    float64 `json:"usableQty"`
		SellableDays float64 `json:"sellableDays"`
		DailyAvg     float64 `json:"dailyAvg"`
		MonthQty     float64 `json:"monthQty"`
	}
	type AggItem struct {
		GoodsNo      string      `json:"goodsNo"`
		GoodsName    string      `json:"goodsName"`
		Category     string      `json:"category"`
		Position     string      `json:"position"`
		UsableQty    float64     `json:"usableQty"`
		SellableDays float64     `json:"sellableDays"`
		DailyAvg     float64     `json:"dailyAvg"`
		MonthQty     float64     `json:"monthQty"`
		AllotQty     float64     `json:"allotQty"` // v1.x: 近30天特殊渠道(京东/猫超/朴朴)调拨件数, 供前端判"调拨在售"不标滞销
		CurrentQty   float64     `json:"currentQty"`
		WhCount      int         `json:"whCount"`
		WhStockout   int         `json:"whStockout"`
		Warehouses   []ChildItem `json:"warehouses"`
	}

	aggMap := map[string]*AggItem{}
	aggOrder := []string{}
	for _, r := range rawItems {
		agg, ok := aggMap[r.GoodsNo]
		if !ok {
			agg = &AggItem{GoodsNo: r.GoodsNo, GoodsName: r.GoodsName, Category: r.Category, Position: r.Position}
			aggMap[r.GoodsNo] = agg
			aggOrder = append(aggOrder, r.GoodsNo)
		}
		agg.UsableQty += r.UsableQty
		agg.MonthQty += r.MonthQty
		agg.CurrentQty += r.CurrentQty
		agg.WhCount++
		agg.Warehouses = append(agg.Warehouses, ChildItem{
			Warehouse: r.WarehouseName, UsableQty: r.UsableQty,
			SellableDays: r.SellableDays, DailyAvg: r.DailyAvg, MonthQty: r.MonthQty,
		})
	}

	// 计算聚合后的日均和可售天数（取有销量的仓库中最差的）
	result := []AggItem{}
	for _, key := range aggOrder {
		agg := aggMap[key]
		allot := allotMap[key]
		agg.AllotQty = allot
		// goods 聚合日均: 有吉客云销量的货保持纯 month_qty(与下面 worstDays 可售天数同基准, 避免日均/可售天数不自洽);
		//   全仓零吉客云销量但靠调拨走量的货, 在下面 !hasAnySales 分支用调拨件数算日均+可售天数(两者同基准, 自洽)。
		agg.DailyAvg = math.Round(agg.MonthQty/30*10) / 10

		// 找有销量的仓库中可售天数最低的
		worstDays := 99999.0
		hasAnySales := false
		for _, wh := range agg.Warehouses {
			if wh.MonthQty <= 0 {
				continue
			}
			hasAnySales = true
			if wh.SellableDays < worstDays {
				worstDays = wh.SellableDays
			}
			// 统计断货仓数量（有销量但库存<=0的仓）
			if wh.UsableQty <= 0 {
				agg.WhStockout++
			}
		}

		if !hasAnySales {
			// v1.x: 全仓吉客云零销量但靠调拨走量的货, 不标滞销, 按调拨件数算日均+可售天数(与计划看板/dead桶口径一致, 二者同基准)
			if allot > 0 && agg.CurrentQty > 0 {
				agg.DailyAvg = math.Round(allot/30*10) / 10
				if agg.UsableQty > 0 {
					agg.SellableDays = math.Round(agg.UsableQty/(allot/30)*10) / 10
				} else {
					agg.SellableDays = -1 // 锁库≥现货=等同断货(避免负可售天数)
				}
			} else if agg.CurrentQty > 0 {
				agg.SellableDays = 9999 // 有库存无销量=滞销
			} else {
				agg.SellableDays = 0
			}
		} else if worstDays < 0 {
			agg.SellableDays = -1
		} else {
			agg.SellableDays = worstDays
		}

		result = append(result, *agg)
	}

	// 限制返回500条聚合结果
	if len(result) > 500 {
		result = result[:500]
	}

	writeStockResponse(w, r, summary, result, h, warehouseScopeCond, warehouseScopeArgs)
}

func writeStockResponse(
	w http.ResponseWriter,
	r *http.Request,
	summary map[string]int,
	items interface{},
	h *DashboardHandler,
	warehouseScopeCond string,
	warehouseScopeArgs []interface{},
) {
	planCond, planArgs := buildPlanWarehouseFilter("warehouse_name")
	whArgs := append([]interface{}{}, warehouseScopeArgs...)
	whArgs = append(whArgs, planArgs...)
	whRows, ok := queryRowsOrWriteError(
		w,
		r,
		h.DB,
		`SELECT DISTINCT warehouse_name FROM stock_quantity WHERE goods_attr = 1 AND warehouse_name != ''`+warehouseScopeCond+planCond+` AND (current_qty > 0 OR month_qty > 0) ORDER BY warehouse_name`,
		whArgs...,
	)
	if !ok {
		return
	}
	warehouses := []string{}
	defer whRows.Close()
	for whRows.Next() {
		var wh string
		if writeDatabaseError(w, whRows.Scan(&wh)) {
			return
		}
		warehouses = append(warehouses, wh)
	}
	if writeDatabaseError(w, whRows.Err()) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code": 200,
		"data": map[string]interface{}{
			"summary":    summary,
			"items":      items,
			"warehouses": warehouses,
		},
	})
}
