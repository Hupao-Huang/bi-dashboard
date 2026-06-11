package main

// 客服-店铺服务分导入 (RPA: Z:\信息部\RPA_客服_店铺服务分\结果数据)。
// 每天一份 YYYY-MM-DD服务分数据.xlsx, 宽表格式:
//   第0行 = 当月日期 (Excel 序列号, 每个日期占 3 列)
//   第1行 = 表头: 平台/店铺/服务分目标 + 每日期下 物流分/商品分/服务分
//   第2行起 = 每行一个店铺, 已有数据的日期填值, 未来日期为空
// 各平台三项分数含义不同 (表头统一写物流/商品/服务, 实际按平台区分):
//   京东自营: 平均响应时间 / 应答率(0-1百分比) / 满意度(0-1百分比)
//   拼多多:   发货分/物流分(一格斜杠俩数) / 商品分 / 服务分/基础分(一格斜杠俩数)
//   其他平台: 物流分 / 商品分 / 服务分 (POP 10分制, 抖音 100分制, 天猫等 5分制)
// 每天新文件覆盖整月截至昨天的数据 → 全量扫描目录所有文件按 (日期,平台,店铺) 幂等覆盖,
// 月末文件留在目录里, 翻月也不会丢上月末尾几天。
// 用法: import-service-score                    全量导整个目录
//       import-service-score 2026-06-08 2026-06-10   只导该文件名日期范围

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/xuri/excelize/v2"
)

var baseDir = `Z:\信息部\RPA_客服_店铺服务分\结果数据`

func main() {
	unlock := importutil.AcquireLock("import-service-score")
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

	if err := ensureServiceScoreTable(db); err != nil {
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
	files, err := os.ReadDir(root)
	if err != nil {
		log.Fatalf("读目录失败: %v", err)
	}

	// 按文件名升序导入: 新文件覆盖旧文件同日期的值, 最终库里留每天最新一份的口径
	names := []string{}
	for _, f := range files {
		name := f.Name()
		if f.IsDir() || strings.HasPrefix(name, "~$") || !strings.HasSuffix(strings.ToLower(name), ".xlsx") {
			continue
		}
		fileDate := ""
		if len(name) >= 10 {
			fileDate = name[:10]
		}
		if startDate != "" && fileDate < startDate {
			continue
		}
		if endDate != "" && fileDate > endDate {
			continue
		}
		names = append(names, name)
	}
	sortStrings(names)

	totalRows, totalFiles := 0, 0
	for _, name := range names {
		cnt, err := importScoreFile(db, filepath.Join(root, name), name)
		if err != nil {
			log.Printf("导入失败 [%s]: %v", name, err)
			continue
		}
		totalRows += cnt
		totalFiles++
		fmt.Printf("[%s] 入库/更新 %d 条\n", name, cnt)
	}
	fmt.Printf("\n服务分导入完成: %d 个文件, 入库/更新 %d 条\n", totalFiles, totalRows)
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func ensureServiceScoreTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS op_service_score_daily (
		id BIGINT PRIMARY KEY AUTO_INCREMENT COMMENT '主键',
		stat_date DATE NOT NULL COMMENT '业务日期(取Excel表头日期列,非文件名)',
		platform VARCHAR(32) NOT NULL DEFAULT '' COMMENT '平台(POP/抖音/快手/天猫/唯品会/小红书/京东自营/拼多多)',
		shop_name VARCHAR(128) NOT NULL DEFAULT '' COMMENT '店铺',
		score1 DECIMAL(10,3) NULL COMMENT '第1项主值(默认=物流分;京东自营=平均响应时间;拼多多=发货分)',
		score1_extra DECIMAL(10,3) NULL COMMENT '第1项斜杠后值(仅拼多多=物流分)',
		score1_raw VARCHAR(32) NOT NULL DEFAULT '' COMMENT '第1项原始文本(备查)',
		score2 DECIMAL(10,3) NULL COMMENT '第2项主值(默认=商品分;京东自营=应答率,0-1存,显示乘100%)',
		score2_raw VARCHAR(32) NOT NULL DEFAULT '' COMMENT '第2项原始文本(备查)',
		score3 DECIMAL(10,3) NULL COMMENT '第3项主值(默认=服务分;京东自营=满意度,0-1存;拼多多=服务分)',
		score3_extra DECIMAL(10,3) NULL COMMENT '第3项斜杠后值(仅拼多多=基础分)',
		score3_raw VARCHAR(32) NOT NULL DEFAULT '' COMMENT '第3项原始文本(备查)',
		target DECIMAL(10,3) NULL COMMENT '服务分目标(京东自营/拼多多无目标为NULL)',
		target_raw VARCHAR(32) NOT NULL DEFAULT '' COMMENT '目标原始文本(/=无目标)',
		source_file VARCHAR(64) NOT NULL DEFAULT '' COMMENT '来源RPA文件名',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
		UNIQUE KEY uk_date_shop (stat_date, platform, shop_name),
		KEY idx_platform_date (platform, stat_date)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='客服-店铺服务分每日明细(RPA抓取,三项分数含义按平台不同)'`)
	return err
}

func importScoreFile(db *sql.DB, path, fname string) (int, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil {
		return 0, err
	}
	if len(rows) < 3 {
		return 0, nil
	}

	// 第0行: 日期列 (Excel 序列号或日期文本), 记录每个日期的起始列号
	type dateCol struct {
		col  int
		date string
	}
	var dateCols []dateCol
	for ci, v := range rows[0] {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		d := parseHeaderDate(v)
		if d == "" {
			log.Printf("  [%s] 表头第 %d 列日期解析失败: %q", fname, ci+1, v)
			continue
		}
		dateCols = append(dateCols, dateCol{col: ci, date: d})
	}
	if len(dateCols) == 0 {
		return 0, fmt.Errorf("表头没有解析到任何日期列")
	}

	cnt := 0
	for ri := 2; ri < len(rows); ri++ {
		r := rows[ri]
		platform := cell(r, 0)
		shop := cell(r, 1)
		targetRaw := cell(r, 2)
		if platform == "" || shop == "" {
			continue
		}
		target := parseNum(targetRaw)

		for _, dc := range dateCols {
			s1raw, s2raw, s3raw := cell(r, dc.col), cell(r, dc.col+1), cell(r, dc.col+2)
			if s1raw == "" && s2raw == "" && s3raw == "" {
				continue // 未来日期/无数据
			}
			s1, s1b := parseSlashNum(s1raw)
			s2 := parseNum(s2raw)
			s3, s3b := parseSlashNum(s3raw)

			if _, err := db.Exec(`INSERT INTO op_service_score_daily
				(stat_date, platform, shop_name,
				 score1, score1_extra, score1_raw, score2, score2_raw,
				 score3, score3_extra, score3_raw, target, target_raw, source_file)
				VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
				ON DUPLICATE KEY UPDATE
				 score1=VALUES(score1), score1_extra=VALUES(score1_extra), score1_raw=VALUES(score1_raw),
				 score2=VALUES(score2), score2_raw=VALUES(score2_raw),
				 score3=VALUES(score3), score3_extra=VALUES(score3_extra), score3_raw=VALUES(score3_raw),
				 target=VALUES(target), target_raw=VALUES(target_raw), source_file=VALUES(source_file)`,
				dc.date, platform, shop,
				s1, s1b, s1raw, s2, s2raw,
				s3, s3b, s3raw, target, targetRaw, fname); err != nil {
				log.Printf("  [%s] %s %s/%s upsert 失败: %v", fname, dc.date, platform, shop, err)
				continue
			}
			cnt++
		}
	}
	return cnt, nil
}

func cell(r []string, i int) string {
	if i >= len(r) {
		return ""
	}
	return strings.TrimSpace(r[i])
}

// parseHeaderDate 表头日期: Excel 序列号 (如 46174) 或常见日期文本 → YYYY-MM-DD
func parseHeaderDate(s string) string {
	if f, err := strconv.ParseFloat(s, 64); err == nil && f > 20000 && f < 100000 {
		if t, err := excelize.ExcelDateToTime(f, false); err == nil {
			return t.Format("2006-01-02")
		}
	}
	for _, l := range []string{"2006-01-02", "2006/1/2", "1/2/06", "1/2/2006", "01-02-06"} {
		if t, err := time.Parse(l, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

// parseNum 数字文本 → *float64; 空/"/"/非数字 → nil (入库 NULL)
func parseNum(s string) *float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "/" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

// parseSlashNum 解析 "4.4/2.6" 斜杠双值 (拼多多) → 主值+副值; 普通数字副值为 nil
func parseSlashNum(s string) (*float64, *float64) {
	s = strings.TrimSpace(s)
	if s == "" || s == "/" {
		return nil, nil
	}
	if i := strings.Index(s, "/"); i >= 0 {
		return parseNum(s[:i]), parseNum(s[i+1:])
	}
	return parseNum(s), nil
}
