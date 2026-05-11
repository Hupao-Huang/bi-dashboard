package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
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

// 季节指数粒度: SKU × 月份(1-12), 大区共用 (大区季节性差异小, 拆分到大区样本太稀疏)
// 公式: 季节系数[sku][m] = (该 SKU 历史月份 m 平均销量) / (该 SKU 全年月均)
//   > 1 = 旺季, < 1 = 淡季, = 1 = 中性
// 历史窗口: 预测月前 24 个月(2 个春节样本)
// 降级: 历史 < 6 月有数据 → 系数全 = 1.0(等于不调整)
// 截断: 系数 clamp 到 [0.3, 3.0] 防异常爆量(新品促销那种异常)
type seasonalMap map[string]map[int]float64

func computeOfflineSeasonalIndex(db *sql.DB, ym string) (seasonalMap, error) {
	s24, e24 := monthsBack(ym, 24)
	cateCond, cateArgs := offlineForecastCateCond()
	args := append([]interface{}{s24, e24}, cateArgs...)
	rows, err := db.Query(`
		SELECT goods_no,
			MONTH(stat_date) AS m,
			SUM(goods_qty) AS month_qty,
			COUNT(DISTINCT DATE_FORMAT(stat_date, '%Y-%m')) AS appear_yrs
		FROM sales_goods_summary
		WHERE department = 'offline'
			AND stat_date BETWEEN ? AND ?
			AND goods_no IS NOT NULL AND goods_no <> ''`+cateCond+`
		GROUP BY goods_no, MONTH(stat_date)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type cell struct {
		qty       float64
		appearYrs int // 该月份在 24 月窗口里出现的"年数"(1-2)
	}
	raw := map[string]map[int]cell{}
	for rows.Next() {
		var sku string
		var m, appear int
		var qty float64
		if err := rows.Scan(&sku, &m, &qty, &appear); err != nil {
			return nil, err
		}
		if raw[sku] == nil {
			raw[sku] = map[int]cell{}
		}
		raw[sku][m] = cell{qty: qty, appearYrs: appear}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sm := seasonalMap{}
	for sku, months := range raw {
		var totalQty float64
		var totalAppear int
		for _, c := range months {
			totalQty += c.qty
			totalAppear += c.appearYrs
		}
		sm[sku] = map[int]float64{}
		if totalAppear < 6 || totalQty <= 0 {
			// 历史数据不足, 全部中性
			for m := 1; m <= 12; m++ {
				sm[sku][m] = 1.0
			}
			continue
		}
		baseline := totalQty / float64(totalAppear) // 全年月均
		for m := 1; m <= 12; m++ {
			c, has := months[m]
			if !has || c.appearYrs == 0 {
				sm[sku][m] = 1.0
				continue
			}
			monthAvg := c.qty / float64(c.appearYrs)
			idx := monthAvg / baseline
			if idx < 0.3 {
				idx = 0.3
			}
			if idx > 3.0 {
				idx = 3.0
			}
			sm[sku][m] = idx
		}
	}
	return sm, nil
}

// applySeasonalAdjust 把"近 3 月均"按季节指数调整成"预测月建议值"
// 公式: 建议 = round(近3月均 / mean(近3月对应系数) × 预测月系数)
//   "近3月均 / mean(近3月对应系数)" = 去季节趋势(deseasonalized)
//   再 × 预测月系数 = 注入预测月的季节性
func applySeasonalAdjust(baseQty float64, recentMonths []int, predictMonth int, idx map[int]float64) float64 {
	if len(idx) == 0 {
		return baseQty
	}
	var sumRecent float64
	var cnt float64
	for _, m := range recentMonths {
		if v, has := idx[m]; has && v > 0 {
			sumRecent += v
			cnt++
		}
	}
	avgRecent := 1.0
	if cnt > 0 && sumRecent > 0 {
		avgRecent = sumRecent / cnt
	}
	predictIdx := 1.0
	if v, has := idx[predictMonth]; has && v > 0 {
		predictIdx = v
	}
	return math.Round(baseQty / avgRecent * predictIdx)
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

	// 0. 算 SKU × 月份(1-12) 季节指数
	seasIdx, err := computeOfflineSeasonalIndex(h.DB, ym)
	if writeDatabaseError(w, err) {
		return
	}
	// 预测月份 + 近 3 月对应的月份数字 (1-12)
	predictTime, _ := time.Parse("2006-01", ym)
	predictMonth := int(predictTime.Month())
	recentMonths := make([]int, 0, 3)
	for i := 1; i <= 3; i++ {
		recentMonths = append(recentMonths, int(predictTime.AddDate(0, -i, 0).Month()))
	}

	// 1. 近 3 月销量原始值 (按 SKU × 大区, 不四舍五入, 后面应用季节系数)
	s3, e3 := monthsBack(ym, 3)
	sugArgs := append([]interface{}{s3, e3}, cateArgs...)
	sugRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT goods_no AS sku_code,
			MAX(goods_name) AS goods_name,
			`+offlineForecastRegionExpr+` AS region,
			SUM(goods_qty) / 3.0 AS base_avg
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
	suggestions := map[cellKey]int{}        // 季节调整后的建议值
	baseAvgMap := map[cellKey]float64{}     // 近 3 月原始均值(tooltip 用)
	skuMeta := map[string]string{}          // sku_code → goods_name
	for sugRows.Next() {
		var sku, region string
		var goodsName sql.NullString
		var base float64
		if writeDatabaseError(w, sugRows.Scan(&sku, &goodsName, &region, &base)) {
			sugRows.Close()
			return
		}
		if base < 0 {
			base = 0
		}
		baseAvgMap[cellKey{sku, region}] = base

		// 应用季节指数
		adjusted := applySeasonalAdjust(base, recentMonths, predictMonth, seasIdx[sku])
		if adjusted < 0 {
			adjusted = 0
		}
		suggestions[cellKey{sku, region}] = int(adjusted)

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
		SkuCode        string             `json:"sku_code"`
		GoodsName      string             `json:"goods_name"`
		Suggestions    map[string]int     `json:"suggestions"`
		Forecasts      map[string]int     `json:"forecasts"`
		BaseAvgs       map[string]float64 `json:"base_avgs"`       // 近 3 月原始均值, tooltip 用
		SeasonalFactor float64            `json:"seasonal_factor"` // 预测月季节系数 (SKU 级)
	}
	items := make([]item, 0, len(skuSet))
	for sku := range skuSet {
		seasonal := 1.0
		if idx, has := seasIdx[sku]; has {
			if v, ok := idx[predictMonth]; ok {
				seasonal = math.Round(v*100) / 100
			}
		}
		it := item{
			SkuCode:        sku,
			GoodsName:      skuMeta[sku],
			Suggestions:    map[string]int{},
			Forecasts:      map[string]int{},
			BaseAvgs:       map[string]float64{},
			SeasonalFactor: seasonal,
		}
		for _, region := range offlineForecastRegions {
			if v, ok := suggestions[cellKey{sku, region}]; ok && v > 0 {
				it.Suggestions[region] = v
			}
			if v, ok := forecasts[cellKey{sku, region}]; ok {
				it.Forecasts[region] = v
			}
			if v, ok := baseAvgMap[cellKey{sku, region}]; ok && v > 0 {
				it.BaseAvgs[region] = math.Round(v*10) / 10
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
