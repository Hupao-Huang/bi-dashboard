package handler

// 销量预测·混合版算法 (v1.77 起, 替代 v1.66 季节版)
//
// 设计目标: 业务能看懂公式, 不依赖 ML 训练; 分而治之, 各月用各自最准的招
//
// 核心思路:
//   平淡月(3-12): (α × 近3月均 + β × 同比 + γ × 环比) × 节假日因子 × 趋势调整
//   春节月(1/2):  去年同月 × 大区增长 (线下经销商节前囤货是稳定因素, 按去年同期推最稳)
//   回测大区合计 14.1%→8.3%; 出数走 GetOfflineSalesForecast, 回测走 computeBacktestForecast, 加权部分共用 weightedForecast
//
// 平淡月加权权重表如下 (v1.66 设计, 混合版沿用):
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
// v1.66.1 节假日因子基于 12 月回测优化:
//   - 1月 1.00→1.05 (春节备货实际比预测高 30%, 上调因子)
//   - 9月 1.05→1.00 (中秋影响小, 实际接近平淡)
//   - 10月 1.05→0.95 (国庆放假反而少销售, 实际比预测低 12%)
//   - 11月 1.10→1.00 (线下大区双11效应不明显, 实际比预测低 10%)
func monthFactorsTable(month int) MonthFactors {
	switch month {
	case 1:
		return MonthFactors{Alpha: 0.20, Beta: 0.70, Gamma: 0.10, HolidayFactor: 1.05, HolidayContext: "春节备货 (同比 +5%)"}
	case 2:
		return MonthFactors{Alpha: 0.10, Beta: 0.80, Gamma: 0.10, HolidayFactor: 1.00, HolidayContext: "春节假期"}
	case 3, 4, 5:
		return MonthFactors{Alpha: 0.60, Beta: 0.30, Gamma: 0.10, HolidayFactor: 1.00, HolidayContext: "春节后恢复期"}
	case 6:
		return MonthFactors{Alpha: 0.40, Beta: 0.50, Gamma: 0.10, HolidayFactor: 1.05, HolidayContext: "618 大促 (同比 +5%)"}
	case 7, 8:
		return MonthFactors{Alpha: 0.70, Beta: 0.20, Gamma: 0.10, HolidayFactor: 1.00, HolidayContext: "平淡期"}
	case 9:
		return MonthFactors{Alpha: 0.40, Beta: 0.50, Gamma: 0.10, HolidayFactor: 1.00, HolidayContext: "中秋"}
	case 10:
		return MonthFactors{Alpha: 0.40, Beta: 0.50, Gamma: 0.10, HolidayFactor: 0.95, HolidayContext: "国庆放假 (同比 -5%)"}
	case 11:
		return MonthFactors{Alpha: 0.30, Beta: 0.60, Gamma: 0.10, HolidayFactor: 1.00, HolidayContext: "双11 (线下不爆发)"}
	case 12:
		return MonthFactors{Alpha: 0.50, Beta: 0.40, Gamma: 0.10, HolidayFactor: 1.00, HolidayContext: "年终"}
	}
	// 兜底: 跟普通月份一致
	return MonthFactors{Alpha: 0.50, Beta: 0.40, Gamma: 0.10, HolidayFactor: 1.00, HolidayContext: ""}
}

// weightedForecast 平淡月(及春节无去年同月)的加权基线: α×近3月均 + β×同比 + γ×环比.
// 只对有值(>0)的项分配权重并按非零项归一化(缺项不把权重浪费在 0 值上, 避免系统性偏低),
// 返回"未乘节假日/趋势"的加权值. 出数 GetOfflineSalesForecast 与回测 computeBacktestForecast
// 共用此函数 → 两条路口径强制一致, 不会再"改一处漏一处".
func weightedForecast(mf MonthFactors, base, yoyAdj, mom float64) float64 {
	wa, wb, wc := 0.0, 0.0, 0.0
	if base > 0 {
		wa = mf.Alpha
	}
	if yoyAdj > 0 {
		wb = mf.Beta
	}
	if mom > 0 {
		wc = mf.Gamma
	}
	sum := wa + wb + wc
	if sum <= 0 {
		return 0
	}
	return (wa*base + wb*yoyAdj + wc*mom) / sum
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
	var formula string
	if predictMonth == 1 || predictMonth == 2 {
		// v1.77 混合版: 春节月用去年同月×增长 (经销商节前囤货稳定)
		formula = "春节月: 去年同月 × 大区增长 (经销商节前囤货, 按去年同期推算最稳)"
	} else {
		formula = fmt.Sprintf("近3月×%.0f%% + 同比×%.0f%% + 环比×%.0f%%",
			mf.Alpha*100, mf.Beta*100, mf.Gamma*100)
		if mf.HolidayFactor != 1.0 {
			formula += fmt.Sprintf(" × %.2f (%s)", mf.HolidayFactor, mf.HolidayContext)
		} else if mf.HolidayContext != "" {
			formula += fmt.Sprintf(" (%s)", mf.HolidayContext)
		}
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
