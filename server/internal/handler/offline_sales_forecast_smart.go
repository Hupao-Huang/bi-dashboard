package handler

// 销量预测·智能算法 (v1.66.0 起唯一算法)
//
// 设计目标: 1 个算法解决所有月份, 业务能看懂公式, 不依赖 ML 训练
//
// 核心思路 (4 因素加权融合):
//   预测值 = (α × 近3月均 + β × 同比 + γ × 环比) × 节假日因子 × 趋势调整
//
// 权重按月份动态调整 (节假日因素):
//
//   月份  | 场景         | α(近3月) | β(同比) | γ(环比) | 节假日因子
//   ------|--------------|---------|--------|--------|----------
//   1月   | 春节备货     | 0.20    | 0.70   | 0.10   | 1.00
//   2月   | 春节假期     | 0.10    | 0.80   | 0.10   | 1.00
//   3-5月 | 春节后恢复   | 0.60    | 0.30   | 0.10   | 1.00
//   6月   | 618 大促     | 0.40    | 0.50   | 0.10   | 1.05
//   7-8月 | 平淡期       | 0.70    | 0.20   | 0.10   | 1.00
//   9-10月| 中秋/国庆    | 0.40    | 0.50   | 0.10   | 1.05
//   11月  | 双11         | 0.30    | 0.60   | 0.10   | 1.10
//   12月  | 年终         | 0.50    | 0.40   | 0.10   | 1.00
//
// 趋势调整 (近12个月线性回归斜率):
//   斜率 / 当前均值 > +5%/月 (持续上升)  → 预测 × 1.05
//   斜率 / 当前均值 < -5%/月 (持续下降)  → 预测 × 0.95
//   其他 (平稳或波动)                    → 预测不变

import (
	"fmt"
	"math"
)

// MonthFactors 单月权重 + 节假日因子
type MonthFactors struct {
	Alpha          float64 // 近3月均权重
	Beta           float64 // 同比权重
	Gamma          float64 // 环比权重
	HolidayFactor  float64 // 节假日因子(乘数)
	HolidayContext string  // 节假日说明 (展示给业务看)
}

// monthFactorsTable 12 个月的权重表
func monthFactorsTable(month int) MonthFactors {
	switch month {
	case 1:
		return MonthFactors{Alpha: 0.20, Beta: 0.70, Gamma: 0.10, HolidayFactor: 1.00, HolidayContext: "春节备货"}
	case 2:
		return MonthFactors{Alpha: 0.10, Beta: 0.80, Gamma: 0.10, HolidayFactor: 1.00, HolidayContext: "春节假期"}
	case 3, 4, 5:
		return MonthFactors{Alpha: 0.60, Beta: 0.30, Gamma: 0.10, HolidayFactor: 1.00, HolidayContext: "春节后恢复期"}
	case 6:
		return MonthFactors{Alpha: 0.40, Beta: 0.50, Gamma: 0.10, HolidayFactor: 1.05, HolidayContext: "618 大促 (同比 +5%)"}
	case 7, 8:
		return MonthFactors{Alpha: 0.70, Beta: 0.20, Gamma: 0.10, HolidayFactor: 1.00, HolidayContext: "平淡期"}
	case 9, 10:
		return MonthFactors{Alpha: 0.40, Beta: 0.50, Gamma: 0.10, HolidayFactor: 1.05, HolidayContext: "中秋/国庆 (同比 +5%)"}
	case 11:
		return MonthFactors{Alpha: 0.30, Beta: 0.60, Gamma: 0.10, HolidayFactor: 1.10, HolidayContext: "双11 (同比 +10%)"}
	case 12:
		return MonthFactors{Alpha: 0.50, Beta: 0.40, Gamma: 0.10, HolidayFactor: 1.00, HolidayContext: "年终"}
	}
	// 兜底: 跟普通月份一致
	return MonthFactors{Alpha: 0.50, Beta: 0.40, Gamma: 0.10, HolidayFactor: 1.00, HolidayContext: ""}
}

// SmartForecastInput 单 SKU × 大区 的输入数据
type SmartForecastInput struct {
	History []float64 // 近 12 个月销量, [t-12, t-11, ..., t-1]
	YoyQty  float64   // 去年同月销量 (t-12)
}

// SmartForecastResult 输出结果
type SmartForecastResult struct {
	Forecast       int     `json:"forecast"`        // 最终预测值
	BaseAvg3       float64 `json:"baseAvg3"`        // 近 3 月均
	YoyQty         float64 `json:"yoyQty"`          // 同比
	MoMQty         float64 `json:"momQty"`          // 环比 (上月)
	WeightedBase   float64 `json:"weightedBase"`    // α×avg3 + β×yoy + γ×mom
	HolidayFactor  float64 `json:"holidayFactor"`   // 节假日因子
	TrendFactor    float64 `json:"trendFactor"`     // 趋势调整因子
	Alpha          float64 `json:"alpha"`           // 权重
	Beta           float64 `json:"beta"`
	Gamma          float64 `json:"gamma"`
	HolidayContext string  `json:"holidayContext"`  // 节假日说明
	TrendContext   string  `json:"trendContext"`    // 趋势说明
	FormulaText    string  `json:"formulaText"`     // 完整公式可读文本
}

// computeSmartForecast 智能预测核心算法
//
// 输入:
//   predictMonth: 目标预测月份 (1-12)
//   in.History:   近12月销量, in.History[0] = t-12月, in.History[11] = 上月
//   in.YoyQty:    去年同月销量 (= in.History[0])
//
// 输出: 完整 SmartForecastResult, 含计算过程方便业务理解
func computeSmartForecast(predictMonth int, in SmartForecastInput) SmartForecastResult {
	mf := monthFactorsTable(predictMonth)

	// 1. 近 3 月均 (上月 + 上上月 + 上上上月)
	var avg3 float64
	if len(in.History) >= 3 {
		avg3 = (in.History[len(in.History)-1] + in.History[len(in.History)-2] + in.History[len(in.History)-3]) / 3
	} else if len(in.History) > 0 {
		// 历史不足 3 月, 用现有的算
		sum := 0.0
		for _, v := range in.History {
			sum += v
		}
		avg3 = sum / float64(len(in.History))
	}

	// 2. 同比 (去年同月)
	yoy := in.YoyQty

	// 3. 环比 (上月)
	var mom float64
	if len(in.History) >= 1 {
		mom = in.History[len(in.History)-1]
	}

	// 4. 加权基线
	// 处理: 如果同比缺失 (yoy=0), 把 β 重分配给 α 和 γ
	alpha, beta, gamma := mf.Alpha, mf.Beta, mf.Gamma
	if yoy <= 0 {
		// β 平分给 α 和 γ
		extraEach := beta / 2
		alpha += extraEach
		gamma += extraEach
		beta = 0
	}
	// 处理: 如果近3月均缺失, β 全部接管
	if avg3 <= 0 {
		beta += alpha
		alpha = 0
	}
	// 处理: 如果环比缺失, 比例平分
	if mom <= 0 {
		alpha += gamma / 2
		beta += gamma / 2
		gamma = 0
	}
	weightedBase := alpha*avg3 + beta*yoy + gamma*mom

	// 5. 节假日因子
	afterHoliday := weightedBase * mf.HolidayFactor

	// 6. 趋势调整 (近12月线性回归斜率)
	trendFactor, trendCtx := computeTrendAdjustment(in.History)
	final := afterHoliday * trendFactor

	if final < 0 {
		final = 0
	}

	formula := fmt.Sprintf("(%.0f×%.0f + %.0f×%.0f + %.0f×%.0f) × %.2f × %.2f = %.0f",
		alpha, avg3, beta, yoy, gamma, mom, mf.HolidayFactor, trendFactor, final)

	return SmartForecastResult{
		Forecast:       int(math.Round(final)),
		BaseAvg3:       math.Round(avg3*10) / 10,
		YoyQty:         yoy,
		MoMQty:         mom,
		WeightedBase:   math.Round(weightedBase*10) / 10,
		HolidayFactor:  mf.HolidayFactor,
		TrendFactor:    math.Round(trendFactor*1000) / 1000,
		Alpha:          alpha,
		Beta:           beta,
		Gamma:          gamma,
		HolidayContext: mf.HolidayContext,
		TrendContext:   trendCtx,
		FormulaText:    formula,
	}
}

// computeTrendAdjustment 近 12 月销量线性回归算趋势, 返回乘数因子
//
// 算法:
//   y[t] = a + b × t  (t = 0..n-1)
//   斜率 b 用最小二乘法
//   月增长率 = b / mean(y)
//   增长率 > +5% → 上升趋势, 因子 1.05
//   增长率 < -5% → 下降趋势, 因子 0.95
//   其他          → 1.00 (平稳)
func computeTrendAdjustment(history []float64) (factor float64, context string) {
	if len(history) < 6 {
		return 1.0, "样本不足, 不应用趋势调整"
	}
	n := float64(len(history))
	var sumX, sumY, sumXY, sumXX float64
	for i, y := range history {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	mean := sumY / n
	if mean <= 0 {
		return 1.0, "历史均值=0, 不应用趋势调整"
	}
	// 最小二乘斜率: b = (n×ΣXY - ΣX×ΣY) / (n×ΣXX - ΣX²)
	denom := n*sumXX - sumX*sumX
	if denom == 0 {
		return 1.0, "无趋势"
	}
	slope := (n*sumXY - sumX*sumY) / denom
	growthPct := slope / mean * 100 // 月增长百分比

	switch {
	case growthPct > 5:
		return 1.05, fmt.Sprintf("上升趋势 +%.1f%%/月, 预测 ×1.05", growthPct)
	case growthPct < -5:
		return 0.95, fmt.Sprintf("下降趋势 %.1f%%/月, 预测 ×0.95", growthPct)
	default:
		return 1.0, fmt.Sprintf("趋势平稳 (%.1f%%/月), 不调整", growthPct)
	}
}

// SmartForecastSummary 给前端展示当前月用了什么权重 (页面顶部 hover 看公式)
type SmartForecastSummary struct {
	Month          int     `json:"month"`
	Alpha          float64 `json:"alpha"`
	Beta           float64 `json:"beta"`
	Gamma          float64 `json:"gamma"`
	HolidayFactor  float64 `json:"holidayFactor"`
	HolidayContext string  `json:"holidayContext"`
	FormulaText    string  `json:"formulaText"`
}

func smartForecastSummary(predictMonth int) SmartForecastSummary {
	mf := monthFactorsTable(predictMonth)
	formula := fmt.Sprintf("近3月×%.0f%% + 同比×%.0f%% + 环比×%.0f%%",
		mf.Alpha*100, mf.Beta*100, mf.Gamma*100)
	if mf.HolidayFactor != 1.0 {
		formula += fmt.Sprintf(" × %.2f (%s)", mf.HolidayFactor, mf.HolidayContext)
	} else if mf.HolidayContext != "" {
		formula += fmt.Sprintf(" (%s)", mf.HolidayContext)
	}
	return SmartForecastSummary{
		Month:          predictMonth,
		Alpha:          mf.Alpha,
		Beta:           mf.Beta,
		Gamma:          mf.Gamma,
		HolidayFactor:  mf.HolidayFactor,
		HolidayContext: mf.HolidayContext,
		FormulaText:    formula,
	}
}
