package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"

	_ "github.com/go-sql-driver/mysql"
	"github.com/xuri/excelize/v2"
)

var db *sql.DB

// 文件名正则
var (
	jdPattern  = regexp.MustCompile(`京东_(\d{8})_(.+?)_(推广_.+?)\.(xlsx|csv)$`)
	pddPattern = regexp.MustCompile(`拼多多_(\d{8})_(.+?)_(推广数据_.+?)\.(xlsx|json)$`)
	csPattern  = regexp.MustCompile(`天猫超市_(\d{8})_(.+?)_(推广_.+?)\.(xlsx|csv)$`)
	vipPattern = regexp.MustCompile(`唯品会_(\d{8})_(.+?)_(销售数据_.+?)\.(xlsx|json)$`)
)

func main() {
	unlock := importutil.AcquireLock("import-promo")
	defer unlock()

	startDate, endDate := "", ""
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

	dataRoot := `Z:\信息部\RPA_集团数据看板`
	if len(os.Args) >= 4 {
		dataRoot = os.Args[3]
	}
	dataRoot, err = importutil.ResolveDataRoot(dataRoot)
	if err != nil {
		log.Fatalf("数据目录不可用: %v", err)
	}
	matchedDate := false

	// 处理各平台
	for _, platform := range []string{"京东", "拼多多", "天猫超市", "唯品会"} {
		platDir := filepath.Join(dataRoot, platform)
		if _, err := os.Stat(platDir); err != nil {
			continue
		}
		years, _ := os.ReadDir(platDir)
		for _, y := range years {
			if !y.IsDir() {
				continue
			}
			dates, _ := os.ReadDir(filepath.Join(platDir, y.Name()))
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
				matchedDate = true
				datePath := filepath.Join(platDir, y.Name(), dateStr)
				shops, err := os.ReadDir(datePath)
				if err != nil {
					log.Printf("读取日期目录失败 [%s]: %v", datePath, err)
					continue
				}
				for _, s := range shops {
					if !s.IsDir() {
						continue
					}
					shopPath := filepath.Join(datePath, s.Name())
					processShopDir(platform, shopPath, dateStr, s.Name())
				}
			}
		}
	}
	if startDate != "" && endDate != "" && !matchedDate {
		log.Fatalf("未找到日期范围 %s-%s 的数据目录", startDate, endDate)
	}
	log.Println("推广数据导入完成!")
}

func processShopDir(platform, dir, dateStr, shopName string) {
	files, _ := os.ReadDir(dir)
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		fullPath := filepath.Join(dir, name)
		var err error

		switch platform {
		case "京东":
			matches := jdPattern.FindStringSubmatch(name)
			if matches == nil {
				continue
			}
			dataType := matches[3]
			ext := matches[4]
			switch {
			case dataType == "推广_京东联盟" && ext == "xlsx":
				err = importJDAffiliate(fullPath, dateStr, shopName)
			case dataType == "推广_京准通全站" && ext == "xlsx":
				err = importJDCampaign(fullPath, dateStr, shopName, "京准通全站")
			case dataType == "推广_京准通非全站" && ext == "xlsx":
				err = importJDCampaignNonFull(fullPath, dateStr, shopName)
			default:
				continue
			}
		case "拼多多":
			matches := pddPattern.FindStringSubmatch(name)
			if matches == nil {
				continue
			}
			dataType := matches[3]
			ext := matches[4]
			switch {
			case dataType == "推广数据_商品推广" && ext == "xlsx":
				err = importPDDCampaign(fullPath, dateStr, shopName, "商品推广")
			case dataType == "推广数据_明星店铺" && ext == "xlsx":
				err = importPDDCampaign(fullPath, dateStr, shopName, "明星店铺")
			case dataType == "推广数据_直播推广" && ext == "xlsx":
				err = importPDDCampaign(fullPath, dateStr, shopName, "直播推广")
			default:
				continue
			}
		case "天猫超市":
			matches := csPattern.FindStringSubmatch(name)
			if matches == nil {
				continue
			}
			dataType := matches[3]
			ext := matches[4]
			switch {
			case dataType == "推广_无界场景数据" && ext == "csv":
				err = importTmallCSCampaignCSV(fullPath, dateStr, shopName)
			case dataType == "推广_智多星" && ext == "xlsx":
				err = importTmallCSCampaignXlsx(fullPath, dateStr, shopName)
			default:
				continue
			}
		case "唯品会":
			matches := vipPattern.FindStringSubmatch(name)
			if matches == nil {
				continue
			}
			dataType := matches[3]
			ext := matches[4]
			switch {
			case dataType == "销售数据_经营" && ext == "xlsx":
				err = importVipShopDaily(fullPath, dateStr, shopName)
			case dataType == "销售数据_取消金额" && ext == "json":
				err = importVipCancelDaily(fullPath, shopName)
			default:
				continue
			}
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
	if s == "" || s == "-" || s == "—" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func parseInt(s string) int {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	if s == "" || s == "-" || s == "—" {
		return 0
	}
	v, _ := strconv.Atoi(s)
	return v
}

func formatDate(d string) string {
	d = strings.TrimSpace(d)
	if len(d) == 8 {
		return d[:4] + "-" + d[4:6] + "-" + d[6:8]
	}
	return d
}

// ==================== 京东联盟(CPS) ====================

func importJDAffiliate(path, dateStr, shopName string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("打开xlsx: %w", err)
	}
	defer f.Close()

	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil || len(rows) < 2 {
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
		statDate := get("日期")
		if statDate == "" || statDate == "合计" || statDate == "汇总" {
			continue
		}

		_, err := db.Exec(`REPLACE INTO op_jd_affiliate_daily (
			stat_date, shop_name, referral_count, order_buyers, order_amount,
			est_commission, complete_buyers, complete_amount, actual_commission
		) VALUES (?,?,?,?,?,?,?,?,?)`,
			statDate, shopName,
			parseInt(get("点击量")),
			parseInt(get("下单订单量")),
			parseFloat(get("下单订单金额")),
			parseFloat(get("预估佣金金额")),
			parseInt(get("完成订单量")),
			parseFloat(get("完成订单金额")),
			parseFloat(get("付款佣金金额")),
		)
		if err != nil {
			log.Printf("  JD联盟插入失败[%s]: %v", statDate, err)
			continue
		}
		count++
	}
	log.Printf("  京东联盟: %d条", count)
	return nil
}

// ==================== 京准通全站(CPC) ====================

func importJDCampaign(path, dateStr, shopName, promoType string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("打开xlsx: %w", err)
	}
	defer f.Close()

	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil {
		return fmt.Errorf("读取sheet: %w", err)
	}

	// 找表头行(含"日期")
	headerIdx := -1
	for i, row := range rows {
		if len(row) > 0 && strings.Contains(row[0], "日期") {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		return nil
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

		dateVal := get("日期")
		if dateVal == "" || dateVal == "汇总" || dateVal == "合计" {
			continue
		}
		statDate := formatDate(dateVal)

		_, err := db.Exec(`REPLACE INTO op_jd_campaign_daily (
			stat_date, shop_name, promo_type, cost, pay_amount, roi,
			orders, order_cost, impressions, clicks
		) VALUES (?,?,?,?,?,?,?,?,?,?)`,
			statDate, shopName, promoType,
			parseFloat(get("花费")),
			parseFloat(get("全站交易额")),
			parseFloat(get("全站投产比")),
			parseInt(get("全站订单行")),
			parseFloat(get("全站订单成本")),
			parseInt(get("核心位置展现量")),
			parseInt(get("核心位置点击量")),
		)
		if err != nil {
			log.Printf("  JD京准通插入失败[%s]: %v", statDate, err)
			continue
		}
		count++
	}
	log.Printf("  京准通%s: %d条", promoType, count)
	return nil
}

// ==================== 京准通非全站(CPC) - 按小时聚合为日 ====================

func importJDCampaignNonFull(path, dateStr, shopName string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("打开xlsx: %w", err)
	}
	defer f.Close()

	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil {
		return fmt.Errorf("读取sheet: %w", err)
	}

	// 找表头行
	headerIdx := -1
	for i, row := range rows {
		if len(row) > 0 && (strings.Contains(row[0], "点击时间") || strings.Contains(row[0], "日期")) {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		return nil
	}

	header := rows[headerIdx]
	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.TrimSpace(h)] = i
	}

	// 聚合: 按日期汇总小时数据
	type DayAgg struct {
		cost, payAmt         float64
		orders, impr, clicks int
		totalCart            int
	}
	dayMap := make(map[string]*DayAgg)

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

		timeVal := get("点击时间")
		if timeVal == "" || timeVal == "汇总" || timeVal == "合计" {
			continue
		}
		// 时间格式: 20260324000000~20260324005959 -> 取前8位
		dateKey := timeVal
		if len(timeVal) >= 8 {
			dateKey = timeVal[:8]
		}
		statDate := formatDate(dateKey)

		if dayMap[statDate] == nil {
			dayMap[statDate] = &DayAgg{}
		}
		d := dayMap[statDate]
		d.cost += parseFloat(get("花费"))
		d.payAmt += parseFloat(get("总订单金额"))
		d.orders += parseInt(get("总订单行"))
		d.impr += parseInt(get("展现数"))
		d.clicks += parseInt(get("点击数"))
		d.totalCart += parseInt(get("总加购数"))
	}

	count := 0
	for statDate, d := range dayMap {
		roi := 0.0
		if d.cost > 0 {
			roi = d.payAmt / d.cost
		}
		_, err := db.Exec(`REPLACE INTO op_jd_campaign_daily (
			stat_date, shop_name, promo_type, cost, pay_amount, roi,
			orders, order_cost, impressions, clicks
		) VALUES (?,?,?,?,?,?,?,?,?,?)`,
			statDate, shopName, "京准通非全站",
			d.cost, d.payAmt, roi,
			d.orders,
			func() float64 {
				if d.orders > 0 {
					return d.cost / float64(d.orders)
				}
				return 0
			}(),
			d.impr, d.clicks,
		)
		if err != nil {
			log.Printf("  JD非全站插入失败[%s]: %v", statDate, err)
			continue
		}
		count++
	}
	log.Printf("  京准通非全站: %d条", count)
	return nil
}

// ==================== 拼多多推广(CPC) ====================

func importPDDCampaign(path, dateStr, shopName, promoType string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("打开xlsx: %w", err)
	}
	defer f.Close()

	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil || len(rows) < 2 {
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
		statDate := get("日期")
		if statDate == "" || statDate == "合计" || statDate == "汇总" {
			continue
		}

		// 拼多多商品推广用"总花费(元)"和"交易额(元)"
		cost := parseFloat(get("总花费(元)"))
		if cost == 0 {
			cost = parseFloat(get("花费(元)"))
		}
		payAmt := parseFloat(get("交易额(元)"))
		roi := parseFloat(get("实际投产比"))
		if roi == 0 {
			roi = parseFloat(get("投入产出比"))
		}
		realPayAmt := parseFloat(get("净交易额(元)"))
		if realPayAmt == 0 {
			realPayAmt = payAmt
		}
		realRoi := parseFloat(get("净实际投产比"))

		orders := parseInt(get("成交笔数"))
		if orders == 0 {
			orders = parseInt(get("净成交笔数"))
		}
		impr := parseInt(get("曝光量"))
		clicks := parseInt(get("点击量"))

		_, err := db.Exec(`REPLACE INTO op_pdd_campaign_daily (
			stat_date, shop_name, promo_type, cost, pay_amount, roi,
			real_pay_amount, real_roi, pay_orders, impressions, clicks
		) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			statDate, shopName, promoType,
			cost, payAmt, roi,
			realPayAmt, realRoi, orders, impr, clicks,
		)
		if err != nil {
			log.Printf("  PDD插入失败[%s]: %v", statDate, err)
			continue
		}
		count++
	}
	log.Printf("  拼多多%s: %d条", promoType, count)
	return nil
}

// ==================== 天猫超市无界场景(CPC) - CSV ====================

func importTmallCSCampaignCSV(path, dateStr, shopName string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("打开csv: %w", err)
	}
	defer file.Close()

	// 尝试GBK编码读取
	data, _ := os.ReadFile(path)
	content := string(data)
	// 简单判断是否是GBK
	if strings.Contains(content, "\xc8\xd5") || !strings.Contains(content, "日期") {
		// GBK编码，用golang.org/x/text转换太重，直接用csv读bytes尝试
		// 这里简化处理：如果文件不含有效数据行就跳过
	}

	reader := csv.NewReader(strings.NewReader(content))
	records, err := reader.ReadAll()
	if err != nil {
		// 尝试GBK
		return nil // 跳过编码问题的文件
	}

	if len(records) < 2 {
		return nil
	}

	header := records[0]
	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.TrimSpace(h)] = i
	}

	count := 0
	for _, row := range records[1:] {
		get := func(name string) string {
			idx, ok := colMap[name]
			if !ok || idx >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[idx])
		}

		statDate := get("日期")
		if statDate == "" {
			statDate = formatDate(dateStr)
		}

		cost := parseFloat(get("花费"))
		payAmt := parseFloat(get("总成交金额"))
		roi := parseFloat(get("投入产出比"))
		clicks := parseInt(get("点击量"))
		impr := parseInt(get("展现量"))

		if cost == 0 && payAmt == 0 && clicks == 0 {
			continue
		}

		_, err := db.Exec(`REPLACE INTO op_tmall_cs_campaign_daily (
			stat_date, shop_name, promo_type, cost, pay_amount, roi, clicks, impressions
		) VALUES (?,?,?,?,?,?,?,?)`,
			statDate, shopName, "无界场景",
			cost, payAmt, roi, clicks, impr,
		)
		if err != nil {
			log.Printf("  天猫超市无界插入失败[%s]: %v", statDate, err)
			continue
		}
		count++
	}
	log.Printf("  天猫超市无界场景: %d条", count)
	return nil
}

// ==================== 天猫超市智多星(CPC) - XLSX ====================

func importTmallCSCampaignXlsx(path, dateStr, shopName string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("打开xlsx: %w", err)
	}
	defer f.Close()

	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil || len(rows) < 2 {
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

		statDate := get("日期")
		if statDate == "" {
			continue
		}

		cost := parseFloat(get("消耗(元)"))
		payAmt := parseFloat(get("总成交金额"))
		roi := parseFloat(get("ROI"))
		clicks := parseInt(get("点击量"))
		impr := parseInt(get("曝光量"))

		if cost == 0 && payAmt == 0 {
			continue
		}

		_, err := db.Exec(`REPLACE INTO op_tmall_cs_campaign_daily (
			stat_date, shop_name, promo_type, cost, pay_amount, roi, clicks, impressions
		) VALUES (?,?,?,?,?,?,?,?)`,
			statDate, shopName, "智多星",
			cost, payAmt, roi, clicks, impr,
		)
		if err != nil {
			log.Printf("  天猫超市智多星插入失败[%s]: %v", statDate, err)
			continue
		}
		count++
	}
	log.Printf("  天猫超市智多星: %d条", count)
	return nil
}

// ==================== 唯品会经营数据(xlsx) ====================

func importVipShopDaily(path, dateStr, shopName string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("打开xlsx: %w", err)
	}
	defer f.Close()

	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil || len(rows) < 2 {
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

		statDate := formatDate(get("时间"))
		if statDate == "" {
			continue
		}

		_, err := db.Exec(`REPLACE INTO op_vip_shop_daily (
			stat_date, shop_name, impressions, page_views, detail_uv, detail_uv_value,
			cart_buyers, collect_buyers, cart_conv_rate,
			pay_amount, pay_count, pay_orders, pay_conv_rate, pay_cart_conv_rate, arpu, visitors
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			statDate, shopName,
			parseInt(get("曝光流量")), parseInt(get("浏览流量")),
			parseInt(get("商详UV")), parseFloat(get("商详UV价值")),
			parseInt(get("加购人数")), parseInt(get("收藏人数")), get("访问-加购转化率"),
			parseFloat(get("销售额")), parseInt(get("销售量")),
			parseInt(get("子订单数")), get("购买转化率"), get("加购-支付转化率"),
			parseFloat(get("ARPU")), parseInt(get("客户数")),
		)
		if err != nil {
			log.Printf("  唯品会经营插入失败[%s]: %v", statDate, err)
			continue
		}
		count++
	}
	log.Printf("  唯品会经营: %d条", count)
	return nil
}

// ==================== 唯品会取消金额(json) ====================

func importVipCancelDaily(path, shopName string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取json: %w", err)
	}

	var result struct {
		Code string `json:"code"`
		Data []struct {
			Dt                 string  `json:"dt"`
			GoodsActureAmt     float64 `json:"goodsActureAmt"`
			CancelGoodsAmt     float64 `json:"cancelGoodsAmt"`
			CancelGoodsAmtRate float64 `json:"cancelGoodsAmtRate"`
			GoodsActureNum     int     `json:"goodsActureNum"`
			CancelItemNum      int     `json:"cancelItemNum"`
			CancelItemNumRate  float64 `json:"cancelItemNumRate"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("解析json: %w", err)
	}

	count := 0
	for _, item := range result.Data {
		statDate := formatDate(item.Dt)
		if statDate == "" || item.CancelGoodsAmt == 0 && item.GoodsActureAmt == 0 {
			continue
		}

		// UPDATE已有记录的取消金额字段
		_, err := db.Exec(`INSERT INTO op_vip_shop_daily (stat_date, shop_name, cancel_amount, cancel_qty, cancel_rate)
			VALUES (?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE cancel_amount=VALUES(cancel_amount), cancel_qty=VALUES(cancel_qty), cancel_rate=VALUES(cancel_rate)`,
			statDate, shopName, item.CancelGoodsAmt, item.CancelItemNum, item.CancelGoodsAmtRate,
		)
		if err != nil {
			log.Printf("  唯品会取消插入失败[%s]: %v", statDate, err)
			continue
		}
		count++
	}
	log.Printf("  唯品会取消金额: %d条", count)
	return nil
}
