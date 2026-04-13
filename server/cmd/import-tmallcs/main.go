package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/xuri/excelize/v2"
)

var baseDir = `Z:\信息部\RPA_集团数据看板\天猫超市`

func main() {
	cfg, _ := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	db, _ := sql.Open("mysql", cfg.Database.DSN())
	defer db.Close()

	startDate, endDate := "", ""
	if len(os.Args) >= 3 {
		startDate, endDate = os.Args[1], os.Args[2]
	}
	resolvedBaseDir, err := importutil.ResolveDataRoot(baseDir)
	if err != nil {
		log.Fatalf("数据目录不可用: %v", err)
	}

	total := map[string]int{}
	matchedDate := false

	for _, yearDir := range []string{"2025", "2026"} {
		yearPath := filepath.Join(resolvedBaseDir, yearDir)
		dateDirs, err := os.ReadDir(yearPath)
		if err != nil {
			continue
		}
		for _, dd := range dateDirs {
			if !dd.IsDir() {
				continue
			}
			dateStr := dd.Name()
			if startDate != "" && dateStr < startDate {
				continue
			}
			if endDate != "" && dateStr > endDate {
				continue
			}
			matchedDate = true
			sqlDate := dateStr[:4] + "-" + dateStr[4:6] + "-" + dateStr[6:8]

			shopDirs, err := os.ReadDir(filepath.Join(yearPath, dateStr))
			if err != nil {
				continue
			}
			for _, sd := range shopDirs {
				if !sd.IsDir() {
					continue
				}
				shopPath := filepath.Join(yearPath, dateStr, sd.Name())

				files, err := os.ReadDir(shopPath)
				if err != nil {
					continue
				}
				for _, f := range files {
					if !strings.HasSuffix(f.Name(), ".xlsx") {
						continue
					}
					name := f.Name()
					fpath := filepath.Join(shopPath, name)

					switch {
					case strings.Contains(name, "经营概况"):
						cnt, _ := importBusinessOverview(db, fpath)
						total["shop"] += cnt
					case strings.Contains(name, "无界场景"):
						cnt, _ := importCampaign(db, fpath, sqlDate, "无界场景")
						total["campaign"] += cnt
					case strings.Contains(name, "智多星"):
						cnt, _ := importCampaign(db, fpath, sqlDate, "智多星")
						total["campaign"] += cnt
					case strings.Contains(name, "淘客"):
						cnt, _ := importCampaign(db, fpath, sqlDate, "淘客")
						total["campaign"] += cnt
					case strings.Contains(name, "市场_行业热词"):
						cnt, _ := importIndustryKeyword(db, fpath)
						total["industry_keyword"] += cnt
					case strings.Contains(name, "市场排名数据_") || strings.Contains(name, "市场数据_排名"):
						// 从文件名提取品类
						category := "排名"
						if idx := strings.Index(name, "市场排名数据_"); idx >= 0 {
							rest := name[idx+len("市场排名数据_"):]
							if dotIdx := strings.Index(rest, "."); dotIdx > 0 {
								category = rest[:dotIdx]
							}
						}
						cnt, _ := importMarketRank(db, fpath, sqlDate, category)
						total["market_rank"] += cnt
					}
				}
			}
			fmt.Printf("[%s] 完成\n", dateStr)
		}
	}
	if startDate != "" && endDate != "" && !matchedDate {
		log.Fatalf("未找到日期范围 %s-%s 的数据目录", startDate, endDate)
	}

	fmt.Println("\n导入完成:")
	for k, v := range total {
		fmt.Printf("  %s: %d 条\n", k, v)
	}
}

// importBusinessOverview 经营概况（含多天历史数据）
// 文件结构: [日期 支付金额 子订单均价 客单价 IPVUV 支付子订单数 支付商品件数 支付转化率 支付用户数]
func importBusinessOverview(db *sql.DB, fpath string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 2 {
		return 0, nil
	}
	count := 0
	for i := 1; i < len(rows); i++ {
		d := rows[i]
		if len(d) < 9 {
			continue
		}
		dateStr := strings.TrimSpace(d[0])
		if len(dateStr) != 8 {
			continue
		}
		statDate := dateStr[:4] + "-" + dateStr[4:6] + "-" + dateStr[6:8]
		_, err = db.Exec(`REPLACE INTO op_tmall_cs_shop_daily
			(stat_date, shop_name, pay_amount, sub_order_avg_price, avg_price, ipv_uv,
			 pay_sub_orders, pay_qty, conv_rate, pay_users)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			statDate, "天猫超市",
			toF(d, 1), toF(d, 2), toF(d, 3), toI(d, 4),
			toI(d, 5), toI(d, 6), toF(d, 7), toI(d, 8),
		)
		if err != nil {
			log.Printf("business_overview失败 %s: %v", statDate, err)
			continue
		}
		count++
	}
	return count, nil
}

// importIndustryKeyword 行业热词
// 文件结构: [统计日期 统计维度 业务渠道 搜索词 搜索曝光热度 引导成交热度 引导成交人气 引导成交规模 引导转化指数 引导访问热度 引导访问人气]
func importIndustryKeyword(db *sql.DB, fpath string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 2 {
		return 0, nil
	}
	count := 0
	for i := 1; i < len(rows); i++ {
		d := rows[i]
		if len(d) < 11 {
			continue
		}
		dateStr := strings.TrimSpace(d[0])
		if len(dateStr) != 8 {
			continue
		}
		statDate := dateStr[:4] + "-" + dateStr[4:6] + "-" + dateStr[6:8]
		keyword := strings.TrimSpace(d[3])
		if keyword == "" {
			continue
		}
		_, err = db.Exec(`REPLACE INTO op_tmall_cs_industry_keyword
			(stat_date, dimension, channel, keyword,
			 search_impression, trade_heat, trade_popularity, trade_scale, conv_index,
			 visit_heat, visit_popularity)
			VALUES (?,?,?,?, ?,?,?,?,?, ?,?)`,
			statDate, strings.TrimSpace(d[1]), strings.TrimSpace(d[2]), keyword,
			toF(d, 4), toF(d, 5), toF(d, 6), toF(d, 7), toF(d, 8),
			toF(d, 9), toF(d, 10),
		)
		if err != nil {
			continue
		}
		count++
	}
	return count, nil
}

// importMarketRank 市场排名
// 文件结构: [品牌名 成交热度 成交人气 访问热度 访问人气 转化指数 成交热度环比 成交人气环比 访问热度环比 访问人气环比 转化指数环比 交易指数 交易指数环比]
func importMarketRank(db *sql.DB, fpath, date, category string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 2 {
		return 0, nil
	}
	count := 0
	for i := 1; i < len(rows); i++ {
		d := rows[i]
		if len(d) < 13 {
			continue
		}
		brand := strings.TrimSpace(d[0])
		if brand == "" {
			continue
		}
		_, err = db.Exec(`REPLACE INTO op_tmall_cs_market_rank
			(stat_date, category, brand_name,
			 trade_heat, trade_popularity, visit_heat, visit_popularity, conv_index,
			 trade_heat_chg, trade_popularity_chg, visit_heat_chg, visit_popularity_chg, conv_index_chg,
			 trade_index, trade_index_chg)
			VALUES (?,?,?, ?,?,?,?,?, ?,?,?,?,?, ?,?)`,
			date, category, brand,
			toF(d, 1), toF(d, 2), toF(d, 3), toF(d, 4), toF(d, 5),
			toF(d, 6), toF(d, 7), toF(d, 8), toF(d, 9), toF(d, 10),
			toF(d, 11), toF(d, 12),
		)
		if err != nil {
			continue
		}
		count++
	}
	return count, nil
}

func importCampaign(db *sql.DB, fpath, date, promoType string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 2 {
		return 0, nil
	}
	d := rows[1]
	if len(d) < 3 {
		return 0, nil
	}
	_, err = db.Exec(`INSERT INTO op_tmall_cs_campaign_daily
		(stat_date, promo_type, cost, pay_amount, roi, clicks, impressions)
		VALUES (?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE cost=VALUES(cost)`,
		date, promoType, toF(d, 1), toF(d, 2), toF(d, 3), toI(d, 4), toI(d, 5))
	if err != nil {
		return 0, err
	}
	return 1, nil
}

func toF(d []string, i int) float64 {
	if i >= len(d) || d[i] == "" {
		return 0
	}
	s := strings.ReplaceAll(strings.ReplaceAll(d[i], ",", ""), "%", "")
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
func toI(d []string, i int) int { return int(toF(d, i)) }
