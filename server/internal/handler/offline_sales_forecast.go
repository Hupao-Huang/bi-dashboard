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

// offlineForecastTotalRegion 业务只按"线下合计"填报 (不再分大区)。
// 出数把 9 大区算出的建议/依据求和到此 key; 存储也用此 region 值存一行/SKU。
// 算法内部仍按 SKU×大区 算 (大区增长/趋势需要), 只是展示与填报聚合到合计。
const offlineForecastTotalRegion = "线下合计"

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
			// v1.66.1: 基于 12 月回测优化, 收紧 clamp [0.85,1.30] → [0.95,1.10]
			// 原因: 上限 1.30 在 9-10 月会过度放大同比项 (实际 9-10 月销量持平/微跌)
			// 收紧后只对"持续小幅增长" (5-15%) 微调, 激进涨/跌都不动
			if g < 0.95 {
				g = 0.95
			}
			if g > 1.10 {
				g = 1.10
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
	// v1.77 混合版: 春节月(1/2)按"去年同月×大区增长"; 平淡月(3-12)按 weightedForecast 加权(近3月均+同比+环比)×节假日×趋势
	// 见 offline_sales_forecast_smart.go: monthFactorsTable / weightedForecast / computeTrendAdjustment
	cateCond, cateArgs := offlineForecastCateCond()

	// 0. 算 SKU × 月份(1-12) 季节指数 (春节无去年同月时兜底用) + 大区同比增长率
	seasIdx, _, err := computeOfflineSeasonalIndex(h.DB, ym)
	if writeDatabaseError(w, err) {
		return
	}
	regionGrowth, err := computeOfflineRegionGrowth(h.DB, ym)
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

	// 1. 一次拉齐 (按 SKU × 大区): 近3月均 + 去年同月(同比) + 上月(环比)
	//    v1.77 混合版需要 SKU×大区 级的去年同月/上月, 原季节版只拉近3月均
	predStart := predictTime.Format("2006-01-02")               // 预测月 1 号 (上界, 不含)
	m3Start := predictTime.AddDate(0, -3, 0).Format("2006-01-02")  // 近3月起
	m1Start := predictTime.AddDate(0, -1, 0).Format("2006-01-02")  // 上月起
	yoyStart := predictTime.AddDate(0, -12, 0).Format("2006-01-02") // 去年同月起
	yoyEnd := predictTime.AddDate(0, -11, 0).Format("2006-01-02")   // 去年同月止 (不含)
	// WHERE 只扫真正用到的两段(近3月含上月 + 去年同月), 不扫中间 8 个月(避免全表多扫 4 倍)
	sugArgs := []interface{}{m3Start, predStart, yoyStart, yoyEnd, m1Start, predStart, m3Start, predStart, yoyStart, yoyEnd}
	sugArgs = append(sugArgs, cateArgs...)
	sugRows, ok := queryRowsOrWriteError(w, r, h.DB, `
		SELECT goods_no AS sku_code,
			MAX(goods_name) AS goods_name,
			`+offlineForecastRegionExpr+` AS region,
			SUM(CASE WHEN stat_date >= ? AND stat_date < ? THEN goods_qty ELSE 0 END) / 3.0 AS base_avg3,
			SUM(CASE WHEN stat_date >= ? AND stat_date < ? THEN goods_qty ELSE 0 END) AS yoy_qty,
			SUM(CASE WHEN stat_date >= ? AND stat_date < ? THEN goods_qty ELSE 0 END) AS mom_qty
		FROM sales_goods_summary
		WHERE department = 'offline'
			AND ((stat_date >= ? AND stat_date < ?) OR (stat_date >= ? AND stat_date < ?))
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
	suggestions := map[cellKey]int{}    // 最终建议值
	baseAvgMap := map[cellKey]float64{} // 近 3 月原始均值(tooltip 用)
	yoyMap := map[cellKey]float64{}     // 去年同月销量(同比)
	momMap := map[cellKey]float64{}     // 上月销量(环比)
	skuMeta := map[string]string{}      // sku_code → goods_name
	for sugRows.Next() {
		var sku, region string
		var goodsName sql.NullString
		var base, yoyQty, momQty float64
		if writeDatabaseError(w, sugRows.Scan(&sku, &goodsName, &region, &base, &yoyQty, &momQty)) {
			sugRows.Close()
			return
		}
		if base < 0 {
			base = 0
		}
		ck := cellKey{sku, region}
		baseAvgMap[ck] = base
		yoyMap[ck] = yoyQty
		momMap[ck] = momQty
		if goodsName.Valid && skuMeta[sku] == "" {
			skuMeta[sku] = goodsName.String
		}
	}
	sugRows.Close()
	if writeDatabaseError(w, sugRows.Err()) {
		return
	}

	// === v1.77 混合版算法 (替代原季节版, 回测大区合计 14.1%→8.3%) ===
	//  春节月(1/2)去年同月有量: 去年同月 × 大区增长 (经销商节前囤货稳定, 按去年同期最稳; 不叠节假日/趋势)
	//  春节月无去年同月(新SKU): 季节版去季节 (近3月含1月备货高峰, 季节版更稳; 回测合计层几乎不命中此支)
	//  平淡月(3-12): weightedForecast 加权 × 节假日因子 × 大区趋势 (与 computeBacktestForecast 共用同一函数, 口径一致)
	monthFactor := monthFactorsTable(predictMonth)
	regionTrend, _ := computeOfflineRegionTrend(h.DB, ym)
	isSpring := predictMonth == 1 || predictMonth == 2
	for ck := range baseAvgMap {
		base := baseAvgMap[ck]
		yoy := yoyMap[ck]
		mom := momMap[ck]
		// 只对近3月有销量(base>0)的格子出建议。
		// 注: 二审曾建议"春节月把去年同期有量但近3月断货的休眠货也纳入", 但 SKU 级实测: 纳入后
		// 春节总量误差从 ~9% 飙到 ~59% (休眠货多数不重复去年销量), 故维持原门槛, 休眠货交业务手工补。
		if base <= 0 {
			continue
		}
		region := ck.region
		growth := 1.0
		if g, ok := regionGrowth[region]; ok && g > 0 {
			growth = g
		}
		var adjusted float64
		if isSpring {
			if yoy > 0 {
				// 春节主算法: 去年同月 × 大区增长
				adjusted = yoy * growth
			} else {
				// 春节无去年同月: 用季节版去季节 (近3月含1月春节备货高峰, 直接加权会把2月带高;
				// 季节版除掉季节性更稳, SKU 级实测加权版总量误差飙到 59%, 季节版则正常)
				adjusted = applySeasonalAdjust(base, recentMonths, predictMonth, seasIdx[ck.sku]) * growth
			}
		} else {
			// 平淡月: 加权版 (与回测共用 weightedForecast, 缺项归一化不浪费权重)
			adjusted = weightedForecast(monthFactor, base, yoy*growth, mom) * monthFactor.HolidayFactor
			if t, has := regionTrend[region]; has {
				adjusted *= t
			}
		}
		if adjusted < 0 {
			adjusted = 0
		}
		suggestions[ck] = int(math.Round(adjusted))
	}

	// 2. SKU 全集 — 近 6 个月销过的 SKU(默认),或全部 SKU
	skuSet := map[string]struct{}{}
	if rangeMode == "all" {
		// 近 12 个月内出现过的 SKU (避免几千行无销量历史 SKU 也铺出来)
		s12, e12 := monthsBack(ym, 12)
		args12 := append([]interface{}{s12, e12}, cateArgs...)
		skuRows, ok2 := queryRowsOrWriteError(w, r, h.DB, `
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
		skuRows, ok2 := queryRowsOrWriteError(w, r, h.DB, `
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

	// 3. 已保存的预测值 (按 SKU 合计; SUM 兼容历史按大区填的多行)
	forecastSum := map[string]int{}
	fRows, ok3 := queryRowsOrWriteError(w, r, h.DB, `
		SELECT sku_code, SUM(forecast_qty) AS total, MAX(goods_name) AS goods_name
		FROM offline_sales_forecast WHERE ym = ? GROUP BY sku_code`, ym)
	if !ok3 {
		return
	}
	for fRows.Next() {
		var sku string
		var qty int
		var name sql.NullString
		if writeDatabaseError(w, fRows.Scan(&sku, &qty, &name)) {
			fRows.Close()
			return
		}
		forecastSum[sku] = qty
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
		BaseAvgs    map[string]float64 `json:"base_avgs"` // 近 3 月原始均值, tooltip 依据
		YoyQtys     map[string]float64 `json:"yoy_qtys"`  // 去年同月销量, tooltip 依据
	}
	predictYear := predictTime.Year()
	holiday := holidayContext(predictYear, predictMonth)
	items := make([]item, 0, len(skuSet))
	for sku := range skuSet {
		it := item{
			SkuCode:     sku,
			GoodsName:   skuMeta[sku],
			Suggestions: map[string]int{},
			Forecasts:   map[string]int{},
			BaseAvgs:    map[string]float64{},
			YoyQtys:     map[string]float64{},
		}
		// 9 大区求和 → 线下合计 (业务只填合计, 算法内部仍按大区算)
		var sumSug int
		var sumBase, sumYoy float64
		for _, region := range offlineForecastRegions {
			sumSug += suggestions[cellKey{sku, region}]
			sumBase += baseAvgMap[cellKey{sku, region}]
			sumYoy += yoyMap[cellKey{sku, region}]
		}
		if sumSug > 0 {
			it.Suggestions[offlineForecastTotalRegion] = sumSug
		}
		if sumBase > 0 {
			it.BaseAvgs[offlineForecastTotalRegion] = math.Round(sumBase*10) / 10
		}
		if sumYoy > 0 {
			it.YoyQtys[offlineForecastTotalRegion] = math.Round(sumYoy*10) / 10
		}
		if v, ok := forecastSum[sku]; ok {
			it.Forecasts[offlineForecastTotalRegion] = v
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
	// 本月公式说明 (前端顶部 Tag hover 展示)
	summary := smartForecastSummary(predictMonth)
	regionTrendOut := map[string]float64{}
	for r, t := range regionTrend {
		regionTrendOut[r] = math.Round(t*1000) / 1000
	}

	writeJSON(w, map[string]interface{}{
		"ym":               ym,
		"regions":          []string{offlineForecastTotalRegion},
		"items":            items,
		"holiday_context":  holiday,
		"region_growth":    growthOut,
		"forecast_summary": summary,        // 本月用了什么权重 + 节假日因子
		"region_trend":     regionTrendOut, // v1.66 各大区近12月趋势调整因子 (1.05/0.95/1.0)
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

	// 只接受"线下合计"填报 (业务不再分大区)
	allowedRegion := map[string]bool{offlineForecastTotalRegion: true}

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
		// 清掉该 SKU 该月历史按大区填的行, 避免读取 SUM 时与合计行重复计
		if _, err := tx.Exec(`DELETE FROM offline_sales_forecast WHERE ym=? AND sku_code=? AND region<>?`,
			req.YM, it.SkuCode, offlineForecastTotalRegion); writeDatabaseError(w, err) {
			return
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

// GetOfflineSalesForecastSKUTrend GET /api/offline/sales-forecast/sku-trend?sku_code=xxx
// 返回该 SKU 近 13 个月的实际销量 (大区合计), 给销量预测页 hover 弹趋势图用
func (h *DashboardHandler) GetOfflineSalesForecastSKUTrend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	sku := strings.TrimSpace(r.URL.Query().Get("sku_code"))
	if sku == "" {
		writeError(w, 400, "缺少 sku_code")
		return
	}
	// 近 13 个月 (按当月起回退), 月级聚合, 大区合计 + 按大区拆 (返回两份)
	rows, err := h.DB.Query(`
		SELECT DATE_FORMAT(stat_date, '%Y-%m') AS ym, SUM(goods_qty) AS qty
		FROM sales_goods_summary
		WHERE department='offline' AND goods_no=?
		  AND stat_date >= DATE_SUB(DATE_FORMAT(CURDATE(), '%Y-%m-01'), INTERVAL 13 MONTH)
		GROUP BY DATE_FORMAT(stat_date, '%Y-%m')
		ORDER BY ym`, sku)
	if err != nil {
		writeServerError(w, 500, "查询失败", err)
		return
	}
	defer rows.Close()
	type point struct {
		Ym  string  `json:"ym"`
		Qty float64 `json:"qty"`
	}
	items := []point{}
	for rows.Next() {
		var p point
		if err := rows.Scan(&p.Ym, &p.Qty); err == nil {
			items = append(items, p)
		}
	}
	// 顺便查货品名 (方便 Tooltip 标题)
	var goodsName string
	_ = h.DB.QueryRow(`SELECT goods_name FROM sales_goods_summary WHERE goods_no=? AND goods_name IS NOT NULL LIMIT 1`, sku).Scan(&goodsName)
	writeJSON(w, map[string]interface{}{
		"sku_code":   sku,
		"goods_name": goodsName,
		"items":      items,
	})
}

// computeOfflineRegionTrend v1.66 各大区近 12 月销量做线性回归, 返回趋势调整因子
//   上升趋势 (>+5%/月) → 1.05
//   下降趋势 (<-5%/月) → 0.95
//   平稳            → 1.00
func computeOfflineRegionTrend(db *sql.DB, ym string) (map[string]float64, error) {
	t, err := time.Parse("2006-01", ym)
	if err != nil {
		return nil, err
	}
	startDate := t.AddDate(0, -12, 0).Format("2006-01-02")
	endDate := t.Format("2006-01-02")
	cateCond, cateArgs := offlineForecastCateCond()
	args := append([]interface{}{startDate, endDate}, cateArgs...)
	rows, err := db.Query(`SELECT DATE_FORMAT(stat_date, '%Y-%m') AS ym,
			`+offlineForecastRegionExpr+` AS region,
			SUM(goods_qty) AS qty
		FROM sales_goods_summary
		WHERE department='offline' AND stat_date >= ? AND stat_date < ?
		  AND `+offlineForecastRegionExpr+` IS NOT NULL`+cateCond+`
		GROUP BY DATE_FORMAT(stat_date, '%Y-%m'), region
		ORDER BY region, ym`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	regionHist := map[string][]float64{}
	for rows.Next() {
		var month, region string
		var qty float64
		if err := rows.Scan(&month, &region, &qty); err == nil {
			regionHist[region] = append(regionHist[region], qty)
		}
	}
	out := map[string]float64{}
	for region, hist := range regionHist {
		factor, _ := computeTrendAdjustment(hist)
		out[region] = factor
	}
	return out, nil
}
