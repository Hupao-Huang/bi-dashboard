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

// parseExcelDate 兼容 Excel 日期列各种格式
func parseExcelDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
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

func main() {
	cfg, _ := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	db, _ := sql.Open("mysql", cfg.Database.DSN())
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
	if len(d) < 5 {
		return 0, nil
	}
	// stat_date 取 Excel 第 0 列（业务日），文件名日期只是 RPA 采集日
	statDate := parseExcelDate(d[0])
	if statDate == "" {
		statDate = date
	}
	_, err = db.Exec(`INSERT INTO op_vip_shop_daily
		(stat_date, shop_name, pay_amount, pay_count, pay_orders, visitors)
		VALUES (?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE pay_amount=VALUES(pay_amount)`,
		statDate, shop, toF(d, 1), toI(d, 2), toI(d, 3), toI(d, 4))
	if err != nil {
		return 0, err
	}
	return 1, nil
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
