package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"

	_ "github.com/go-sql-driver/mysql"
	"github.com/xuri/excelize/v2"
)

var db *sql.DB

// 文件名正则: 天猫_{date}_{shop}_{source}_{type}.{ext}
var filePattern = regexp.MustCompile(`天猫_(\d{8})_(.+?)_(生意参谋|万象台|淘宝联盟|数据银行|达摩盘|集客)_(.+?)\.(xlsx|xls|csv|json)$`)

func main() {
	// 参数: import-tmall [起始日期YYYYMMDD] [结束日期YYYYMMDD]
	// 不传则导入所有
	// 参数: import-tmall [起始日期] [结束日期] [数据根目录]
	startDate := ""
	endDate := ""
	if len(os.Args) >= 3 {
		startDate = os.Args[1]
		endDate = os.Args[2]
	}

	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	db, err = sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)

	dataRoot := `Z:\信息部\RPA_集团数据看板\天猫`
	if len(os.Args) >= 4 {
		dataRoot = os.Args[3]
	}
	dataRoot, err = importutil.ResolveDataRoot(dataRoot)
	if err != nil {
		log.Fatalf("数据目录不可用: %v", err)
	}

	// 遍历年份目录
	years, err := os.ReadDir(dataRoot)
	if err != nil {
		log.Fatalf("读取数据目录失败: %v", err)
	}

	// 第一遍：收集匹配日期范围的所有日期，找出最大日期
	var allDates []struct {
		dateStr string
		path    string
	}
	for _, y := range years {
		if !y.IsDir() {
			continue
		}
		yearPath := filepath.Join(dataRoot, y.Name())
		dates, err := os.ReadDir(yearPath)
		if err != nil {
			log.Printf("读取年份目录失败 [%s]: %v", yearPath, err)
			continue
		}
		for _, d := range dates {
			if !d.IsDir() {
				continue
			}
			dateStr := d.Name()
			if startDate != "" && dateStr < startDate {
				continue
			}
			if endDate != "" && dateStr > endDate {
				continue
			}
			allDates = append(allDates, struct {
				dateStr string
				path    string
			}{dateStr, filepath.Join(yearPath, dateStr)})
		}
	}
	if startDate != "" && endDate != "" && len(allDates) == 0 {
		log.Fatalf("未找到日期范围 %s-%s 的数据目录", startDate, endDate)
	}
	// 找出范围内最大日期，多天数据文件只在最大日期处理
	maxDateStr := ""
	for _, d := range allDates {
		if d.dateStr > maxDateStr {
			maxDateStr = d.dateStr
		}
	}
	// 第二遍：实际导入
	for _, d := range allDates {
		isLatest := d.dateStr == maxDateStr
		shops, err := os.ReadDir(d.path)
		if err != nil {
			log.Printf("读取日期目录失败 [%s]: %v", d.path, err)
			continue
		}
		for _, s := range shops {
			if !s.IsDir() {
				continue
			}
			shopPath := filepath.Join(d.path, s.Name())
			processShopDir(shopPath, d.dateStr, s.Name(), isLatest)
		}
	}
	log.Println("导入完成!")
}

func processShopDir(dir, dateStr, shopName string, isLatest bool) {
	files, _ := os.ReadDir(dir)
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		matches := filePattern.FindStringSubmatch(name)
		if matches == nil {
			continue
		}
		// matches: [full, date, shop, source, type, ext]
		source := matches[3]
		dataType := matches[4]
		ext := matches[5]
		fullPath := filepath.Join(dir, name)

		var err error
		switch {
		case source == "生意参谋" && dataType == "店铺销售数据" && (ext == "xlsx" || ext == "xls"):
			err = importShopDaily(fullPath, dateStr, shopName)
		case source == "生意参谋" && dataType == "商品销售数据" && (ext == "xlsx" || ext == "xls"):
			err = importGoodsDaily(fullPath, dateStr, shopName)
		case source == "万象台" && dataType == "营销场景数据" && (ext == "xlsx" || ext == "xls"):
			err = importCampaignDaily(fullPath, dateStr, shopName)
		case source == "万象台" && dataType == "营销明细数据" && (ext == "xlsx" || ext == "xls"):
			err = importCampaignDetailDaily(fullPath, dateStr, shopName)
		case source == "淘宝联盟" && dataType == "营销场景数据" && (ext == "xlsx" || ext == "xls"):
			err = importCPSDaily(fullPath, dateStr, shopName)
		case source == "生意参谋" && dataType == "业绩询单" && ext == "xlsx":
			err = importServiceInquiry(fullPath, dateStr, shopName)
		case source == "生意参谋" && dataType == "咨询接待" && ext == "xlsx":
			err = importServiceConsult(fullPath, dateStr, shopName)
		case source == "生意参谋" && dataType == "客单价客服" && ext == "xlsx":
			err = importServiceAvgPrice(fullPath, dateStr, shopName)
		case source == "生意参谋" && dataType == "接待评价" && ext == "xlsx":
			err = importServiceEvaluation(fullPath, dateStr, shopName)
		// 多天数据文件：只在日期范围最大的那一天处理（其他天的内容是相同的滚动数据）
		case source == "生意参谋" && dataType == "会员数据" && ext == "json":
			if !isLatest {
				continue
			}
			err = importMemberDaily(fullPath, shopName)
		case source == "数据银行" && dataType == "品牌数据" && ext == "json":
			if !isLatest {
				continue
			}
			err = importBrandDaily(fullPath, shopName)
		case source == "达摩盘" && dataType == "人群数据" && ext == "json":
			if !isLatest {
				continue
			}
			err = importCrowdDaily(fullPath, shopName)
		case source == "集客" && dataType == "复购数据" && (ext == "xlsx" || ext == "xls"):
			if !isLatest {
				continue
			}
			err = importRepurchaseMonthly(fullPath, shopName)
		case source == "集客" && dataType == "行业数据" && (ext == "xlsx" || ext == "xls"):
			if !isLatest {
				continue
			}
			err = importIndustryMonthly(fullPath, shopName)
		default:
			continue // 跳过重复格式(如csv有对应xlsx)
		}

		if err != nil {
			log.Printf("导入失败 [%s]: %v", name, err)
		} else {
			log.Printf("导入成功 [%s]", name)
		}
	}
}

// ==================== 工具函数 ====================

func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "%", "")
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func parseInt(s string) int {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0
	}
	v, _ := strconv.Atoi(s)
	return v
}

func cellStr(row []string, idx int) string {
	if idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func formatDate(dateStr string) string {
	// YYYYMMDD -> YYYY-MM-DD
	if len(dateStr) == 8 {
		return dateStr[:4] + "-" + dateStr[4:6] + "-" + dateStr[6:8]
	}
	return dateStr
}

// parseExcelDate 兼容 Excel 日期列各种格式：2026-04-17 / 2026/4/17 / 2026年4月17日 / 20260417
// 为空返回空串（调用方自行 fallback）
func parseExcelDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "年", "-")
	s = strings.ReplaceAll(s, "月", "-")
	s = strings.ReplaceAll(s, "日", "")
	// YYYYMMDD（无分隔符）
	if len(s) == 8 && !strings.Contains(s, "-") {
		return s[:4] + "-" + s[4:6] + "-" + s[6:8]
	}
	// YYYY-M-D / YYYY-MM-DD 补 0
	parts := strings.Split(s, "-")
	if len(parts) == 3 {
		y, m, d := parts[0], parts[1], parts[2]
		if len(m) == 1 {
			m = "0" + m
		}
		if len(d) == 1 {
			d = "0" + d
		}
		if len(y) == 4 {
			return y + "-" + m + "-" + d
		}
	}
	return s
}

// ==================== 生意参谋-店铺销售 ====================

func importShopDaily(path, dateStr, shopName string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("打开xlsx失败: %w", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil {
		return fmt.Errorf("读取sheet失败: %w", err)
	}

	if len(rows) < 2 {
		return fmt.Errorf("没有数据行")
	}

	// 第0行是表头，第1行是数据
	header := rows[0]
	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.TrimSpace(h)] = i
	}

	for _, row := range rows[1:] {
		get := func(name string) string {
			idx, ok := colMap[name]
			if !ok || idx >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[idx])
		}
		getF := func(name string) float64 { return parseFloat(get(name)) }
		getI := func(name string) int { return parseInt(get(name)) }

		statDate := get("统计日期")
		if statDate == "" {
			statDate = formatDate(dateStr)
		}

		_, err := db.Exec(`REPLACE INTO op_tmall_shop_daily (
			stat_date, shop_name,
			visitors, visitors_wireless, page_views, product_visitors, product_views,
			avg_stay_time, bounce_rate,
			cart_buyers, cart_qty, collect_buyers,
			order_amount, order_buyers, order_qty, order_conv_rate,
			pay_amount, pay_buyers, pay_qty, pay_sub_orders, pay_conv_rate,
			unit_price, uv_value,
			old_visitors, new_visitors, pay_new_buyers, pay_old_buyers, old_buyer_pay_amount,
			total_ad_cost, keyword_ad_cost, crowd_ad_cost, smart_ad_cost, taobaoke_fee,
			refund_amount, review_count,
			member_total, member_new, member_active, member_pay_amount, member_pay_buyers,
			follow_count
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			statDate, shopName,
			getI("访客数"), getI("无线端访客数"), getI("浏览量"), getI("商品访客数"), getI("商品浏览量"),
			getF("平均停留时长"), getF("跳失率"),
			getI("加购人数"), getI("加购件数"), getI("商品收藏买家数"),
			getF("下单金额"), getI("下单买家数"), getI("下单件数"), getF("下单转化率"),
			getF("支付金额"), getI("支付买家数"), getI("支付件数"), getI("支付子订单数"), getF("支付转化率"),
			getF("客单价"), getF("UV价值"),
			getI("老访客数"), getI("新访客数"), getI("支付新买家数"), getI("支付老买家数"), getF("老买家支付金额"),
			getF("全站推广花费"), getF("关键词推广花费"), getF("精准人群推广花费"), getF("智能场景花费"), getF("淘宝客佣金"),
			getF("成功退款金额"), getI("评价数"),
			getI("会员总数"), getI("新增会员数"), getI("活跃会员数"), getF("会员成交金额"), getI("会员成交人数"),
			getI("关注店铺人数"),
		)
		if err != nil {
			return fmt.Errorf("插入shop_daily失败: %w", err)
		}
	}
	return nil
}

// ==================== 生意参谋-商品销售 ====================

func importGoodsDaily(path, dateStr, shopName string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("打开xlsx失败: %w", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil {
		return fmt.Errorf("读取sheet失败: %w", err)
	}

	// 找表头行(含"统计日期")
	headerIdx := -1
	for i, row := range rows {
		if len(row) > 0 && strings.Contains(row[0], "统计日期") {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		return fmt.Errorf("未找到表头行")
	}

	header := rows[headerIdx]
	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.TrimSpace(h)] = i
	}

	count := 0
	for _, row := range rows[headerIdx+1:] {
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
		getF := func(name string) float64 { return parseFloat(get(name)) }
		getI := func(name string) int { return parseInt(get(name)) }

		productID := get("商品ID")
		if productID == "" {
			continue
		}

		statDate := get("统计日期")
		if statDate == "" {
			statDate = formatDate(dateStr)
		}

		_, err := db.Exec(`REPLACE INTO op_tmall_goods_daily (
			stat_date, shop_name, product_id, product_name, main_product_id,
			product_type, product_no, product_status,
			visitors, page_views, avg_stay_time, detail_bounce_rate,
			collect_buyers, cart_qty, cart_buyers,
			order_buyers, order_qty, order_amount, order_conv_rate,
			pay_buyers, pay_qty, pay_amount, pay_conv_rate,
			pay_new_buyers, pay_old_buyers, old_buyer_pay_amount,
			uv_value, refund_amount,
			year_pay_amount, month_pay_amount, month_pay_qty
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			statDate, shopName, productID, get("商品名称"), get("主商品ID"),
			get("商品类型"), get("货号"), get("商品状态"),
			getI("商品访客数"), getI("商品浏览量"), getF("平均停留时长"), get("商品详情页跳出率"),
			getI("商品收藏人数"), getI("商品加购件数"), getI("商品加购人数"),
			getI("下单买家数"), getI("下单件数"), getF("下单金额"), get("下单转化率"),
			getI("支付买家数"), getI("支付件数"), getF("支付金额"), get("商品支付转化率"),
			getI("支付新买家数"), getI("支付老买家数"), getF("老买家支付金额"),
			getF("访客平均价值"), getF("成功退款金额"),
			getF("年累计支付金额"), getF("月累计支付金额"), getI("月累计支付件数"),
		)
		if err != nil {
			log.Printf("插入goods_daily失败 [%s]: %v", productID, err)
			continue
		}
		count++
	}
	log.Printf("  商品销售: %d条", count)
	return nil
}

// ==================== 万象台-CPC推广 ====================

func importCampaignDaily(path, dateStr, shopName string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("打开xlsx失败: %w", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil {
		return fmt.Errorf("读取sheet失败: %w", err)
	}

	if len(rows) < 2 {
		return nil // 没有数据
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
		getF := func(name string) float64 { return parseFloat(get(name)) }
		getI := func(name string) int { return parseInt(get(name)) }

		statDate := get("日期")
		if statDate == "" {
			statDate = formatDate(dateStr)
		}

		sceneID := get("场景ID")
		if sceneID == "" {
			sceneID = "0"
		}

		_, err := db.Exec(`REPLACE INTO op_tmall_campaign_daily (
			stat_date, shop_name, scene_id, scene_name, orig_scene_id, orig_scene_name,
			impressions, clicks, cost, click_rate, avg_click_cost, cpm,
			total_pay_amount, total_pay_count, direct_pay_amount, indirect_pay_amount,
			click_conv_rate, roi,
			total_cart, cart_rate, total_collect,
			new_customer_count, new_customer_rate,
			member_first_buy, member_pay_amount, member_pay_count
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			statDate, shopName, sceneID, get("场景名字"), get("原二级场景ID"), get("原二级场景名字"),
			getI("展现量"), getI("点击量"), getF("花费"), getF("点击率"), getF("平均点击花费"), getF("千次展现花费"),
			getF("总成交金额"), getI("总成交笔数"), getF("直接成交金额"), getF("间接成交金额"),
			getF("点击转化率"), getF("投入产出比"),
			getI("总购物车数"), getF("加购率"), getI("总收藏数"),
			getI("成交新客数"), get("成交新客占比"),
			getI("会员首购人数"), getF("会员成交金额"), getI("会员成交笔数"),
		)
		if err != nil {
			log.Printf("插入campaign_daily失败: %v", err)
			continue
		}
		count++
	}
	log.Printf("  万象台: %d条", count)
	return nil
}

// ==================== 万象台-营销明细(商品级) ====================
// 文件: 天猫_{date}_{shop}_万象台_营销明细数据.xlsx
// 粒度: 日期 × 店铺 × 商品(主体ID)
// stat_date 取 Excel "日期" 列(业务日期)，文件名日期只是 RPA 采集日
func importCampaignDetailDaily(path, dateStr, shopName string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("打开xlsx失败: %w", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil {
		return fmt.Errorf("读取sheet失败: %w", err)
	}
	if len(rows) < 2 {
		return nil
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
		getF := func(name string) float64 { return parseFloat(get(name)) }
		getI := func(name string) int { return parseInt(get(name)) }

		productID := get("主体ID")
		if productID == "" {
			continue
		}
		statDate := get("日期")
		if statDate == "" {
			statDate = formatDate(dateStr)
		}

		_, err := db.Exec(`REPLACE INTO op_tmall_campaign_detail_daily (
			stat_date, shop_name, product_id, entity_type, product_name,
			impressions, clicks, cost, click_rate, avg_click_cost, cpm,
			presale_total_amount, presale_total_count,
			presale_direct_amount, presale_direct_count,
			presale_indirect_amount, presale_indirect_count,
			direct_pay_amount, indirect_pay_amount, total_pay_amount, total_pay_count,
			direct_pay_count, indirect_pay_count,
			click_conv_rate, roi, roi_with_presale, total_pay_cost,
			total_cart, direct_cart, indirect_cart, cart_rate, cart_cost,
			goods_collect_count, shop_collect_count, shop_collect_cost,
			total_cart_collect, total_cart_collect_cost,
			goods_cart_collect, goods_cart_collect_cost,
			total_collect, goods_collect_cost, goods_collect_rate,
			direct_goods_collect, indirect_goods_collect,
			place_order_count, place_order_amount,
			coupon_claim_count,
			shop_money_recharge_count, shop_money_recharge_amount,
			wangwang_consult_count,
			guide_visit_count, guide_visit_users, guide_visit_potential,
			guide_visit_potential_rate, member_join_rate, member_join_count,
			guide_visit_rate, deep_visit_count, avg_page_views,
			new_customer_count, new_customer_rate,
			member_first_buy, member_pay_amount, member_pay_count,
			buyer_count, avg_pay_count_per_user, avg_pay_amount_per_user,
			natural_flow_pay_amount, natural_flow_impressions,
			platform_total_pay, platform_direct_pay, platform_clicks
		) VALUES (?,?,?,?,?, ?,?,?,?,?,?, ?,?, ?,?, ?,?, ?,?,?,?, ?,?, ?,?,?,?, ?,?,?,?,?, ?,?,?, ?,?, ?,?, ?,?,?, ?,?, ?,?, ?, ?,?, ?, ?,?,?, ?,?,?, ?,?,?, ?,?, ?,?,?, ?,?,?, ?,?, ?,?,?)`,
			statDate, shopName, productID, get("主体类型"), get("主体名称"),
			getI("展现量"), getI("点击量"), getF("花费"), getF("点击率"), getF("平均点击花费"), getF("千次展现花费"),
			getF("总预售成交金额"), getI("总预售成交笔数"),
			getF("直接预售成交金额"), getI("直接预售成交笔数"),
			getF("间接预售成交金额"), getI("间接预售成交笔数"),
			getF("直接成交金额"), getF("间接成交金额"), getF("总成交金额"), getI("总成交笔数"),
			getI("直接成交笔数"), getI("间接成交笔数"),
			getF("点击转化率"), getF("投入产出比"), getF("含预售投产比"), getF("总成交成本"),
			getI("总购物车数"), getI("直接购物车数"), getI("间接购物车数"), getF("加购率"), getF("加购成本"),
			getI("收藏宝贝数"), getI("收藏店铺数"), getF("店铺收藏成本"),
			getI("总收藏加购数"), getF("总收藏加购成本"),
			getI("宝贝收藏加购数"), getF("宝贝收藏加购成本"),
			getI("总收藏数"), getF("宝贝收藏成本"), getF("宝贝收藏率"),
			getI("直接收藏宝贝数"), getI("间接收藏宝贝数"),
			getI("拍下订单笔数"), getF("拍下订单金额"),
			getI("优惠券领取量"),
			getI("购物金充值笔数"), getF("购物金充值金额"),
			getI("旺旺咨询量"),
			getI("引导访问量"), getI("引导访问人数"), getI("引导访问潜客数"),
			getF("引导访问潜客占比"), getF("入会率"), getI("入会量"),
			getF("引导访问率"), getI("深度访问量"), getF("平均访问页面数"),
			getI("成交新客数"), getF("成交新客占比"),
			getI("会员首购人数"), getF("会员成交金额"), getI("会员成交笔数"),
			getI("成交人数"), getF("人均成交笔数"), getF("人均成交金额"),
			getF("自然流量转化金额"), getI("自然流量曝光量"),
			getF("平台助推总成交"), getF("平台助推直接成交"), getI("平台助推点击"),
		)
		if err != nil {
			log.Printf("插入campaign_detail_daily失败 [%s]: %v", productID, err)
			continue
		}
		count++
	}
	log.Printf("  万象台明细: %d条", count)
	return nil
}

// ==================== 淘宝联盟-CPS推广 ====================

func importCPSDaily(path, dateStr, shopName string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("打开xlsx失败: %w", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil {
		return fmt.Errorf("读取xlsx失败: %w", err)
	}

	if len(rows) < 2 {
		return nil
	}

	header := rows[0]
	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.TrimSpace(h)] = i
	}

	count := 0
	for _, row := range rows[1:] {
		get := func(name string) string {
			idx, ok := colMap[name]
			if !ok || idx >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[idx])
		}
		getF := func(name string) float64 { return parseFloat(get(name)) }
		getI := func(name string) int { return parseInt(get(name)) }

		statDate := get("日期")
		if statDate == "" {
			statDate = formatDate(dateStr)
		}
		// 日期可能有trailing tab
		statDate = strings.TrimSpace(statDate)

		planName := get("数据内容")
		if planName == "" {
			continue
		}

		_, err := db.Exec(`REPLACE INTO op_tmall_cps_daily (
			stat_date, shop_name, plan_name,
			clicks, click_users, click_conv_rate,
			pay_users, pay_amount, pay_orders, pay_qty,
			pay_commission, pay_service_fee, pay_commission_rate, pay_total_cost,
			settle_users, settle_orders, settle_amount,
			settle_commission, settle_service_fee, settle_total_cost,
			confirm_users, confirm_amount, confirm_orders, per_item_cost
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			statDate, shopName, planName,
			getI("点击量(即进店量)"), getI("点击人数(即进店人数)"), get("点击转化率(即付款转化率)"),
			getI("付款人数"), getF("付款金额"), getI("付款笔数"), getI("付款件数"),
			getF("付款佣金支出"), getF("付款服务费支出"), get("付款佣金率"), getF("付款支出费用"),
			getI("结算人数"), getI("结算笔数"), getF("结算金额"),
			getF("结算佣金支出"), getF("结算服务费支出"), getF("结算支出费用"),
			getI("确认收货人数"), getF("确认收货金额"), getI("确认收货笔数"), getF("单件商品付款支出费用"),
		)
		if err != nil {
			log.Printf("插入cps_daily失败: %v", err)
			continue
		}
		count++
	}
	log.Printf("  淘宝联盟: %d条", count)
	return nil
}

// ==================== 生意参谋-会员数据(JSON) ====================

type MemberJSON struct {
	Code int `json:"code"`
	Data struct {
		PaidMbrCnt   []interface{} `json:"paidMbrCnt"`
		StatDate     []int64       `json:"statDate"`
		MbrPayAmt    []float64     `json:"mbrPayAmt"`
		MbrUnitPrice []float64     `json:"mbrUnitPrice"`
		TotalMbrCnt  []interface{} `json:"totalMbrCnt"`
		RepurMbrRate []float64     `json:"repurMbrRate"`
	} `json:"data"`
}

func importMemberDaily(path, shopName string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取json失败: %w", err)
	}

	var m MemberJSON
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("解析json失败: %w", err)
	}

	count := 0
	for i, ts := range m.Data.StatDate {
		statDate := time.Unix(ts/1000, 0).Format("2006-01-02")

		paidCnt := 0
		if i < len(m.Data.PaidMbrCnt) {
			paidCnt = toInt(m.Data.PaidMbrCnt[i])
		}
		payAmt := 0.0
		if i < len(m.Data.MbrPayAmt) {
			payAmt = m.Data.MbrPayAmt[i]
		}
		unitPrice := 0.0
		if i < len(m.Data.MbrUnitPrice) {
			unitPrice = m.Data.MbrUnitPrice[i]
		}
		totalCnt := 0
		if i < len(m.Data.TotalMbrCnt) {
			totalCnt = toInt(m.Data.TotalMbrCnt[i])
		}
		repurRate := 0.0
		if i < len(m.Data.RepurMbrRate) {
			repurRate = m.Data.RepurMbrRate[i]
		}

		_, err := db.Exec(`REPLACE INTO op_tmall_member_daily (
			stat_date, shop_name, paid_member_cnt, member_pay_amount,
			member_unit_price, total_member_cnt, repurchase_rate
		) VALUES (?,?,?,?,?,?,?)`,
			statDate, shopName, paidCnt, payAmt, unitPrice, totalCnt, repurRate)
		if err != nil {
			log.Printf("插入member_daily失败 [%s]: %v", statDate, err)
			continue
		}
		count++
	}
	log.Printf("  会员数据: %d条", count)
	return nil
}

func toInt(v interface{}) int {
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case json.Number:
		i, _ := val.Int64()
		return int(i)
	default:
		return 0
	}
}

// ==================== 数据银行-品牌数据(JSON) ====================

type BrandJSON struct {
	Data struct {
		Compare map[string]struct {
			Date   []string  `json:"date"`
			Values []float64 `json:"values"`
		} `json:"compare"`
	} `json:"data"`
}

func importBrandDaily(path, shopName string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取json失败: %w", err)
	}

	var b BrandJSON
	if err := json.Unmarshal(data, &b); err != nil {
		return fmt.Errorf("解析json失败: %w", err)
	}

	// 收集所有日期
	dateMap := make(map[string]map[string]float64)
	for metric, series := range b.Data.Compare {
		for i, dateStr := range series.Date {
			d := formatDate(dateStr)
			if dateMap[d] == nil {
				dateMap[d] = make(map[string]float64)
			}
			if i < len(series.Values) {
				dateMap[d][metric] = series.Values[i]
			}
		}
	}

	count := 0
	for statDate, metrics := range dateMap {
		_, err := db.Exec(`REPLACE INTO op_tmall_brand_daily (
			stat_date, shop_name,
			member_pay_amount, customer_volume, loyal_volume, awareness_volume,
			brand_pay_amount, deepen_ratio, deepen_uv,
			purchase_volume, interest_volume
		) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			statDate, shopName,
			metrics["mbrPayAmt"], int64(metrics["customerVolume"]),
			int64(metrics["loyalVolume"]), int64(metrics["awarenessVolume"]),
			metrics["brandPayAmt"], metrics["deepenRatio"], int64(metrics["deepenUv"]),
			int64(metrics["purchaseVolume"]), int64(metrics["interestVolume"]),
		)
		if err != nil {
			log.Printf("插入brand_daily失败 [%s]: %v", statDate, err)
			continue
		}
		count++
	}
	log.Printf("  数据银行: %d条", count)
	return nil
}

// ==================== 达摩盘-人群数据(JSON) ====================

type CrowdJSON struct {
	Data struct {
		List []map[string]interface{} `json:"list"`
	} `json:"data"`
}

func importCrowdDaily(path, shopName string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取json失败: %w", err)
	}

	var c CrowdJSON
	if err := json.Unmarshal(data, &c); err != nil {
		return fmt.Errorf("解析json失败: %w", err)
	}

	count := 0
	for _, item := range c.Data.List {
		statDate, _ := item["date"].(string)
		if statDate == "" {
			continue
		}

		getF := func(key string) float64 {
			v, _ := item[key].(float64)
			return v
		}
		getI := func(key string) int64 {
			v, _ := item[key].(float64)
			return int64(v)
		}

		_, err := db.Exec(`REPLACE INTO op_tmall_crowd_daily (
			stat_date, shop_name, coverage,
			ta_concentrate_ratio, ta_permeability_ratio,
			ta_permeability_visit, ta_permeability_repurchase,
			ta_permeability_prospect, ta_permeability_purchase, ta_permeability_interest,
			shop_alipay_amount, shop_alipay_cnt, shop_alipay_uv
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			statDate, shopName, getI("coverage"),
			getF("taConcentrateRatio"), getF("taPermeabilityRatio"),
			getF("taPermeabilityRatioFangWen"), getF("taPermeabilityRatioFuGou"),
			getF("taPermeabilityRatioQianKe"), getF("taPermeabilityRatioShouGou"),
			getF("taPermeabilityRatioXingQu"),
			getF("shopAmt"), getI("shopAlipayCnt"), getI("shopAlipayUv"),
		)
		if err != nil {
			log.Printf("插入crowd_daily失败 [%s]: %v", statDate, err)
			continue
		}
		count++
	}
	log.Printf("  达摩盘: %d条", count)
	return nil
}

// ==================== 集客-复购数据(xlsx) ====================

func importRepurchaseMonthly(path, shopName string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("打开xlsx失败: %w", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil {
		return fmt.Errorf("读取sheet失败: %w", err)
	}

	if len(rows) < 2 {
		return nil
	}

	header := rows[0]
	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.TrimSpace(h)] = i
	}

	count := 0
	for _, row := range rows[1:] {
		get := func(name string) string {
			idx, ok := colMap[name]
			if !ok || idx >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[idx])
		}
		getF := func(name string) float64 { return parseFloat(get(name)) }

		statMonth := get("时间")
		if statMonth == "" {
			continue
		}

		_, err := db.Exec(`REPLACE INTO op_tmall_repurchase_monthly (
			stat_month, shop_name, category,
			new_ratio, new_sales_ratio,
			new_repurchase_30d, new_repurchase_60d, new_repurchase_90d, new_repurchase_180d, new_repurchase_360d,
			old_ratio, old_sales_ratio,
			old_repurchase_30d, old_repurchase_60d, old_repurchase_90d, old_repurchase_180d, old_repurchase_360d,
			shop_repurchase_30d, shop_repurchase_180d, shop_repurchase_360d,
			lost_repurchase_rate, last_repurchase_days, unit_price
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			statMonth, shopName, get("行业/类目"),
			getF("新客占比"), getF("新客销售额占比"),
			getF("新客复购率 (30天)"), getF("新客复购率 (60天)"), getF("新客复购率(90天)"),
			getF("新客复购率 (180天)"), getF("新客复购率(360天)"),
			getF("老客占比"), getF("老客销售额占比"),
			getF("老客复购率(30天)"), getF("老客复购率(60天)"), getF("老客复购率(90天)"),
			getF("老客复购率(180天)"), getF("老客复购率(360天)"),
			getF("店铺复购率(30天)"), getF("店铺复购率(180天)"), getF("店铺复购率(360天)"),
			getF("流失客户回购率"), parseInt(get("最后一次回购间隔(天)")), getF("客单价(元)"),
		)
		if err != nil {
			log.Printf("插入repurchase_monthly失败: %v", err)
			continue
		}
		count++
	}
	log.Printf("  集客复购: %d条", count)
	return nil
}

// ==================== 集客-行业数据(xlsx) ====================

func importIndustryMonthly(path, shopName string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("打开xlsx失败: %w", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil {
		return fmt.Errorf("读取sheet失败: %w", err)
	}

	if len(rows) < 2 {
		return nil
	}

	header := rows[0]
	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.TrimSpace(h)] = i
	}

	count := 0
	for _, row := range rows[1:] {
		get := func(name string) string {
			idx, ok := colMap[name]
			if !ok || idx >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[idx])
		}
		getF := func(name string) float64 { return parseFloat(get(name)) }

		statMonth := get("时间")
		valueType := get("取值方式")
		if statMonth == "" || valueType == "" {
			continue
		}

		_, err := db.Exec(`REPLACE INTO op_tmall_industry_monthly (
			stat_month, shop_name, category, value_type,
			new_ratio, new_sales_ratio,
			new_repurchase_30d, new_repurchase_60d, new_repurchase_90d, new_repurchase_180d, new_repurchase_360d,
			old_ratio, old_sales_ratio,
			old_repurchase_30d, old_repurchase_60d, old_repurchase_90d, old_repurchase_180d, old_repurchase_360d,
			shop_repurchase_30d, shop_repurchase_180d, shop_repurchase_360d,
			lost_repurchase_rate, last_repurchase_days, unit_price
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			statMonth, shopName, get("行业/类目"), valueType,
			getF("新客占比"), getF("新客销售额占比"),
			getF("新客复购率 (30天)"), getF("新客复购率 (60天)"), getF("新客复购率(90天)"),
			getF("新客复购率 (180天)"), getF("新客复购率(360天)"),
			getF("老客占比"), getF("老客销售额占比"),
			getF("老客复购率(30天)"), getF("老客复购率(60天)"), getF("老客复购率(90天)"),
			getF("老客复购率(180天)"), getF("老客复购率(360天)"),
			getF("店铺复购率(30天)"), getF("店铺复购率(180天)"), getF("店铺复购率(360天)"),
			getF("流失客户回购率"), parseInt(get("最后一次回购间隔(天)")), getF("客单价(元)"),
		)
		if err != nil {
			log.Printf("插入industry_monthly失败: %v", err)
			continue
		}
		count++
	}
	log.Printf("  集客行业: %d条", count)
	return nil
}

// ==================== 客服-业绩询单 ====================

// 客服4个文件结构相同：行0表头，行1当日店铺数据，后面是汇总/全店/同行
// stat_date 取 Excel 第 0 列"日期"（业务日），文件名日期只是 RPA 采集日
func importServiceInquiry(path, dateStr, shopName string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("打开xlsx失败: %w", err)
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 2 {
		return nil
	}
	d := rows[1]
	statDate := parseExcelDate(cellStr(d, 0))
	if statDate == "" {
		statDate = formatDate(dateStr)
	}
	// 行1: [日期 询单人数 当日询单人数 当日付款人数 当日付款金额 最终付款人数 最终付款金额 询单当日付款转化率 询单最终付款转化率]
	_, err = db.Exec(`REPLACE INTO op_tmall_service_inquiry
		(stat_date, shop_name, inquiry_users, daily_inquiry_users, daily_pay_users, daily_pay_amount,
		 final_pay_users, final_pay_amount, daily_conv_rate, final_conv_rate)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		statDate, shopName,
		parseInt(cellStr(d, 1)), parseInt(cellStr(d, 2)), parseInt(cellStr(d, 3)), parseFloat(cellStr(d, 4)),
		parseInt(cellStr(d, 5)), parseFloat(cellStr(d, 6)), parseFloat(cellStr(d, 7)), parseFloat(cellStr(d, 8)),
	)
	return err
}

// ==================== 客服-咨询接待 ====================
// stat_date 取 Excel 第 0 列"日期"（业务日），文件名日期只是 RPA 采集日
func importServiceConsult(path, dateStr, shopName string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("打开xlsx失败: %w", err)
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 2 {
		return nil
	}
	d := rows[1]
	statDate := parseExcelDate(cellStr(d, 0))
	if statDate == "" {
		statDate = formatDate(dateStr)
	}
	// 行1: [日期 咨询人数 接待人数 未回复人数 有效接待人数 平均响应时长 3分钟人工响应率 咨询客服人次 客服回复人次 客服未回复人次 旺旺回复率 首次响应时长 慢响应人数 长接待人数 买家发起人数 客服主动跟进人数 总消息数 买家消息条数 客服消息条数 答问比 客服字数 平均接待时长]
	_, err = db.Exec(`REPLACE INTO op_tmall_service_consult
		(stat_date, shop_name, consult_users, receive_users, no_reply_users, effective_users,
		 avg_response_sec, three_min_resp_rate, consult_count, reply_count, no_reply_count, ww_reply_rate,
		 first_resp_sec, slow_resp_users, long_receive_users, buyer_initiated, cs_followed,
		 total_msgs, buyer_msgs, cs_msgs, qa_ratio, cs_words, avg_receive_time)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		statDate, shopName,
		parseInt(cellStr(d, 1)), parseInt(cellStr(d, 2)), parseInt(cellStr(d, 3)), parseInt(cellStr(d, 4)),
		parseFloat(cellStr(d, 5)), parseFloat(cellStr(d, 6)), parseInt(cellStr(d, 7)), parseInt(cellStr(d, 8)),
		parseInt(cellStr(d, 9)), parseFloat(cellStr(d, 10)), parseFloat(cellStr(d, 11)),
		parseInt(cellStr(d, 12)), parseInt(cellStr(d, 13)), parseInt(cellStr(d, 14)), parseInt(cellStr(d, 15)),
		parseInt(cellStr(d, 16)), parseInt(cellStr(d, 17)), parseInt(cellStr(d, 18)),
		parseFloat(cellStr(d, 19)), parseInt(cellStr(d, 20)), cellStr(d, 21),
	)
	return err
}

// ==================== 客服-客单价 ====================
// stat_date 取 Excel 第 0 列"日期"（业务日），文件名日期只是 RPA 采集日
func importServiceAvgPrice(path, dateStr, shopName string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("打开xlsx失败: %w", err)
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 2 {
		return nil
	}
	d := rows[1]
	statDate := parseExcelDate(cellStr(d, 0))
	if statDate == "" {
		statDate = formatDate(dateStr)
	}
	// 行1: [日期 销售额 销售量 销售人数 客单价 客件数 件均价]
	_, err = db.Exec(`REPLACE INTO op_tmall_service_avgprice
		(stat_date, shop_name, sales_amount, sales_qty, sales_users, avg_price, avg_qty, unit_price)
		VALUES (?,?,?,?,?,?,?,?)`,
		statDate, shopName,
		parseFloat(cellStr(d, 1)), parseInt(cellStr(d, 2)), parseInt(cellStr(d, 3)),
		parseFloat(cellStr(d, 4)), parseFloat(cellStr(d, 5)), parseFloat(cellStr(d, 6)),
	)
	return err
}

// ==================== 客服-接待评价 ====================
// stat_date 取 Excel 第 0 列"日期"（业务日），文件名日期只是 RPA 采集日
func importServiceEvaluation(path, dateStr, shopName string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("打开xlsx失败: %w", err)
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	// 接待评价文件有两行表头(行0是分组名 行1是字段名)，行2是数据
	if len(rows) < 3 {
		return nil
	}
	d := rows[2]
	statDate := parseExcelDate(cellStr(d, 0))
	if statDate == "" {
		statDate = formatDate(dateStr)
	}
	// 行2字段顺序: [日期 接待人数 总-发出 总-收到 总-很满意 总-满意 总-一般 总-不满 总-很不满 总-发送率 总-返回率 总-满意率 总-服务度 邀请-发出 邀请-收到 邀请-很满意 邀请-满意 邀请-一般 邀请-不满 邀请-很不满 邀请-发送率 邀请-返回率 邀请-满意率 邀请-服务度 自主-发出 自主-收到 自主-很满意 自主-满意 自主-一般 自主-不满 自主-很不满 自主-发送率 自主-返回率 自主-满意率 自主-服务度]
	_, err = db.Exec(`REPLACE INTO op_tmall_service_evaluation
		(stat_date, shop_name, receive_users,
		 total_send_eval, total_recv_eval, total_very_satisfied, total_satisfied, total_normal, total_unsatisfied, total_very_unsatisfied,
		 total_send_rate, total_return_rate, total_satisfaction_rate, total_service_score,
		 invite_send_eval, invite_recv_eval, invite_very_satisfied, invite_satisfied, invite_normal, invite_unsatisfied, invite_very_unsatisfied,
		 invite_send_rate, invite_return_rate, invite_satisfaction_rate, invite_service_score,
		 selfdone_send_eval, selfdone_recv_eval, selfdone_very_satisfied, selfdone_satisfied, selfdone_normal, selfdone_unsatisfied, selfdone_very_unsatisfied,
		 selfdone_send_rate, selfdone_return_rate, selfdone_satisfaction_rate, selfdone_service_score)
		VALUES (?,?,?, ?,?,?,?,?,?,?, ?,?,?,?, ?,?,?,?,?,?,?, ?,?,?,?, ?,?,?,?,?,?,?, ?,?,?,?)`,
		statDate, shopName, parseInt(cellStr(d, 1)),
		parseInt(cellStr(d, 2)), parseInt(cellStr(d, 3)), parseInt(cellStr(d, 4)), parseInt(cellStr(d, 5)),
		parseInt(cellStr(d, 6)), parseInt(cellStr(d, 7)), parseInt(cellStr(d, 8)),
		parseFloat(cellStr(d, 9)), parseFloat(cellStr(d, 10)), parseFloat(cellStr(d, 11)), parseFloat(cellStr(d, 12)),
		parseInt(cellStr(d, 13)), parseInt(cellStr(d, 14)), parseInt(cellStr(d, 15)), parseInt(cellStr(d, 16)),
		parseInt(cellStr(d, 17)), parseInt(cellStr(d, 18)), parseInt(cellStr(d, 19)),
		parseFloat(cellStr(d, 20)), parseFloat(cellStr(d, 21)), parseFloat(cellStr(d, 22)), parseFloat(cellStr(d, 23)),
		parseInt(cellStr(d, 24)), parseInt(cellStr(d, 25)), parseInt(cellStr(d, 26)), parseInt(cellStr(d, 27)),
		parseInt(cellStr(d, 28)), parseInt(cellStr(d, 29)), parseInt(cellStr(d, 30)),
		parseFloat(cellStr(d, 31)), parseFloat(cellStr(d, 32)), parseFloat(cellStr(d, 33)), parseFloat(cellStr(d, 34)),
	)
	return err
}
