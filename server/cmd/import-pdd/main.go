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
	if err := ensurePDDCampaignVideoTables(db); err != nil {
		log.Fatalf("初始化拼多多商品推广/视频表失败: %v", err)
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
						gCnt, _ := importCampaignGoods(db, fpath, sqlDate, shopName)
						total["campaign_goods"] += gCnt
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

// ensurePDDCampaignVideoTables 创建拼多多商品推广SKU级日数据表 + 多多视频日数据表
// 表结构来自实际生产 SHOW CREATE TABLE，含完整中文 COMMENT + UK + 索引
func ensurePDDCampaignVideoTables(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS op_pdd_campaign_goods_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL COMMENT '业务日期(读Excel日期列,fallback文件名)',
			shop_name VARCHAR(128) NOT NULL COMMENT '店铺名',
			goods_id VARCHAR(64) NOT NULL COMMENT '商品ID',
			goods_name VARCHAR(500) DEFAULT NULL COMMENT '商品名称',
			promo_scene VARCHAR(64) NOT NULL DEFAULT '' COMMENT '推广场景(稳定成本推广/标准推广等)',
			promo_name VARCHAR(500) DEFAULT NULL COMMENT '推广计划名称',
			bid_method VARCHAR(128) DEFAULT NULL COMMENT '出价方式(目标投产比/目标成交花费等)',
			group_name VARCHAR(128) DEFAULT NULL COMMENT '分组',
			is_deleted VARCHAR(16) DEFAULT NULL COMMENT '是否已删除',
			cost_pay DECIMAL(14,2) DEFAULT 0.00 COMMENT '成交花费(元)',
			pay_amount DECIMAL(14,2) DEFAULT 0.00 COMMENT '交易额(元)',
			roi DECIMAL(10,4) DEFAULT 0.0000 COMMENT '实际投产比',
			cost_total DECIMAL(14,2) DEFAULT 0.00 COMMENT '总花费(元)',
			net_pay_amount DECIMAL(14,2) DEFAULT 0.00 COMMENT '净交易额(元)',
			net_roi DECIMAL(10,4) DEFAULT 0.0000 COMMENT '净实际投产比',
			net_pay_orders INT DEFAULT 0 COMMENT '净成交笔数',
			net_pay_cost_per_order DECIMAL(10,4) DEFAULT 0.0000 COMMENT '每笔净成交花费(元)',
			net_pay_amount_ratio DECIMAL(8,4) DEFAULT NULL COMMENT '净交易额占比',
			net_pay_orders_ratio DECIMAL(8,4) DEFAULT NULL COMMENT '净成交笔数占比',
			net_pay_amount_per_order DECIMAL(14,4) DEFAULT 0.0000 COMMENT '每笔净成交金额(元)',
			settle_pay_amount DECIMAL(14,2) DEFAULT 0.00 COMMENT '结算交易额(元)',
			settle_roi DECIMAL(10,4) DEFAULT 0.0000 COMMENT '结算投产比',
			settle_pay_orders INT DEFAULT 0 COMMENT '结算成交笔数',
			refund_exempt_rate DECIMAL(8,4) DEFAULT NULL COMMENT '退款豁免率',
			cancel_exempt_rate DECIMAL(8,4) DEFAULT NULL COMMENT '退单豁免率',
			settle_cost_per_order DECIMAL(10,4) DEFAULT 0.0000 COMMENT '每笔结算成交花费(元)',
			settle_pay_amount_rate DECIMAL(8,4) DEFAULT NULL COMMENT '交易额结算率',
			settle_pay_orders_rate DECIMAL(8,4) DEFAULT NULL COMMENT '订单结算率',
			settle_pay_amount_per_order DECIMAL(14,4) DEFAULT 0.0000 COMMENT '每笔结算成交金额(元)',
			pay_orders INT DEFAULT 0 COMMENT '成交笔数',
			cost_per_order DECIMAL(10,4) DEFAULT 0.0000 COMMENT '每笔成交花费(元)',
			pay_amount_per_order DECIMAL(14,4) DEFAULT 0.0000 COMMENT '每笔成交金额(元)',
			direct_pay_amount DECIMAL(14,2) DEFAULT 0.00 COMMENT '直接交易额(元)',
			indirect_pay_amount DECIMAL(14,2) DEFAULT 0.00 COMMENT '间接交易额(元)',
			direct_pay_orders INT DEFAULT 0 COMMENT '直接成交笔数',
			indirect_pay_orders INT DEFAULT 0 COMMENT '间接成交笔数',
			impressions BIGINT DEFAULT 0 COMMENT '曝光量',
			clicks INT DEFAULT 0 COMMENT '点击量',
			inquiry_cost DECIMAL(14,2) DEFAULT 0.00 COMMENT '询单花费(元)',
			inquiry_count INT DEFAULT 0 COMMENT '询单量',
			inquiry_avg_cost DECIMAL(10,4) DEFAULT 0.0000 COMMENT '平均询单成本(元)',
			collect_cost DECIMAL(14,2) DEFAULT 0.00 COMMENT '收藏花费(元)',
			collect_count INT DEFAULT 0 COMMENT '收藏量',
			collect_avg_cost DECIMAL(10,4) DEFAULT 0.0000 COMMENT '平均收藏成本(元)',
			follow_cost DECIMAL(14,2) DEFAULT 0.00 COMMENT '关注花费(元)',
			follow_count INT DEFAULT 0 COMMENT '关注量',
			follow_avg_cost DECIMAL(10,4) DEFAULT 0.0000 COMMENT '平均关注成本(元)',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_date_shop_goods_scene (stat_date, shop_name, goods_id, promo_scene),
			KEY idx_stat_date (stat_date),
			KEY idx_shop_goods (shop_name, goods_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='拼多多商品推广SKU级日数据'`,
		`CREATE TABLE IF NOT EXISTS op_pdd_video_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL COMMENT '统计日期',
			shop_name VARCHAR(100) NOT NULL COMMENT '店铺名称',
			total_gmv DECIMAL(14,2) DEFAULT 0.00 COMMENT '总GMV',
			order_count INT DEFAULT 0 COMMENT '订单数',
			order_uv INT DEFAULT 0 COMMENT '下单用户数',
			feed_count INT DEFAULT 0 COMMENT '作品数',
			video_view_cnt INT DEFAULT 0 COMMENT '视频播放量',
			goods_click_cnt INT DEFAULT 0 COMMENT '商品点击数',
			has_explain_goods_total INT DEFAULT NULL COMMENT '有讲解的商品数',
			un_open_window_goods_total INT DEFAULT NULL COMMENT '未开通橱窗的商品数',
			explain_cover_rate DECIMAL(8,4) DEFAULT NULL COMMENT '讲解覆盖率(=有讲解商品/总商品)',
			total_gmv_rate DECIMAL(10,4) DEFAULT NULL COMMENT 'GMV环比变化率',
			order_count_rate DECIMAL(10,4) DEFAULT NULL COMMENT '订单数环比变化率',
			order_uv_rate DECIMAL(10,4) DEFAULT NULL COMMENT '买家数环比变化率',
			feed_count_rate DECIMAL(10,4) DEFAULT NULL COMMENT '视频条数环比变化率',
			video_view_cnt_rate DECIMAL(10,4) DEFAULT NULL COMMENT '视频播放数环比变化率',
			goods_click_cnt_growth_rate DECIMAL(10,4) DEFAULT NULL COMMENT '商品点击数环比变化率',
			has_explain_goods_total_growth_rate DECIMAL(10,4) DEFAULT NULL COMMENT '讲解商品数环比变化率',
			un_open_window_goods_growth_total DECIMAL(10,4) DEFAULT NULL COMMENT '未开橱窗商品数环比变化率',
			explain_cover_growth_rate DECIMAL(10,4) DEFAULT NULL COMMENT '讲解覆盖率环比变化',
			use_flow_card_cnt INT DEFAULT NULL COMMENT '已使用流量卡数量',
			obtain_flow_card_cnt INT DEFAULT NULL COMMENT '已获得流量卡数量',
			usable_flow_card_cnt INT DEFAULT NULL COMMENT '可用流量卡数量',
			own_top_goods JSON DEFAULT NULL COMMENT '自营热销商品Top列表(原始JSON数组)',
			other_top_goods JSON DEFAULT NULL COMMENT '其他热销商品Top列表(原始JSON数组)',
			UNIQUE KEY uk_date_shop (stat_date, shop_name)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='拼多多多多视频日数据'`,
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

// importCampaignGoods 读拼多多"商品推广.xlsx"的第二个 sheet（商品_分天数据_xxx），
// 按表头名映射写入 op_pdd_campaign_goods_daily。每行 = 一个 (商品ID × 推广场景) 当日数据。
// 仅商品推广文件有此 sheet；明星店铺/直播推广跳过。
func importCampaignGoods(db *sql.DB, fpath, date, shop string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	// 找以 "商品_" 开头的 sheet
	var goodsSheet string
	for _, name := range f.GetSheetList() {
		if strings.HasPrefix(name, "商品_") {
			goodsSheet = name
			break
		}
	}
	if goodsSheet == "" {
		return 0, nil // 无商品 sheet（可能是其他推广文件）
	}

	rows, _ := f.GetRows(goodsSheet)
	if len(rows) < 2 {
		return 0, nil
	}
	header := rows[0]
	idx := headerIdx(header)

	colDate := findCol(idx, "日期", "时间", "统计日期")
	colGoodsID := findCol(idx, "商品ID")
	if colGoodsID < 0 {
		return 0, fmt.Errorf("商品 sheet 表头缺少'商品ID'列: %v", header)
	}

	count := 0
	for ri := 1; ri < len(rows); ri++ {
		d := rows[ri]
		if len(d) <= colGoodsID {
			continue
		}
		goodsID := strings.TrimSpace(d[colGoodsID])
		if goodsID == "" || goodsID == "-" || goodsID == "总计" {
			continue
		}

		// 业务日期优先 Excel，fallback 文件名
		statDate := date
		if colDate >= 0 && colDate < len(d) {
			if pd := parseExcelDate(d[colDate]); pd != "" {
				statDate = pd
			}
		}

		_, err = db.Exec(`INSERT INTO op_pdd_campaign_goods_daily
			(stat_date, shop_name, goods_id, goods_name, promo_scene, promo_name, bid_method, group_name, is_deleted,
			 cost_pay, pay_amount, roi, cost_total, net_pay_amount, net_roi, net_pay_orders, net_pay_cost_per_order,
			 net_pay_amount_ratio, net_pay_orders_ratio, net_pay_amount_per_order,
			 settle_pay_amount, settle_roi, settle_pay_orders, refund_exempt_rate, cancel_exempt_rate,
			 settle_cost_per_order, settle_pay_amount_rate, settle_pay_orders_rate, settle_pay_amount_per_order,
			 pay_orders, cost_per_order, pay_amount_per_order,
			 direct_pay_amount, indirect_pay_amount, direct_pay_orders, indirect_pay_orders,
			 impressions, clicks,
			 inquiry_cost, inquiry_count, inquiry_avg_cost,
			 collect_cost, collect_count, collect_avg_cost,
			 follow_cost, follow_count, follow_avg_cost)
			VALUES (?,?,?,?,?,?,?,?,?, ?,?,?,?,?,?,?,?, ?,?,?, ?,?,?,?,?, ?,?,?,?, ?,?,?, ?,?,?,?, ?,?, ?,?,?, ?,?,?, ?,?,?)
			ON DUPLICATE KEY UPDATE
			 goods_name=VALUES(goods_name), promo_name=VALUES(promo_name), bid_method=VALUES(bid_method),
			 group_name=VALUES(group_name), is_deleted=VALUES(is_deleted),
			 cost_pay=VALUES(cost_pay), pay_amount=VALUES(pay_amount), roi=VALUES(roi),
			 cost_total=VALUES(cost_total), net_pay_amount=VALUES(net_pay_amount), net_roi=VALUES(net_roi),
			 net_pay_orders=VALUES(net_pay_orders), net_pay_cost_per_order=VALUES(net_pay_cost_per_order),
			 net_pay_amount_ratio=VALUES(net_pay_amount_ratio), net_pay_orders_ratio=VALUES(net_pay_orders_ratio),
			 net_pay_amount_per_order=VALUES(net_pay_amount_per_order),
			 settle_pay_amount=VALUES(settle_pay_amount), settle_roi=VALUES(settle_roi), settle_pay_orders=VALUES(settle_pay_orders),
			 refund_exempt_rate=VALUES(refund_exempt_rate), cancel_exempt_rate=VALUES(cancel_exempt_rate),
			 settle_cost_per_order=VALUES(settle_cost_per_order), settle_pay_amount_rate=VALUES(settle_pay_amount_rate),
			 settle_pay_orders_rate=VALUES(settle_pay_orders_rate), settle_pay_amount_per_order=VALUES(settle_pay_amount_per_order),
			 pay_orders=VALUES(pay_orders), cost_per_order=VALUES(cost_per_order), pay_amount_per_order=VALUES(pay_amount_per_order),
			 direct_pay_amount=VALUES(direct_pay_amount), indirect_pay_amount=VALUES(indirect_pay_amount),
			 direct_pay_orders=VALUES(direct_pay_orders), indirect_pay_orders=VALUES(indirect_pay_orders),
			 impressions=VALUES(impressions), clicks=VALUES(clicks),
			 inquiry_cost=VALUES(inquiry_cost), inquiry_count=VALUES(inquiry_count), inquiry_avg_cost=VALUES(inquiry_avg_cost),
			 collect_cost=VALUES(collect_cost), collect_count=VALUES(collect_count), collect_avg_cost=VALUES(collect_avg_cost),
			 follow_cost=VALUES(follow_cost), follow_count=VALUES(follow_count), follow_avg_cost=VALUES(follow_avg_cost)`,
			statDate, shop, goodsID,
			toSS(d, findCol(idx, "商品名称")),
			toSS(d, findCol(idx, "推广场景")),
			toSS(d, findCol(idx, "推广名称")),
			toSS(d, findCol(idx, "出价方式")),
			toSS(d, findCol(idx, "分组")),
			toSS(d, findCol(idx, "是否已删除")),
			getFloat(d, findCol(idx, "成交花费(元)", "成交花费")),
			getFloat(d, findCol(idx, "交易额(元)")),
			getFloat(d, findCol(idx, "实际投产比")),
			getFloat(d, findCol(idx, "总花费(元)", "总花费")),
			getFloat(d, findCol(idx, "净交易额(元)")),
			getFloat(d, findCol(idx, "净实际投产比")),
			getInt(d, findCol(idx, "净成交笔数")),
			getFloat(d, findCol(idx, "每笔净成交花费(元)")),
			getPctFloat(d, findCol(idx, "净交易额占比")),
			getPctFloat(d, findCol(idx, "净成交笔数占比")),
			getFloat(d, findCol(idx, "每笔净成交金额(元)")),
			getFloat(d, findCol(idx, "结算交易额(元)")),
			getFloat(d, findCol(idx, "结算投产比")),
			getInt(d, findCol(idx, "结算成交笔数")),
			getPctFloat(d, findCol(idx, "退款豁免率")),
			getPctFloat(d, findCol(idx, "退单豁免率")),
			getFloat(d, findCol(idx, "每笔结算成交花费(元)")),
			getPctFloat(d, findCol(idx, "交易额结算率")),
			getPctFloat(d, findCol(idx, "订单结算率")),
			getFloat(d, findCol(idx, "每笔结算成交金额(元)")),
			getInt(d, findCol(idx, "成交笔数")),
			getFloat(d, findCol(idx, "每笔成交花费(元)")),
			getFloat(d, findCol(idx, "每笔成交金额(元)")),
			getFloat(d, findCol(idx, "直接交易额(元)")),
			getFloat(d, findCol(idx, "间接交易额(元)")),
			getInt(d, findCol(idx, "直接成交笔数")),
			getInt(d, findCol(idx, "间接成交笔数")),
			getInt(d, findCol(idx, "曝光量")),
			getInt(d, findCol(idx, "点击量")),
			getFloat(d, findCol(idx, "询单花费(元)")),
			getInt(d, findCol(idx, "询单量")),
			getFloat(d, findCol(idx, "平均询单成本(元)")),
			getFloat(d, findCol(idx, "收藏花费(元)")),
			getInt(d, findCol(idx, "收藏量")),
			getFloat(d, findCol(idx, "平均收藏成本(元)")),
			getFloat(d, findCol(idx, "关注花费(元)")),
			getInt(d, findCol(idx, "关注量")),
			getFloat(d, findCol(idx, "平均关注成本(元)")),
		)
		if err != nil {
			log.Printf("写入商品级失败 [%s/%s]: %v", goodsID, shop, err)
			continue
		}
		count++
	}
	return count, nil
}

// getPctFloat 解析百分比字符串："83.33%" → 0.8333；"-" / "" → 0
func getPctFloat(d []string, i int) interface{} {
	if i < 0 || i >= len(d) {
		return nil
	}
	s := strings.TrimSpace(d[i])
	if s == "" || s == "-" {
		return nil
	}
	hasPct := strings.HasSuffix(s, "%")
	s = strings.TrimSuffix(s, "%")
	s = strings.ReplaceAll(s, ",", "")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	if hasPct {
		return v / 100
	}
	return v
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
			TotalGMV                       string          `json:"totalGMV"`
			OrderCount                     string          `json:"orderCount"`
			OrderUV                        string          `json:"orderUV"`
			FeedCount                      string          `json:"feedCount"`
			VideoViewCnt                   string          `json:"videoViewCnt"`
			GoodsClkCnt                    string          `json:"goodsClkCnt"`
			HasExplainGoodsTotal           string          `json:"hasExplainGoodsTotal"`
			UnOpenWindowGoodsTotal         string          `json:"unOpenWindowGoodsTotal"`
			ExplainCoverRate               string          `json:"explainCoverRate"`
			TotalGMVRate                   *float64        `json:"totalGMVRate"`
			OrderCountRate                 *float64        `json:"orderCountRate"`
			OrderUVRate                    *float64        `json:"orderUVRate"`
			FeedCountRate                  *float64        `json:"feedCountRate"`
			VideoViewCntRate               *float64        `json:"videoViewCntRate"`
			GoodsClkCntGrowthRate          *float64        `json:"goodsClkCntGrowthRate"`
			HasExplainGoodsTotalGrowthRate *float64        `json:"hasExplainGoodsTotalGrowthRate"`
			UnOpenWindowGoodsGrowthTotal   *float64        `json:"unOpenWindowGoodsGrowthTotal"`
			ExplainCoverGrowthRate         *float64        `json:"explainCoverGrowthRate"`
			UseFlowCardCnt                 *int            `json:"useFlowCardCnt"`
			ObtainFlowCardCnt              *int            `json:"obtainFlowCardCnt"`
			UsableFlowCardCnt              *int            `json:"usableFlowCardCnt"`
			OwnTopGoods                    json.RawMessage `json:"ownTopGoods"`
			OtherTopGoods                  json.RawMessage `json:"otherTopGoods"`
			StartPt                        string          `json:"startPt"`
		} `json:"result"`
	}
	json.Unmarshal(data, &resp)
	r := resp.Result

	// 优先用 json 里的 startPt（业务日期），否则用 date（文件名日期）作为 fallback
	actualDate := date
	if r.StartPt != "" {
		actualDate = r.StartPt
	}

	// 讲解覆盖率 "83.33%" → 0.8333
	explainCoverRate := pctToFraction(r.ExplainCoverRate)

	// nullable helper
	nz := func(p *float64) interface{} {
		if p == nil {
			return nil
		}
		return *p
	}
	nzi := func(p *int) interface{} {
		if p == nil {
			return nil
		}
		return *p
	}
	nzj := func(b json.RawMessage) interface{} {
		if len(b) == 0 || string(b) == "null" {
			return nil
		}
		return string(b)
	}
	nzs := func(s string) interface{} {
		s = strings.TrimSpace(s)
		if s == "" || s == "-" {
			return nil
		}
		return toIS(s)
	}
	nzPct := func(s string) interface{} {
		s = strings.TrimSpace(s)
		if s == "" || s == "-" {
			return nil
		}
		return explainCoverRate
	}
	_ = nzPct // 仅保留 explain_cover_rate 一处用，下方直接传 explainCoverRate

	_, err = db.Exec(`INSERT INTO op_pdd_video_daily
		(stat_date, shop_name, total_gmv, order_count, order_uv, feed_count, video_view_cnt, goods_click_cnt,
		 has_explain_goods_total, un_open_window_goods_total, explain_cover_rate,
		 total_gmv_rate, order_count_rate, order_uv_rate, feed_count_rate, video_view_cnt_rate,
		 goods_click_cnt_growth_rate, has_explain_goods_total_growth_rate, un_open_window_goods_growth_total, explain_cover_growth_rate,
		 use_flow_card_cnt, obtain_flow_card_cnt, usable_flow_card_cnt,
		 own_top_goods, other_top_goods)
		VALUES (?,?,?,?,?,?,?,?, ?,?,?, ?,?,?,?,?, ?,?,?,?, ?,?,?, ?,?)
		ON DUPLICATE KEY UPDATE
		 total_gmv=VALUES(total_gmv), order_count=VALUES(order_count), order_uv=VALUES(order_uv),
		 feed_count=VALUES(feed_count), video_view_cnt=VALUES(video_view_cnt), goods_click_cnt=VALUES(goods_click_cnt),
		 has_explain_goods_total=VALUES(has_explain_goods_total), un_open_window_goods_total=VALUES(un_open_window_goods_total),
		 explain_cover_rate=VALUES(explain_cover_rate),
		 total_gmv_rate=VALUES(total_gmv_rate), order_count_rate=VALUES(order_count_rate), order_uv_rate=VALUES(order_uv_rate),
		 feed_count_rate=VALUES(feed_count_rate), video_view_cnt_rate=VALUES(video_view_cnt_rate),
		 goods_click_cnt_growth_rate=VALUES(goods_click_cnt_growth_rate),
		 has_explain_goods_total_growth_rate=VALUES(has_explain_goods_total_growth_rate),
		 un_open_window_goods_growth_total=VALUES(un_open_window_goods_growth_total),
		 explain_cover_growth_rate=VALUES(explain_cover_growth_rate),
		 use_flow_card_cnt=VALUES(use_flow_card_cnt), obtain_flow_card_cnt=VALUES(obtain_flow_card_cnt), usable_flow_card_cnt=VALUES(usable_flow_card_cnt),
		 own_top_goods=VALUES(own_top_goods), other_top_goods=VALUES(other_top_goods)`,
		actualDate, shop, toFS(r.TotalGMV), toIS(r.OrderCount), toIS(r.OrderUV),
		toIS(r.FeedCount), toIS(r.VideoViewCnt), toIS(r.GoodsClkCnt),
		nzs(r.HasExplainGoodsTotal), nzs(r.UnOpenWindowGoodsTotal), explainCoverRate,
		nz(r.TotalGMVRate), nz(r.OrderCountRate), nz(r.OrderUVRate), nz(r.FeedCountRate), nz(r.VideoViewCntRate),
		nz(r.GoodsClkCntGrowthRate), nz(r.HasExplainGoodsTotalGrowthRate), nz(r.UnOpenWindowGoodsGrowthTotal), nz(r.ExplainCoverGrowthRate),
		nzi(r.UseFlowCardCnt), nzi(r.ObtainFlowCardCnt), nzi(r.UsableFlowCardCnt),
		nzj(r.OwnTopGoods), nzj(r.OtherTopGoods))
	if err != nil {
		return 0, err
	}
	return 1, nil
}

// pctToFraction "83.33%" → 0.8333；"" / "-" / null → nil（用于 SQL）
func pctToFraction(s string) interface{} {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return nil
	}
	hasPct := strings.HasSuffix(s, "%")
	s = strings.TrimSuffix(s, "%")
	s = strings.ReplaceAll(s, ",", "")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	if hasPct {
		return v / 100
	}
	return v
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
