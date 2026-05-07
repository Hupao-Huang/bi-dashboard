// audit-allot-sku.exe: 一次性 audit
// 1) 读 价格体系.xlsx 全部 3 个 sheet 的 SKU/编码/条码
// 2) 查 trade_goods_YYYYMM × 3 个特殊渠道, 拿出现过的 SKU
// 3) 输出: Excel 缺了哪些 SKU (可能调拨单里会出现, 但 Excel 没维护价格)
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/xuri/excelize/v2"
)

type Config struct {
	Database struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		User     string `json:"user"`
		Password string `json:"password"`
		Dbname   string `json:"dbname"`
	} `json:"database"`
}

type ExcelSKU struct {
	Sheet    string  // 京东/猫超/朴朴
	GoodsNo  string  // 编码 (sku_code)
	Barcode  string  // erp条码
	Name     string  // 商品名
	Price    string  // 采购价/单价
}

func main() {
	xlsx := flag.String("xlsx", `C:\Users\Administrator\Desktop\价格体系.xlsx`, "价格体系文件路径")
	configPath := flag.String("config", `C:\Users\Administrator\bi-dashboard\server\config.json`, "数据库配置")
	months := flag.String("months", "202602,202603,202604,202605", "trade_goods 月份")
	flag.Parse()

	// 1) 读 Excel
	skus, err := readPriceXlsx(*xlsx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读 Excel 失败:", err)
		os.Exit(1)
	}
	fmt.Printf("📊 价格体系.xlsx 读到 %d 条 SKU\n", len(skus))
	bySheet := map[string][]ExcelSKU{}
	for _, s := range skus {
		bySheet[s.Sheet] = append(bySheet[s.Sheet], s)
	}
	for k, v := range bySheet {
		fmt.Printf("   - Sheet %s: %d 条\n", k, len(v))
	}

	// 2) 连数据库
	db, err := connectDB(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "连数据库失败:", err)
		os.Exit(1)
	}
	defer db.Close()

	// 3) 3 个特殊渠道映射 → 对应 Excel sheet
	channelToSheet := map[string]string{
		"ds-京东-清心湖自营":              "京东",
		"ds-天猫超市-寄售":               "猫超",
		"js-即时零售事业一部（世创）-朴朴": "朴朴",
	}

	monthList := strings.Split(*months, ",")

	for channel, sheet := range channelToSheet {
		fmt.Printf("\n========================================\n")
		fmt.Printf("🔍 渠道: %s  →  价格表 sheet: %s\n", channel, sheet)
		fmt.Printf("========================================\n")

		// 销售单出现过的 SKU
		dbSKUs, err := queryChannelSKUs(db, channel, monthList)
		if err != nil {
			fmt.Fprintln(os.Stderr, "查 SKU 失败:", err)
			continue
		}
		fmt.Printf("📦 销售单 %s 月份共 %d 个 SKU\n", *months, len(dbSKUs))

		// Excel 里这个 sheet 的 SKU 编码
		excelGoodsNo := map[string]ExcelSKU{}
		excelBarcode := map[string]ExcelSKU{}
		for _, s := range bySheet[sheet] {
			if s.GoodsNo != "" {
				excelGoodsNo[s.GoodsNo] = s
			}
			if s.Barcode != "" {
				excelBarcode[s.Barcode] = s
			}
		}

		// 对比: 销售单里有 但 Excel 没的
		var missing []dbSKU
		var matched int
		for _, ds := range dbSKUs {
			if _, ok := excelGoodsNo[ds.GoodsNo]; ok {
				matched++
				continue
			}
			if ds.Barcode != "" {
				if _, ok := excelBarcode[ds.Barcode]; ok {
					matched++
					continue
				}
			}
			missing = append(missing, ds)
		}

		fmt.Printf("✅ 已匹配: %d 个 SKU\n", matched)
		fmt.Printf("❌ 缺失(Excel 没有): %d 个 SKU\n", len(missing))
		if len(missing) > 0 {
			fmt.Println("   ↓ 缺失明细 (按销售单数量降序):")
			for i, m := range missing {
				if i >= 30 {
					fmt.Printf("   ... 还有 %d 个\n", len(missing)-30)
					break
				}
				fmt.Printf("   [%d] goods_no=%-15s  barcode=%-15s  cnt=%d  名称=%s\n",
					i+1, m.GoodsNo, m.Barcode, m.Count, m.Name)
			}
		}

		// 反向: Excel 有但销售单从来没用
		dbGoodsSet := map[string]bool{}
		for _, ds := range dbSKUs {
			if ds.GoodsNo != "" {
				dbGoodsSet[ds.GoodsNo] = true
			}
		}
		var unused []ExcelSKU
		for _, e := range bySheet[sheet] {
			if !dbGoodsSet[e.GoodsNo] {
				unused = append(unused, e)
			}
		}
		if len(unused) > 0 {
			fmt.Printf("⚠️  Excel 有但销售单没出现过: %d 个\n", len(unused))
			for i, u := range unused {
				if i >= 10 {
					fmt.Printf("   ... 还有 %d 个\n", len(unused)-10)
					break
				}
				fmt.Printf("   - %s  %s  价=%s\n", u.GoodsNo, u.Name, u.Price)
			}
		}
	}
}

type dbSKU struct {
	GoodsNo string
	Barcode string
	Name    string
	Count   int
}

func readPriceXlsx(path string) ([]ExcelSKU, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var skus []ExcelSKU
	for _, sheetName := range f.GetSheetList() {
		rows, err := f.GetRows(sheetName)
		if err != nil || len(rows) < 2 {
			continue
		}
		// 京东:    [SKU, erp条码, 编码, 商品名称, 箱规, 采购价]
		// 猫超:    [erp条码, 编码, 名称, 终点仓, 采购价]
		// 朴朴:    [商品编码, 商品名称, 单位, 单价, 条码, 规格]
		header := rows[0]
		idxBarcode, idxGoodsNo, idxName, idxPrice := -1, -1, -1, -1
		for i, h := range header {
			h = strings.TrimSpace(h)
			switch h {
			case "erp条码", "条码":
				idxBarcode = i
			case "编码", "商品编码":
				idxGoodsNo = i
			case "名称", "商品名称":
				idxName = i
			case "采购价", "单价":
				idxPrice = i
			}
		}
		for ri, r := range rows {
			if ri == 0 {
				continue
			}
			s := ExcelSKU{Sheet: sheetName}
			if idxBarcode >= 0 && idxBarcode < len(r) {
				s.Barcode = strings.TrimSpace(r[idxBarcode])
			}
			if idxGoodsNo >= 0 && idxGoodsNo < len(r) {
				s.GoodsNo = strings.TrimSpace(r[idxGoodsNo])
			}
			if idxName >= 0 && idxName < len(r) {
				s.Name = strings.TrimSpace(r[idxName])
			}
			if idxPrice >= 0 && idxPrice < len(r) {
				s.Price = strings.TrimSpace(r[idxPrice])
			}
			if s.GoodsNo == "" && s.Barcode == "" {
				continue // 空行
			}
			skus = append(skus, s)
		}
	}
	return skus, nil
}

func connectDB(configPath string) (*sql.DB, error) {
	bs, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(bs, &cfg); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		cfg.Database.User, cfg.Database.Password,
		cfg.Database.Host, cfg.Database.Port, cfg.Database.Dbname)
	return sql.Open("mysql", dsn)
}

func queryChannelSKUs(db *sql.DB, channel string, months []string) ([]dbSKU, error) {
	// UNION 多个月表
	var unions []string
	args := []interface{}{}
	for _, m := range months {
		unions = append(unions,
			fmt.Sprintf(`SELECT tg.goods_no, tg.barcode, tg.goods_name, COUNT(*) AS cnt
                         FROM trade_goods_%s tg
                         JOIN trade_%s t ON t.trade_id = tg.trade_id
                         WHERE t.shop_name = ?
                         GROUP BY tg.goods_no, tg.barcode, tg.goods_name`, m, m))
		args = append(args, channel)
	}
	q := fmt.Sprintf(`SELECT goods_no, barcode, goods_name, SUM(cnt) AS total
                      FROM (%s) u
                      GROUP BY goods_no, barcode, goods_name
                      ORDER BY total DESC`, strings.Join(unions, " UNION ALL "))
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []dbSKU
	for rows.Next() {
		var s dbSKU
		var bc, nm sql.NullString
		if err := rows.Scan(&s.GoodsNo, &bc, &nm, &s.Count); err != nil {
			return nil, err
		}
		s.Barcode = bc.String
		s.Name = nm.String
		out = append(out, s)
	}
	return out, nil
}
