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

var baseDir = `Z:\信息部\RPA_集团数据看板\唯品会`

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

	startDate, endDate := "", ""
	if len(os.Args) >= 3 {
		startDate, endDate = os.Args[1], os.Args[2]
	}
	resolvedBaseDir, err := importutil.ResolveDataRoot(baseDir)
	if err != nil {
		log.Fatalf("数据目录不可用: %v", err)
	}

	total := 0
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
					switch {
					case strings.Contains(name, "销售数据_经营") && strings.HasSuffix(name, ".xlsx"):
						cnt, err := importShopDaily(db, fpath, sqlDate, shopName)
						if err != nil {
							log.Printf("[%s] %s 失败: %v", dateStr, name, err)
						}
						total += cnt
					case strings.Contains(name, "销售数据_取消金额") && strings.HasSuffix(name, ".json"):
						importCancelAmount(db, fpath, shopName)
					case strings.Contains(name, "推广_TargetMax") && strings.HasSuffix(name, ".json"):
						importTargetMax(db, fpath, sqlDate, shopName)
					case strings.Contains(name, "推广_唯享客") && strings.HasSuffix(name, ".json"):
						importWeixiangke(db, fpath, shopName)
					}
				}
			}
			fmt.Printf("[%s] 完成\n", dateStr)
		}
	}
	if startDate != "" && endDate != "" && !matchedDate {
		log.Fatalf("未找到日期范围 %s-%s 的数据目录", startDate, endDate)
	}
	fmt.Printf("\n导入完成: %d 条\n", total)
}

// importShopDaily 导入唯品会店铺销售经营数据。按表头名映射，避免原版硬编码索引把
// "曝光流量/浏览流量/商详UV/商详UV价值"错存成"销售额/销售量/子订单数/客户数"。
// Excel 真实列：时间/曝光流量/浏览流量/商详UV/商详UV价值/加购人数/收藏人数/兴趣人数/
//   兴趣人次/访问-加购转化率/销售额/销售量/客户数/子订单数/ARPU/超V销售额/超V客户数/
//   加购-支付转化率/购买转化率/接待数/客服转化数/客服转化金额/客服转化率/...
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
	header := rows[0]
	d := rows[1]
	idx := headerIdx(header)

	// 核心字段缺失直接报错跳过（Excel 格式彻底换了时保护数据）
	colSales := findCol(idx, "销售额")
	if colSales < 0 {
		return 0, fmt.Errorf("表头格式未识别（找不到'销售额'列）: %v", header)
	}

	// stat_date 取 Excel 第一列业务日期（文件名日期只是 RPA 采集日）
	colDate := findCol(idx, "时间", "日期", "统计日期")
	statDate := ""
	if colDate >= 0 && colDate < len(d) {
		statDate = parseExcelDate(d[colDate])
	}
	if statDate == "" {
		statDate = date
	}

	_, err = db.Exec(`INSERT INTO op_vip_shop_daily
		(stat_date, shop_name, impressions, page_views, detail_uv, detail_uv_value,
		 cart_buyers, collect_buyers, cart_conv_rate,
		 pay_amount, pay_count, pay_orders, pay_conv_rate, pay_cart_conv_rate, arpu, visitors)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
		 impressions=VALUES(impressions), page_views=VALUES(page_views),
		 detail_uv=VALUES(detail_uv), detail_uv_value=VALUES(detail_uv_value),
		 cart_buyers=VALUES(cart_buyers), collect_buyers=VALUES(collect_buyers),
		 cart_conv_rate=VALUES(cart_conv_rate),
		 pay_amount=VALUES(pay_amount), pay_count=VALUES(pay_count), pay_orders=VALUES(pay_orders),
		 pay_conv_rate=VALUES(pay_conv_rate), pay_cart_conv_rate=VALUES(pay_cart_conv_rate),
		 arpu=VALUES(arpu), visitors=VALUES(visitors)`,
		statDate, shop,
		getInt(d, findCol(idx, "曝光流量")),
		getInt(d, findCol(idx, "浏览流量")),
		getInt(d, findCol(idx, "商详UV")),
		getFloat(d, findCol(idx, "商详UV价值")),
		getInt(d, findCol(idx, "加购人数")),
		getInt(d, findCol(idx, "收藏人数")),
		getStr(d, findCol(idx, "访问-加购转化率")),
		getFloat(d, colSales),
		getInt(d, findCol(idx, "销售量")),
		getInt(d, findCol(idx, "子订单数")),
		getStr(d, findCol(idx, "购买转化率")),
		getStr(d, findCol(idx, "加购-支付转化率")),
		getFloat(d, findCol(idx, "ARPU")),
		getInt(d, findCol(idx, "客户数")),
	)
	if err != nil {
		return 0, err
	}
	return 1, nil
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

// importCancelAmount 取消金额（按dt多天数据）
func importCancelAmount(db *sql.DB, fpath, shop string) {
	data, err := os.ReadFile(fpath)
	if err != nil {
		return
	}
	var resp struct {
		Code    string `json:"code"`
		Data    []struct {
			Dt                  string  `json:"dt"`
			GoodsActureAmt      float64 `json:"goodsActureAmt"`
			CancelGoodsAmt      float64 `json:"cancelGoodsAmt"`
			CancelGoodsAmtRate  float64 `json:"cancelGoodsAmtRate"`
			GoodsActureNum      int     `json:"goodsActureNum"`
			CancelItemNum       int     `json:"cancelItemNum"`
			CancelItemNumRate   float64 `json:"cancelItemNumRate"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		log.Printf("解析取消金额失败 %s: %v", fpath, err)
		return
	}
	for _, item := range resp.Data {
		if len(item.Dt) != 8 {
			continue
		}
		statDate := item.Dt[:4] + "-" + item.Dt[4:6] + "-" + item.Dt[6:8]
		db.Exec(`REPLACE INTO op_vip_cancel
			(stat_date, shop_name, goods_acture_amt, cancel_goods_amt, cancel_goods_amt_rate,
			 goods_acture_num, cancel_item_num, cancel_item_num_rate)
			VALUES (?,?,?,?,?,?,?,?)`,
			statDate, shop, item.GoodsActureAmt, item.CancelGoodsAmt, item.CancelGoodsAmtRate,
			item.GoodsActureNum, item.CancelItemNum, item.CancelItemNumRate,
		)
	}
}

// importTargetMax 唯品会-TargetMax推广
func importTargetMax(db *sql.DB, fpath, date, shop string) {
	data, err := os.ReadFile(fpath)
	if err != nil {
		return
	}
	var resp struct {
		Data struct {
			Reports []struct {
				Statistics map[string]interface{} `json:"statistics"`
			} `json:"reports"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return
	}
	if len(resp.Data.Reports) == 0 {
		return
	}
	s := resp.Data.Reports[0].Statistics
	getF := func(k string) float64 {
		v, ok := s[k]
		if !ok || v == nil {
			return 0
		}
		f, _ := v.(float64)
		return f
	}
	getI := func(k string) int { return int(getF(k)) }
	rawJson, _ := json.Marshal(s)
	db.Exec(`REPLACE INTO op_vip_targetmax
		(stat_date, shop_name, impression_count, click_count, cost, click_rate, cost_per_click, cost_per_mille,
		 uv, new_uv, old_uv, buy_uv, sales_amount, goods_actureamt, roi,
		 customer, new_customer, old_customer, order_cnt, buyer_cnt, raw_json)
		VALUES (?,?,?,?,?,?,?,?, ?,?,?,?,?,?,?, ?,?,?,?,?,?)`,
		date, shop, getI("impressionCount"), getI("clickCount"), getF("cost"),
		getF("clickRate"), getF("costPerClick"), getF("costPerMille"),
		getI("uv"), getI("newUv"), getI("oldUv"), getI("buyUv"),
		getF("salesAmount"), getF("goodsActureamt"), getF("roi"),
		getI("customer"), getI("newCustomer"), getI("oldCustomer"),
		getI("orderCnt"), getI("buyerCnt"), string(rawJson),
	)
}

// importWeixiangke 唯品会-唯享客推广（含多天数据）
func importWeixiangke(db *sql.DB, fpath, shop string) {
	data, err := os.ReadFile(fpath)
	if err != nil {
		return
	}
	var resp struct {
		Data struct {
			GoodsStatisticsList []struct {
				DataTime                  string `json:"dataTime"`
				GoodsStatisticsTimeDetail struct {
					AddUserCount         string `json:"addUserCount"`
					BrandNewUserCount    string `json:"brandNewUserCount"`
					BrandRepurchaseCount string `json:"brandRepurchaseCount"`
					BringUserCount       string `json:"bringUserCount"`
					ConversionRate       string `json:"conversionRate"`
					OrderCount           string `json:"orderCount"`
					OrderUserCount       string `json:"orderUserCount"`
					PromotionAmount      string `json:"promotionAmount"`
					Roi                  string `json:"roi"`
					SalesAmount          string `json:"salesAmount"`
					SalesAmountMerchant  string `json:"salesAmountMerchant"`
					ServeAmount          string `json:"serveAmount"`
				} `json:"goodsStatisticsTimeDetail"`
			} `json:"goodsStatisticsList"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return
	}
	for _, item := range resp.Data.GoodsStatisticsList {
		if item.DataTime == "" {
			continue
		}
		d := item.GoodsStatisticsTimeDetail
		db.Exec(`REPLACE INTO op_vip_weixiangke
			(stat_date, shop_name, add_user_count, brand_new_user_count, brand_repurchase_count, bring_user_count,
			 conversion_rate, order_count, order_user_count, promotion_amount, roi, sales_amount,
			 sales_amount_merchant, serve_amount)
			VALUES (?,?,?,?,?,?, ?,?,?,?,?,?, ?,?)`,
			item.DataTime, shop,
			parseIntStr(d.AddUserCount), parseIntStr(d.BrandNewUserCount), parseIntStr(d.BrandRepurchaseCount), parseIntStr(d.BringUserCount),
			parseFloatStr(d.ConversionRate), parseIntStr(d.OrderCount), parseIntStr(d.OrderUserCount),
			parseFloatStr(d.PromotionAmount), parseFloatStr(d.Roi), parseFloatStr(d.SalesAmount),
			parseFloatStr(d.SalesAmountMerchant), parseFloatStr(d.ServeAmount),
		)
	}
}

func parseFloatStr(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
func parseIntStr(s string) int { return int(parseFloatStr(s)) }

func toF(d []string, i int) float64 {
	if i >= len(d) || d[i] == "" {
		return 0
	}
	s := strings.ReplaceAll(strings.ReplaceAll(d[i], ",", ""), "%", "")
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
func toI(d []string, i int) int { return int(toF(d, i)) }
