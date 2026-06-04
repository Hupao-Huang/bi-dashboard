package handler

import (
	"math"
	"testing"
	"time"
)

// TestMixedForecastBacktestAccuracy 用真实历史月度序列(线下9大区合计,10成品品类)跑
// 生产的混合版回测函数 computeBacktestForecast + computeGrowthRate, 验证平均绝对误差 ~8.3%
// (golden-sample 准确度回归守护; 数据来自 sales_goods_summary 2024-04~2026-05 实查)
func TestMixedForecastBacktestAccuracy(t *testing.T) {
	series := map[string]float64{
		"2024-04": 224264, "2024-05": 215313, "2024-06": 199831, "2024-07": 202552,
		"2024-08": 209725, "2024-09": 279196.8, "2024-10": 220921, "2024-11": 267766,
		"2024-12": 291637, "2025-01": 395009, "2025-02": 223966, "2025-03": 274021,
		"2025-04": 284018, "2025-05": 288905, "2025-06": 311717, "2025-07": 305495,
		"2025-08": 299230, "2025-09": 325666, "2025-10": 248357, "2025-11": 273975,
		"2025-12": 322921, "2026-01": 501767, "2026-02": 213118, "2026-03": 298976,
		"2026-04": 352050, "2026-05": 316660,
	}
	// 回测窗口: 凑够12月历史 → 2025-04 ~ 2026-05
	months := []string{
		"2025-04", "2025-05", "2025-06", "2025-07", "2025-08", "2025-09",
		"2025-10", "2025-11", "2025-12", "2026-01", "2026-02", "2026-03",
		"2026-04", "2026-05",
	}
	var sumAbs float64
	var n int
	for _, ym := range months {
		cur, _ := time.Parse("2006-01", ym)
		actual := series[ym]
		if actual == 0 {
			continue
		}
		hist := make([]float64, 0, 12)
		for j := 12; j >= 1; j-- {
			hist = append(hist, series[cur.AddDate(0, -j, 0).Format("2006-01")])
		}
		yoy := series[cur.AddDate(0, -12, 0).Format("2006-01")]
		growth := computeGrowthRate(series, cur)
		predicted := computeBacktestForecast(int(cur.Month()), hist, yoy, growth)
		absErr := math.Abs(float64(predicted)-actual) / actual * 100
		sumAbs += absErr
		n++
		t.Logf("%s 预测=%6d 实际=%6.0f 误差=%5.1f%%", ym, predicted, actual, absErr)
	}
	avg := sumAbs / float64(n)
	t.Logf("=== 混合版平均绝对误差 = %.1f%% (%d 个月) ===", avg, n)
	if avg > 9.0 {
		t.Errorf("混合版回测平均误差 %.1f%% 超预期 (应 ~8.3%%, 阈值 9.0%%)", avg)
	}
}
