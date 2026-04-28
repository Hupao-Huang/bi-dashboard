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
	"regexp"
	"strconv"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/xuri/excelize/v2"
)

// dateRe 从形如 "2026/04/26 07:00:29" / "2026-04-26" 的字符串里抽出 YYYY-MM-DD
var dateRe = regexp.MustCompile(`(\d{4})[/-](\d{1,2})[/-](\d{1,2})`)

// extractDate 抽取业务日期；抽不到返回 ""，调用方 fallback 到 RPA 文件名日期
func extractDate(s string) string {
	m := dateRe.FindStringSubmatch(s)
	if len(m) < 4 {
		return ""
	}
	mn, _ := strconv.Atoi(m[2])
	d, _ := strconv.Atoi(m[3])
	return fmt.Sprintf("%s-%02d-%02d", m[1], mn, d)
}

var baseDir = `Z:\信息部\RPA_集团数据看板\抖音`

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

	totalLive, totalGoods, totalChannel, totalFunnel, totalAdLive, totalAnchor, totalMaterial := 0, 0, 0, 0, 0, 0, 0

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
					fpath := filepath.Join(shopPath, f.Name())

					switch {
					case strings.HasSuffix(f.Name(), ".xlsx") && strings.Contains(f.Name(), "_直播数据"):
						n := importLiveDaily(db, fpath, sqlDate, shopName)
						totalLive += n
					case strings.HasSuffix(f.Name(), ".xlsx") && strings.Contains(f.Name(), "_商品数据"):
						n := importGoodsDaily(db, fpath, sqlDate, shopName)
						totalGoods += n
					case strings.HasSuffix(f.Name(), ".xlsx") && strings.Contains(f.Name(), "_推广直播间画面"):
						n := importAdLiveDaily(db, fpath, sqlDate, shopName)
						totalAdLive += n
					case strings.HasSuffix(f.Name(), ".json") && strings.Contains(f.Name(), "_渠道分析"):
						n := importChannelDaily(db, fpath, sqlDate, shopName)
						totalChannel += n
					case strings.HasSuffix(f.Name(), ".json") && strings.Contains(f.Name(), "_转化漏斗"):
						n := importFunnelDaily(db, fpath, sqlDate, shopName)
						totalFunnel += n
					case strings.HasSuffix(f.Name(), ".xlsx") && strings.Contains(f.Name(), "_主播分析"):
						n := importAnchorDaily(db, fpath, sqlDate, shopName)
						totalAnchor += n
					case strings.HasSuffix(f.Name(), ".xlsx") && strings.Contains(f.Name(), "_推广视频素材"):
						n := importAdMaterialDaily(db, fpath, sqlDate, shopName)
						totalMaterial += n
					}
				}
			}
		}
	}

	fmt.Printf("\n导入完成:\n  直播数据: %d 条\n  商品数据: %d 条\n  渠道分析: %d 条\n  转化漏斗: %d 条\n  推广直播间: %d 条\n  主播分析: %d 条\n  推广素材: %d 条\n",
		totalLive, totalGoods, totalChannel, totalFunnel, totalAdLive, totalAnchor, totalMaterial)
}

func importLiveDaily(db *sql.DB, path, sqlDate, shopName string) int {
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

		anchorName := get("主播昵称")
		if anchorName == "" {
			continue
		}

		// 业务日期优先从"直播开始时间"提取（RPA 文件夹名是抓取日，业务日是直播实际日期）
		statDate := extractDate(get("直播开始时间"))
		if statDate == "" {
			statDate = sqlDate
		}

		_, err := db.Exec(`REPLACE INTO op_douyin_live_daily (
			stat_date, shop_name, anchor_name, anchor_id,
			start_time, end_time, duration_min,
			exposure_uv, watch_uv, watch_pv, max_online, avg_online,
			avg_watch_min, comments, new_fans,
			product_count, product_exposure_uv, product_click_uv,
			order_count, order_amount, pay_amount, pay_per_hour,
			pay_qty, pay_uv, refund_count, refund_amount,
			ad_cost_bindshop, ad_cost_invested,
			net_order_amount, net_order_count, refund_1h_rate
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			statDate, shopName, anchorName, get("主播抖音号"),
			parseTime(get("直播开始时间")), parseTime(get("直播结束时间")), getF("直播时长(分钟)"),
			getI("直播间曝光人数"), getI("直播间观看人数"), getI("直播间观看次数"),
			getI("最高在线人数"), getI("平均在线人数"),
			getF("人均观看时长(分钟)"), getI("评论次数"), getI("新增粉丝数"),
			getI("带货商品数"), getI("直播间商品曝光人数"), getI("直播间商品点击人数"),
			getI("直播间成交订单数"), getF("直播间成交金额"), getF("直播间用户支付金额"),
			getF("单小时用户支付金额"),
			getI("直播间成交件数"), getI("直播间成交人数"),
			getI("直播间退款订单数"), getF("直播间退款金额"),
			getF("投放消耗(店铺绑定)"), getF("投放消耗(店铺被投)"),
			getF("净成交金额"), getI("净成交订单数"), getF("1小时退款率"),
		)
		if err != nil {
			log.Printf("直播写入失败: %v", err)
			continue
		}
		count++
	}
	return count
}

func importGoodsDaily(db *sql.DB, path, sqlDate, shopName string) int {
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

		_, err := db.Exec(`REPLACE INTO op_douyin_goods_daily (
			stat_date, shop_name, product_id, product_name,
			explain_count, live_price, pay_amount, pay_qty,
			presale_count, click_uv, click_rate, conv_rate,
			cpm_pay_amount, pre_refund_count, pre_refund_amount,
			post_refund_count, post_refund_amount
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			sqlDate, shopName, productID, get("商品名称"),
			getI("讲解次数"), get("直播间价格"), getF("用户支付金额"), getI("成交件数"),
			getI("预售订单数"), getI("商品点击人数"),
			getF("商品曝光-点击率（人数）")/100, getF("商品点击-成交转化率（人数）")/100,
			getF("千次曝光用户支付金额"),
			getI("发货前退款订单数"), getF("发货前退款金额"),
			getI("发货后退款订单数"), getF("发货后退款金额"),
		)
		if err != nil {
			log.Printf("商品写入失败: %v", err)
			continue
		}
		count++
	}
	return count
}

func parseTime(s string) interface{} {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return nil
	}
	return s
}

// ==================== 渠道分析 (JSON) ====================

func importChannelDaily(db *sql.DB, path, sqlDate, shopName string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("读取失败 %s: %v", filepath.Base(path), err)
		return 0
	}

	var raw struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		log.Printf("解析JSON失败 %s: %v", filepath.Base(path), err)
		return 0
	}

	count := 0
	for _, item := range raw.Data {
		var row map[string]interface{}
		json.Unmarshal(item, &row)
		ci, ok := row["cell_info"].(map[string]interface{})
		if !ok {
			continue
		}

		chName := extractStr(ci, "channel_name", "channel_name_value")
		if chName == "" || chName == "整体" {
			continue
		}

		_, err := db.Exec(`REPLACE INTO op_douyin_channel_daily (
			stat_date, shop_name, channel_name,
			watch_ucnt, watch_cnt, avg_watch_duration,
			pay_amt, pay_cnt, avg_pay_order_amt,
			watch_pay_cnt_ratio, interact_watch_cnt_ratio,
			ad_costed_amt, stat_cost
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			sqlDate, shopName, chName,
			extractInt(ci, "watch_ucnt"), extractInt(ci, "watch_cnt"),
			extractFloat(ci, "avg_watch_duration"),
			extractFloat(ci, "pay_amt"), extractInt(ci, "pay_cnt"),
			extractFloat(ci, "avg_pay_order_amt"),
			extractFloat(ci, "watch_pay_cnt_ratio"),
			extractFloat(ci, "interact_watch_cnt_ratio"),
			extractFloat(ci, "ad_costed_amt"), extractFloat(ci, "stat_cost"),
		)
		if err != nil {
			log.Printf("渠道写入失败: %v", err)
			continue
		}
		count++
	}
	return count
}

func extractStr(ci map[string]interface{}, key, valueKey string) string {
	obj, ok := ci[key].(map[string]interface{})
	if !ok {
		return ""
	}
	vObj, ok := obj[valueKey].(map[string]interface{})
	if !ok {
		return ""
	}
	val, ok := vObj["value"].(map[string]interface{})
	if !ok {
		return ""
	}
	if s, ok := val["value_str"].(string); ok {
		return s
	}
	return ""
}

func extractFloat(ci map[string]interface{}, key string) float64 {
	obj, ok := ci[key].(map[string]interface{})
	if !ok {
		return 0
	}
	for _, v := range obj {
		vMap, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		// cell_type=2: index_values.value.value
		if iv, ok := vMap["index_values"].(map[string]interface{}); ok {
			if val, ok := iv["value"].(map[string]interface{}); ok {
				if f, ok := val["value"].(float64); ok {
					return f
				}
			}
		}
		// cell_type=1: value.value (有些字段用这个结构)
		if val, ok := vMap["value"].(map[string]interface{}); ok {
			if f, ok := val["value"].(float64); ok {
				return f
			}
		}
	}
	return 0
}

func extractInt(ci map[string]interface{}, key string) int {
	return int(extractFloat(ci, key))
}

// ==================== 转化漏斗 (JSON) ====================

func importFunnelDaily(db *sql.DB, path, sqlDate, shopName string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("读取失败 %s: %v", filepath.Base(path), err)
		return 0
	}

	var raw struct {
		Data struct {
			GmvChange []struct {
				IndexName string  `json:"index_name"`
				Value     float64 `json:"value"`
			} `json:"gmv_change"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		log.Printf("解析JSON失败 %s: %v", filepath.Base(path), err)
		return 0
	}

	count := 0
	for i, step := range raw.Data.GmvChange {
		if step.IndexName == "" {
			continue
		}
		_, err := db.Exec(`REPLACE INTO op_douyin_funnel_daily (
			stat_date, shop_name, step_name, step_value, step_order
		) VALUES (?,?,?,?,?)`,
			sqlDate, shopName, step.IndexName, int64(step.Value), i,
		)
		if err != nil {
			log.Printf("漏斗写入失败: %v", err)
			continue
		}
		count++
	}
	return count
}

// ==================== 推广直播间画面 (xlsx) ====================

func importAdLiveDaily(db *sql.DB, path, sqlDate, shopName string) int {
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
		rowDate := get("日期")
		if douyinName == "" || rowDate == "" || rowDate == "全部" {
			continue
		}
		// stat_date 取 Excel "日期" 列(业务日)，文件名日期只是 RPA 采集日
		statDate := rowDate
		if statDate == "" {
			statDate = sqlDate
		}

		_, err := db.Exec(`REPLACE INTO op_douyin_ad_live_daily (
			stat_date, shop_name, douyin_name, douyin_id,
			net_roi, net_amount, net_order_count, net_order_cost,
			user_pay_amount, net_settle_rate, refund_1h_rate,
			impressions, clicks, click_rate, conv_rate,
			cost, pay_amount, roi, cpm
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			statDate, shopName, douyinName, get("抖音号ID"),
			getF("净成交ROI"), getF("净成交金额"), getI("净成交订单数"), getF("净成交订单成本"),
			getF("用户实际支付金额"), getF("净成交金额结算率")/100, getF("1小时内退款率")/100,
			getI("整体展示次数"), getI("整体点击次数"), getF("整体点击率")/100, getF("整体转化率")/100,
			getF("整体消耗"), getF("整体成交金额"), getF("整体支付ROI")/1, getF("整体千次展现费用"),
		)
		if err != nil {
			log.Printf("推广直播间写入失败: %v", err)
			continue
		}
		count++
	}
	return count
}

// ==================== 主播分析 (xlsx) ====================

func importAnchorDaily(db *sql.DB, path, sqlDate, shopName string) int {
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

		anchorName := get("主播名称")
		if anchorName == "" || anchorName == "-" {
			continue
		}

		db.Exec(`REPLACE INTO op_douyin_anchor_daily (
			stat_date, shop_name, account, anchor_name,
			duration, pay_amount, pay_per_hour, max_online, avg_online, avg_item_price,
			exposure_watch_rate, retention_rate, interact_rate,
			new_fans, fans_rate, new_group, uv_value, pay_uv,
			crowd_top3, gender, age_top3, city_level_top3, region_top3
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			sqlDate, shopName, get("账号"), anchorName,
			get("开播时长"), getF("直播间用户支付金额"), getF("单小时用户支付金额"),
			getI("最高在线人数"), getI("平均在线人数"), getF("平均件单价"),
			get("引流能力 曝光-观看率"), get("留人能力 留存率"), get("互动能力 观看-互动率"),
			getI("吸粉能力 新增粉丝数"), get("吸粉能力 观看-关注率"), getI("吸粉能力 新加团人数"),
			getF("转化能力 单UV价值(元)"), getI("成交人数"),
			get("策略人群TOP3"), get("性别"), get("年龄段TOP3"), get("城市等级TOP3"), get("地域分布TOP3"),
		)
		count++
	}
	return count
}

// ==================== 推广视频素材 (xlsx) ====================

func importAdMaterialDaily(db *sql.DB, path, sqlDate, shopName string) int {
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

		materialName := get("素材名称")
		if materialName == "" || materialName == "-" {
			continue
		}

		db.Exec(`INSERT INTO op_douyin_ad_material_daily (
			stat_date, shop_name, material_name, material_id,
			material_eval, material_duration, material_source,
			net_roi, net_amount, net_order_count, refund_1h_rate,
			impressions, clicks, click_rate, conv_rate,
			cost, pay_amount, roi, cpm
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE cost=VALUES(cost), pay_amount=VALUES(pay_amount)`,
			sqlDate, shopName, materialName, get("素材ID"),
			get("素材评估"), get("素材时长"), get("素材来源"),
			getF("净成交ROI"), getF("净成交金额"), getI("净成交订单数"), getF("1小时内退款率")/100,
			getI("整体展现次数"), getI("整体点击次数"), getF("整体点击率")/100, getF("整体转化率")/100,
			getF("整体消耗"), getF("整体成交金额"), getF("整体支付ROI"), getF("整体千次展现费用"),
		)
		count++
	}
	return count
}
