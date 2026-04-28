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

	_ "github.com/go-sql-driver/mysql"
	"github.com/xuri/excelize/v2"
)

var baseDir = `Z:\信息部\RPA_集团数据看板`

// parseExcelDate 严格解析 Excel 日期列，格式不合规返回 ""（调用方 fallback 到文件名日期）
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
	cfg, err := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()

	if err := ensureTables(db); err != nil {
		log.Fatalf("初始化客服表失败: %v", err)
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
		yearPath := filepath.Join(resolvedBaseDir)
		_ = yearPath
		// 平台目录下的年份目录
		for _, platform := range []string{"京东自营", "抖音", "快手", "小红书"} {
			platformYearPath := filepath.Join(resolvedBaseDir, platform, yearDir)
			dateDirs, err := os.ReadDir(platformYearPath)
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

				datePath := filepath.Join(platformYearPath, dateStr)
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
						if f.IsDir() {
							continue
						}
						name := f.Name()
						if strings.HasPrefix(name, "~$") {
							continue
						}
						fullPath := filepath.Join(shopPath, name)
						var cnt int
						var err error

						switch {
						case platform == "京东自营" && strings.Contains(name, "客服_服务工作量") && strings.HasSuffix(strings.ToLower(name), ".xlsx"):
							cnt, err = importJDWorkload(db, fullPath, sqlDate, shopName)
							total["jd_cs_workload"] += cnt
						case platform == "京东自营" && strings.Contains(name, "客服_销售绩效") && strings.HasSuffix(strings.ToLower(name), ".xlsx"):
							cnt, err = importJDSalesPerf(db, fullPath, sqlDate, shopName)
							total["jd_cs_sales_perf"] += cnt
						case platform == "抖音" && strings.Contains(name, "飞鸽_客服表现") && strings.HasSuffix(strings.ToLower(name), ".xlsx"):
							cnt, err = importDouyinFeige(db, fullPath, sqlDate, shopName)
							total["douyin_cs_feige"] += cnt
						case platform == "快手" && strings.Contains(name, "客服_考核数据") && strings.HasSuffix(strings.ToLower(name), ".xlsx"):
							cnt, err = importKuaishouAssessment(db, fullPath, sqlDate, shopName)
							total["kuaishou_cs_assessment"] += cnt
						case platform == "小红书" && strings.Contains(name, "客服分析") && strings.HasSuffix(strings.ToLower(name), ".json"):
							cnt, err = importXHSAnalysis(db, fullPath, sqlDate, shopName)
							total["xhs_cs_analysis"] += cnt
						}
						if err != nil {
							log.Printf("导入失败 [%s/%s/%s]: %v", platform, shopName, name, err)
						}
					}
				}
				fmt.Printf("[%s][%s] 完成\n", platform, dateStr)
			}
		}
	}

	if startDate != "" && endDate != "" && !matchedDate {
		log.Fatalf("未找到日期范围 %s-%s 的客服数据目录", startDate, endDate)
	}

	fmt.Println("\n客服导入完成:")
	for k, v := range total {
		fmt.Printf("  %s: %d 条\n", k, v)
	}
}

func ensureTables(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS op_jd_cs_workload_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			shop_name VARCHAR(128) NOT NULL,
			on_duty_cs_count INT DEFAULT 0,
			login_hours DECIMAL(10,4) DEFAULT 0,
			shop_service_hours DECIMAL(10,4) DEFAULT 0,
			upv INT DEFAULT 0,
			consult_count INT DEFAULT 0,
			receive_count INT DEFAULT 0,
			connect_rate DECIMAL(10,4) DEFAULT 0,
			reply_rate DECIMAL(10,4) DEFAULT 0,
			resp_30s_rate DECIMAL(10,4) DEFAULT 0,
			satisfaction_rate DECIMAL(10,4) DEFAULT 0,
			invite_eval_rate DECIMAL(10,4) DEFAULT 0,
			avg_reply_msg_count DECIMAL(10,4) DEFAULT 0,
			timeout_reply_count INT DEFAULT 0,
			avg_session_minutes DECIMAL(10,4) DEFAULT 0,
			first_avg_resp_seconds DECIMAL(10,4) DEFAULT 0,
			new_avg_resp_seconds DECIMAL(10,4) DEFAULT 0,
			message_consult_count INT DEFAULT 0,
			message_assign_count INT DEFAULT 0,
			message_receive_count INT DEFAULT 0,
			message_reply_rate DECIMAL(10,4) DEFAULT 0,
			message_resp_rate DECIMAL(10,4) DEFAULT 0,
			merchant_message_rate DECIMAL(10,4) DEFAULT 0,
			resolve_rate DECIMAL(10,4) DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_stat_shop (stat_date, shop_name)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='京东自营客服-服务工作量日报'`,
		`CREATE TABLE IF NOT EXISTS op_jd_cs_sales_perf_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			shop_name VARCHAR(128) NOT NULL,
			on_duty_cs_count INT DEFAULT 0,
			presale_receive_users INT DEFAULT 0,
			order_users INT DEFAULT 0,
			shipped_users INT DEFAULT 0,
			order_count INT DEFAULT 0,
			shipped_order_count INT DEFAULT 0,
			order_goods_count INT DEFAULT 0,
			shipped_goods_count INT DEFAULT 0,
			order_goods_amount DECIMAL(18,4) DEFAULT 0,
			shipped_goods_amount DECIMAL(18,4) DEFAULT 0,
			consult_to_order_rate DECIMAL(10,4) DEFAULT 0,
			consult_to_ship_rate DECIMAL(10,4) DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_stat_shop (stat_date, shop_name)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='京东自营客服-销售绩效日报'`,
		`CREATE TABLE IF NOT EXISTS op_douyin_cs_feige_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			shop_name VARCHAR(128) NOT NULL,
			online_cs_count INT DEFAULT 0,
			service_sessions INT DEFAULT 0,
			pending_reply_sessions INT DEFAULT 0,
			received_users INT DEFAULT 0,
			transfer_out_sessions INT DEFAULT 0,
			valid_eval_count INT DEFAULT 0,
			valid_good_eval_count INT DEFAULT 0,
			valid_bad_eval_count INT DEFAULT 0,
			all_day_dissatisfaction_rate DECIMAL(10,4) DEFAULT 0,
			all_day_satisfaction_rate DECIMAL(10,4) DEFAULT 0,
			all_day_avg_reply_seconds DECIMAL(10,4) DEFAULT 0,
			all_day_first_reply_seconds DECIMAL(10,4) DEFAULT 0,
			all_day_3min_reply_rate DECIMAL(10,4) DEFAULT 0,
			inquiry_users INT DEFAULT 0,
			order_users INT DEFAULT 0,
			pay_users INT DEFAULT 0,
			refund_users INT DEFAULT 0,
			inquiry_pay_amount DECIMAL(18,4) DEFAULT 0,
			refund_amount DECIMAL(18,4) DEFAULT 0,
			net_sales_amount DECIMAL(18,4) DEFAULT 0,
			inquiry_conv_rate DECIMAL(10,4) DEFAULT 0,
			raw_json JSON NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_stat_shop (stat_date, shop_name)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音飞鸽客服-店铺汇总日报'`,
		`CREATE TABLE IF NOT EXISTS op_kuaishou_cs_assessment_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			shop_name VARCHAR(128) NOT NULL,
			reply_3min_rate_person DECIMAL(10,4) DEFAULT 0,
			reply_3min_rate_session DECIMAL(10,4) DEFAULT 0,
			no_service_rate DECIMAL(10,4) DEFAULT 0,
			consult_users INT DEFAULT 0,
			consult_times INT DEFAULT 0,
			manual_sessions INT DEFAULT 0,
			avg_reply_seconds DECIMAL(10,4) DEFAULT 0,
			good_rate_person DECIMAL(10,4) DEFAULT 0,
			good_rate_session DECIMAL(10,4) DEFAULT 0,
			bad_rate_person DECIMAL(10,4) DEFAULT 0,
			bad_rate_session DECIMAL(10,4) DEFAULT 0,
			im_dissatisfaction_rate_person DECIMAL(10,4) DEFAULT 0,
			inquiry_users INT DEFAULT 0,
			order_users INT DEFAULT 0,
			pay_users INT DEFAULT 0,
			inquiry_conv_rate DECIMAL(10,4) DEFAULT 0,
			inquiry_unit_price DECIMAL(18,4) DEFAULT 0,
			cs_sales_amount DECIMAL(18,4) DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_stat_shop (stat_date, shop_name)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='快手客服-考核数据日报'`,
		`CREATE TABLE IF NOT EXISTS op_xhs_cs_analysis_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			shop_name VARCHAR(128) NOT NULL,
			case_count DECIMAL(18,4) DEFAULT 0,
			reply_case_count DECIMAL(18,4) DEFAULT 0,
			reply_case_ratio DECIMAL(10,4) DEFAULT 0,
			avg_reply_duration_min DECIMAL(18,4) DEFAULT 0,
			inquiry_pay_case_ratio DECIMAL(10,4) DEFAULT 0,
			first_reply_45s_ratio DECIMAL(10,4) DEFAULT 0,
			reply_in_3min_case_ratio DECIMAL(10,4) DEFAULT 0,
			pay_gmv DECIMAL(18,4) DEFAULT 0,
			inquiry_pay_gmv_ratio DECIMAL(10,4) DEFAULT 0,
			pay_pkg_count DECIMAL(18,4) DEFAULT 0,
			inquiry_pay_pkg_count DECIMAL(18,4) DEFAULT 0,
			positive_case_count DECIMAL(18,4) DEFAULT 0,
			positive_case_ratio DECIMAL(10,4) DEFAULT 0,
			negative_case_count DECIMAL(18,4) DEFAULT 0,
			negative_case_ratio DECIMAL(10,4) DEFAULT 0,
			evaluate_case_count DECIMAL(18,4) DEFAULT 0,
			evaluate_case_ratio DECIMAL(10,4) DEFAULT 0,
			raw_json JSON NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_stat_shop (stat_date, shop_name)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='小红书客服-客服分析日报'`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func importJDWorkload(db *sql.DB, path, date, shop string) (int, error) {
	row, err := readJDDataRow(path)
	if err != nil {
		return 0, err
	}
	// stat_date 取 Excel 第 0 列日期（readJDDataRow 保证含 "-"），文件名日期只是 RPA 采集日
	statDate := date
	if len(row) > 0 && strings.Contains(row[0], "-") {
		if v := parseExcelDate(row[0]); v != "" {
			statDate = v
		}
	}
	_, err = db.Exec(`INSERT INTO op_jd_cs_workload_daily
		(stat_date, shop_name, on_duty_cs_count, login_hours, shop_service_hours, upv, consult_count, receive_count,
		 connect_rate, reply_rate, resp_30s_rate, satisfaction_rate, invite_eval_rate, avg_reply_msg_count,
		 timeout_reply_count, avg_session_minutes, first_avg_resp_seconds, new_avg_resp_seconds, message_consult_count,
		 message_assign_count, message_receive_count, message_reply_rate, message_resp_rate, merchant_message_rate, resolve_rate)
		VALUES (?,?,?,?,?,?,?,?, ?,?,?,?,?,?,?, ?,?,?,?,?, ?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
		 on_duty_cs_count=VALUES(on_duty_cs_count), login_hours=VALUES(login_hours), shop_service_hours=VALUES(shop_service_hours),
		 upv=VALUES(upv), consult_count=VALUES(consult_count), receive_count=VALUES(receive_count), connect_rate=VALUES(connect_rate),
		 reply_rate=VALUES(reply_rate), resp_30s_rate=VALUES(resp_30s_rate), satisfaction_rate=VALUES(satisfaction_rate),
		 invite_eval_rate=VALUES(invite_eval_rate), avg_reply_msg_count=VALUES(avg_reply_msg_count), timeout_reply_count=VALUES(timeout_reply_count),
		 avg_session_minutes=VALUES(avg_session_minutes), first_avg_resp_seconds=VALUES(first_avg_resp_seconds),
		 new_avg_resp_seconds=VALUES(new_avg_resp_seconds), message_consult_count=VALUES(message_consult_count),
		 message_assign_count=VALUES(message_assign_count), message_receive_count=VALUES(message_receive_count),
		 message_reply_rate=VALUES(message_reply_rate), message_resp_rate=VALUES(message_resp_rate),
		 merchant_message_rate=VALUES(merchant_message_rate), resolve_rate=VALUES(resolve_rate)`,
		statDate, shop,
		toInt(row, 1), toFloat(row, 2), toFloat(row, 3), toInt(row, 4), toInt(row, 5), toInt(row, 6),
		toFloat(row, 7), toFloat(row, 8), toFloat(row, 9), toFloat(row, 10), toFloat(row, 11), toFloat(row, 12),
		toInt(row, 13), toFloat(row, 14), toFloat(row, 15), toFloat(row, 16), toInt(row, 17),
		toInt(row, 18), toInt(row, 19), toFloat(row, 20), toFloat(row, 21), toFloat(row, 22), toFloat(row, 23),
	)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

func importJDSalesPerf(db *sql.DB, path, date, shop string) (int, error) {
	row, err := readJDDataRow(path)
	if err != nil {
		return 0, err
	}
	// stat_date 取 Excel 第 0 列日期，文件名日期只是 RPA 采集日
	statDate := date
	if len(row) > 0 && strings.Contains(row[0], "-") {
		if v := parseExcelDate(row[0]); v != "" {
			statDate = v
		}
	}
	_, err = db.Exec(`INSERT INTO op_jd_cs_sales_perf_daily
		(stat_date, shop_name, on_duty_cs_count, presale_receive_users, order_users, shipped_users,
		 order_count, shipped_order_count, order_goods_count, shipped_goods_count, order_goods_amount,
		 shipped_goods_amount, consult_to_order_rate, consult_to_ship_rate)
		VALUES (?,?,?,?,?,?, ?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
		 on_duty_cs_count=VALUES(on_duty_cs_count), presale_receive_users=VALUES(presale_receive_users),
		 order_users=VALUES(order_users), shipped_users=VALUES(shipped_users), order_count=VALUES(order_count),
		 shipped_order_count=VALUES(shipped_order_count), order_goods_count=VALUES(order_goods_count),
		 shipped_goods_count=VALUES(shipped_goods_count), order_goods_amount=VALUES(order_goods_amount),
		 shipped_goods_amount=VALUES(shipped_goods_amount), consult_to_order_rate=VALUES(consult_to_order_rate),
		 consult_to_ship_rate=VALUES(consult_to_ship_rate)`,
		statDate, shop,
		toInt(row, 1), toInt(row, 2), toInt(row, 3), toInt(row, 4), toInt(row, 5), toInt(row, 6), toInt(row, 7),
		toInt(row, 8), toFloat(row, 9), toFloat(row, 10), toFloat(row, 11), toFloat(row, 12),
	)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

func importDouyinFeige(db *sql.DB, path, date, shop string) (int, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 2 {
		return 0, nil
	}

	header := rows[0]
	data := rows[1]
	avgData := rows[1]
	if len(rows) >= 2 {
		for i := 1; i < len(rows); i++ {
			r := rows[i]
			if len(r) > 0 {
				label := strings.TrimSpace(strings.Join(r, "|"))
				if strings.Contains(label, "店铺汇总值") {
					data = r
				}
				if strings.Contains(label, "店铺平均值") {
					avgData = r
				}
			}
		}
	}
	metric := headerMapData(header, data)
	metricAvg := headerMapData(header, avgData)

	pick := func(name string) string {
		v := strings.TrimSpace(getByName(metric, name))
		if v != "" && v != "-" && v != "--" {
			return v
		}
		return strings.TrimSpace(getByName(metricAvg, name))
	}

	pickFloat := func(name string) float64 {
		return parseFloatText(pick(name))
	}

	pickDuration := func(name string) float64 {
		return parseDurationSeconds(pick(name))
	}

	raw, _ := json.Marshal(metric)

	_, err = db.Exec(`INSERT INTO op_douyin_cs_feige_daily
		(stat_date, shop_name, online_cs_count, service_sessions, pending_reply_sessions, received_users, transfer_out_sessions,
		 valid_eval_count, valid_good_eval_count, valid_bad_eval_count, all_day_dissatisfaction_rate, all_day_satisfaction_rate,
		 all_day_avg_reply_seconds, all_day_first_reply_seconds, all_day_3min_reply_rate, inquiry_users, order_users, pay_users,
		 refund_users, inquiry_pay_amount, refund_amount, net_sales_amount, inquiry_conv_rate, raw_json)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
		 online_cs_count=VALUES(online_cs_count), service_sessions=VALUES(service_sessions), pending_reply_sessions=VALUES(pending_reply_sessions),
		 received_users=VALUES(received_users), transfer_out_sessions=VALUES(transfer_out_sessions), valid_eval_count=VALUES(valid_eval_count),
		 valid_good_eval_count=VALUES(valid_good_eval_count), valid_bad_eval_count=VALUES(valid_bad_eval_count),
		 all_day_dissatisfaction_rate=VALUES(all_day_dissatisfaction_rate), all_day_satisfaction_rate=VALUES(all_day_satisfaction_rate),
		 all_day_avg_reply_seconds=VALUES(all_day_avg_reply_seconds), all_day_first_reply_seconds=VALUES(all_day_first_reply_seconds),
		 all_day_3min_reply_rate=VALUES(all_day_3min_reply_rate), inquiry_users=VALUES(inquiry_users), order_users=VALUES(order_users),
		 pay_users=VALUES(pay_users), refund_users=VALUES(refund_users), inquiry_pay_amount=VALUES(inquiry_pay_amount),
		 refund_amount=VALUES(refund_amount), net_sales_amount=VALUES(net_sales_amount), inquiry_conv_rate=VALUES(inquiry_conv_rate),
		 raw_json=VALUES(raw_json)`,
		date, shop,
		toIntByName(metric, "客服在线天数"),
		toIntByName(metric, "接待会话量"),
		toIntByName(metric, "待回复会话量"),
		toIntByName(metric, "已接待人数"),
		toIntByName(metric, "转出会话量"),
		toIntByName(metric, "有效评价数"),
		toIntByName(metric, "有效好评数"),
		toIntByName(metric, "有效差评数"),
		pickFloat("全天不满意率"),
		pickFloat("全天满意率"),
		pickDuration("全天平响时长"),
		pickDuration("全天首响时长"),
		pickFloat("全天3分钟回复率"),
		toIntByName(metric, "询单人数"),
		toIntByName(metric, "下单人数"),
		toIntByName(metric, "支付人数"),
		toIntByName(metric, "退款人数"),
		toFloatByName(metric, "客服询单支付额"),
		toFloatByName(metric, "退款金额"),
		toFloatByName(metric, "退款后销售额"),
		pickFloat("询单转化率"),
		string(raw),
	)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

func importKuaishouAssessment(db *sql.DB, path, date, shop string) (int, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 2 {
		return 0, nil
	}
	header := rows[0]
	data := rows[1] // 店铺数据
	metric := headerMapData(header, data)

	_, err = db.Exec(`INSERT INTO op_kuaishou_cs_assessment_daily
		(stat_date, shop_name, reply_3min_rate_person, reply_3min_rate_session, no_service_rate, consult_users, consult_times,
		 manual_sessions, avg_reply_seconds, good_rate_person, good_rate_session, bad_rate_person, bad_rate_session,
		 im_dissatisfaction_rate_person, inquiry_users, order_users, pay_users, inquiry_conv_rate, inquiry_unit_price, cs_sales_amount)
		VALUES (?,?,?,?,?,?,?, ?,?,?,?,?,?, ?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
		 reply_3min_rate_person=VALUES(reply_3min_rate_person), reply_3min_rate_session=VALUES(reply_3min_rate_session),
		 no_service_rate=VALUES(no_service_rate), consult_users=VALUES(consult_users), consult_times=VALUES(consult_times),
		 manual_sessions=VALUES(manual_sessions), avg_reply_seconds=VALUES(avg_reply_seconds), good_rate_person=VALUES(good_rate_person),
		 good_rate_session=VALUES(good_rate_session), bad_rate_person=VALUES(bad_rate_person), bad_rate_session=VALUES(bad_rate_session),
		 im_dissatisfaction_rate_person=VALUES(im_dissatisfaction_rate_person), inquiry_users=VALUES(inquiry_users),
		 order_users=VALUES(order_users), pay_users=VALUES(pay_users), inquiry_conv_rate=VALUES(inquiry_conv_rate),
		 inquiry_unit_price=VALUES(inquiry_unit_price), cs_sales_amount=VALUES(cs_sales_amount)`,
		date, shop,
		toFloatByName(metric, "三分钟回复率（人维度）"),
		toFloatByName(metric, "三分钟回复率（会话）"),
		toFloatByName(metric, "不服务率"),
		toIntByName(metric, "咨询人数"),
		toIntByName(metric, "咨询人次"),
		toIntByName(metric, "人工会话量"),
		parseDurationSeconds(getByName(metric, "人工平均回复时长")),
		toFloatByName(metric, "好评率（人维度）"),
		toFloatByName(metric, "好评率（会话）"),
		toFloatByName(metric, "差评率（人维度）"),
		toFloatByName(metric, "差评率（会话）"),
		toFloatByName(metric, "IM不满意率（人维度）"),
		toIntByName(metric, "询单人数"),
		toIntByName(metric, "下单人数"),
		toIntByName(metric, "支付人数"),
		toFloatByName(metric, "询单转化率"),
		toFloatByName(metric, "询单客单价"),
		toFloatByName(metric, "客服销售额"),
	)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

func importXHSAnalysis(db *sql.DB, path, date, shop string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var root map[string]interface{}
	if err := json.Unmarshal(b, &root); err != nil {
		return 0, err
	}

	// 业务日期默认 = RPA 文件名日期；如果 sellerCSOverall.dtm.value 存在则用那个
	statDate := date

	// 按blockKey分类提取数据
	metrics := map[string]interface{}{}
	var trendRows []map[string]interface{}
	var excellentRows []map[string]interface{}
	if dataArr, ok := root["data"].([]interface{}); ok {
		for _, item := range dataArr {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			blockKey := toString(m["blockKey"])
			rows, _ := m["data"].([]interface{})
			switch blockKey {
			case "sellerCSOverall":
				if len(rows) > 0 {
					if row, ok := rows[0].(map[string]interface{}); ok {
						metrics = row
						// 从 dtm.value 读业务日期（YYYYMMDD 整数 或 YYYY-MM-DD 字符串）
						if dtm, ok := row["dtm"].(map[string]interface{}); ok {
							switch v := dtm["value"].(type) {
							case float64:
								n := int(v)
								if n > 19000000 && n < 99999999 {
									statDate = fmt.Sprintf("%04d-%02d-%02d", n/10000, (n/100)%100, n%100)
								}
							case string:
								if len(v) == 10 && v[4] == '-' && v[7] == '-' {
									statDate = v
								} else if len(v) == 8 {
									statDate = v[:4] + "-" + v[4:6] + "-" + v[6:8]
								}
							}
						}
					}
				}
			case "sellerCSTrend":
				for _, r := range rows {
					if row, ok := r.(map[string]interface{}); ok {
						trendRows = append(trendRows, row)
					}
				}
			case "sellerCSExcellentTrend":
				for _, r := range rows {
					if row, ok := r.(map[string]interface{}); ok {
						excellentRows = append(excellentRows, row)
					}
				}
			}
		}
	}

	raw, _ := json.Marshal(root)
	_, err = db.Exec(`INSERT INTO op_xhs_cs_analysis_daily
		(stat_date, shop_name, case_count, reply_case_count, reply_case_ratio, avg_reply_duration_min,
		 inquiry_pay_case_ratio, first_reply_45s_ratio, reply_in_3min_case_ratio, pay_gmv, inquiry_pay_gmv, inquiry_pay_gmv_ratio, pay_pkg_count,
		 inquiry_pay_pkg_count, positive_case_count, positive_case_ratio, negative_case_count, negative_case_ratio,
		 evaluate_case_count, evaluate_case_ratio,
		 remove_na_case_count, reply_in_3min_case_count, reply_na_in_3min_case_count,
		 raw_json)
		VALUES (?,?,?,?,?,?, ?,?,?,?,?,?,?, ?,?,?,?,?, ?,?, ?,?,?, ?)
		ON DUPLICATE KEY UPDATE
		 case_count=VALUES(case_count), reply_case_count=VALUES(reply_case_count), reply_case_ratio=VALUES(reply_case_ratio),
		 avg_reply_duration_min=VALUES(avg_reply_duration_min), inquiry_pay_case_ratio=VALUES(inquiry_pay_case_ratio),
		 first_reply_45s_ratio=VALUES(first_reply_45s_ratio), reply_in_3min_case_ratio=VALUES(reply_in_3min_case_ratio),
		 pay_gmv=VALUES(pay_gmv), inquiry_pay_gmv=VALUES(inquiry_pay_gmv), inquiry_pay_gmv_ratio=VALUES(inquiry_pay_gmv_ratio),
		 pay_pkg_count=VALUES(pay_pkg_count), inquiry_pay_pkg_count=VALUES(inquiry_pay_pkg_count), positive_case_count=VALUES(positive_case_count),
		 positive_case_ratio=VALUES(positive_case_ratio), negative_case_count=VALUES(negative_case_count), negative_case_ratio=VALUES(negative_case_ratio),
		 evaluate_case_count=VALUES(evaluate_case_count), evaluate_case_ratio=VALUES(evaluate_case_ratio),
		 remove_na_case_count=VALUES(remove_na_case_count), reply_in_3min_case_count=VALUES(reply_in_3min_case_count),
		 reply_na_in_3min_case_count=VALUES(reply_na_in_3min_case_count),
		 raw_json=VALUES(raw_json)`,
		statDate, shop,
		nestedValue(metrics, "caseCnt"),
		nestedValue(metrics, "replyCaseCnt"),
		nestedValue(metrics, "replyCaseRatio"),
		nestedValue(metrics, "avgRplDur"),
		nestedValue(metrics, "inquiryPayCaseRatio"),
		nestedValue(metrics, "firstReplyIn45sCaseRatio"),
		nestedValue(metrics, "replyIn3minCaseRatio"),
		nestedValue(metrics, "payGmv"),
		nestedValue(metrics, "inquiryPayGmv"),
		nestedValue(metrics, "inquiryPayGmvRatio"),
		nestedValue(metrics, "payPkgCnt"),
		nestedValue(metrics, "inquiryPayPkgCnt"),
		nestedValue(metrics, "positiveCaseCnt"),
		nestedValue(metrics, "positiveCaseRatio"),
		nestedValue(metrics, "negativeCaseCnt"),
		nestedValue(metrics, "negativeCaseRatio"),
		nestedValue(metrics, "evaluateCaseCnt"),
		nestedValue(metrics, "evaluateCaseRatio"),
		nestedValue(metrics, "removeNaCaseCnt"),
		nestedValue(metrics, "replyIn3minCaseCnt"),
		nestedValue(metrics, "replyNAIn3minCaseCnt"),
		string(raw),
	)
	if err != nil {
		return 0, err
	}

	// 导入客服趋势数据
	for _, row := range trendRows {
		dtmVal := xhsExtractDtm(row)
		if dtmVal == "" {
			continue
		}
		db.Exec(`INSERT INTO op_xhs_cs_trend_daily
			(stat_date, report_date, shop_name, case_count, avg_reply_duration, reply_case_count, reply_case_ratio,
			 reply_in_3min_case_ratio, first_reply_45s_ratio, evaluate_case_count, positive_case_count, negative_case_count,
			 evaluate_case_ratio, negative_case_ratio, positive_case_ratio,
			 inquiry_pay_pkg_count, inquiry_pay_gmv, inquiry_pay_case_ratio, inquiry_pay_gmv_ratio, remove_na_case_count)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE
			 case_count=VALUES(case_count), avg_reply_duration=VALUES(avg_reply_duration),
			 reply_case_count=VALUES(reply_case_count), reply_case_ratio=VALUES(reply_case_ratio),
			 reply_in_3min_case_ratio=VALUES(reply_in_3min_case_ratio), first_reply_45s_ratio=VALUES(first_reply_45s_ratio),
			 evaluate_case_count=VALUES(evaluate_case_count), positive_case_count=VALUES(positive_case_count),
			 negative_case_count=VALUES(negative_case_count), evaluate_case_ratio=VALUES(evaluate_case_ratio),
			 negative_case_ratio=VALUES(negative_case_ratio), positive_case_ratio=VALUES(positive_case_ratio),
			 inquiry_pay_pkg_count=VALUES(inquiry_pay_pkg_count), inquiry_pay_gmv=VALUES(inquiry_pay_gmv),
			 inquiry_pay_case_ratio=VALUES(inquiry_pay_case_ratio), inquiry_pay_gmv_ratio=VALUES(inquiry_pay_gmv_ratio),
			 remove_na_case_count=VALUES(remove_na_case_count)`,
			dtmVal, date, shop,
			nestedValue(row, "caseCnt"), nestedValue(row, "avgRplDur"),
			nestedValue(row, "replyCaseCnt"), nestedValue(row, "replyCaseRatio"),
			nestedValue(row, "replyIn3minCaseRatio"), nestedValue(row, "firstReplyIn45sCaseRatio"),
			nestedValue(row, "evaluateCaseCnt"), nestedValue(row, "positiveCaseCnt"),
			nestedValue(row, "negativeCaseCnt"), nestedValue(row, "evaluateCaseRatio"),
			nestedValue(row, "negativeCaseRatio"), nestedValue(row, "positiveCaseRatio"),
			nestedValue(row, "inquiryPayPkgCnt"), nestedValue(row, "inquiryPayGmv"),
			nestedValue(row, "inquiryPayCaseRatio"), nestedValue(row, "inquiryPayGmvRatio"),
			nestedValue(row, "removeNaCaseCnt"),
		)
	}

	// 导入行业优秀趋势数据
	for _, row := range excellentRows {
		dtmVal := xhsExtractDtm(row)
		if dtmVal == "" {
			continue
		}
		db.Exec(`INSERT INTO op_xhs_cs_excellent_trend_daily
			(stat_date, report_date, shop_name, case_count, avg_reply_duration, reply_case_ratio,
			 reply_in_3min_case_ratio, first_reply_45s_ratio, evaluate_case_count, positive_case_count, negative_case_count,
			 evaluate_case_ratio, negative_case_ratio, positive_case_ratio,
			 inquiry_pay_pkg_count, inquiry_pay_gmv, inquiry_pay_case_ratio, inquiry_pay_gmv_ratio)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE
			 case_count=VALUES(case_count), avg_reply_duration=VALUES(avg_reply_duration),
			 reply_case_ratio=VALUES(reply_case_ratio),
			 reply_in_3min_case_ratio=VALUES(reply_in_3min_case_ratio), first_reply_45s_ratio=VALUES(first_reply_45s_ratio),
			 evaluate_case_count=VALUES(evaluate_case_count), positive_case_count=VALUES(positive_case_count),
			 negative_case_count=VALUES(negative_case_count), evaluate_case_ratio=VALUES(evaluate_case_ratio),
			 negative_case_ratio=VALUES(negative_case_ratio), positive_case_ratio=VALUES(positive_case_ratio),
			 inquiry_pay_pkg_count=VALUES(inquiry_pay_pkg_count), inquiry_pay_gmv=VALUES(inquiry_pay_gmv),
			 inquiry_pay_case_ratio=VALUES(inquiry_pay_case_ratio), inquiry_pay_gmv_ratio=VALUES(inquiry_pay_gmv_ratio)`,
			dtmVal, date, shop,
			nestedValue(row, "caseCnt"), nestedValue(row, "avgRplDur"),
			nestedValue(row, "replyCaseRatio"),
			nestedValue(row, "replyIn3minCaseRatio"), nestedValue(row, "firstReplyIn45sCaseRatio"),
			nestedValue(row, "evaluateCaseCnt"), nestedValue(row, "positiveCaseCnt"),
			nestedValue(row, "negativeCaseCnt"), nestedValue(row, "evaluateCaseRatio"),
			nestedValue(row, "negativeCaseRatio"), nestedValue(row, "positiveCaseRatio"),
			nestedValue(row, "inquiryPayPkgCnt"), nestedValue(row, "inquiryPayGmv"),
			nestedValue(row, "inquiryPayCaseRatio"), nestedValue(row, "inquiryPayGmvRatio"),
		)
	}

	return 1, nil
}

func readJDDataRow(path string) ([]string, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 4 {
		return nil, nil
	}
	if len(rows[3]) > 0 && strings.Contains(rows[3][0], "-") {
		return rows[3], nil
	}
	return rows[1], nil
}

func headerMapData(header, data []string) map[string]string {
	m := map[string]string{}
	for i, h := range header {
		key := strings.TrimSpace(h)
		if key == "" {
			continue
		}
		val := ""
		if i < len(data) {
			val = strings.TrimSpace(data[i])
		}
		m[key] = val
	}
	return m
}

func getByName(m map[string]string, name string) string {
	if v, ok := m[name]; ok {
		return v
	}
	return ""
}

func toIntByName(m map[string]string, name string) int {
	return int(parseFloatText(getByName(m, name)))
}

func toFloatByName(m map[string]string, name string) float64 {
	return parseFloatText(getByName(m, name))
}

func toFloat(row []string, idx int) float64 {
	if idx >= len(row) {
		return 0
	}
	return parseFloatText(row[idx])
}

func toInt(row []string, idx int) int {
	return int(toFloat(row, idx))
}

func parseFloatText(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "%", "")
	s = strings.ReplaceAll(s, "元", "")
	s = strings.TrimSpace(s)
	if s == "" || s == "-" || s == "--" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// parseDurationSeconds 支持: 11秒 / 2分30秒 / 7小时41分52秒
func parseDurationSeconds(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0
	}
	if strings.HasSuffix(s, "秒") && !strings.Contains(s, "分") && !strings.Contains(s, "小时") {
		return parseFloatText(strings.TrimSuffix(s, "秒"))
	}
	total := 0.0
	rest := s
	if idx := strings.Index(rest, "小时"); idx > 0 {
		total += parseFloatText(rest[:idx]) * 3600
		rest = rest[idx+len("小时"):]
	}
	if idx := strings.Index(rest, "分"); idx > 0 {
		total += parseFloatText(rest[:idx]) * 60
		rest = rest[idx+len("分"):]
	}
	if idx := strings.Index(rest, "秒"); idx > 0 {
		total += parseFloatText(rest[:idx])
	}
	return total
}

func nestedValue(metrics map[string]interface{}, key string) float64 {
	v, ok := metrics[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case map[string]interface{}:
		if iv, ok := t["value"]; ok {
			return anyToFloat(iv)
		}
		return 0
	default:
		return anyToFloat(t)
	}
}

func anyToFloat(v interface{}) float64 {
	switch t := v.(type) {
	case nil:
		return 0
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	case string:
		return parseFloatText(t)
	default:
		return parseFloatText(fmt.Sprintf("%v", t))
	}
}

// xhsExtractDtm 从趋势行中提取日期，返回 "2026-04-12" 格式
func xhsExtractDtm(row map[string]interface{}) string {
	dtmRaw, ok := row["dtm"]
	if !ok {
		return ""
	}
	switch v := dtmRaw.(type) {
	case map[string]interface{}:
		val := fmt.Sprintf("%v", v["value"])
		val = strings.TrimSpace(val)
		if len(val) == 8 {
			return val[:4] + "-" + val[4:6] + "-" + val[6:8]
		}
		if len(val) == 10 && val[4] == '-' {
			return val
		}
		return val
	case float64:
		s := fmt.Sprintf("%.0f", v)
		if len(s) == 8 {
			return s[:4] + "-" + s[4:6] + "-" + s[6:8]
		}
		return s
	case string:
		if len(v) == 8 {
			return v[:4] + "-" + v[4:6] + "-" + v[6:8]
		}
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}
