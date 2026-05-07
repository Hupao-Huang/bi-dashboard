// import-channel-price.exe: 从 价格体系.xlsx 导入特殊渠道价格表
// 用法: import-channel-price.exe [--xlsx=path] [--config=path]
// 默认读 桌面/价格体系.xlsx 的 3 个 sheet (京东/猫超/朴朴) → channel_special_price 表
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
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

func main() {
	xlsxPath := flag.String("xlsx", `C:\Users\Administrator\Desktop\价格体系.xlsx`, "价格体系文件路径")
	configPath := flag.String("config", `C:\Users\Administrator\bi-dashboard\server\config.json`, "配置")
	flag.Parse()

	db, err := connectDB(*configPath)
	if err != nil {
		log.Fatal("连数据库失败:", err)
	}
	defer db.Close()

	f, err := excelize.OpenFile(*xlsxPath)
	if err != nil {
		log.Fatal("打开 xlsx 失败:", err)
	}
	defer f.Close()

	allowedSheets := map[string]bool{"京东": true, "猫超": true, "朴朴": true}
	totalImported := 0
	for _, sheetName := range f.GetSheetList() {
		if !allowedSheets[sheetName] {
			continue
		}
		rows, _ := f.GetRows(sheetName)
		if len(rows) < 2 {
			fmt.Printf("⚠️  sheet %s 无数据\n", sheetName)
			continue
		}
		header := rows[0]
		idxBC, idxGN, idxNM, idxPR := -1, -1, -1, -1
		for i, h := range header {
			h = strings.TrimSpace(h)
			switch h {
			case "erp条码", "条码":
				idxBC = i
			case "编码", "商品编码":
				idxGN = i
			case "名称", "商品名称":
				idxNM = i
			case "采购价", "单价":
				idxPR = i
			}
		}
		if idxGN < 0 {
			fmt.Printf("⚠️  sheet %s 找不到编码列, 跳过\n", sheetName)
			continue
		}

		// 先清空当前 channel 的旧数据再插
		if _, err := db.Exec("DELETE FROM channel_special_price WHERE channel_key=?", sheetName); err != nil {
			log.Fatalf("清旧数据 %s 失败: %v", sheetName, err)
		}

		count := 0
		for ri, r := range rows {
			if ri == 0 {
				continue
			}
			goodsNo := safeCell(r, idxGN)
			barcode := safeCell(r, idxBC)
			name := safeCell(r, idxNM)
			priceStr := safeCell(r, idxPR)
			if goodsNo == "" && barcode == "" {
				continue
			}
			price, _ := strconv.ParseFloat(priceStr, 64)

			_, err := db.Exec(`INSERT INTO channel_special_price
				(channel_key, goods_no, barcode, goods_name, price, source_xlsx)
				VALUES (?, ?, ?, ?, ?, ?)
				ON DUPLICATE KEY UPDATE
				barcode=VALUES(barcode), goods_name=VALUES(goods_name),
				price=VALUES(price), source_xlsx=VALUES(source_xlsx)`,
				sheetName, goodsNo, barcode, name, price, *xlsxPath)
			if err != nil {
				log.Printf("⚠️  插入失败 %s/%s: %v", sheetName, goodsNo, err)
				continue
			}
			count++
		}
		fmt.Printf("✅ sheet %s 导入 %d 条\n", sheetName, count)
		totalImported += count
	}

	fmt.Printf("\n🎯 共导入 %d 条价格记录\n", totalImported)
}

func safeCell(r []string, idx int) string {
	if idx < 0 || idx >= len(r) {
		return ""
	}
	return strings.TrimSpace(r[idx])
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
