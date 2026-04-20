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

var baseDir = `Z:\信息部\RPA_集团数据看板\京东`

func main() {
	cfg, _ := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// 可选参数：起止日期
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

	// 遍历年份/日期/店铺
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
			dateStr := dd.Name() // YYYYMMDD
			if startDate != "" && dateStr < startDate {
				continue
			}
			if endDate != "" && dateStr > endDate {
				continue
			}
			matchedDate = true
			sqlDate := dateStr[:4] + "-" + dateStr[4:6] + "-" + dateStr[6:8]

			datePath := filepath.Join(yearPath, dateStr)
			shopDirs, err := os.ReadDir(datePath)
			if err != nil {
				continue
			}
			for _, sd := range shopDirs {
				if !sd.IsDir() {
					continue
				}
				shopName := sd.Name()
				shopPath := filepath.Join(datePath, shopName)

				files, _ := os.ReadDir(shopPath)
				for _, f := range files {
					if f.IsDir() || !strings.HasSuffix(f.Name(), ".xlsx") {
						continue
					}
					if strings.HasSuffix(f.Name(), ".xls.xlsx") {
						continue
					} // 跳过转换的
					name := f.Name()
					fpath := filepath.Join(shopPath, name)

					var cnt int
					var err error
					switch {
					case strings.Contains(name, "销售数据") && !strings.HasSuffix(name, ".xls"):
						cnt, err = importShopDaily(db, fpath, sqlDate, shopName)
						total["shop_daily"] += cnt
					// 京东联盟由 import-promo/importJDAffiliate 处理（遍历 Excel 日期列，支持多天数据）
					// 这里移除 importAffiliate 调用，避免双工具冲突写入 op_jd_affiliate_daily
					case strings.Contains(name, "洞察"):
						cnt, err = importCustomerDaily(db, fpath, sqlDate, shopName)
						total["customer_daily"] += cnt
					case strings.Contains(name, "新老客"):
						cnt, err = importCustomerType(db, fpath, sqlDate, shopName)
						total["customer_type"] += cnt
					case strings.Contains(name, "便宜包邮"):
						cnt, err = importPromoSku(db, fpath, sqlDate, shopName, "便宜包邮")
						total["promo_sku"] += cnt
					case strings.Contains(name, "百亿补贴"):
						cnt, err = importPromoDaily(db, fpath, sqlDate, shopName, "百亿补贴")
						total["promo_daily"] += cnt
					case strings.Contains(name, "秒杀活动"):
						cnt, err = importPromoDaily(db, fpath, sqlDate, shopName, "秒杀活动")
						total["promo_daily"] += cnt
					case strings.Contains(name, "交易榜"):
						cnt, err = importIndustryKeyword(db, fpath, sqlDate, shopName)
						total["industry"] += cnt
					case strings.Contains(name, "热搜榜") && strings.HasSuffix(name, ".xlsx"):
						cnt, err = importIndustryRank(db, fpath, sqlDate, shopName, "hot_search")
						total["industry_rank"] += cnt
					case strings.Contains(name, "飙升榜") && strings.HasSuffix(name, ".xlsx"):
						cnt, err = importIndustryRank(db, fpath, sqlDate, shopName, "surge")
						total["industry_rank"] += cnt
					}
					if err != nil {
						log.Printf("[%s/%s] %s 失败: %v", dateStr, shopName, name, err)
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

func importShopDaily(db *sql.DB, fpath, date, shop string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	rows, _ := f.GetRows(sheet)
	if len(rows) < 2 {
		return 0, nil
	}
	// 第一行表头，第二行数据
	// 列布局: [0]时间 [1]成交金额 [2]成交商品件数 [3]成交客户数 [4]成交单量
	// [5]店铺成交转化率 [6]客单价 [7]店铺浏览量 [8]店铺访客数 [9]平均停留时长
	// [10]加购客户数 [11]加购商品件数 [12]加购转化率 [13]商品成交转化率 [14]件单价
	// [15]UV价值 [16]店铺人均浏览量 [17]商品浏览量 [18]商品访客数 [19]商品人均浏览量
	// [20]商详平均停留时长 [21]加购商品件数(正向) [22]加购商品件数(负向)
	// [23]下单金额 [24]下单商品件数 [25]下单单量 [26]下单客户数 [27]下单转化率
	// [28]下单成交转化率 [29]退款金额
	d := rows[1]
	if len(d) < 9 {
		return 0, fmt.Errorf("列数不足: %d", len(d))
	}

	visitors := toInt2(d, 8)
	payAmount := toFloat(d[1])
	uvValue := toFloat2(d, 15)

	_, err = db.Exec(`INSERT INTO op_jd_shop_daily
		(stat_date, shop_name, visitors, visitors_change, page_views, page_views_change,
		 avg_visit_depth, avg_stay_time, bounce_rate,
		 pay_customers, pay_customers_change, pay_count, pay_count_change,
		 pay_amount, pay_amount_change, pay_orders, unit_price, conv_rate, uv_value,
		 refund_amount, cart_customers, collect_customers)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
		 visitors=VALUES(visitors), visitors_change=VALUES(visitors_change),
		 page_views=VALUES(page_views), page_views_change=VALUES(page_views_change),
		 avg_visit_depth=VALUES(avg_visit_depth), avg_stay_time=VALUES(avg_stay_time),
		 bounce_rate=VALUES(bounce_rate), pay_customers=VALUES(pay_customers),
		 pay_customers_change=VALUES(pay_customers_change), pay_count=VALUES(pay_count),
		 pay_count_change=VALUES(pay_count_change), pay_amount=VALUES(pay_amount),
		 pay_amount_change=VALUES(pay_amount_change), pay_orders=VALUES(pay_orders),
		 unit_price=VALUES(unit_price), conv_rate=VALUES(conv_rate), uv_value=VALUES(uv_value),
		 refund_amount=VALUES(refund_amount), cart_customers=VALUES(cart_customers)`,
		date, shop,
		visitors, nil,             // 访客数, 环比(无)
		toInt2(d, 7), nil,         // 店铺浏览量, 环比(无)
		toFloat2(d, 16),           // 店铺人均浏览量
		toFloat2(d, 9),            // 平均停留时长
		0,                         // 跳失率(无)
		toInt2(d, 3), nil,         // 成交客户数, 环比(无)
		toInt2(d, 2), nil,         // 成交商品件数, 环比(无)
		payAmount, nil,            // 成交金额, 环比(无)
		toInt2(d, 4),              // 成交单量
		toFloat2(d, 6),            // 客单价
		toFloat2(d, 5),            // 店铺成交转化率
		uvValue,                   // UV价值
		toFloat2(d, 29),           // 退款金额
		toInt2(d, 10),             // 加购客户数
		0)                         // collect_customers(无对应列)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

func importCustomerDaily(db *sql.DB, fpath, date, shop string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	rows, _ := f.GetRows(sheet)
	if len(rows) < 2 {
		return 0, nil
	}
	d := rows[1]
	if len(d) < 12 {
		return 0, fmt.Errorf("列数不足: %d", len(d))
	}

	_, err = db.Exec(`INSERT INTO op_jd_customer_daily
		(stat_date, shop_name, browse_customers, cart_customers, order_customers,
		 pay_customers, repurchase_customers, lost_customers)
		VALUES (?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE browse_customers=VALUES(browse_customers)`,
		date, shop, toInt(d[1]), toInt(d[3]), toInt(d[5]),
		toInt(d[7]), toInt(d[9]), toInt(d[11]))
	if err != nil {
		return 0, err
	}
	return 1, nil
}

func importCustomerType(db *sql.DB, fpath, date, shop string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	rows, _ := f.GetRows(sheet)
	cnt := 0
	for i, d := range rows {
		if i == 0 {
			continue
		} // 跳过表头
		if len(d) < 7 {
			continue
		}
		custType := d[0]
		if custType == "" {
			continue
		}

		_, err := db.Exec(`INSERT INTO op_jd_customer_type_daily
			(stat_date, shop_name, customer_type, pay_customers, pay_pct, conv_rate,
			 unit_price, unit_count, item_price)
			VALUES (?,?,?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE pay_customers=VALUES(pay_customers)`,
			date, shop, custType, toInt(d[1]), toFloat(d[3]), toFloat(d[5]),
			toFloat(d[7]), toFloat(d[9]), toFloat(d[11]))
		if err != nil {
			continue
		}
		cnt++
	}
	return cnt, nil
}

func importPromoSku(db *sql.DB, fpath, date, shop, promoType string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	rows, _ := f.GetRows(sheet)
	cnt := 0
	for i, d := range rows {
		if i == 0 {
			continue
		}
		if len(d) < 5 {
			continue
		}

		_, err := db.Exec(`INSERT INTO op_jd_promo_sku_daily
			(stat_date, shop_name, promo_type, sku_id, goods_name, uv, pv,
			 pay_count, pay_amount, pay_users, pay_orders, avg_price)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			date, shop, promoType, gs2(d, 0), gs2(d, 1), toInt2(d, 2), toInt2(d, 3),
			toInt2(d, 4), toFloat2(d, 5), toInt2(d, 6), toInt2(d, 7), toFloat2(d, 8))
		if err != nil {
			continue
		}
		cnt++
	}
	return cnt, nil
}

func importPromoDaily(db *sql.DB, fpath, date, shop, promoType string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	rows, _ := f.GetRows(sheet)
	if len(rows) < 2 {
		return 0, nil
	}
	d := rows[1]
	if len(d) < 5 {
		return 0, nil
	}

	_, err = db.Exec(`INSERT INTO op_jd_promo_daily
		(stat_date, shop_name, promo_type, pay_goods_count, pay_amount, pay_count,
		 pay_users, conv_rate, uv, pv)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE pay_amount=VALUES(pay_amount)`,
		date, shop, promoType, toInt2(d, 1), toFloat2(d, 2), toInt2(d, 3),
		toInt2(d, 4), toFloat2(d, 5), toInt2(d, 6), toInt2(d, 7))
	if err != nil {
		return 0, err
	}
	return 1, nil
}

func importIndustryKeyword(db *sql.DB, fpath, date, shop string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	rows, _ := f.GetRows(sheet)
	cnt := 0
	for i, d := range rows {
		if i == 0 {
			continue
		}
		if len(d) < 5 {
			continue
		}
		if d[0] == "" {
			continue
		}

		_, err := db.Exec(`INSERT INTO op_jd_industry_keyword
			(stat_date, shop_name, keyword, search_rank, compete_rank, click_rank,
			 pay_amount_range, conv_rate_range, related_goods, cart_ref, top_brand)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			date, shop, d[0], gs2(d, 1), gs2(d, 2), gs2(d, 3),
			gs2(d, 5), gs2(d, 7), toInt2(d, 9), gs2(d, 10), gs2(d, 11))
		if err != nil {
			continue
		}
		cnt++
	}
	return cnt, nil
}

// 工具函数
func toInt(s string) int {
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "%", "")
	v, _ := strconv.ParseFloat(s, 64)
	return int(v)
}

func toFloat(s string) float64 {
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "%", "")
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func gs(d []string, i int) interface{} {
	if i >= len(d) || d[i] == "" {
		return nil
	}
	return d[i]
}

func gs2(d []string, i int) interface{} {
	if i >= len(d) || d[i] == "" {
		return nil
	}
	return d[i]
}

func toInt2(d []string, i int) int {
	if i >= len(d) {
		return 0
	}
	return toInt(d[i])
}

func toFloat2(d []string, i int) float64 {
	if i >= len(d) {
		return 0
	}
	return toFloat(d[i])
}

// importIndustryRank 导入行业榜单（热搜榜/飙升榜）
// 热搜榜列: [关键词 搜索人数 搜索次数 点击人数 点击次数 点击率 成交金额 成交单量 成交转化率 在线商品数 快车参考价 最优品类]
// 飙升榜列: [关键词 搜索增长幅度 搜索人数 搜索次数 点击人数 点击次数 点击率 成交金额 成交单量 成交转化率 在线商品数 快车参考价 最优品类]
func importIndustryRank(db *sql.DB, fpath, date, shop, rankType string) (int, error) {
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
	hasGrowth := rankType == "surge" // 飙升榜多一列
	for i := 1; i < len(rows); i++ {
		d := rows[i]
		if len(d) < 12 {
			continue
		}
		keyword := strings.TrimSpace(d[0])
		if keyword == "" {
			continue
		}
		var growth, su, sc, cu, cc, ctr, payAmt, payOrd, conv, cartRef, bestCat string
		var online int
		if hasGrowth {
			if len(d) < 13 {
				continue
			}
			growth = strings.TrimSpace(d[1])
			su, sc, cu, cc = strings.TrimSpace(d[2]), strings.TrimSpace(d[3]), strings.TrimSpace(d[4]), strings.TrimSpace(d[5])
			ctr = strings.TrimSpace(d[6])
			payAmt, payOrd, conv = strings.TrimSpace(d[7]), strings.TrimSpace(d[8]), strings.TrimSpace(d[9])
			online = toInt(strings.TrimSpace(d[10]))
			cartRef, bestCat = strings.TrimSpace(d[11]), strings.TrimSpace(d[12])
		} else {
			su, sc, cu, cc = strings.TrimSpace(d[1]), strings.TrimSpace(d[2]), strings.TrimSpace(d[3]), strings.TrimSpace(d[4])
			ctr = strings.TrimSpace(d[5])
			payAmt, payOrd, conv = strings.TrimSpace(d[6]), strings.TrimSpace(d[7]), strings.TrimSpace(d[8])
			online = toInt(strings.TrimSpace(d[9]))
			cartRef, bestCat = strings.TrimSpace(d[10]), strings.TrimSpace(d[11])
		}
		_, err = db.Exec(`REPLACE INTO op_jd_industry_rank
			(stat_date, shop_name, rank_type, keyword, search_growth,
			 search_users, search_count, click_users, click_count, ctr_range,
			 pay_amount_range, pay_orders_range, conv_rate_range,
			 online_goods, cart_ref, best_category)
			VALUES (?,?,?,?,?, ?,?,?,?,?, ?,?,?, ?,?,?)`,
			date, shop, rankType, keyword, growth,
			su, sc, cu, cc, ctr,
			payAmt, payOrd, conv,
			online, cartRef, bestCat,
		)
		if err != nil {
			continue
		}
		count++
	}
	return count, nil
}
