// forecast-baseline-backtest
// 给销量预测做 4 个 baseline 算法的回测, 入库 offline_sales_forecast_backtest
// 用法: ./forecast-baseline-backtest --months=2026-01,2026-02,2026-03,2026-04
//
// 4 个 baseline 算法 (大区合计维度, 与 Prophet/StatsForecast 口径一致):
//   1. last_month — 上月销量直接当预测 (最朴素)
//   2. yoy        — 去年同月销量当预测
//   3. avg3m      — 近 3 个月均值
//   4. wma3       — 0.5×y[t-1] + 0.3×y[t-2] + 0.2×y[t-3] 加权移动平均
package main

import (
	"bi-dashboard/internal/config"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// 与 Prophet / StatsForecast 脚本口径完全一致
const regionMapSQL = `CASE
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

const cateFilter = "cate_name IN ('调味料','酱油','调味汁','干制面','素蚝油','酱类','醋','汤底','番茄沙司','糖')"

var regions = []string{"华北大区", "华东大区", "华中大区", "华南大区", "西南大区", "西北大区", "东北大区", "山东大区", "重客"}

// fetchMonthlyByRegion 拉指定月份每大区销量
func fetchMonthlyByRegion(db *sql.DB, ym string) (map[string]float64, error) {
	t, err := time.Parse("2006-01", ym)
	if err != nil {
		return nil, err
	}
	first := t.Format("2006-01-02")
	next := t.AddDate(0, 1, 0).Format("2006-01-02")
	sql := fmt.Sprintf(`SELECT %s AS region, SUM(goods_qty) AS qty
		FROM sales_goods_summary
		WHERE department='offline' AND stat_date >= ? AND stat_date < ? AND %s
		GROUP BY region HAVING region IS NOT NULL`, regionMapSQL, cateFilter)
	rows, err := db.Query(sql, first, next)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var r string
		var q float64
		if err := rows.Scan(&r, &q); err == nil {
			out[r] = q
		}
	}
	return out, nil
}

// shiftMonth 把 "2026-03" 偏移 n 个月, 返回 "YYYY-MM"
func shiftMonth(ym string, delta int) string {
	t, _ := time.Parse("2006-01", ym)
	return t.AddDate(0, delta, 0).Format("2006-01")
}

// upsertBacktest 写入回测表
func upsertBacktest(db *sql.DB, algo, ym, trainEnd, region string, fc, actual float64) error {
	if actual == 0 {
		return nil
	}
	errPct := math.Round((fc-actual)/actual*1000) / 10
	absErr := math.Abs(errPct)
	_, err := db.Exec(`INSERT INTO offline_sales_forecast_backtest
		(ym, algo, region, forecast_qty, actual_qty, err_pct, abs_err_pct, train_end_date, run_at)
		VALUES (?,?,?,?,?,?,?,?, NOW())
		ON DUPLICATE KEY UPDATE
		  forecast_qty=VALUES(forecast_qty),
		  actual_qty=VALUES(actual_qty),
		  err_pct=VALUES(err_pct),
		  abs_err_pct=VALUES(abs_err_pct),
		  train_end_date=VALUES(train_end_date),
		  run_at=NOW()`,
		ym, algo, region, int(math.Round(fc)), int(math.Round(actual)),
		errPct, absErr, trainEnd)
	return err
}

// computeBaselinesForMonth 对单一回测月份算 4 个 baseline 值并入库
func computeBaselinesForMonth(db *sql.DB, ym string) error {
	// 训练截至 = 该月 1 号 - 1 天
	t, _ := time.Parse("2006-01", ym)
	trainEnd := t.AddDate(0, 0, -1).Format("2006-01-02")

	// 实际销量
	actual, err := fetchMonthlyByRegion(db, ym)
	if err != nil {
		return err
	}

	// 准备 baseline 用的历史月数据
	// last_month: y[ym-1]
	prev1, err := fetchMonthlyByRegion(db, shiftMonth(ym, -1))
	if err != nil {
		return err
	}
	// avg3m: y[ym-1..ym-3]
	prev2, err := fetchMonthlyByRegion(db, shiftMonth(ym, -2))
	if err != nil {
		return err
	}
	prev3, err := fetchMonthlyByRegion(db, shiftMonth(ym, -3))
	if err != nil {
		return err
	}
	// yoy: y[ym-12]
	prevYear, err := fetchMonthlyByRegion(db, shiftMonth(ym, -12))
	if err != nil {
		return err
	}

	fmt.Printf("\n== %s (训练截至 %s) ==\n", ym, trainEnd)
	for _, region := range regions {
		a := actual[region]
		if a == 0 {
			fmt.Printf("  %s 实际=0, 跳过\n", region)
			continue
		}

		// 1. last_month
		lm := prev1[region]
		if err := upsertBacktest(db, "last_month", ym, trainEnd, region, lm, a); err != nil {
			log.Printf("[WARN] %s/%s/last_month: %v", ym, region, err)
		}

		// 2. yoy
		yoy := prevYear[region]
		if yoy > 0 {
			if err := upsertBacktest(db, "yoy", ym, trainEnd, region, yoy, a); err != nil {
				log.Printf("[WARN] %s/%s/yoy: %v", ym, region, err)
			}
		}

		// 3. avg3m
		p1, p2, p3 := prev1[region], prev2[region], prev3[region]
		var avg3 float64
		var cnt int
		for _, v := range []float64{p1, p2, p3} {
			if v > 0 {
				avg3 += v
				cnt++
			}
		}
		if cnt > 0 {
			avg3 = avg3 / float64(cnt)
			if err := upsertBacktest(db, "avg3m", ym, trainEnd, region, avg3, a); err != nil {
				log.Printf("[WARN] %s/%s/avg3m: %v", ym, region, err)
			}
		}

		// 4. wma3 — 加权移动平均, 越近权重越大
		if p1 > 0 || p2 > 0 || p3 > 0 {
			wma := 0.5*p1 + 0.3*p2 + 0.2*p3
			// 权重归一化处理 (如果某月缺失, 补 0 会偏低)
			totalW := 0.0
			if p1 > 0 {
				totalW += 0.5
			}
			if p2 > 0 {
				totalW += 0.3
			}
			if p3 > 0 {
				totalW += 0.2
			}
			if totalW > 0 {
				wma = wma / totalW
			}
			if err := upsertBacktest(db, "wma3", ym, trainEnd, region, wma, a); err != nil {
				log.Printf("[WARN] %s/%s/wma3: %v", ym, region, err)
			}
		}

		fmt.Printf("  %s: last=%6.0f yoy=%6.0f avg3=%6.0f wma3=%6.0f | actual=%6.0f\n",
			region, lm, yoy, avg3, 0.5*p1+0.3*p2+0.2*p3, a)
	}
	return nil
}

func main() {
	// 默认上月 (例: 2026-05-13 跑 → 2026-04)
	defaultLast := time.Now().AddDate(0, -1, 0).Format("2006-01")
	monthsArg := flag.String("months", defaultLast, "逗号分隔的回测月份, 默认上月")
	flag.Parse()
	months := strings.Split(*monthsArg, ",")

	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("连接 MySQL 失败: %v", err)
	}
	defer db.Close()

	for _, ym := range months {
		ym = strings.TrimSpace(ym)
		if err := computeBaselinesForMonth(db, ym); err != nil {
			log.Printf("[ERROR] %s: %v", ym, err)
		}
	}
	fmt.Println("\n[OK] baseline 回测已 UPSERT 入 offline_sales_forecast_backtest (algo=last_month/yoy/avg3m/wma3)")
}
