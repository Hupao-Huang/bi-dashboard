package main

// 小红书灵犀(品牌搜索洞察)数据导入器。数据是 JSON(非 csv/xlsx)。
// 目录: Z:\信息部\RPA_集团数据看板\小红书灵犀\<年>\<YYYYMMDD>\<店铺>\
//   小红书灵犀_<日期>_<店铺>_品牌_搜索趋势数据_<减钠|减盐|松鲜鲜>.json → op_lingxi_search_trend
//   小红书灵犀_<日期>_<店铺>_品牌_搜索上下游数据_<减钠|减盐|松鲜鲜>.json → op_lingxi_search_updown
//   小红书灵犀_<日期>_<店铺>_品牌_搜索趋势榜单.json                      → op_lingxi_search_rank
// JSON 结构: { data: { tableResult: [ {字段...全是字符串值} ] } }，字段名见各 headers.fieldEn。
// 口径：趋势数据 stat_date 取行内 dtm(7天窗口); 上下游/榜单 取目录日(快照)。brand_word 取文件名后缀。
// 用法: import-lingxi.exe [startDate endDate]  日期 YYYYMMDD(目录日)，不传则全量。

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

var baseDir = `Z:\信息部\RPA_集团数据看板\小红书灵犀`

type lxEnvelope struct {
	Data struct {
		TableResult []map[string]interface{} `json:"tableResult"`
	} `json:"data"`
}

func main() {
	unlock := importutil.AcquireLock("import-lingxi")
	defer unlock()

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
		log.Fatalf("建表失败: %v", err)
	}

	startDate, endDate := "", ""
	if len(os.Args) >= 3 {
		startDate, endDate = os.Args[1], os.Args[2]
	}
	root, err := importutil.ResolveDataRoot(baseDir)
	if err != nil {
		log.Fatalf("数据目录不可用: %v", err)
	}

	trend, updown, rank := 0, 0, 0
	yearEntries, _ := os.ReadDir(root)
	for _, ye := range yearEntries {
		if !ye.IsDir() || len(ye.Name()) != 4 {
			continue
		}
		yearPath := filepath.Join(root, ye.Name())
		dateDirs, _ := os.ReadDir(yearPath)
		for _, dd := range dateDirs {
			if !dd.IsDir() || len(dd.Name()) != 8 {
				continue
			}
			dirDate := dd.Name()
			if (startDate != "" && dirDate < startDate) || (endDate != "" && dirDate > endDate) {
				continue
			}
			statDate := dirDate[:4] + "-" + dirDate[4:6] + "-" + dirDate[6:8]
			datePath := filepath.Join(yearPath, dirDate)
			shopDirs, _ := os.ReadDir(datePath)
			for _, sd := range shopDirs {
				if !sd.IsDir() {
					continue
				}
				shopName := sd.Name()
				shopPath := filepath.Join(datePath, shopName)
				files, _ := os.ReadDir(shopPath)
				for _, fl := range files {
					name := fl.Name()
					if fl.IsDir() || strings.HasPrefix(name, "~$") || !strings.HasSuffix(name, ".json") {
						continue
					}
					fpath := filepath.Join(shopPath, name)
					switch {
					case strings.Contains(name, "_搜索趋势数据_"):
						trend += importTrend(db, fpath, statDate, shopName, brandWord(name, "_搜索趋势数据_"))
					case strings.Contains(name, "_搜索上下游数据_"):
						updown += importUpdown(db, fpath, statDate, shopName, brandWord(name, "_搜索上下游数据_"))
					case strings.Contains(name, "_搜索趋势榜单"):
						rank += importRank(db, fpath, statDate, shopName)
					}
				}
			}
		}
	}
	fmt.Printf("\n小红书灵犀导入完成:\n  搜索趋势: %d 条\n  搜索上下游: %d 条\n  搜索榜单: %d 条\n", trend, updown, rank)
}

// brandWord 从文件名取 marker 之后、.json 之前的品牌词(减钠/减盐/松鲜鲜)
func brandWord(name, marker string) string {
	i := strings.Index(name, marker)
	if i < 0 {
		return ""
	}
	return strings.TrimSuffix(name[i+len(marker):], ".json")
}

func parseRows(path string) []map[string]interface{} {
	b, err := os.ReadFile(path)
	if err != nil {
		log.Printf("读取失败 %s: %v", filepath.Base(path), err)
		return nil
	}
	var env lxEnvelope
	if err := json.Unmarshal(b, &env); err != nil {
		log.Printf("解析JSON失败 %s: %v", filepath.Base(path), err)
		return nil
	}
	return env.Data.TableResult
}

// toS 把任意 JSON 值转字符串(灵犀的值基本都是字符串，数字兜底)
func toS(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func ensureTables(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS op_lingxi_search_trend (
		id BIGINT PRIMARY KEY AUTO_INCREMENT,
		stat_date DATE NOT NULL COMMENT '趋势日期(行内dtm)',
		file_date DATE NOT NULL COMMENT '导出文件日期(目录日)',
		shop_name VARCHAR(128) NOT NULL COMMENT '店铺/品牌',
		brand_word VARCHAR(32) NOT NULL COMMENT '品牌词(减钠/减盐/松鲜鲜)',
		search_num BIGINT DEFAULT 0 COMMENT '搜索量',
		peak_flag TINYINT DEFAULT 0 COMMENT '是否波峰(1是)',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		UNIQUE KEY uk_lx_trend (stat_date, shop_name, brand_word),
		KEY idx_lx_trend_date (stat_date)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='小红书灵犀-品牌搜索趋势(时间序列,按品牌词)'`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS op_lingxi_search_updown (
		id BIGINT PRIMARY KEY AUTO_INCREMENT,
		stat_date DATE NOT NULL COMMENT '统计日期(目录日,快照)',
		shop_name VARCHAR(128) NOT NULL COMMENT '店铺/品牌',
		brand_word VARCHAR(32) NOT NULL COMMENT '品牌词(减钠/减盐/松鲜鲜)',
		session VARCHAR(8) NOT NULL COMMENT '上下游类型(0上游/1下游)',
		rank_no INT NOT NULL COMMENT '排名',
		search_word VARCHAR(255) DEFAULT '' COMMENT '搜索词',
		search_num BIGINT DEFAULT 0 COMMENT '搜索量',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		UNIQUE KEY uk_lx_updown (stat_date, shop_name, brand_word, session, rank_no),
		KEY idx_lx_updown_date (stat_date)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='小红书灵犀-搜索上下游词(按品牌词,上游/下游)'`); err != nil {
		return err
	}
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS op_lingxi_search_rank (
		id BIGINT PRIMARY KEY AUTO_INCREMENT,
		stat_date DATE NOT NULL COMMENT '统计日期(目录日,快照)',
		shop_name VARCHAR(128) NOT NULL COMMENT '店铺/品牌',
		trend_word VARCHAR(128) NOT NULL COMMENT '趋势词',
		trend_code VARCHAR(128) DEFAULT '' COMMENT '趋势code',
		trend_type VARCHAR(16) DEFAULT '' COMMENT '趋势类型',
		rank_no INT DEFAULT 0 COMMENT '排名',
		index_num BIGINT DEFAULT 0 COMMENT '指数',
		rank_trans_num INT DEFAULT 0 COMMENT '排名变化',
		new_word_flag TINYINT DEFAULT 0 COMMENT '是否新词(1是)',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		UNIQUE KEY uk_lx_rank (stat_date, shop_name, trend_word),
		KEY idx_lx_rank_date (stat_date)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='小红书灵犀-搜索趋势榜单(每日快照)'`)
	return err
}

func importTrend(db *sql.DB, path, fileDate, shopName, brand string) int {
	count := 0
	for _, row := range parseRows(path) {
		dtm := toS(row["dtm"])
		if len(dtm) != 10 {
			continue
		}
		peakFlag := 0
		if strings.Contains(toS(row["peak"]), "\"flag\":true") {
			peakFlag = 1
		}
		if _, err := db.Exec(`REPLACE INTO op_lingxi_search_trend
			(stat_date, file_date, shop_name, brand_word, search_num, peak_flag) VALUES (?,?,?,?,?,?)`,
			dtm, fileDate, shopName, brand, int64(importutil.ParseFloat(toS(row["searchNum"]))), peakFlag); err != nil {
			log.Printf("趋势写入失败 brand=%s dtm=%s: %v", brand, dtm, err)
			continue
		}
		count++
	}
	return count
}

func importUpdown(db *sql.DB, path, statDate, shopName, brand string) int {
	count := 0
	for _, row := range parseRows(path) {
		rankNo := int64(importutil.ParseFloat(toS(row["rank"])))
		if _, err := db.Exec(`REPLACE INTO op_lingxi_search_updown
			(stat_date, shop_name, brand_word, session, rank_no, search_word, search_num) VALUES (?,?,?,?,?,?,?)`,
			statDate, shopName, brand, toS(row["session"]), rankNo, toS(row["searchWord"]),
			int64(importutil.ParseFloat(toS(row["searchNum"])))); err != nil {
			log.Printf("上下游写入失败 brand=%s rank=%d: %v", brand, rankNo, err)
			continue
		}
		count++
	}
	return count
}

func importRank(db *sql.DB, path, statDate, shopName string) int {
	count := 0
	for _, row := range parseRows(path) {
		word := toS(row["trendWord"])
		if word == "" {
			continue
		}
		newFlag := 0
		if toS(row["newWordFlag"]) == "1" {
			newFlag = 1
		}
		if _, err := db.Exec(`REPLACE INTO op_lingxi_search_rank
			(stat_date, shop_name, trend_word, trend_code, trend_type, rank_no, index_num, rank_trans_num, new_word_flag)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			statDate, shopName, word, toS(row["trendCode"]), toS(row["trendType"]),
			int64(importutil.ParseFloat(toS(row["rank"]))), int64(importutil.ParseFloat(toS(row["indexNum"]))),
			int64(importutil.ParseFloat(toS(row["rankTransNum"]))), newFlag); err != nil {
			log.Printf("榜单写入失败 word=%s: %v", word, err)
			continue
		}
		count++
	}
	return count
}
