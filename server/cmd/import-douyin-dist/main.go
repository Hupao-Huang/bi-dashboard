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

var baseDir = `Z:\信息部\RPA_集团数据看板\抖音分销`

func main() {
	cfg, _ := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	startDate, endDate := "", ""
	if len(os.Args) >= 3 {
		startDate, endDate = os.Args[1], os.Args[2]
	}
	resolvedBaseDir, err := importutil.ResolveDataRoot(baseDir)
	if err != nil {
		log.Fatalf("数据目录不可用: %v", err)
	}

	totalProduct, totalAccount, totalMaterial, totalPromote := 0, 0, 0, 0

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
			sqlDate := dateStr[:4] + "-" + dateStr[4:6] + "-" + dateStr[6:8]
			datePath := filepath.Join(yearPath, dateStr)

			accountDirs, err := os.ReadDir(datePath)
			if err != nil {
				continue
			}
			for _, ad := range accountDirs {
				if !ad.IsDir() {
					continue
				}
				accountName := ad.Name()
				accountPath := filepath.Join(datePath, accountName)

				files, _ := os.ReadDir(accountPath)
				for _, f := range files {
					if f.IsDir() {
						continue
					}
					fpath := filepath.Join(accountPath, f.Name())

					switch {
					case strings.HasSuffix(f.Name(), ".xlsx") && strings.Contains(f.Name(), "_推商品"):
						n := importDistProduct(db, fpath, sqlDate, accountName)
						totalProduct += n
					case strings.HasSuffix(f.Name(), ".xlsx") && strings.Contains(f.Name(), "_推抖音号"):
						n := importDistAccount(db, fpath, sqlDate, accountName)
						totalAccount += n
					case strings.HasSuffix(f.Name(), ".xlsx") && strings.Contains(f.Name(), "_推素材"):
						n := importDistMaterial(db, fpath, sqlDate, accountName)
						totalMaterial += n
					case strings.HasSuffix(f.Name(), ".json") && strings.Contains(f.Name(), "_随心推"):
						n := importDistPromote(db, fpath, sqlDate, accountName)
						totalPromote += n
					}
				}
			}
		}
	}

	fmt.Printf("\n导入完成:\n  推商品: %d 条\n  推抖音号: %d 条\n  推素材: %d 条\n  随心推: %d 条\n", totalProduct, totalAccount, totalMaterial, totalPromote)
}

func importDistProduct(db *sql.DB, path, sqlDate, accountName string) int {
	f, err := excelize.OpenFile(path)
	if err != nil {
		log.Printf("打开失败 %s: %v", filepath.Base(path), err)
		return 0
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	rows, _ := f.GetRows(sheet)
	if len(rows) < 2 {
		return 0
	}

	header := rows[0]
	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.TrimSpace(h)] = i
	}

	count := 0
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		get := func(name string) string {
			idx, ok := colMap[name]
			if !ok || idx >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[idx])
		}
		getF := func(name string) float64 {
			s := strings.ReplaceAll(strings.ReplaceAll(get(name), ",", ""), "¥", "")
			s = strings.ReplaceAll(s, "%", "")
			v, _ := strconv.ParseFloat(s, 64)
			return v
		}
		getI := func(name string) int {
			s := strings.ReplaceAll(get(name), ",", "")
			v, _ := strconv.Atoi(s)
			return v
		}

		productID := get("商品ID")
		if productID == "" {
			continue
		}
		// 跳过"全部"汇总行
		if get("日期") == "全部" {
			continue
		}

		_, err := db.Exec(`REPLACE INTO op_douyin_dist_product_daily (
			stat_date, account_name, product_id, product_name,
			impressions, clicks, click_rate, conv_rate,
			cost, pay_amount, roi, order_cost,
			user_pay_amount, subsidy_amount,
			net_roi, net_amount, net_order_cost, net_settle_rate, refund_1h_rate
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			sqlDate, accountName, productID, get("商品名称"),
			getI("整体展示次数"), getI("整体点击次数"),
			getF("整体点击率")/100, getF("整体转化率")/100,
			getF("整体消耗"), getF("整体成交金额"), getF("整体支付ROI"), getF("整体成交订单成本"),
			getF("用户实际支付金额"), getF("电商平台补贴金额"),
			getF("净成交ROI"), getF("净成交金额"), getF("净成交订单成本"),
			getF("净成交金额结算率")/100, getF("1小时内退款率")/100,
		)
		if err != nil {
			log.Printf("推商品写入失败: %v", err)
			continue
		}
		count++
	}
	return count
}

func importDistAccount(db *sql.DB, path, sqlDate, accountName string) int {
	f, err := excelize.OpenFile(path)
	if err != nil {
		log.Printf("打开失败 %s: %v", filepath.Base(path), err)
		return 0
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	rows, _ := f.GetRows(sheet)
	if len(rows) < 2 {
		return 0
	}

	header := rows[0]
	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.TrimSpace(h)] = i
	}

	count := 0
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		get := func(name string) string {
			idx, ok := colMap[name]
			if !ok || idx >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[idx])
		}
		getF := func(name string) float64 {
			s := strings.ReplaceAll(strings.ReplaceAll(get(name), ",", ""), "¥", "")
			s = strings.ReplaceAll(s, "%", "")
			v, _ := strconv.ParseFloat(s, 64)
			return v
		}
		getI := func(name string) int {
			s := strings.ReplaceAll(get(name), ",", "")
			v, _ := strconv.Atoi(s)
			return v
		}

		douyinName := get("抖音号名称")
		if douyinName == "" {
			continue
		}
		if get("日期") == "全部" {
			continue
		}

		_, err := db.Exec(`REPLACE INTO op_douyin_dist_account_daily (
			stat_date, account_name, douyin_name, douyin_id,
			cost, order_count, pay_amount, roi, order_cost,
			user_pay_amount, subsidy_amount,
			net_roi, net_amount, net_settle_rate, refund_1h_rate
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			sqlDate, accountName, douyinName, get("抖音号ID"),
			getF("整体消耗"), getI("整体成交订单数"), getF("整体成交金额"),
			getF("整体支付ROI"), getF("整体成交订单成本"),
			getF("用户实际支付金额"), getF("电商平台补贴金额"),
			getF("净成交ROI"), getF("净成交金额"),
			getF("净成交金额结算率")/100, getF("1小时内退款率")/100,
		)
		if err != nil {
			log.Printf("推抖音号写入失败: %v", err)
			continue
		}
		count++
	}
	return count
}

func importDistMaterial(db *sql.DB, path, sqlDate, accountName string) int {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 2 {
		return 0
	}
	header := rows[0]
	colMap := make(map[string]int)
	for i, h := range header {
		// 处理可能的英文列名
		name := strings.TrimSpace(h)
		if name == "material_id" { name = "素材ID" }
		if name == "roi2_material_video_name" { name = "素材视频名称" }
		if name == "material_create_time_v2" { name = "素材创建时间" }
		colMap[name] = i
	}

	count := 0
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		get := func(name string) string {
			idx, ok := colMap[name]
			if !ok || idx >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[idx])
		}
		getF := func(name string) float64 {
			s := strings.ReplaceAll(strings.ReplaceAll(get(name), ",", ""), "¥", "")
			s = strings.ReplaceAll(s, "%", "")
			v, _ := strconv.ParseFloat(s, 64)
			return v
		}
		getI := func(name string) int {
			s := strings.ReplaceAll(get(name), ",", "")
			v, _ := strconv.Atoi(s)
			return v
		}

		materialID := get("素材ID")
		if materialID == "" || materialID == "-" || get("日期") == "全部" {
			continue
		}

		db.Exec(`INSERT INTO op_douyin_dist_material_daily (
			stat_date, account_name, material_id, material_name,
			impressions, clicks, click_rate, conv_rate,
			cost, order_count, pay_amount, roi, order_cost,
			user_pay_amount, cpm, cpc,
			net_roi, net_amount, net_order_count, net_settle_rate, refund_1h_rate
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE cost=VALUES(cost), pay_amount=VALUES(pay_amount)`,
			sqlDate, accountName, materialID, get("素材视频名称"),
			getI("整体展示次数"), getI("整体点击次数"), getF("整体点击率")/100, getF("整体转化率")/100,
			getF("整体消耗"), getI("整体成交订单数"), getF("整体成交金额"), getF("整体支付ROI"), getF("整体成交订单成本"),
			getF("用户实际支付金额"), getF("整体千次展现费用"), getF("整体点击单价"),
			getF("净成交ROI"), getF("净成交金额"), getI("净成交订单数"),
			getF("净成交金额结算率")/100, getF("1小时内退款率")/100,
		)
		count++
	}
	return count
}

func importDistPromote(db *sql.DB, path, sqlDate, accountName string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	var raw struct {
		Data struct {
			StatsData struct {
				Rows []struct {
					Dimensions map[string]struct {
						ValueStr string `json:"ValueStr"`
					} `json:"Dimensions"`
					Metrics map[string]struct {
						Value float64 `json:"Value"`
					} `json:"Metrics"`
				} `json:"Rows"`
			} `json:"StatsData"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return 0
	}

	count := 0
	for _, row := range raw.Data.StatsData.Rows {
		hour := ""
		if dim, ok := row.Dimensions["stat_time_hour"]; ok {
			// "2026-04-01 23:00" → "23:00"
			parts := strings.Split(dim.ValueStr, " ")
			if len(parts) >= 2 {
				hour = parts[1]
			}
		}
		if hour == "" {
			continue
		}

		m := row.Metrics
		db.Exec(`REPLACE INTO op_douyin_dist_promote_hourly (
			stat_date, account_name, stat_hour,
			cost, settle_amount, settle_count, roi, refund_rate
		) VALUES (?,?,?,?,?,?,?,?)`,
			sqlDate, accountName, hour,
			m["stat_cost_for_roi2"].Value,
			m["total_order_settle_amount_for_roi2_1h"].Value,
			int(m["total_order_settle_count_for_roi2_1h"].Value),
			m["total_prepay_and_pay_settle_roi2_1h"].Value,
			m["total_refund_order_gmv_for_roi2_1h_rate"].Value/100,
		)
		count++
	}
	return count
}
