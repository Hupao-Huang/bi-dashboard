package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
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

// 农历春节日期(近 6 年)
// 现有算法盲区: 春节有时落 1 月有时落 2 月, 直接按"自然月"算系数会把"春节当月"和"春节前月"混在一起
// 修正: 算 1/2 月系数时, 只看"春节落点跟预测年同月"的历史年份, 同口径对比
var springFestivalDates = map[int]string{
	2023: "2023-01-22",
	2024: "2024-02-10",
	2025: "2025-01-29",
	2026: "2026-02-17",
	2027: "2027-02-06",
	2028: "2028-01-26",
	2029: "2029-02-13",
	2030: "2030-02-03",
}

// springFestivalMonth 返回某年春节所在月份, 不在表中返回 0
func springFestivalMonth(year int) int {
	if s, ok := springFestivalDates[year]; ok {
		t, err := time.Parse("2006-01-02", s)
		if err == nil {
			return int(t.Month())
		}
	}
	return 0
}

// 季节指数粒度: SKU × 月份(1-12), 大区共用 (大区季节性差异小, 拆分到大区样本太稀疏)
// 公式: 季节系数[sku][m] = (该 SKU 历史月份 m 平均销量) / (该 SKU 全年月均)
//   > 1 = 旺季, < 1 = 淡季, = 1 = 中性
// 历史窗口: 预测月前 24 个月(2 个春节样本)
// 降级: 历史 < 6 月有数据 → 系数全 = 1.0(等于不调整)
// 截断: 系数 clamp 到 [0.3, 3.0] 防异常爆量(新品促销那种异常)
type seasonalMap map[string]map[int]float64

// replacedMap[sku][month] = true 表示该格用了品类中位数替代 (营销污染), false = 保留单品自身
type replacedMap map[string]map[int]bool

// 客观度污染阈值 — 单 SKU 同月份 2 年销量波动 >30% 视为"营销污染", 换用品类中位数
const objectivePollutionThreshold = 0.30

// computeOfflineRegionMoM 大区环比加速度 — 近 1 月销量 ÷ 近 3 月均
// 反映最近月份是上升趋势 (>1) 还是下降 (<1)
// clamp [0.85, 1.15] 防春节那种异常月带飞 (异常月用季节系数 + 春节修正已处理)
func computeOfflineRegionMoM(db *sql.DB, ym string) (map[string]float64, error) {
	curr3Start, curr3End := monthsBack(ym, 3)
	curr1Start, curr1End := monthsBack(ym, 1)
	cateCond, cateArgs := offlineForecastCateCond()
	args := []interface{}{curr1Start, curr1End, curr3Start, curr3End, curr3Start, curr3End}
	args = append(args, cateArgs...)

	rows, err := db.Query(`
		SELECT region,
			SUM(CASE WHEN stat_date BETWEEN ? AND ? THEN goods_qty ELSE 0 END) AS last1_qty,
			SUM(CASE WHEN stat_date BETWEEN ? AND ? THEN goods_qty ELSE 0 END) AS last3_qty
		FROM (
			SELECT goods_qty, stat_date, `+offlineForecastRegionExpr+` AS region
			FROM sales_goods_summary
			WHERE department='offline'
				AND stat_date BETWEEN ? AND ?`+cateCond+`
		) t
		WHERE region IS NOT NULL
		GROUP BY region`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	mom := map[string]float64{}
	for rows.Next() {
		var region string
		var last1, last3 float64
		if err := rows.Scan(&region, &last1, &last3); err != nil {
			return nil, err
		}
		v := 1.0
		if last3 > 0 {
			m3avg := last3 / 3.0
			if m3avg > 0 {
				v = last1 / m3avg
				// clamp ±8% — 环比对季节异常月(春节回声)敏感, 收紧防带飞
				if v < 0.92 {
					v = 0.92
				}
				if v > 1.08 {
					v = 1.08
				}
			}
		}
		mom[region] = v
	}
	return mom, rows.Err()
}

// computeOfflineRegionGrowth 算每个大区"近 3 月销量"vs"去年同期 3 月"的同比增长率
// 用于捕获年度业务扩张趋势, 防止季节系数算法低估增长期
// clamp 到 [0.7, 1.6] 防异常 (单月暴增/暴跌不影响)
func computeOfflineRegionGrowth(db *sql.DB, ym string) (map[string]float64, error) {
	// 当年近 3 月 = ym 前 3 月
	currStart, currEnd := monthsBack(ym, 3)
	cateCond, cateArgs := offlineForecastCateCond()
	// SQL 中 ? 顺序: 4 个 CASE SUM 用, 2 个子查询 stat_date 用, 然后 cateArgs
	args := []interface{}{currStart, currEnd, currStart, currEnd, currStart, currEnd}
	args = append(args, cateArgs...)

	rows, err := db.Query(`
		SELECT region,
			SUM(CASE WHEN stat_date BETWEEN ? AND ? THEN goods_qty ELSE 0 END) AS curr_qty,
			SUM(CASE WHEN stat_date BETWEEN DATE_SUB(?, INTERVAL 1 YEAR) AND DATE_SUB(?, INTERVAL 1 YEAR) THEN goods_qty ELSE 0 END) AS prev_qty
		FROM (
			SELECT goods_qty, stat_date, `+offlineForecastRegionExpr+` AS region
			FROM sales_goods_summary
			WHERE department='offline'
				AND stat_date BETWEEN DATE_SUB(?, INTERVAL 1 YEAR) AND ?`+cateCond+`
		) t
		WHERE region IS NOT NULL
		GROUP BY region`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	growth := map[string]float64{}
	for rows.Next() {
		var region string
		var curr, prev float64
		if err := rows.Scan(&region, &curr, &prev); err != nil {
			return nil, err
		}
		g := 1.0
		if prev > 0 {
			g = curr / prev
			// clamp 到 [0.85, 1.30] 防异常 — Q1 增长率推 4 月易过激, 限制 ±30% 内
			if g < 0.85 {
				g = 0.85
			}
			if g > 1.30 {
				g = 1.30
			}
		}
		growth[region] = g
	}
	return growth, rows.Err()
}

// v1.50: 算法升级 — 客观度判定 + 品类中位数替代
// 单 SKU 自身月度系数受促销/新品/异常事件污染 → 用同品类下"客观稳定 SKU"的月份系数中位数替代
// "客观稳定" = 该 SKU 该月份 同月 2 年销量波动 < 30%
func computeOfflineSeasonalIndex(db *sql.DB, ym string) (seasonalMap, replacedMap, error) {
	predictTime, _ := time.Parse("2006-01", ym)
	predictYear := predictTime.Year()
	predictSpringMonth := springFestivalMonth(predictYear) // 1 / 2 / 0

	s24, e24 := monthsBack(ym, 24)
	cateCond, cateArgs := offlineForecastCateCond()
	args := append([]interface{}{s24, e24}, cateArgs...)

	rows, err := db.Query(`
		SELECT goods_no,
			cate_name,
			YEAR(stat_date) AS y,
			MONTH(stat_date) AS m,
			SUM(goods_qty) AS month_qty
		FROM sales_goods_summary
		WHERE department = 'offline'
			AND stat_date BETWEEN ? AND ?
			AND goods_no IS NOT NULL AND goods_no <> ''`+cateCond+`
		GROUP BY goods_no, cate_name, YEAR(stat_date), MONTH(stat_date)`, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	type ymKey struct{ y, m int }
	rawByYM := map[string]map[ymKey]float64{} // sku → (year, month) → qty
	skuCate := map[string]string{}            // sku → cate_name
	for rows.Next() {
		var sku string
		var cate sql.NullString
		var y, m int
		var qty float64
		if err := rows.Scan(&sku, &cate, &y, &m, &qty); err != nil {
			return nil, nil, err
		}
		if rawByYM[sku] == nil {
			rawByYM[sku] = map[ymKey]float64{}
		}
		rawByYM[sku][ymKey{y, m}] = qty
		if cate.Valid && skuCate[sku] == "" {
			skuCate[sku] = cate.String
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	// 第 1 遍: 算每个 SKU × MONTH 的"单品原始系数"+ 同月 2 年波动率
	type cellData struct {
		rawIdx float64
		fluc   float64 // 同月不同年波动率
		multi  bool    // 是否有多年样本(>=2)
	}
	rawIdxMap := map[string]map[int]cellData{}
	for sku, ymQty := range rawByYM {
		var totalQty float64
		totalAppear := len(ymQty)
		for _, q := range ymQty {
			totalQty += q
		}
		rawIdxMap[sku] = map[int]cellData{}
		if totalAppear < 6 || totalQty <= 0 {
			for m := 1; m <= 12; m++ {
				rawIdxMap[sku][m] = cellData{rawIdx: 1.0}
			}
			continue
		}
		baseline := totalQty / float64(totalAppear)
		for m := 1; m <= 12; m++ {
			var sumQty float64
			var cnt int
			var yrQtys []float64
			for k, q := range ymQty {
				if k.m != m {
					continue
				}
				// 春节修正(1/2 月)
				if (m == 1 || m == 2) && predictSpringMonth != 0 {
					yrSpring := springFestivalMonth(k.y)
					if yrSpring != 0 && yrSpring != predictSpringMonth {
						continue
					}
				}
				sumQty += q
				cnt++
				yrQtys = append(yrQtys, q)
			}
			if cnt == 0 {
				rawIdxMap[sku][m] = cellData{rawIdx: 1.0}
				continue
			}
			monthAvg := sumQty / float64(cnt)
			idx := monthAvg / baseline
			if idx < 0.3 {
				idx = 0.3
			}
			if idx > 3.0 {
				idx = 3.0
			}
			// 同月 2 年波动率
			var fluc float64
			if len(yrQtys) >= 2 {
				sort.Float64s(yrQtys)
				low := yrQtys[0]
				high := yrQtys[len(yrQtys)-1]
				if low > 0 {
					fluc = (high - low) / low
				}
			}
			rawIdxMap[sku][m] = cellData{rawIdx: idx, fluc: fluc, multi: len(yrQtys) >= 2}
		}
	}

	// 第 2 遍: 按品类计算"客观稳定 SKU 的月份系数中位数" (波动 < 30% 的 SKU 才入样本)
	cateSkus := map[string][]string{}
	for sku, cate := range skuCate {
		cateSkus[cate] = append(cateSkus[cate], sku)
	}
	cateMedianIdx := map[string]map[int]float64{}
	for cate, skus := range cateSkus {
		cateMedianIdx[cate] = map[int]float64{}
		for m := 1; m <= 12; m++ {
			var samples []float64
			for _, sku := range skus {
				d := rawIdxMap[sku][m]
				if d.rawIdx > 0 && d.fluc < objectivePollutionThreshold {
					samples = append(samples, d.rawIdx)
				}
			}
			if len(samples) >= 3 {
				sort.Float64s(samples)
				mid := samples[len(samples)/2]
				cateMedianIdx[cate][m] = mid
			} else {
				cateMedianIdx[cate][m] = 1.0 // 客观样本不足,中性
			}
		}
	}

	// 第 3 遍: 应用替换规则 — 单品污染月用品类中位数; 客观月保留单品自身
	sm := seasonalMap{}
	rm := replacedMap{}
	for sku, monthsData := range rawIdxMap {
		sm[sku] = map[int]float64{}
		rm[sku] = map[int]bool{}
		cate := skuCate[sku]
		for m := 1; m <= 12; m++ {
			d := monthsData[m]
			if d.multi && d.fluc > objectivePollutionThreshold {
				if v, ok := cateMedianIdx[cate][m]; ok && v > 0 {
					sm[sku][m] = v
				} else {
					sm[sku][m] = 1.0
				}
				rm[sku][m] = true
			} else {
				sm[sku][m] = d.rawIdx
				rm[sku][m] = false
			}
		}
	}
	return sm, rm, nil
}

// holidayContext 给前端展示用 — 该月份是否含中国传统节假日
// 用于 SKU Tag 旁边的提示文字 (业务可以参考人工微调)
func holidayContext(year, month int) string {
	parts := []string{}
	// 春节
	if sm := springFestivalMonth(year); sm == month {
		parts = append(parts, "春节假期")
	} else if sm := springFestivalMonth(year); sm-1 == month || (sm == 1 && month == 12 && year == 2029-1) {
		parts = append(parts, "春节囤货")
	}
	// 固定假期
	switch month {
	case 1:
		parts = append(parts, "元旦")
	case 4:
		parts = append(parts, "清明")
	case 5:
		parts = append(parts, "五一")
	case 6:
		parts = append(parts, "端午") // 多数年份在 6 月
	case 9:
		parts = append(parts, "中秋") // 多数年份在 9 月
	case 10:
		parts = append(parts, "国庆")
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "/")
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
	algo := r.URL.Query().Get("algo") // "" (= auto) / builtin / prophet / statsforecast / auto
	if algo == "" {
		algo = "auto"
	}
	// v1.63 智能路由 — 改成数据驱动: 看历史回测 MAPE, 选最准的算法
	// 候选: prophet / statsforecast (前端业务能切的 ML 算法 + 已有回测数据)
	// fallback: 没回测数据 → 按月份硬编码 (1/2 月 Prophet 春节先验, 其他 SF)
	autoReason := ""
	if algo == "auto" {
		algo, autoReason = chooseAutoAlgo(h.DB, ym)
	}

	cateCond, cateArgs := offlineForecastCateCond()

	// 如果选 prophet / statsforecast 算法, 拉大区合计预测做大区增长锚点
	mlRegionQty := map[string]float64{}
	if algo == "prophet" {
		prRows, prErr := h.DB.Query(`SELECT region, forecast_qty FROM offline_sales_forecast_prophet WHERE ym = ?`, ym)
		if prErr == nil {
			for prRows.Next() {
				var rg string
				var q int
				if scanErr := prRows.Scan(&rg, &q); scanErr == nil {
					mlRegionQty[rg] = float64(q)
				}
			}
			prRows.Close()
		}
	} else if algo == "statsforecast" {
		sfRows, sfErr := h.DB.Query(`SELECT region, forecast_qty FROM offline_sales_forecast_statsforecast WHERE ym = ?`, ym)
		if sfErr == nil {
			for sfRows.Next() {
				var rg string
				var q int
				if scanErr := sfRows.Scan(&rg, &q); scanErr == nil {
					mlRegionQty[rg] = float64(q)
				}
			}
			sfRows.Close()
		}
	}

	// 0. 算 SKU × 月份(1-12) 季节指数 + 客观度标识 + 大区同比增长率
	seasIdx, replaced, err := computeOfflineSeasonalIndex(h.DB, ym)
	if writeDatabaseError(w, err) {
		return
	}
	regionGrowth, err := computeOfflineRegionGrowth(h.DB, ym)
	if writeDatabaseError(w, err) {
		return
	}
	regionMoM, err := computeOfflineRegionMoM(h.DB, ym)
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
	// 环比启用判定: 近 1 月 (= recentMonths[0]) 不能落 1/2/3 月
	// 春节直接影响近 1 月才会反向带飞; 近 3 月含但近 1 月不含 (如 5/6 月预测) 时启用环比仍有效
	useMoM := true
	if len(recentMonths) > 0 {
		near1 := recentMonths[0]
		if near1 >= 1 && near1 <= 3 {
			useMoM = false
		}
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

		// 应用季节指数 + 大区同比 (年度扩张) + 大区环比 (短期趋势加速)
		adjusted := applySeasonalAdjust(base, recentMonths, predictMonth, seasIdx[sku])
		if g, ok := regionGrowth[region]; ok && g > 0 {
			adjusted *= g
		}
		if useMoM {
			if m, ok := regionMoM[region]; ok && m > 0 {
				adjusted *= m
			}
		}
		if adjusted < 0 {
			adjusted = 0
		}
		// Prophet 缓存于后:此处先按内置算法记 raw, 后面如果 algo=prophet 会按大区总量重新校准
		suggestions[cellKey{sku, region}] = int(math.Round(adjusted))

		if goodsName.Valid && skuMeta[sku] == "" {
			skuMeta[sku] = goodsName.String
		}
	}
	sugRows.Close()
	if writeDatabaseError(w, sugRows.Err()) {
		return
	}

	// ML 算法 (Prophet / StatsForecast): 按大区合计校准, 保留 SKU 间相对比例
	if (algo == "prophet" || algo == "statsforecast") && len(mlRegionQty) > 0 {
		regionRawSum := map[string]int{}
		for k, v := range suggestions {
			regionRawSum[k.region] += v
		}
		for k, v := range suggestions {
			rawSum := regionRawSum[k.region]
			ml, has := mlRegionQty[k.region]
			if rawSum > 0 && has && ml > 0 {
				suggestions[k] = int(math.Round(float64(v) * ml / float64(rawSum)))
			}
		}
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
		SkuCode          string             `json:"sku_code"`
		GoodsName        string             `json:"goods_name"`
		Suggestions      map[string]int     `json:"suggestions"`
		Forecasts        map[string]int     `json:"forecasts"`
		BaseAvgs         map[string]float64 `json:"base_avgs"`          // 近 3 月原始均值, tooltip 用
		SeasonalFactor   float64            `json:"seasonal_factor"`    // 预测月季节系数 (SKU 级)
		RecentSeasonAvg  float64            `json:"recent_season_avg"`  // 近 3 月对应系数均值, tooltip 用
		SeasonalReplaced bool               `json:"seasonal_replaced"`  // 预测月系数是否被品类中位数替代(营销污染)
	}
	predictYear := predictTime.Year()
	holiday := holidayContext(predictYear, predictMonth)
	items := make([]item, 0, len(skuSet))
	for sku := range skuSet {
		seasonal := 1.0
		recentAvg := 1.0
		if idx, has := seasIdx[sku]; has {
			if v, ok := idx[predictMonth]; ok {
				seasonal = math.Round(v*100) / 100
			}
			var sum float64
			var cnt float64
			for _, m := range recentMonths {
				if v, ok := idx[m]; ok && v > 0 {
					sum += v
					cnt++
				}
			}
			if cnt > 0 && sum > 0 {
				recentAvg = math.Round(sum/cnt*100) / 100
			}
		}
		replacedThis := false
		if rm, has := replaced[sku]; has {
			replacedThis = rm[predictMonth]
		}
		it := item{
			SkuCode:          sku,
			GoodsName:        skuMeta[sku],
			Suggestions:      map[string]int{},
			Forecasts:        map[string]int{},
			BaseAvgs:         map[string]float64{},
			SeasonalFactor:   seasonal,
			RecentSeasonAvg:  recentAvg,
			SeasonalReplaced: replacedThis,
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

	// 大区同比 + 环比 (前端 tooltip 展示)
	growthOut := map[string]float64{}
	for r, g := range regionGrowth {
		growthOut[r] = math.Round(g*100) / 100
	}
	momOut := map[string]float64{}
	for r, v := range regionMoM {
		momOut[r] = math.Round(v*100) / 100
	}

	writeJSON(w, map[string]interface{}{
		"ym":               ym,
		"regions":          offlineForecastRegions,
		"items":            items,
		"holiday_context":  holiday,
		"region_growth":    growthOut,
		"region_mom":       momOut,
		"effective_algo":   algo,        // 智能模式下实际使用的算法
		"effective_reason": autoReason,  // v1.63 智能路由的选择理由 (空表示非智能模式)
	})
}

// POST /api/offline/sales-forecast/clear?ym=2026-06
// 删除指定月份的全部预测 (清空操作), 用于业务从零开始重新预测
func (h *DashboardHandler) ClearOfflineSalesForecast(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	ym := r.URL.Query().Get("ym")
	if !validYM(ym) {
		writeError(w, 400, "ym 格式错误(YYYY-MM)")
		return
	}
	res, err := h.DB.Exec(`DELETE FROM offline_sales_forecast WHERE ym = ?`, ym)
	if writeDatabaseError(w, err) {
		return
	}
	deleted, _ := res.RowsAffected()
	writeJSON(w, map[string]interface{}{
		"message": fmt.Sprintf("已清空(%d 条)", deleted),
		"deleted": deleted,
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

// chooseAutoAlgo v1.63 智能路由 — 数据驱动版
// 根据 offline_sales_forecast_backtest 表的历史 MAPE 选最准的算法
// 候选: prophet / statsforecast (前端业务能切的 ML 算法 + 已有回测)
// 策略:
//   1. 优先看预测月份对应的"同月份"历史 MAPE (例: 预测 2026-05 → 看历史所有 5 月的 MAPE)
//   2. 同月份样本不够 (<2 算法) → 退回看全部历史 MAPE
//   3. 全部历史也没数据 → fallback 按月份硬编码 (1/2 月 Prophet, 其他 SF)
func chooseAutoAlgo(db *sql.DB, ym string) (algo string, reason string) {
	type cand struct {
		algo    string
		mape    float64
		samples int
	}

	queryMape := func(sql string, args ...interface{}) []cand {
		rows, err := db.Query(sql, args...)
		if err != nil {
			return nil
		}
		defer rows.Close()
		var out []cand
		for rows.Next() {
			var c cand
			if err := rows.Scan(&c.algo, &c.mape, &c.samples); err == nil {
				out = append(out, c)
			}
		}
		return out
	}

	// 1) 先看预测月份对应的"同月份"历史 (例: 预测 5 月 → 看历史所有 5 月)
	t, terr := time.Parse("2006-01", ym)
	if terr == nil {
		mm := fmt.Sprintf("%02d", int(t.Month()))
		sameMonthCands := queryMape(`
			SELECT algo, AVG(abs_err_pct) AS mape, COUNT(*) AS samples
			FROM offline_sales_forecast_backtest
			WHERE algo IN ('prophet','statsforecast')
			  AND SUBSTRING(ym, 6, 2) = ?
			GROUP BY algo
			ORDER BY mape ASC`, mm)
		if len(sameMonthCands) >= 2 {
			best := sameMonthCands[0]
			return best.algo, fmt.Sprintf("基于历史同月 (%s 月) MAPE 选: %s 最准 (MAPE %.1f%%, %d 条样本)",
				mm, algoLabelCN(best.algo), best.mape, best.samples)
		}
	}

	// 2) 看全部历史 MAPE
	allCands := queryMape(`
		SELECT algo, AVG(abs_err_pct) AS mape, COUNT(*) AS samples
		FROM offline_sales_forecast_backtest
		WHERE algo IN ('prophet','statsforecast')
		GROUP BY algo
		ORDER BY mape ASC`)
	if len(allCands) >= 2 {
		best := allCands[0]
		return best.algo, fmt.Sprintf("基于全部历史回测 MAPE 选: %s 最准 (MAPE %.1f%%, %d 条样本)",
			algoLabelCN(best.algo), best.mape, best.samples)
	}

	// 3) Fallback: 没回测数据 → 按月份硬编码
	if terr == nil {
		m := int(t.Month())
		if m == 1 || m == 2 {
			return "prophet", "兜底规则: 1/2 月按春节先验选 Prophet (暂无回测数据可参考)"
		}
	}
	return "statsforecast", "兜底规则: 默认 StatsForecast (暂无回测数据可参考)"
}

func algoLabelCN(algo string) string {
	m := map[string]string{
		"prophet":       "贝叶斯时序",
		"statsforecast": "统计集成",
		"builtin":       "内置公式",
		"lightgbm":      "梯度提升·大区",
		"lightgbm_sku":  "梯度提升·SKU级",
		"last_month":    "上月直推",
		"yoy":           "去年同期",
		"avg3m":         "近3月均",
		"wma3":          "加权3月均",
	}
	if v, ok := m[algo]; ok {
		return v
	}
	return algo
}

// GetOfflineSalesForecastBacktest GET /api/offline/sales-forecast/backtest
// 返回销量预测算法回测结果 (按 月 × 算法 × 大区)
// 数据来源: offline_sales_forecast_backtest 表 (由 Python 脚本 prophet_backtest.py / statsforecast_backtest_v2.py 写入)
func (h *DashboardHandler) GetOfflineSalesForecastBacktest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	rows, err := h.DB.Query(`SELECT ym, algo, region, forecast_qty, actual_qty,
		IFNULL(err_pct, 0), IFNULL(abs_err_pct, 0),
		IFNULL(DATE_FORMAT(train_end_date,'%Y-%m-%d'), ''),
		DATE_FORMAT(run_at,'%Y-%m-%d %H:%i:%s')
		FROM offline_sales_forecast_backtest
		ORDER BY ym DESC, algo, region`)
	if err != nil {
		writeServerError(w, 500, "查询回测结果失败", err)
		return
	}
	defer rows.Close()

	type backtestItem struct {
		Ym           string  `json:"ym"`
		Algo         string  `json:"algo"`
		Region       string  `json:"region"`
		ForecastQty  int     `json:"forecastQty"`
		ActualQty    int     `json:"actualQty"`
		ErrPct       float64 `json:"errPct"`
		AbsErrPct    float64 `json:"absErrPct"`
		TrainEndDate string  `json:"trainEndDate"`
		RunAt        string  `json:"runAt"`
	}
	items := []backtestItem{}
	for rows.Next() {
		var it backtestItem
		if err := rows.Scan(&it.Ym, &it.Algo, &it.Region, &it.ForecastQty, &it.ActualQty,
			&it.ErrPct, &it.AbsErrPct, &it.TrainEndDate, &it.RunAt); err != nil {
			writeServerError(w, 500, "扫描失败", err)
			return
		}
		items = append(items, it)
	}

	// 按 月 × 算法 汇总 MAPE
	type summaryKey struct{ ym, algo string }
	type summaryAgg struct {
		Forecast, Actual int
		AbsErrSum        float64
		Count            int
	}
	summaryMap := map[summaryKey]*summaryAgg{}
	for _, it := range items {
		k := summaryKey{it.Ym, it.Algo}
		if _, ok := summaryMap[k]; !ok {
			summaryMap[k] = &summaryAgg{}
		}
		s := summaryMap[k]
		s.Forecast += it.ForecastQty
		s.Actual += it.ActualQty
		s.AbsErrSum += it.AbsErrPct
		s.Count++
	}

	type summaryItem struct {
		Ym          string  `json:"ym"`
		Algo        string  `json:"algo"`
		ForecastQty int     `json:"forecastQty"`
		ActualQty   int     `json:"actualQty"`
		TotalErrPct float64 `json:"totalErrPct"` // 大区合计的相对误差
		Mape        float64 `json:"mape"`        // 大区平均绝对误差% (MAPE)
		RegionCount int     `json:"regionCount"`
	}
	summary := []summaryItem{}
	for k, s := range summaryMap {
		var totalErr float64
		if s.Actual > 0 {
			totalErr = math.Round(float64(s.Forecast-s.Actual)/float64(s.Actual)*1000) / 10
		}
		var mape float64
		if s.Count > 0 {
			mape = math.Round(s.AbsErrSum/float64(s.Count)*10) / 10
		}
		summary = append(summary, summaryItem{
			Ym: k.ym, Algo: k.algo,
			ForecastQty: s.Forecast, ActualQty: s.Actual,
			TotalErrPct: totalErr, Mape: mape, RegionCount: s.Count,
		})
	}
	sort.Slice(summary, func(i, j int) bool {
		if summary[i].Ym != summary[j].Ym {
			return summary[i].Ym > summary[j].Ym
		}
		return summary[i].Algo < summary[j].Algo
	})

	writeJSON(w, map[string]interface{}{
		"items":   items,
		"summary": summary,
		"regions": offlineForecastRegions,
	})
}
