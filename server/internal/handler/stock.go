package handler

import (
	"encoding/json"
	"math"
	"net/http"
)

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

	// 1. 预警统计卡片（按SKU+仓库维度）
	var stockout, urgent, low, overstock, dead, total int
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
			SUM(CASE WHEN month_qty = 0 AND current_qty > 0 THEN 1 ELSE 0 END)
		FROM stock_quantity WHERE goods_attr = 1 AND warehouse_name != ''`+warehouseCond,
		warehouseArgs...,
	).Scan(&total, &stockout, &urgent, &low, &overstock, &dead); err != nil {
		writeError(w, 500, "database query failed")
		return
	}

	summary := map[string]int{
		"total": total, "stockout": stockout, "urgent": urgent,
		"low": low, "overstock": overstock, "dead": dead,
	}

	// 2. 查明细数据
	query := `
		SELECT goods_no, goods_name, unit_name,
			warehouse_name,
			ROUND(current_qty - locked_qty, 2) AS usable_qty,
			month_qty,
			ROUND(month_qty / 30, 1) AS daily_avg,
			CASE
				WHEN month_qty > 0 AND (current_qty - locked_qty) <= 0 THEN -1
				WHEN month_qty > 0 THEN ROUND((current_qty - locked_qty) / (month_qty/30), 1)
				WHEN current_qty > 0 THEN 9999
				ELSE 0
			END AS sellable_days,
			current_qty
		FROM stock_quantity
		WHERE goods_attr = 1 AND warehouse_name != ''
	`
	query += warehouseCond
	args := append([]interface{}{}, warehouseArgs...)
	if keyword != "" {
		query += " AND (goods_no LIKE ? OR goods_name LIKE ?)"
		kw := "%" + keyword + "%"
		args = append(args, kw, kw)
	}

	switch warning {
	case "stockout":
		query += " AND (current_qty - locked_qty) <= 0 AND month_qty > 0"
	case "urgent":
		query += " AND (current_qty - locked_qty) > 0 AND month_qty > 0 AND (current_qty - locked_qty) / (month_qty/30) < 7"
	case "low":
		query += " AND (current_qty - locked_qty) > 0 AND month_qty > 0 AND (current_qty - locked_qty) / (month_qty/30) BETWEEN 7 AND 14"
	case "overstock":
		query += " AND (current_qty - locked_qty) > 0 AND month_qty > 0 AND (current_qty - locked_qty) / (month_qty/30) > 90"
	case "dead":
		query += " AND month_qty = 0 AND current_qty > 0"
	default:
		query += " AND (current_qty > 0 OR month_qty > 0)"
	}

	query += " ORDER BY CASE WHEN month_qty > 0 AND (current_qty - locked_qty) <= 0 THEN 0 WHEN month_qty > 0 THEN (current_qty - locked_qty) / (month_qty/30) ELSE 99999 END ASC LIMIT 2000"

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
	}

	rawItems := []RawItem{}
	for rows.Next() {
		var item RawItem
		if writeDatabaseError(w, rows.Scan(&item.GoodsNo, &item.GoodsName, &item.UnitName,
			&item.WarehouseName, &item.UsableQty,
			&item.MonthQty, &item.DailyAvg, &item.SellableDays, &item.CurrentQty)) {
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
				Warehouse: r.WarehouseName, UsableQty: r.UsableQty,
				SellableDays: r.SellableDays, DailyAvg: r.DailyAvg,
				MonthQty: r.MonthQty, CurrentQty: r.CurrentQty,
			})
		}
		writeStockResponse(w, summary, flat, h, warehouseScopeCond, warehouseScopeArgs)
		return
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
		UsableQty    float64     `json:"usableQty"`
		SellableDays float64     `json:"sellableDays"`
		DailyAvg     float64     `json:"dailyAvg"`
		MonthQty     float64     `json:"monthQty"`
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
			agg = &AggItem{GoodsNo: r.GoodsNo, GoodsName: r.GoodsName}
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
			if agg.CurrentQty > 0 {
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

	writeStockResponse(w, summary, result, h, warehouseScopeCond, warehouseScopeArgs)
}

func writeStockResponse(
	w http.ResponseWriter,
	summary map[string]int,
	items interface{},
	h *DashboardHandler,
	warehouseScopeCond string,
	warehouseScopeArgs []interface{},
) {
	whRows, ok := queryRowsOrWriteError(
		w,
		h.DB,
		`SELECT DISTINCT warehouse_name FROM stock_quantity WHERE goods_attr = 1 AND warehouse_name != ''`+warehouseScopeCond+` AND (current_qty > 0 OR month_qty > 0) ORDER BY warehouse_name`,
		warehouseScopeArgs...,
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
