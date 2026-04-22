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

// parseExcelDate 严格解析 Excel 日期列，格式不合规返回 ""（调用方 fallback 到文件名日期）
// 支持: YYYY-MM-DD / YYYY/MM/DD / YYYY.MM.DD / YYYY年MM月DD日 / YYYYMMDD / YYYY-M-D
// 注意：YY 两位年份格式不受支持（避免误解析导致数据污染）
func parseExcelDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if idx := strings.Index(s, " "); idx > 0 {
		s = s[:idx]
	}
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ReplaceAll(s, "年", "-")
	s = strings.ReplaceAll(s, "月", "-")
	s = strings.ReplaceAll(s, "日", "")
	if len(s) == 8 && !strings.Contains(s, "-") {
		return s[:4] + "-" + s[4:6] + "-" + s[6:8]
	}
	parts := strings.Split(s, "-")
	if len(parts) != 3 {
		return ""
	}
	y, m, d := parts[0], parts[1], parts[2]
	if len(y) != 4 {
		return ""
	}
	if len(m) == 1 {
		m = "0" + m
	}
	if len(d) == 1 {
		d = "0" + d
	}
	if len(m) != 2 || len(d) != 2 {
		return ""
	}
	return y + "-" + m + "-" + d
}

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

// importShopDaily 导入京东店铺销售数据。
// RPA 抓到的 Excel 至少有两套列布局：
//   老格式: 时间 / 成交金额 / 成交商品件数 / 成交客户数 / 成交单量 / 店铺成交转化率 / 客单价 / 店铺浏览量 / 店铺访客数 / 平均停留时长 ...
//   新格式: 日期 / 浏览量 / 浏览量环比 / 访客数 / 访客数环比 / 人均浏览量 / 平均停留时间 / 跳失率 / 成交客户数 / 成交单量 / 成交金额 / 客单价 / ...
// 按列索引硬编码会错位（v0.27 之前版本），现改为按表头名查列 + 同义词兼容 + 核心字段缺失报错。
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
	header := rows[0]
	d := rows[1]
	idx := headerIdx(header)

	// 核心字段缺失校验：浏览量/访客数/成交金额 任一缺失说明 Excel 彻底换了，报错跳过
	colViews := findCol(idx, "店铺浏览量", "浏览量")
	colVisitors := findCol(idx, "店铺访客数", "访客数")
	colPayAmount := findCol(idx, "成交金额")
	if colViews < 0 || colVisitors < 0 || colPayAmount < 0 {
		return 0, fmt.Errorf("表头格式未识别（浏览量/访客数/成交金额 有缺失）: %v", header)
	}

	// stat_date 取 Excel 第一列业务日期（时间/日期），文件名日期只是 RPA 采集日
	colDate := findCol(idx, "时间", "日期")
	statDate := ""
	if colDate >= 0 && colDate < len(d) {
		statDate = parseExcelDate(d[colDate])
	}
	if statDate == "" {
		statDate = date
	}

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
		statDate, shop,
		getInt(d, colVisitors),
		getStr(d, findCol(idx, "访客数环比")),
		getInt(d, colViews),
		getStr(d, findCol(idx, "浏览量环比")),
		getFloat(d, findCol(idx, "店铺人均浏览量", "人均浏览量")),
		getFloat(d, findCol(idx, "店铺平均停留时长", "平均停留时长", "平均停留时间")),
		getFloat(d, findCol(idx, "跳失率")),
		getInt(d, findCol(idx, "成交客户数")),
		getStr(d, findCol(idx, "成交客户数环比")),
		getInt(d, findCol(idx, "成交商品件数")),
		getStr(d, findCol(idx, "成交商品件数环比")),
		getFloat(d, colPayAmount),
		getStr(d, findCol(idx, "成交金额环比")),
		getInt(d, findCol(idx, "成交单量")),
		getFloat(d, findCol(idx, "客单价")),
		getFloat(d, findCol(idx, "店铺成交转化率", "成交转化率")),
		getFloat(d, findCol(idx, "UV价值")), // 新格式无此列，fallback 0
		getFloat(d, findCol(idx, "退款金额", "取消及售后退款金额")),
		getInt(d, findCol(idx, "加购客户数")), // 新格式无此列，fallback 0
		0)                                 // collect_customers 无对应列
	if err != nil {
		return 0, err
	}
	return 1, nil
}

// importCustomerDaily 导入京东客户数据-洞察。按表头名映射。
// Excel 真实列：日期/进店客户数/进店同比/加购客户数/加购同比/下单客户数/下单同比/
//   成交客户数/成交同比/出库客户数/出库同比/复购客户数/复购同比
// 原版按奇数列索引硬编码，但 repurchase_customers 取了 d[9]=出库客户数（错）、
// lost_customers 取了 d[11]=复购客户数（错，应视为 repurchase），造成两列错位。
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
	header := rows[0]
	d := rows[1]
	idx := headerIdx(header)

	colBrowse := findCol(idx, "进店客户数")
	if colBrowse < 0 {
		return 0, fmt.Errorf("表头格式未识别（找不到'进店客户数'）: %v", header)
	}

	colDate := findCol(idx, "日期", "时间", "统计日期")
	statDate := ""
	if colDate >= 0 && colDate < len(d) {
		statDate = parseExcelDate(d[colDate])
	}
	if statDate == "" {
		statDate = date
	}

	_, err = db.Exec(`INSERT INTO op_jd_customer_daily
		(stat_date, shop_name, browse_customers, cart_customers, order_customers,
		 pay_customers, repurchase_customers, lost_customers)
		VALUES (?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
		 browse_customers=VALUES(browse_customers), cart_customers=VALUES(cart_customers),
		 order_customers=VALUES(order_customers), pay_customers=VALUES(pay_customers),
		 repurchase_customers=VALUES(repurchase_customers), lost_customers=VALUES(lost_customers)`,
		statDate, shop,
		getInt(d, colBrowse),
		getInt(d, findCol(idx, "加购客户数")),
		getInt(d, findCol(idx, "下单客户数")),
		getInt(d, findCol(idx, "成交客户数")),
		getInt(d, findCol(idx, "复购客户数")),            // 原版错位：存了出库客户数
		getInt(d, findCol(idx, "流失客户数")),            // Excel 无此列 fallback 0
	)
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

	// stat_date 取 Excel 第 0 列"日期"（业务日），文件名日期只是 RPA 采集日
	statDate := parseExcelDate(d[0])
	if statDate == "" {
		statDate = date
	}

	_, err = db.Exec(`INSERT INTO op_jd_promo_daily
		(stat_date, shop_name, promo_type, pay_goods_count, pay_amount, pay_count,
		 pay_users, conv_rate, uv, pv)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE pay_amount=VALUES(pay_amount)`,
		statDate, shop, promoType, toInt2(d, 1), toFloat2(d, 2), toInt2(d, 3),
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

// headerIdx 构建 Excel 表头 → 列索引映射。重复表头取第一次出现位置。
func headerIdx(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, h := range header {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		if _, ok := m[h]; !ok {
			m[h] = i
		}
	}
	return m
}

// findCol 按同义词列表查列索引，任一匹配即返回；都找不到返回 -1。
func findCol(idx map[string]int, aliases ...string) int {
	for _, a := range aliases {
		if i, ok := idx[a]; ok {
			return i
		}
	}
	return -1
}

// getInt / getFloat / getStr 按列索引取值，索引 <0 或越界返回零值。
func getInt(d []string, i int) int {
	if i < 0 || i >= len(d) {
		return 0
	}
	return toInt(d[i])
}

func getFloat(d []string, i int) float64 {
	if i < 0 || i >= len(d) {
		return 0
	}
	return toFloat(d[i])
}

func getStr(d []string, i int) interface{} {
	if i < 0 || i >= len(d) || d[i] == "" {
		return nil
	}
	return d[i]
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
	// 按 header 自适应是否有"搜索增长幅度"列（不依赖 rankType 参数 —
	// RPA 偶发把热搜榜格式错放到飙升榜文件会让原版 surge 分支整行错位）
	hasGrowth := false
	if len(rows[0]) > 1 && strings.TrimSpace(rows[0][1]) == "搜索增长幅度" {
		hasGrowth = true
	}
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
