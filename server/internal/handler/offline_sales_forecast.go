package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// 线下 9 大区白名单（前后端共用,与 dashboard_department.go 的 offlineRegionExpr 对齐）
var offlineForecastRegions = []string{
	"华北大区", "华东大区", "华中大区", "华南大区",
	"西南大区", "西北大区", "东北大区", "山东大区", "重客",
}

// offlineForecastCateCond 仅看成品 — 复用 supply_chain.go 的 planCategories 10 品类白名单
// (调味料/酱油/调味汁/干制面/素蚝油/酱类/醋/汤底/番茄沙司/糖)
// 排除: 广宣品 / 快递包材 / 成品礼盒 / 半保产品 / 测试 等非成品品类
// 改这里没用,改 supply_chain.go planCategories 才是单一真相源
func offlineForecastCateCond() (string, []interface{}) {
	if len(planCategories) == 0 {
		return "", nil
	}
	args := make([]interface{}, len(planCategories))
	holders := make([]string, len(planCategories))
	for i, c := range planCategories {
		args[i] = c
		holders[i] = "?"
	}
	return ` AND cate_name IN (` + strings.Join(holders, ",") + `)`, args
}

// shop_name → 大区 的 CASE WHEN 表达式（与 dashboard_department.go 保持一致）
const offlineForecastRegionExpr = `CASE
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

// validYM 校验 YYYY-MM 月份格式
func validYM(ym string) bool {
	if len(ym) != 7 {
		return false
	}
	_, err := time.Parse("2006-01", ym)
	return err == nil
}

// monthsBack 从指定月份往前推 n 个月,返回起止日期 (YYYY-MM-01 ~ YYYY-MM-末日)
func monthsBack(targetYM string, n int) (string, string) {
	t, _ := time.Parse("2006-01", targetYM)
	start := t.AddDate(0, -n, 0)
	end := t.AddDate(0, 0, -1) // targetYM 前一天
	return start.Format("2006-01-02"), end.Format("2006-01-02")
}

// GET /api/offline/sales-forecast?ym=2026-06&range=recent6m|all
// 返回: { ym, regions, items: [{sku_code, goods_name, suggestions{region→qty}, forecasts{region→qty}}] }
// suggestions = 近 3 个月在该 SKU×大区 实际发货量均值（系统建议值）
// forecasts   = 该月已保存的预测值（用户填的）
func (h *DashboardHandler) GetOfflineSalesForecast(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	ym := r.URL.Query().Get("ym")
	if !validYM(ym) {
		writeError(w, 400, "ym 格式错误(YYYY-MM)")
		return
	}
	rangeMode := r.URL.Query().Get("range")
	if rangeMode == "" {
		rangeMode = "recent6m"
	}

	cateCond, cateArgs := offlineForecastCateCond()

	// 1. 系统建议值 — 近 3 个月线下部门各 SKU×大区 实际销量均值 (仅成品)
	s3, e3 := monthsBack(ym, 3)
	sugArgs := append([]interface{}{s3, e3}, cateArgs...)
	sugRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT goods_no AS sku_code,
			MAX(goods_name) AS goods_name,
			`+offlineForecastRegionExpr+` AS region,
			ROUND(SUM(goods_qty) / 3.0, 0) AS avg_qty
		FROM sales_goods_summary
		WHERE department = 'offline'
			AND stat_date BETWEEN ? AND ?
			AND goods_no IS NOT NULL AND goods_no <> ''
			AND `+offlineForecastRegionExpr+` IS NOT NULL`+cateCond+`
		GROUP BY goods_no, region`, sugArgs...)
	if !ok {
		return
	}
	type cellKey struct {
		sku    string
		region string
	}
	suggestions := map[cellKey]int{}
	skuMeta := map[string]string{} // sku_code → goods_name
	for sugRows.Next() {
		var sku, region string
		var goodsName sql.NullString
		var qty float64
		if writeDatabaseError(w, sugRows.Scan(&sku, &goodsName, &region, &qty)) {
			sugRows.Close()
			return
		}
		if qty < 0 {
			qty = 0
		}
		suggestions[cellKey{sku, region}] = int(qty)
		if goodsName.Valid && skuMeta[sku] == "" {
			skuMeta[sku] = goodsName.String
		}
	}
	sugRows.Close()
	if writeDatabaseError(w, sugRows.Err()) {
		return
	}

	// 2. SKU 全集 — 近 6 个月销过的 SKU(默认),或全部 SKU
	skuSet := map[string]struct{}{}
	if rangeMode == "all" {
		// 近 12 个月内出现过的 SKU (避免几千行无销量历史 SKU 也铺出来)
		s12, e12 := monthsBack(ym, 12)
		args12 := append([]interface{}{s12, e12}, cateArgs...)
		skuRows, ok2 := queryRowsOrWriteError(w, h.DB, `
			SELECT DISTINCT goods_no, MAX(goods_name)
			FROM sales_goods_summary
			WHERE department = 'offline' AND stat_date BETWEEN ? AND ?
				AND goods_no IS NOT NULL AND goods_no <> ''`+cateCond+`
			GROUP BY goods_no`, args12...)
		if !ok2 {
			return
		}
		for skuRows.Next() {
			var sku string
			var name sql.NullString
			if writeDatabaseError(w, skuRows.Scan(&sku, &name)) {
				skuRows.Close()
				return
			}
			skuSet[sku] = struct{}{}
			if name.Valid && skuMeta[sku] == "" {
				skuMeta[sku] = name.String
			}
		}
		skuRows.Close()
	} else {
		// 近 6 个月
		s6, e6 := monthsBack(ym, 6)
		args6 := append([]interface{}{s6, e6}, cateArgs...)
		skuRows, ok2 := queryRowsOrWriteError(w, h.DB, `
			SELECT DISTINCT goods_no, MAX(goods_name)
			FROM sales_goods_summary
			WHERE department = 'offline' AND stat_date BETWEEN ? AND ?
				AND goods_no IS NOT NULL AND goods_no <> ''`+cateCond+`
			GROUP BY goods_no`, args6...)
		if !ok2 {
			return
		}
		for skuRows.Next() {
			var sku string
			var name sql.NullString
			if writeDatabaseError(w, skuRows.Scan(&sku, &name)) {
				skuRows.Close()
				return
			}
			skuSet[sku] = struct{}{}
			if name.Valid && skuMeta[sku] == "" {
				skuMeta[sku] = name.String
			}
		}
		skuRows.Close()
	}

	// 3. 已保存的预测值
	forecasts := map[cellKey]int{}
	fRows, ok3 := queryRowsOrWriteError(w, h.DB, `
		SELECT sku_code, region, forecast_qty, goods_name
		FROM offline_sales_forecast WHERE ym = ?`, ym)
	if !ok3 {
		return
	}
	for fRows.Next() {
		var sku, region string
		var qty int
		var name sql.NullString
		if writeDatabaseError(w, fRows.Scan(&sku, &region, &qty, &name)) {
			fRows.Close()
			return
		}
		forecasts[cellKey{sku, region}] = qty
		skuSet[sku] = struct{}{} // 已存预测的 SKU 也要进列表
		if name.Valid && skuMeta[sku] == "" {
			skuMeta[sku] = name.String
		}
	}
	fRows.Close()

	// 4. 组装返回
	type item struct {
		SkuCode     string         `json:"sku_code"`
		GoodsName   string         `json:"goods_name"`
		Suggestions map[string]int `json:"suggestions"`
		Forecasts   map[string]int `json:"forecasts"`
	}
	items := make([]item, 0, len(skuSet))
	for sku := range skuSet {
		it := item{
			SkuCode:     sku,
			GoodsName:   skuMeta[sku],
			Suggestions: map[string]int{},
			Forecasts:   map[string]int{},
		}
		for _, region := range offlineForecastRegions {
			if v, ok := suggestions[cellKey{sku, region}]; ok && v > 0 {
				it.Suggestions[region] = v
			}
			if v, ok := forecasts[cellKey{sku, region}]; ok {
				it.Forecasts[region] = v
			}
		}
		items = append(items, it)
	}
	// 简单按货品名排序,稳定显示
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if strings.Compare(items[i].GoodsName, items[j].GoodsName) > 0 {
				items[i], items[j] = items[j], items[i]
			}
		}
	}

	writeJSON(w, map[string]interface{}{
		"ym":      ym,
		"regions": offlineForecastRegions,
		"items":   items,
	})
}

// POST /api/offline/sales-forecast/save
// body: {"ym":"2026-06","items":[{"sku_code":"xxx","goods_name":"减钠鲜酱油","region":"华北大区","forecast_qty":100}, ...]}
func (h *DashboardHandler) SaveOfflineSalesForecast(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	var req struct {
		YM    string `json:"ym"`
		Items []struct {
			SkuCode     string `json:"sku_code"`
			GoodsName   string `json:"goods_name"`
			Region      string `json:"region"`
			ForecastQty int    `json:"forecast_qty"`
		} `json:"items"`
	}
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	if err := dec.Decode(&req); err != nil {
		writeError(w, 400, "参数错误: "+err.Error())
		return
	}
	if !validYM(req.YM) {
		writeError(w, 400, "ym 格式错误(YYYY-MM)")
		return
	}

	// 大区白名单校验
	allowedRegion := map[string]bool{}
	for _, r := range offlineForecastRegions {
		allowedRegion[r] = true
	}

	operator := r.Header.Get("X-User-Name")
	if operator == "" {
		operator = "unknown"
	}

	tx, err := h.DB.Begin()
	if writeDatabaseError(w, err) {
		return
	}
	defer tx.Rollback()

	saved := 0
	for _, it := range req.Items {
		if it.SkuCode == "" || !allowedRegion[it.Region] {
			continue
		}
		if it.ForecastQty < 0 {
			it.ForecastQty = 0
		}
		_, err := tx.Exec(`
			INSERT INTO offline_sales_forecast (ym, region, sku_code, goods_name, forecast_qty, operator)
			VALUES (?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				forecast_qty = VALUES(forecast_qty),
				goods_name = VALUES(goods_name),
				operator = VALUES(operator),
				updated_at = NOW()`,
			req.YM, it.Region, it.SkuCode, it.GoodsName, it.ForecastQty, operator)
		if writeDatabaseError(w, err) {
			return
		}
		saved++
	}
	if writeDatabaseError(w, tx.Commit()) {
		return
	}
	writeJSON(w, map[string]interface{}{
		"message": fmt.Sprintf("保存成功(%d 条)", saved),
		"saved":   saved,
	})
}
