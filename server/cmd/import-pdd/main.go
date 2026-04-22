package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/extrame/xls"
	_ "github.com/go-sql-driver/mysql"
	"github.com/xuri/excelize/v2"
)

var baseDir = `Z:\信息部\RPA_集团数据看板\拼多多`

// parseExcelDate 拼多多专用 Excel 日期解析
// 拼多多"统计时间"列格式是 MM-DD-YY（美式短年份）：例如 "12-29-25" = 2025-12-29
// 也兼容 YYYY-MM-DD / YYYYMMDD / YYYY年MM月DD日 等标准格式
// 格式不合规返回 ""（调用方 fallback 到文件名日期）
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
	// YYYYMMDD（无分隔符）
	if len(s) == 8 && !strings.Contains(s, "-") {
		return s[:4] + "-" + s[4:6] + "-" + s[6:8]
	}
	parts := strings.Split(s, "-")
	if len(parts) != 3 {
		return ""
	}
	// 情况 A: YYYY-MM-DD（4 位年份，标准 ISO）
	if len(parts[0]) == 4 {
		y, m, d := parts[0], parts[1], parts[2]
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
	// 情况 B: MM-DD-YY（拼多多美式短年份）
	// 识别条件: 三段都是 1-2 位，parts[0] ≤ 12（月份合法），parts[2] 是 YY
	if len(parts[0]) <= 2 && len(parts[1]) <= 2 && len(parts[2]) == 2 {
		mNum, e1 := strconv.Atoi(parts[0])
		dNum, e2 := strconv.Atoi(parts[1])
		yyNum, e3 := strconv.Atoi(parts[2])
		if e1 != nil || e2 != nil || e3 != nil {
			return ""
		}
		if mNum < 1 || mNum > 12 || dNum < 1 || dNum > 31 {
			return ""
		}
		// YY 补全：00-69 → 20xx, 70-99 → 19xx
		year := 2000 + yyNum
		if yyNum >= 70 {
			year = 1900 + yyNum
		}
		return fmt.Sprintf("%04d-%02d-%02d", year, mNum, dNum)
	}
	return ""
}

func main() {
	cfg, _ := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	db, _ := sql.Open("mysql", cfg.Database.DSN())
	defer db.Close()
	if err := ensurePDDCustomerTables(db); err != nil {
		log.Fatalf("初始化拼多多客服表失败: %v", err)
	}

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
				shopName := sd.Name()
				shopPath := filepath.Join(yearPath, dateStr, shopName)

				files, _ := os.ReadDir(shopPath)
				for _, f := range files {
					if f.IsDir() {
						continue
					}
					name := f.Name()
					fpath := filepath.Join(shopPath, name)

					var cnt int
					switch {
					case strings.Contains(name, "交易概况") && strings.HasSuffix(name, ".xlsx"):
						cnt, _ = importShopDaily(db, fpath, sqlDate, shopName)
						total["shop"] += cnt
					case strings.Contains(name, "商品概况") && strings.HasSuffix(name, ".xlsx"):
						cnt, _ = importGoodsDaily(db, fpath, sqlDate, shopName)
						total["goods"] += cnt
					case strings.Contains(name, "商品推广") && strings.HasSuffix(name, ".xlsx"):
						cnt, _ = importCampaign(db, fpath, sqlDate, shopName, "商品推广")
						total["campaign"] += cnt
					case strings.Contains(name, "明星店铺") && strings.HasSuffix(name, ".xlsx"):
						cnt, _ = importCampaign(db, fpath, sqlDate, shopName, "明星店铺")
						total["campaign"] += cnt
					case strings.Contains(name, "直播推广") && strings.HasSuffix(name, ".xlsx"):
						cnt, _ = importCampaign(db, fpath, sqlDate, shopName, "直播推广")
						total["campaign"] += cnt
					case strings.Contains(name, "多多视频") && strings.HasSuffix(name, ".json"):
						cnt, _ = importVideo(db, fpath, sqlDate, shopName)
						total["video"] += cnt
					case strings.Contains(name, "服务概况") && strings.HasSuffix(name, ".xlsx"):
						cnt, _ = importServiceOverview(db, fpath, sqlDate, shopName)
						total["service"] += cnt
					case strings.Contains(name, "客服_服务数据") && strings.HasSuffix(name, ".xlsx"):
						cnt, _ = importCustomerService(db, fpath, sqlDate, shopName)
						total["cs_service"] += cnt
					case strings.Contains(name, "客服_销售数据") && strings.HasSuffix(name, ".xlsx"):
						cnt, _ = importCustomerSales(db, fpath, shopName)
						total["cs_sales"] += cnt
					case strings.Contains(name, "商品数据") && strings.HasSuffix(name, ".json"):
						cnt, _ = importGoodsDetailJson(db, fpath, sqlDate, shopName)
						total["goods_detail"] += cnt
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

func ensurePDDCustomerTables(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS op_pdd_cs_service_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			shop_name VARCHAR(128) NOT NULL,
			consult_users INT DEFAULT 0,
			need_manual_reply_users INT DEFAULT 0,
			manual_receive_users INT DEFAULT 0,
			three_min_reply_rate_823 DECIMAL(10,4) DEFAULT 0,
			thirty_sec_reply_rate_823 DECIMAL(10,4) DEFAULT 0,
			avg_reply_minutes DECIMAL(10,4) DEFAULT 0,
			low_score_orders INT DEFAULT 0,
			dispute_refund_count INT DEFAULT 0,
			complaint_cs_users INT DEFAULT 0,
			complaint_long_no_reply_users INT DEFAULT 0,
			complaint_bad_attitude_users INT DEFAULT 0,
			complaint_abuse_users INT DEFAULT 0,
			complaint_spam_users INT DEFAULT 0,
			complaint_rate DECIMAL(10,4) DEFAULT 0,
			complaint_rate_30d DECIMAL(10,4) DEFAULT 0,
			three_min_reply_rate_all DECIMAL(10,4) DEFAULT 0,
			thirty_sec_reply_rate_all DECIMAL(10,4) DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_stat_shop (stat_date, shop_name),
			KEY idx_shop_date (shop_name, stat_date)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS op_pdd_cs_sales_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			shop_name VARCHAR(128) NOT NULL,
			consult_users INT DEFAULT 0,
			inquiry_users INT DEFAULT 0,
			final_group_users INT DEFAULT 0,
			inquiry_conv_rate DECIMAL(10,4) DEFAULT 0,
			cs_sales_amount DECIMAL(18,4) DEFAULT 0,
			cs_improvable_sales_amount DECIMAL(18,4) DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_stat_shop (stat_date, shop_name),
			KEY idx_shop_date (shop_name, stat_date)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// importCustomerService 拼多多-客服_服务数据(xlsx)
func importCustomerService(db *sql.DB, fpath, date, shop string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 4 {
		return 0, nil
	}
	count := 0
	for i := 3; i < len(rows); i++ {
		d := rows[i]
		if len(d) == 0 {
			continue
		}
		statDate := normalizeDate(toSS(d, 0))
		if statDate == "" {
			statDate = date
		}
		if statDate == "" {
			continue
		}

		_, err = db.Exec(`INSERT INTO op_pdd_cs_service_daily
			(stat_date, shop_name, consult_users, need_manual_reply_users, manual_receive_users,
			 three_min_reply_rate_823, thirty_sec_reply_rate_823, avg_reply_minutes, low_score_orders, dispute_refund_count,
			 complaint_cs_users, complaint_long_no_reply_users, complaint_bad_attitude_users, complaint_abuse_users, complaint_spam_users,
			 complaint_rate, complaint_rate_30d, three_min_reply_rate_all, thirty_sec_reply_rate_all)
			VALUES (?,?,?,?,?, ?,?,?,?, ?,?,?,?,?, ?,?,?,?,?)
			ON DUPLICATE KEY UPDATE
			 consult_users=VALUES(consult_users),
			 need_manual_reply_users=VALUES(need_manual_reply_users),
			 manual_receive_users=VALUES(manual_receive_users),
			 three_min_reply_rate_823=VALUES(three_min_reply_rate_823),
			 thirty_sec_reply_rate_823=VALUES(thirty_sec_reply_rate_823),
			 avg_reply_minutes=VALUES(avg_reply_minutes),
			 low_score_orders=VALUES(low_score_orders),
			 dispute_refund_count=VALUES(dispute_refund_count),
			 complaint_cs_users=VALUES(complaint_cs_users),
			 complaint_long_no_reply_users=VALUES(complaint_long_no_reply_users),
			 complaint_bad_attitude_users=VALUES(complaint_bad_attitude_users),
			 complaint_abuse_users=VALUES(complaint_abuse_users),
			 complaint_spam_users=VALUES(complaint_spam_users),
			 complaint_rate=VALUES(complaint_rate),
			 complaint_rate_30d=VALUES(complaint_rate_30d),
			 three_min_reply_rate_all=VALUES(three_min_reply_rate_all),
			 thirty_sec_reply_rate_all=VALUES(thirty_sec_reply_rate_all)`,
			statDate, shop,
			toI(d, 1), toI(d, 2), toI(d, 3),
			toF(d, 4), toF(d, 5), toF(d, 6), toI(d, 7), toI(d, 8),
			toI(d, 9), toI(d, 10), toI(d, 11), toI(d, 12), toI(d, 13),
			toF(d, 14), toF(d, 15), toF(d, 16), toF(d, 17),
		)
		if err != nil {
			log.Printf("客服_服务数据入库失败 [%s %s]: %v", statDate, shop, err)
			continue
		}
		count++
	}
	return count, nil
}

// importCustomerSales 拼多多-客服_销售数据(xlsx扩展名，实际为xls二进制)
func importCustomerSales(db *sql.DB, fpath, shop string) (int, error) {
	wb, err := xls.Open(fpath, "utf-8")
	if err != nil {
		return 0, err
	}
	sheet := wb.GetSheet(0)
	if sheet == nil {
		return 0, nil
	}

	count := 0
	for i := 3; i <= int(sheet.MaxRow); i++ {
		row := sheet.Row(i)
		if row == nil {
			continue
		}
		statDate := normalizeDate(strings.TrimSpace(row.Col(0)))
		if statDate == "" {
			continue
		}
		_, err = db.Exec(`INSERT INTO op_pdd_cs_sales_daily
			(stat_date, shop_name, consult_users, inquiry_users, final_group_users, inquiry_conv_rate, cs_sales_amount, cs_improvable_sales_amount)
			VALUES (?,?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE
			 consult_users=VALUES(consult_users),
			 inquiry_users=VALUES(inquiry_users),
			 final_group_users=VALUES(final_group_users),
			 inquiry_conv_rate=VALUES(inquiry_conv_rate),
			 cs_sales_amount=VALUES(cs_sales_amount),
			 cs_improvable_sales_amount=VALUES(cs_improvable_sales_amount)`,
			statDate, shop,
			toIS(row.Col(1)), toIS(row.Col(2)), toIS(row.Col(3)), toFS(row.Col(4)), toFS(row.Col(5)), toFS(row.Col(6)),
		)
		if err != nil {
			log.Printf("客服_销售数据入库失败 [%s %s]: %v", statDate, shop, err)
			continue
		}
		count++
	}
	return count, nil
}

func importShopDaily(db *sql.DB, fpath, date, shop string) (int, error) {
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
	if len(d) < 10 {
		return 0, nil
	}
	// stat_date 取 Excel 第 0 列"统计时间"（业务日），文件名日期只是 RPA 采集日
	statDate := parseExcelDate(d[0])
	if statDate == "" {
		statDate = date
	}
	// 文件结构: [统计时间 成交金额 较上周期 成交订单数 较上周期 成交买家数 较上周期 成交转化率 较上周期 客单价 较上周期 老买家占比 较上周期 关注用户数 较上周期 退款金额 较上周期 退款单数 较上周期 平均访客价值 较上周期]
	_, err = db.Exec(`REPLACE INTO op_pdd_shop_daily
		(stat_date, shop_name,
		 pay_amount, pay_amount_change, pay_count, pay_count_change,
		 pay_buyers, pay_buyers_change, conv_rate, conv_rate_change,
		 unit_price, unit_price_change, old_buyer_rate, old_buyer_rate_change,
		 follow_users, follow_users_change, refund_amount, refund_amount_change,
		 refund_orders, refund_orders_change, uv_value, uv_value_change)
		VALUES (?,?, ?,?,?,?, ?,?,?,?, ?,?,?,?, ?,?,?,?, ?,?,?,?)`,
		statDate, shop,
		toF(d, 1), toS(d, 2), toI(d, 3), toS(d, 4),
		toI(d, 5), toS(d, 6), toF(d, 7), toS(d, 8),
		toF(d, 9), toS(d, 10), toF(d, 11), toS(d, 12),
		toI(d, 13), toS(d, 14), toF(d, 15), toS(d, 16),
		toI(d, 17), toS(d, 18), toF(d, 19), toS(d, 20),
	)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

func importGoodsDaily(db *sql.DB, fpath, date, shop string) (int, error) {
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
	if len(d) < 10 {
		return 0, nil
	}
	// stat_date 取 Excel 第 0 列"统计时间"（业务日），文件名日期只是 RPA 采集日
	statDate := parseExcelDate(d[0])
	if statDate == "" {
		statDate = date
	}
	// 文件结构: [统计时间 商品访客 较上 浏览量 较上 收藏 较上 被访问商品 较上 成交金额 较上 成交订单 较上 成交买家 较上 成交转化率 较上]
	_, err = db.Exec(`REPLACE INTO op_pdd_goods_daily
		(stat_date, shop_name,
		 goods_visitors, goods_visitors_change, goods_views, goods_views_change,
		 goods_collect, goods_collect_change, sale_goods_count, sale_goods_change,
		 pay_amount, pay_amount_change, pay_count, pay_count_change,
		 pay_buyers, pay_buyers_change, conv_rate, conv_rate_change)
		VALUES (?,?, ?,?,?,?, ?,?,?,?, ?,?,?,?, ?,?,?,?)`,
		statDate, shop,
		toI(d, 1), toS(d, 2), toI(d, 3), toS(d, 4),
		toI(d, 5), toS(d, 6), toI(d, 7), toS(d, 8),
		toF(d, 9), toS(d, 10), toI(d, 11), toS(d, 12),
		toI(d, 13), toS(d, 14), toF(d, 15), toS(d, 16),
	)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

// importServiceOverview 拼多多-服务概况
func importServiceOverview(db *sql.DB, fpath, date, shop string) (int, error) {
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
	if len(d) < 12 {
		return 0, nil
	}
	// stat_date 取 Excel 第 0 列"统计时间"（业务日），文件名日期只是 RPA 采集日
	statDate := parseExcelDate(d[0])
	if statDate == "" {
		statDate = date
	}
	// [统计时间 纠纷退款数 纠纷退款率 介入订单数 平台介入率 品质退款率 平均退款时长 成功退款订单数 成功退款金额 成功退款率 退货退款自主完结时长 退款自主完结时长]
	_, err = db.Exec(`REPLACE INTO op_pdd_service_overview
		(stat_date, shop_name, dispute_refund_count, dispute_refund_rate, intervene_orders, intervene_rate,
		 quality_refund_rate, avg_refund_hours, success_refund_orders, success_refund_amount, success_refund_rate,
		 return_self_close, refund_self_close)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		statDate, shop,
		toI(d, 1), toF(d, 2), toI(d, 3), toF(d, 4),
		toF(d, 5), toS(d, 6), toI(d, 7), toF(d, 8), toF(d, 9),
		toS(d, 10), toS(d, 11),
	)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

// importGoodsDetailJson 拼多多-商品详细数据(json)
func importGoodsDetailJson(db *sql.DB, fpath, date, shop string) (int, error) {
	data, err := os.ReadFile(fpath)
	if err != nil {
		return 0, err
	}
	var resp struct {
		Result struct {
			GoodsDetailList []struct {
				StatDate        string `json:"statDate"`
				GoodsId         int64  `json:"goodsId"`
				GoodsName       string `json:"goodsName"`
				GoodsFavCnt     string `json:"goodsFavCnt"`
				GoodsUv         string `json:"goodsUv"`
				GoodsPv         string `json:"goodsPv"`
				PayOrdrCnt      string `json:"payOrdrCnt"`
				GoodsVcr        string `json:"goodsVcr"`
				PayOrdrGoodsQty string `json:"payOrdrGoodsQty"`
				PayOrdrUsrCnt   string `json:"payOrdrUsrCnt"`
				PayOrdrAmt      string `json:"payOrdrAmt"`
				CfmOrdrCnt      string `json:"cfmOrdrCnt"`
				CfmOrdrGoodsQty string `json:"cfmOrdrGoodsQty"`
			} `json:"goodsDetailList"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return 0, err
	}
	count := 0
	for _, g := range resp.Result.GoodsDetailList {
		statDate := g.StatDate
		if statDate == "" {
			statDate = date
		}
		_, err := db.Exec(`REPLACE INTO op_pdd_goods_detail
			(stat_date, shop_name, goods_id, goods_name,
			 goods_fav_cnt, goods_uv, goods_pv, pay_orders, goods_conv_rate,
			 pay_qty, pay_users, pay_amount, confirm_orders, confirm_qty)
			VALUES (?,?,?,?, ?,?,?,?,?, ?,?,?,?,?)`,
			statDate, shop, fmt.Sprintf("%d", g.GoodsId), g.GoodsName,
			toIS(g.GoodsFavCnt), toIS(g.GoodsUv), toIS(g.GoodsPv), toIS(g.PayOrdrCnt), toFS(g.GoodsVcr),
			toIS(g.PayOrdrGoodsQty), toIS(g.PayOrdrUsrCnt), toFS(g.PayOrdrAmt),
			toIS(g.CfmOrdrCnt), toIS(g.CfmOrdrGoodsQty),
		)
		if err != nil {
			log.Printf("插入goods_detail失败 [%d]: %v", g.GoodsId, err)
			continue
		}
		count++
	}
	return count, nil
}

// importCampaign 导入拼多多推广数据（商品推广/明星店铺/直播推广）。
// 三种推广类型的 Excel 列布局完全不同（原版共用硬编码索引导致字段全部错位）。
// 现按表头名映射并配同义词：
//   - cost:      总花费/花费 (明星店铺只有"花费"；商品推广/直播推广有"总花费")
//   - pay_amount: 交易额 (全系列)
//   - roi:       实际投产比/投入产出比
//   - real_pay_amount/real_roi: 净交易额+净实际投产比 (只有商品推广有；其他 fallback cost/roi)
//   - pay_orders: 成交笔数
//   - impressions: 曝光量
//   - clicks:    点击量 (明星店铺有，直播推广无)
func importCampaign(db *sql.DB, fpath, date, shop, promoType string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 2 {
		return 0, nil
	}
	header := rows[0]
	d := rows[1]
	idx := headerIdx(header)

	// 核心字段缺失保护
	colTrade := findCol(idx, "交易额(元)", "交易额")
	if colTrade < 0 {
		return 0, fmt.Errorf("[%s] 表头格式未识别（找不到'交易额'列）: %v", promoType, header)
	}

	// stat_date 取 Excel 第一列业务日期
	colDate := findCol(idx, "日期", "时间", "统计日期")
	statDate := ""
	if colDate >= 0 && colDate < len(d) {
		statDate = parseExcelDate(d[colDate])
	}
	if statDate == "" {
		statDate = date
	}

	cost := getFloat(d, findCol(idx, "总花费(元)", "花费(元)", "总花费", "花费"))
	payAmount := getFloat(d, colTrade)
	roi := getFloat(d, findCol(idx, "实际投产比", "投入产出比"))
	// 净*字段只有商品推广有；其他类型 fallback 为 cost/roi（语义近似）
	realPayAmount := getFloat(d, findCol(idx, "净交易额(元)", "净交易额"))
	if realPayAmount == 0 {
		realPayAmount = payAmount
	}
	realROI := getFloat(d, findCol(idx, "净实际投产比"))
	if realROI == 0 {
		realROI = roi
	}

	_, err = db.Exec(`INSERT INTO op_pdd_campaign_daily
		(stat_date, shop_name, promo_type, cost, pay_amount, roi, real_pay_amount, real_roi, pay_orders, impressions, clicks)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE cost=VALUES(cost), pay_amount=VALUES(pay_amount), roi=VALUES(roi),
		 real_pay_amount=VALUES(real_pay_amount), real_roi=VALUES(real_roi),
		 pay_orders=VALUES(pay_orders), impressions=VALUES(impressions), clicks=VALUES(clicks)`,
		statDate, shop, promoType, cost, payAmount, roi, realPayAmount, realROI,
		getInt(d, findCol(idx, "成交笔数")),
		getInt(d, findCol(idx, "曝光量")),
		getInt(d, findCol(idx, "点击量")),
	)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

// headerIdx/findCol/getInt/getFloat/getStr - 通用按表头名映射 helper
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

func findCol(idx map[string]int, aliases ...string) int {
	for _, a := range aliases {
		if i, ok := idx[a]; ok {
			return i
		}
	}
	return -1
}

func getInt(d []string, i int) int {
	if i < 0 || i >= len(d) {
		return 0
	}
	return toI(d, i)
}

func getFloat(d []string, i int) float64 {
	if i < 0 || i >= len(d) {
		return 0
	}
	return toF(d, i)
}

func getStr(d []string, i int) interface{} {
	if i < 0 || i >= len(d) || d[i] == "" {
		return nil
	}
	return d[i]
}

func importVideo(db *sql.DB, fpath, date, shop string) (int, error) {
	data, err := os.ReadFile(fpath)
	if err != nil {
		return 0, err
	}
	var resp struct {
		Result struct {
			TotalGMV     string `json:"totalGMV"`
			OrderCount   string `json:"orderCount"`
			OrderUV      string `json:"orderUV"`
			FeedCount    string `json:"feedCount"`
			VideoViewCnt string `json:"videoViewCnt"`
			GoodsClkCnt  string `json:"goodsClkCnt"`
		} `json:"result"`
	}
	json.Unmarshal(data, &resp)
	r := resp.Result
	_, err = db.Exec(`INSERT INTO op_pdd_video_daily
		(stat_date, shop_name, total_gmv, order_count, order_uv, feed_count, video_view_cnt, goods_click_cnt)
		VALUES (?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE total_gmv=VALUES(total_gmv)`,
		date, shop, toFS(r.TotalGMV), toIS(r.OrderCount), toIS(r.OrderUV),
		toIS(r.FeedCount), toIS(r.VideoViewCnt), toIS(r.GoodsClkCnt))
	if err != nil {
		return 0, err
	}
	return 1, nil
}

func toF(d []string, i int) float64 {
	if i >= len(d) || d[i] == "" {
		return 0
	}
	return parseFloatText(d[i])
}
func toI(d []string, i int) int {
	if i >= len(d) || d[i] == "" {
		return 0
	}
	return int(toF(d, i))
}
func toSS(d []string, i int) string {
	if i >= len(d) {
		return ""
	}
	return strings.TrimSpace(d[i])
}
func toS(d []string, i int) interface{} {
	if i >= len(d) || d[i] == "" {
		return nil
	}
	return d[i]
}
func toFS(s string) float64 { return parseFloatText(s) }
func toIS(s string) int     { return int(parseFloatText(s)) }

func parseFloatText(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "%", "")
	if s == "" || s == "-" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func normalizeDate(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "/", "-")
	if len(s) == 8 && isDigits(s) {
		return s[:4] + "-" + s[4:6] + "-" + s[6:8]
	}
	if len(s) == 10 && s[4] == '-' && s[7] == '-' {
		return s
	}
	return ""
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
