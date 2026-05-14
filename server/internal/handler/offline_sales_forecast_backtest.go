package handler

// 销量预测·实时回测 (v1.66.2 起)
//
// 对最近 N 月按当前智能算法逐月回测, 展示预测 vs 实际, 让业务直观看到准确度.
// 大区合计维度 (业务关心的口径).
//
// 算法跟生产环境一致: monthFactorsTable + regionGrowth (clamp [0.95, 1.10]) + 节假日因子
// 区别: 这里在大区合计维度算 (省 SKU 计算), 业务跟实际汇报口径吻合.

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"net/http"
	"sort"
	"time"
)

// BacktestRow 单月回测结果
type BacktestRow struct {
	Ym             string  `json:"ym"`
	Predicted      int     `json:"predicted"`      // 算法预测 (大区合计)
	Actual         int     `json:"actual"`         // 实际销量 (大区合计)
	Diff           int     `json:"diff"`           // 预测 - 实际
	ErrPct         float64 `json:"errPct"`         // 相对误差%
	AbsErrPct      float64 `json:"absErrPct"`      // 绝对误差%
	HolidayContext string  `json:"holidayContext"` // 节假日上下文
	FormulaText    string  `json:"formulaText"`    // 当月公式
}

// BacktestSummary 整体指标
type BacktestSummary struct {
	Months          int     `json:"months"`          // 回测覆盖月数
	AvgAbsErrPct    float64 `json:"avgAbsErrPct"`    // 平均绝对误差% (越低越好)
	MedianAbsErrPct float64 `json:"medianAbsErrPct"` // 中位数绝对误差%
	BestMonth       string  `json:"bestMonth"`       // 最准月份
	BestErrPct      float64 `json:"bestErrPct"`
	WorstMonth      string  `json:"worstMonth"`      // 最大偏差月份
	WorstErrPct     float64 `json:"worstErrPct"`
}

// GetOfflineSalesForecastBacktestRecent GET /api/offline/sales-forecast/backtest-recent
// query: months=12 (默认 12, 最大 24)
// 对最近 N 月按当前算法逐月回测
func (h *DashboardHandler) GetOfflineSalesForecastBacktestRecent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	months := 12
	if v := r.URL.Query().Get("months"); v != "" {
		_, _ = fmt.Sscanf(v, "%d", &months)
		if months < 3 {
			months = 3
		}
		if months > 24 {
			months = 24
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// 1. 拉过去 (months + 12) 月的销量月度合计 (按 stat_date 月分)
	//    +12 是为了 回测最早的月份 也能看到去年同月数据
	now := time.Now()
	// 当月不计 (本月还没结束), 从上月开始倒数
	endMonth := now.AddDate(0, -1, 0) // 最后一个回测月 = 上月
	startMonth := endMonth.AddDate(0, -(months + 12), 0)
	startDate := startMonth.Format("2006-01-02")
	endDate := time.Date(endMonth.Year(), endMonth.Month()+1, 1, 0, 0, 0, 0, time.Local).Format("2006-01-02")

	cateCond, cateArgs := offlineForecastCateCond()
	args := append([]interface{}{startDate, endDate}, cateArgs...)
	rows, err := h.DB.QueryContext(ctx, `
		SELECT DATE_FORMAT(stat_date, '%Y-%m') AS ym,
			SUM(goods_qty) AS qty
		FROM sales_goods_summary
		WHERE department='offline' AND stat_date >= ? AND stat_date < ?
		  AND `+offlineForecastRegionExpr+` IS NOT NULL`+cateCond+`
		GROUP BY DATE_FORMAT(stat_date, '%Y-%m')
		ORDER BY ym`, args...)
	if err != nil {
		writeServerError(w, 500, "查询历史销量失败", err)
		return
	}
	defer rows.Close()
	monthQty := map[string]float64{}
	for rows.Next() {
		var ym string
		var qty float64
		if err := rows.Scan(&ym, &qty); err == nil {
			monthQty[ym] = qty
		}
	}

	// 2. 对每个回测月份算预测
	backtestList := []BacktestRow{}
	cur := endMonth
	for i := 0; i < months; i++ {
		ymStr := cur.Format("2006-01")
		actual := monthQty[ymStr]
		if actual == 0 {
			cur = cur.AddDate(0, -1, 0)
			continue
		}
		// 历史数据: 截至 ymStr 上月
		hist := []float64{}
		for j := 12; j >= 1; j-- {
			t := cur.AddDate(0, -j, 0)
			if v, ok := monthQty[t.Format("2006-01")]; ok {
				hist = append(hist, v)
			} else {
				hist = append(hist, 0)
			}
		}
		yoyMonth := cur.AddDate(0, -12, 0).Format("2006-01")
		yoy := monthQty[yoyMonth]
		// 算近 3 月增长率 (近 3 月 vs 去年同期 3 月)
		growth := computeGrowthRate(monthQty, cur)

		predictMonth := int(cur.Month())
		predicted := computeBacktestForecast(predictMonth, hist, yoy, growth)
		mf := monthFactorsTable(predictMonth)
		formula := fmt.Sprintf("近3月×%.0f%% + 同比×%.0f%%(×%.2f增长) + 环比×%.0f%% × %.2f节假日",
			mf.Alpha*100, mf.Beta*100, growth, mf.Gamma*100, mf.HolidayFactor)

		diff := predicted - int(math.Round(actual))
		errPct := math.Round(float64(diff)/actual*1000) / 10
		backtestList = append(backtestList, BacktestRow{
			Ym:             ymStr,
			Predicted:      predicted,
			Actual:         int(math.Round(actual)),
			Diff:           diff,
			ErrPct:         errPct,
			AbsErrPct:      math.Abs(errPct),
			HolidayContext: mf.HolidayContext,
			FormulaText:    formula,
		})

		cur = cur.AddDate(0, -1, 0)
	}

	// 反转, 让最早的月份在前
	for i, j := 0, len(backtestList)-1; i < j; i, j = i+1, j-1 {
		backtestList[i], backtestList[j] = backtestList[j], backtestList[i]
	}

	// 3. 算整体指标
	summary := BacktestSummary{Months: len(backtestList)}
	if len(backtestList) > 0 {
		var sumAbs float64
		absList := make([]float64, 0, len(backtestList))
		bestErr := math.MaxFloat64
		worstErr := 0.0
		for _, b := range backtestList {
			sumAbs += b.AbsErrPct
			absList = append(absList, b.AbsErrPct)
			if b.AbsErrPct < bestErr {
				bestErr = b.AbsErrPct
				summary.BestMonth = b.Ym
				summary.BestErrPct = b.ErrPct
			}
			if b.AbsErrPct > worstErr {
				worstErr = b.AbsErrPct
				summary.WorstMonth = b.Ym
				summary.WorstErrPct = b.ErrPct
			}
		}
		summary.AvgAbsErrPct = math.Round(sumAbs/float64(len(backtestList))*10) / 10
		sort.Float64s(absList)
		mid := len(absList) / 2
		if len(absList)%2 == 0 {
			summary.MedianAbsErrPct = math.Round((absList[mid-1]+absList[mid])/2*10) / 10
		} else {
			summary.MedianAbsErrPct = math.Round(absList[mid]*10) / 10
		}
	}

	writeJSON(w, map[string]interface{}{
		"items":   backtestList,
		"summary": summary,
	})
}

// computeGrowthRate 近 3 月销量 vs 去年同期 3 月增长率, clamp [0.95, 1.10]
// 跟 computeOfflineRegionGrowth 一致 (但这里是大区合计维度)
func computeGrowthRate(monthQty map[string]float64, cur time.Time) float64 {
	curr3, prev3 := 0.0, 0.0
	for j := 1; j <= 3; j++ {
		curr3 += monthQty[cur.AddDate(0, -j, 0).Format("2006-01")]
		prev3 += monthQty[cur.AddDate(0, -j-12, 0).Format("2006-01")]
	}
	if prev3 == 0 || curr3 == 0 {
		return 1.0
	}
	g := curr3 / prev3
	if g < 0.95 {
		g = 0.95
	}
	if g > 1.10 {
		g = 1.10
	}
	return g
}

// computeBacktestForecast 大区合计维度的预测算法
// hist: 近 12 月销量 [t-12, t-11, ..., t-1]
// yoy: 去年同月销量 (= hist[0])
// growth: 业务增长系数
func computeBacktestForecast(predictMonth int, hist []float64, yoy, growth float64) int {
	if len(hist) < 3 {
		return 0
	}
	mf := monthFactorsTable(predictMonth)
	avg3 := (hist[len(hist)-1] + hist[len(hist)-2] + hist[len(hist)-3]) / 3
	mom := hist[len(hist)-1]
	yoyAdjusted := yoy * growth

	alpha, beta, gamma := mf.Alpha, mf.Beta, mf.Gamma
	// 缺失项重分配权重
	if yoyAdjusted <= 0 {
		extraEach := beta / 2
		alpha += extraEach
		gamma += extraEach
		beta = 0
	}
	if avg3 <= 0 {
		beta += alpha
		alpha = 0
	}
	if mom <= 0 {
		alpha += gamma / 2
		beta += gamma / 2
		gamma = 0
	}

	weighted := alpha*avg3 + beta*yoyAdjusted + gamma*mom
	withHoliday := weighted * mf.HolidayFactor
	// 趋势调整 (用近 12 月线性回归)
	trendFactor, _ := computeTrendAdjustment(hist)
	final := withHoliday * trendFactor
	if final < 0 {
		final = 0
	}
	return int(math.Round(final))
}

var _ = sql.ErrNoRows // 防止 unused import (因为 import 上面写了 database/sql)
