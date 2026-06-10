package main

// 客服-店铺评价评论数据导入 (RPA: Z:\信息部\RPA_客服_店铺后台评价抓取\结果汇总)。
// 每天一份 YYYY-MM-DD评论数据.xlsx, 单 sheet「抓取数据汇总」, 7 列多平台混排:
//   平台 / 店铺 / 时间 / 订单编号 / 商品名称 / 评价内容 / 评分
// 清洗: 时间统一成年月日; 订单编号剥掉「订单编号：」前缀(字符串存防精度); 按内容 hash 去重幂等。
// 用法: import-comment            全量导整个目录
//       import-comment 2026-06-01 2026-06-09   只导该文件名日期范围

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	_ "github.com/go-sql-driver/mysql"
	"github.com/xuri/excelize/v2"
)

var baseDir = `Z:\信息部\RPA_客服_店铺后台评价抓取\结果汇总`

const commentSheet = "抓取数据汇总"

func main() {
	unlock := importutil.AcquireLock("import-comment")
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

	if err := ensureCommentTable(db); err != nil {
		log.Fatalf("建表失败: %v", err)
	}

	// 可选: os.Args[1]=起始日期 os.Args[2]=结束日期 (按文件名日期过滤, YYYY-MM-DD)
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

	totalRows, totalFiles, totalSkip := 0, 0, 0
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		if strings.HasPrefix(name, "~$") {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(name), ".xlsx") {
			continue
		}
		// 文件名 YYYY-MM-DD评论数据.xlsx → 取前 10 位做范围过滤(仅过滤用, 业务日期仍读单元格)
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

		cnt, skip, err := importCommentFile(db, filepath.Join(root, name), name)
		if err != nil {
			log.Printf("导入失败 [%s]: %v", name, err)
			continue
		}
		totalRows += cnt
		totalSkip += skip
		totalFiles++
		fmt.Printf("[%s] 入库 %d 条 (跳过 %d)\n", name, cnt, skip)
	}
	fmt.Printf("\n评论数据导入完成: %d 个文件, 入库/更新 %d 条, 跳过 %d 条\n", totalFiles, totalRows, totalSkip)
}

func ensureCommentTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS op_customer_comment (
		id BIGINT PRIMARY KEY AUTO_INCREMENT COMMENT '主键',
		platform VARCHAR(32) NOT NULL DEFAULT '' COMMENT '平台',
		shop_name VARCHAR(128) NOT NULL DEFAULT '' COMMENT '店铺',
		comment_date DATE NULL COMMENT '评论日期(只年月日)',
		comment_time_raw VARCHAR(64) NOT NULL DEFAULT '' COMMENT '原始时间文本(备查)',
		order_no VARCHAR(64) NOT NULL DEFAULT '' COMMENT '订单编号(剥前缀后,字符串)',
		order_no_raw VARCHAR(128) NOT NULL DEFAULT '' COMMENT '原始订单编号文本(备查)',
		product_name VARCHAR(255) NOT NULL DEFAULT '' COMMENT '商品名称',
		comment_content TEXT COMMENT '评价内容',
		score TINYINT NULL COMMENT '评分数字(能解析:1-5; 天猫等文字评分为NULL)',
		score_raw VARCHAR(32) NOT NULL DEFAULT '' COMMENT '原始评分(天猫是文字如负面评价, 其他是数字)',
		source_file VARCHAR(64) NOT NULL DEFAULT '' COMMENT '来源RPA文件名',
		content_hash CHAR(32) NOT NULL COMMENT '去重指纹=md5(平台|店铺|原始订单号|商品|评价内容)',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
		UNIQUE KEY uk_hash (content_hash),
		KEY idx_platform_date (platform, comment_date),
		KEY idx_shop (shop_name)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='客服-店铺评价评论明细(RPA抓取)'`)
	if err != nil {
		return err
	}
	// 兼容已存在的旧表: 补 score_raw 列(已存在则忽略报错)
	_, _ = db.Exec(`ALTER TABLE op_customer_comment ADD COLUMN score_raw VARCHAR(32) NOT NULL DEFAULT '' COMMENT '原始评分文字'`)
	return nil
}

func importCommentFile(db *sql.DB, path, fname string) (int, int, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	sheet := commentSheet
	if idx, _ := f.GetSheetIndex(sheet); idx < 0 {
		sheet = f.GetSheetName(0)
	}
	rows, err := f.GetRows(sheet)
	if err != nil {
		return 0, 0, err
	}
	if len(rows) < 2 {
		return 0, 0, nil
	}

	cnt, skip := 0, 0
	for i, r := range rows {
		if i == 0 {
			continue // 表头
		}
		platform := cell(r, 0)
		shop := cell(r, 1)
		timeRaw := cell(r, 2)
		orderRaw := cell(r, 3)
		product := cell(r, 4)
		content := cell(r, 5)
		scoreRaw := cell(r, 6)
		if platform == "" && shop == "" && content == "" {
			continue // 空行
		}

		commentDate := parseCommentDate(timeRaw)
		orderNo := extractOrderNo(orderRaw)
		score := parseScore(scoreRaw)
		hash := contentHash(platform, shop, orderRaw, product, content)

		var dateArg interface{}
		if commentDate != "" {
			dateArg = commentDate
		}
		var scoreArg interface{}
		if score >= 0 {
			scoreArg = score
		}

		if _, err := db.Exec(`INSERT INTO op_customer_comment
			(platform, shop_name, comment_date, comment_time_raw, order_no, order_no_raw,
			 product_name, comment_content, score, score_raw, source_file, content_hash)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE
			 platform=VALUES(platform), shop_name=VALUES(shop_name), comment_date=VALUES(comment_date),
			 comment_time_raw=VALUES(comment_time_raw), order_no=VALUES(order_no), order_no_raw=VALUES(order_no_raw),
			 product_name=VALUES(product_name), comment_content=VALUES(comment_content), score=VALUES(score),
			 score_raw=VALUES(score_raw), source_file=VALUES(source_file)`,
			platform, shop, dateArg, timeRaw, orderNo, orderRaw,
			product, content, scoreArg, scoreRaw, fname, hash); err != nil {
			skip++
			log.Printf("  [%s] 行 %d upsert 失败: %v", fname, i+1, err)
			continue
		}
		cnt++
	}
	return cnt, skip, nil
}

func cell(r []string, i int) string {
	if i >= len(r) {
		return ""
	}
	return strings.TrimSpace(r[i])
}

// parseCommentDate 把各种时间格式统一成 YYYY-MM-DD; 解析不了返回 ""。
// 覆盖: 抖音文本「2026年06月07日 12:39:34」/ excelize 渲染的美式「6/7/26 13:40」(M/D/YY)/
//       标准「2026-06-07 13:40:07」/ Excel 日期序列号。
func parseCommentDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// 剥掉日期前的非数字前缀(如天猫「初次评价: 」); 日期都以数字打头
	if i := strings.IndexFunc(s, func(r rune) bool { return r >= '0' && r <= '9' }); i > 0 {
		s = strings.TrimSpace(s[i:])
	}
	// 1) 中文年月日 (抖音)
	if strings.Contains(s, "年") {
		d := s
		if idx := strings.Index(d, " "); idx > 0 {
			d = d[:idx]
		}
		d = strings.NewReplacer("年", "-", "月", "-", "日", "").Replace(d)
		d = strings.TrimSuffix(d, "-")
		parts := strings.Split(d, "-")
		if len(parts) == 3 && len(parts[0]) == 4 {
			return parts[0] + "-" + pad2(parts[1]) + "-" + pad2(parts[2])
		}
		return ""
	}
	// 2) Excel 日期序列号 (纯数字; 2 万~10 万对应 1954~2173 年)
	if f, err := strconv.ParseFloat(s, 64); err == nil && f > 20000 && f < 100000 {
		if t, err := excelize.ExcelDateToTime(f, false); err == nil {
			return t.Format("2006-01-02")
		}
	}
	// 3) 常见文本日期格式 (含拼多多 excelize 渲染的美式 M/D/YY「6/7/26 13:40」)
	for _, l := range []string{
		"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02",
		"2006/1/2 15:04:05", "2006/1/2 15:04", "2006/1/2",
		"1/2/06 15:04:05", "1/2/06 15:04", "1/2/06",
		"1/2/2006 15:04:05", "1/2/2006 15:04", "1/2/2006",
		"2006.01.02",
	} {
		if t, err := time.Parse(l, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

func pad2(s string) string {
	if len(s) == 1 {
		return "0" + s
	}
	return s
}

// extractOrderNo 剥掉「订单编号：」之类前缀(中文/英文冒号都认), 只留真号。
// 订单号本身不含冒号, 故取最后一个冒号之后即可; 抖音纯数字无冒号则原样返回。
func extractOrderNo(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.LastIndexAny(s, ":："); i >= 0 {
		_, size := utf8.DecodeRuneInString(s[i:])
		s = s[i+size:]
	}
	return strings.TrimSpace(s)
}

// parseScore 评分转 int; 空/非数字返回 -1 (入库存 NULL)。
func parseScore(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return -1
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return -1
	}
	return int(f)
}

// contentHash 去重指纹 (用稳定的原始值, 不含会被清洗改变的 date)。
func contentHash(platform, shop, orderNoRaw, product, content string) string {
	h := md5.Sum([]byte(platform + "|" + shop + "|" + orderNoRaw + "|" + product + "|" + content))
	return hex.EncodeToString(h[:])
}
