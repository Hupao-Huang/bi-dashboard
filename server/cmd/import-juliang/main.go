package main

// 巨量云图(抖音/Ocean Engine 数据洞察)数据导入器。
// 目录: Z:\信息部\RPA_集团数据看板\巨量云图\<年>\<YYYYMMDD>\<店铺>\
//   巨量云图_<日期>_<店铺>_品牌_达人投放.csv          → op_juliang_talent_daily
//   巨量云图_<日期>_<店铺>_品牌_关键词趋势_本品.csv   → op_juliang_keyword_daily (source_type=本品)
//   巨量云图_<日期>_<店铺>_品牌_关键词趋势_自定义.csv → op_juliang_keyword_daily (source_type=自定义)
// 口径：
//   - stat_date 取目录日(快照，文件内无业务日期列)。
//   - 抖音号/视频ID 带"抖音号:"/"视频ID:"前缀，导入剥掉。
//   - 空值"-"转0；率类去"%"(达人率本身是小数无%，关键词率带%，统一按源数值存)。
//   - 用 encoding/csv 解析。
// 用法: import-juliang.exe [startDate endDate]  日期 YYYYMMDD(目录日)，不传则全量。

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"database/sql"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/xuri/excelize/v2"
)

var baseDir = `Z:\信息部\RPA_集团数据看板\巨量云图`

func main() {
	unlock := importutil.AcquireLock("import-juliang")
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

	talent, keyword := 0, 0
	yearEntries, err := os.ReadDir(root)
	if err != nil {
		log.Fatalf("读取巨量云图目录失败: %v", err)
	}
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
			if startDate != "" && dirDate < startDate {
				continue
			}
			if endDate != "" && dirDate > endDate {
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
				// 优先 csv，没有则回退 xlsx
				if p := pickFile(shopPath, "_品牌_达人投放"); p != "" {
					talent += importTalent(db, p, statDate, shopName)
				}
				if p := pickFile(shopPath, "_品牌_关键词趋势_本品"); p != "" {
					keyword += importKeyword(db, p, statDate, shopName, "本品")
				}
				if p := pickFile(shopPath, "_品牌_关键词趋势_自定义"); p != "" {
					keyword += importKeyword(db, p, statDate, shopName, "自定义")
				}
			}
		}
	}
	fmt.Printf("\n巨量云图导入完成:\n  达人投放: %d 条\n  关键词趋势: %d 条\n", talent, keyword)
}

// pickFile 在 dir 里按 marker 选数据文件：优先 .csv，没有则 .xlsx，都没返回 ""
func pickFile(dir, marker string) string {
	files, _ := os.ReadDir(dir)
	var csvP, xlsxP string
	for _, fl := range files {
		name := fl.Name()
		if fl.IsDir() || strings.HasPrefix(name, "~$") || !strings.Contains(name, marker) {
			continue
		}
		if strings.HasSuffix(name, ".csv") {
			csvP = filepath.Join(dir, name)
		} else if strings.HasSuffix(name, ".xlsx") {
			xlsxP = filepath.Join(dir, name)
		}
	}
	if csvP != "" {
		return csvP
	}
	return xlsxP
}

// readTable 读 csv 或 xlsx，返回表头列名→下标 + 数据行(xlsx 走 GetRows)
func readTable(path string) (map[string]int, [][]string, bool) {
	var rows [][]string
	if strings.HasSuffix(path, ".csv") {
		f, err := os.Open(path)
		if err != nil {
			log.Printf("打开失败 %s: %v", filepath.Base(path), err)
			return nil, nil, false
		}
		defer f.Close()
		rd := csv.NewReader(f)
		rd.FieldsPerRecord = -1
		if rows, err = rd.ReadAll(); err != nil {
			log.Printf("解析CSV失败 %s: %v", filepath.Base(path), err)
			return nil, nil, false
		}
	} else {
		f, err := excelize.OpenFile(path)
		if err != nil {
			log.Printf("打开xlsx失败 %s: %v", filepath.Base(path), err)
			return nil, nil, false
		}
		defer f.Close()
		if rows, err = f.GetRows(f.GetSheetName(0)); err != nil {
			log.Printf("读xlsx失败 %s: %v", filepath.Base(path), err)
			return nil, nil, false
		}
	}
	if len(rows) < 2 {
		return nil, nil, false
	}
	colMap := make(map[string]int)
	for i, h := range rows[0] {
		if len(h) >= 3 && h[0] == 0xEF && h[1] == 0xBB && h[2] == 0xBF {
			h = h[3:] // 剥 UTF-8 BOM
		}
		colMap[strings.TrimSpace(h)] = i
	}
	return colMap, rows[1:], true
}

// afterColon 剥"抖音号:xxx"/"视频ID:xxx"前缀(半角/全角冒号都认)，无冒号返回原值
func afterColon(s string) string {
	for _, sep := range []string{"：", ":"} {
		if i := strings.LastIndex(s, sep); i >= 0 {
			return strings.TrimSpace(s[i+len(sep):])
		}
	}
	return s
}

func ensureTables(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS op_juliang_talent_daily (
		id BIGINT PRIMARY KEY AUTO_INCREMENT,
		stat_date DATE NOT NULL COMMENT '统计日期(取目录日,快照)',
		shop_name VARCHAR(128) NOT NULL COMMENT '店铺/品牌(目录名)',
		video_id VARCHAR(32) NOT NULL COMMENT '视频ID(已剥前缀)',
		talent_name VARCHAR(128) DEFAULT '' COMMENT '达人名称',
		douyin_id VARCHAR(64) DEFAULT '' COMMENT '抖音号(已剥前缀)',
		coop_status VARCHAR(32) DEFAULT '' COMMENT '合作状态',
		viral_count INT DEFAULT 0 COMMENT '爆文数',
		viral_rate DECIMAL(12,6) DEFAULT 0 COMMENT '爆文率(小数)',
		quality_viral_count INT DEFAULT 0 COMMENT '质爆数',
		quality_viral_rate DECIMAL(12,6) DEFAULT 0 COMMENT '质爆率(小数)',
		mind_viral_count INT DEFAULT 0 COMMENT '心智爆数',
		mind_viral_rate DECIMAL(12,6) DEFAULT 0 COMMENT '心智爆率(小数)',
		volume_viral_count INT DEFAULT 0 COMMENT '量爆数',
		volume_viral_rate DECIMAL(12,6) DEFAULT 0 COMMENT '量爆率(小数)',
		mcn VARCHAR(128) DEFAULT '' COMMENT '所属MCN',
		content_type VARCHAR(64) DEFAULT '' COMMENT '内容类型',
		fit_industry VARCHAR(128) DEFAULT '' COMMENT '适合行业',
		content_title VARCHAR(500) DEFAULT '' COMMENT '内容标题',
		content_url VARCHAR(500) DEFAULT '' COMMENT '内容链接',
		post_search_count INT DEFAULT 0 COMMENT '看后搜索次数',
		post_search_uv INT DEFAULT 0 COMMENT '看后搜索人数',
		post_search_rate DECIMAL(12,6) DEFAULT 0 COMMENT '看后搜索率(小数)',
		back_search_count INT DEFAULT 0 COMMENT '回搜次数',
		back_search_uv INT DEFAULT 0 COMMENT '回搜人数',
		back_search_rate DECIMAL(12,6) DEFAULT 0 COMMENT '回搜率(小数)',
		star_push_ratio DECIMAL(12,6) DEFAULT 0 COMMENT '星推比',
		video_keywords VARCHAR(500) DEFAULT '' COMMENT '视频关键词',
		publish_date VARCHAR(16) DEFAULT '' COMMENT '发布时间(YYYYMMDD原值)',
		fans_count INT DEFAULT 0 COMMENT '粉丝数',
		has_cart VARCHAR(8) DEFAULT '' COMMENT '是否挂车',
		heat_type VARCHAR(32) DEFAULT '' COMMENT '加热投放类型',
		first_heat_time VARCHAR(32) DEFAULT '' COMMENT '首次加热时间',
		heat_amount DECIMAL(14,2) DEFAULT 0 COMMENT '加热投放金额',
		star_task_amount DECIMAL(14,2) DEFAULT 0 COMMENT '星图任务金额',
		roi_with_heat DECIMAL(12,4) DEFAULT 0 COMMENT '含加热ROI',
		post_search_cost_with_heat DECIMAL(12,4) DEFAULT 0 COMMENT '含加热看后搜索成本',
		total_play BIGINT DEFAULT 0 COMMENT '总播放量',
		natural_play BIGINT DEFAULT 0 COMMENT '自然播放量',
		heat_play BIGINT DEFAULT 0 COMMENT '加热播放量',
		total_interact BIGINT DEFAULT 0 COMMENT '总互动量',
		natural_interact BIGINT DEFAULT 0 COMMENT '自然互动量',
		heat_interact BIGINT DEFAULT 0 COMMENT '加热互动量',
		total_interact_rate DECIMAL(12,6) DEFAULT 0 COMMENT '总互动率(小数)',
		natural_interact_rate DECIMAL(12,6) DEFAULT 0 COMMENT '自然互动率(小数)',
		heat_interact_rate DECIMAL(12,6) DEFAULT 0 COMMENT '加热互动率(小数)',
		nps DECIMAL(12,4) DEFAULT 0 COMMENT 'NPS',
		total_new_a3 BIGINT DEFAULT 0 COMMENT '总新增A3量',
		natural_new_a3 BIGINT DEFAULT 0 COMMENT '自然新增A3量',
		heat_new_a3 BIGINT DEFAULT 0 COMMENT '加热新增A3量',
		total_new_a3_rate DECIMAL(12,6) DEFAULT 0 COMMENT '总新增A3率(小数)',
		finish_rate DECIMAL(12,6) DEFAULT 0 COMMENT '完播率(小数)',
		cpm_with_heat DECIMAL(12,4) DEFAULT 0 COMMENT '含加热CPM',
		cart_sales DECIMAL(14,2) DEFAULT 0 COMMENT '挂车销售额',
		cpe_with_heat DECIMAL(12,4) DEFAULT 0 COMMENT '含加热CPE',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		UNIQUE KEY uk_jl_talent (stat_date, shop_name, video_id),
		KEY idx_jl_talent_date (stat_date)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='巨量云图-品牌达人投放(每日每店每视频快照)'`); err != nil {
		return err
	}

	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS op_juliang_keyword_daily (
		id BIGINT PRIMARY KEY AUTO_INCREMENT,
		stat_date DATE NOT NULL COMMENT '统计日期(取目录日,快照)',
		shop_name VARCHAR(128) NOT NULL COMMENT '店铺/品牌(目录名)',
		source_type VARCHAR(16) NOT NULL COMMENT '来源(本品/自定义)',
		mind VARCHAR(128) NOT NULL COMMENT '心智(关键词)',
		mind_category VARCHAR(128) DEFAULT '' COMMENT '心智分类',
		own_assoc_pct DECIMAL(12,4) DEFAULT 0 COMMENT '本品联想占比(%)',
		own_assoc_pct_chain DECIMAL(12,4) DEFAULT 0 COMMENT '本品联想占比环比(%)',
		industry_assoc_share DECIMAL(12,4) DEFAULT 0 COMMENT '行业联想份额(%)',
		industry_assoc_share_chain DECIMAL(12,4) DEFAULT 0 COMMENT '行业联想份额环比(%)',
		reputation DECIMAL(12,4) DEFAULT 0 COMMENT '美誉度(%)',
		preference DECIMAL(12,4) DEFAULT 0 COMMENT '偏爱度(%)',
		content_volume BIGINT DEFAULT 0 COMMENT '内容量',
		spread_volume BIGINT DEFAULT 0 COMMENT '传播量',
		search_volume BIGINT DEFAULT 0 COMMENT '搜索量',
		assoc_share_rank INT DEFAULT 0 COMMENT '联想份额排名',
		content_volume_rank INT DEFAULT 0 COMMENT '内容量排名',
		spread_volume_rank INT DEFAULT 0 COMMENT '传播量排名',
		search_volume_rank INT DEFAULT 0 COMMENT '搜索量排名',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		UNIQUE KEY uk_jl_keyword (stat_date, shop_name, source_type, mind),
		KEY idx_jl_keyword_date (stat_date)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='巨量云图-品牌关键词趋势(每日每店每心智,本品/自定义)'`)
	return err
}

func importTalent(db *sql.DB, path, statDate, shopName string) int {
	colMap, rows, ok := readTable(path)
	if !ok {
		return 0
	}
	count := 0
	for _, row := range rows {
		get := func(name string) string {
			idx, ok := colMap[name]
			if !ok || idx >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[idx])
		}
		getF := func(name string) float64 {
			v := get(name)
			if v == "" || v == "-" {
				return 0
			}
			return importutil.ParseFloat(strings.TrimSuffix(v, "%"))
		}
		getI := func(name string) int64 { return int64(getF(name)) }

		videoID := afterColon(get("视频ID"))
		if videoID == "" || videoID == "-" {
			continue
		}
		if _, err := db.Exec(`REPLACE INTO op_juliang_talent_daily (
			stat_date, shop_name, video_id, talent_name, douyin_id, coop_status,
			viral_count, viral_rate, quality_viral_count, quality_viral_rate,
			mind_viral_count, mind_viral_rate, volume_viral_count, volume_viral_rate,
			mcn, content_type, fit_industry, content_title, content_url,
			post_search_count, post_search_uv, post_search_rate, back_search_count, back_search_uv, back_search_rate,
			star_push_ratio, video_keywords, publish_date, fans_count, has_cart,
			heat_type, first_heat_time, heat_amount, star_task_amount, roi_with_heat, post_search_cost_with_heat,
			total_play, natural_play, heat_play, total_interact, natural_interact, heat_interact,
			total_interact_rate, natural_interact_rate, heat_interact_rate, nps,
			total_new_a3, natural_new_a3, heat_new_a3, total_new_a3_rate, finish_rate,
			cpm_with_heat, cart_sales, cpe_with_heat
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			statDate, shopName, videoID, get("达人名称"), afterColon(get("抖音号")), get("合作状态"),
			getI("爆文数"), getF("爆文率"), getI("质爆数"), getF("质爆率"),
			getI("心智爆数"), getF("心智爆率"), getI("量爆数"), getF("量爆率"),
			get("所属MCN"), get("内容类型"), get("适合行业"), get("内容标题"), get("内容链接"),
			getI("看后搜索次数"), getI("看后搜索人数"), getF("看后搜索率"), getI("回搜次数"), getI("回搜人数"), getF("回搜率"),
			getF("星推比"), get("视频关键词"), get("发布时间"), getI("粉丝数"), get("是否挂车"),
			get("加热投放类型"), get("首次加热时间"), getF("加热投放金额"), getF("星图任务金额"), getF("含加热ROI"), getF("含加热看后搜索成本"),
			getI("总播放量"), getI("自然播放量"), getI("加热播放量"), getI("总互动量"), getI("自然互动量"), getI("加热互动量"),
			getF("总互动率"), getF("自然互动率"), getF("加热互动率"), getF("NPS"),
			getI("总新增A3量"), getI("自然新增A3量"), getI("加热新增A3量"), getF("总新增A3率"), getF("完播率"),
			getF("含加热CPM"), getF("挂车销售额"), getF("含加热CPE"),
		); err != nil {
			log.Printf("达人投放写入失败 shop=%s video=%s: %v", shopName, videoID, err)
			continue
		}
		count++
	}
	return count
}

func importKeyword(db *sql.DB, path, statDate, shopName, sourceType string) int {
	colMap, rows, ok := readTable(path)
	if !ok {
		return 0
	}
	count := 0
	for _, row := range rows {
		get := func(name string) string {
			idx, ok := colMap[name]
			if !ok || idx >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[idx])
		}
		getF := func(name string) float64 {
			v := get(name)
			if v == "" || v == "-" {
				return 0
			}
			return importutil.ParseFloat(strings.TrimSuffix(v, "%"))
		}
		getI := func(name string) int64 { return int64(getF(name)) }

		mind := get("心智")
		if mind == "" {
			continue
		}
		if _, err := db.Exec(`REPLACE INTO op_juliang_keyword_daily (
			stat_date, shop_name, source_type, mind, mind_category,
			own_assoc_pct, own_assoc_pct_chain, industry_assoc_share, industry_assoc_share_chain,
			reputation, preference, content_volume, spread_volume, search_volume,
			assoc_share_rank, content_volume_rank, spread_volume_rank, search_volume_rank
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			statDate, shopName, sourceType, mind, get("心智分类"),
			getF("本品联想占比"), getF("本品联想占比环比"), getF("行业联想份额"), getF("行业联想份额环比"),
			getF("美誉度"), getF("偏爱度"), getI("内容量"), getI("传播量"), getI("搜索量"),
			getI("联想份额排名"), getI("内容量排名"), getI("传播量排名"), getI("搜索量排名"),
		); err != nil {
			log.Printf("关键词趋势写入失败 shop=%s mind=%s: %v", shopName, mind, err)
			continue
		}
		count++
	}
	return count
}
