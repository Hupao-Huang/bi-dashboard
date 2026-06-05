// sync-employee-position 从钉钉"人员导出"Excel 同步员工岗位职级到 hesi_employee_contract_company
// 用途: 规则 7-3 住宿标准 / 规则 11 出差补贴 按职级判定 (总裁/副总裁/集团总监/集团经理/主管和其他)
// 匹配方式: 用中文姓名 join hesi_employee_contract_company.hesi_name (合思无 staff_id 对照)
//
// 2026-06-05 跑哥: 以钉钉"人员导出"表为准 (453 人, 比旧 SSC 表全)。该表"职级"列是 9 种写法,
// 经 levelToTier 映射到 5 个标准档。其中"集团总监、副总裁（非线下）"按集团总监档 (跑哥拍板)。
// 默认自动取桌面最新的 人员导出*.xlsx; 也可 -excel 指定。
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/xuri/excelize/v2"
)

type dbConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Dbname   string `json:"dbname"`
}

type config struct {
	Database dbConfig `json:"database"`
}

// levelToTier 钉钉职级(9种) -> 住宿/补贴标准档(5种)
var levelToTier = map[string]string{
	"总裁（非线下）":         "总裁",
	"副总裁（非线下）":        "副总裁",
	"集团总监、副总裁（非线下）":   "集团总监", // 跑哥 2026-06-05: 这9人按集团总监档
	"集团总监（线下）":        "集团总监",
	"集团经理（非线下）":       "集团经理",
	"大区经理及以上（线下）":     "集团经理", // 线下大区经理 ≈ 经理档
	"集团主管及其他员工（非线下）":  "主管和其他",
	"其他员工（线下）":        "主管和其他",
	"默认职级":            "主管和其他", // 企业账号/未分配 兜底
}

func loadDB() (*sql.DB, error) {
	data, err := os.ReadFile("config.json")
	if err != nil {
		return nil, err
	}
	var c config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true",
		c.Database.User, c.Database.Password, c.Database.Host, c.Database.Port, c.Database.Dbname)
	return sql.Open("mysql", dsn)
}

// newestExport 找桌面最新的 人员导出*.xlsx
func newestExport() string {
	cands, _ := filepath.Glob(`C:\Users\Administrator\Desktop\人员导出*.xlsx`)
	if len(cands) == 0 {
		return ""
	}
	sort.Slice(cands, func(i, j int) bool {
		fi, _ := os.Stat(cands[i])
		fj, _ := os.Stat(cands[j])
		return fi.ModTime().After(fj.ModTime())
	})
	return cands[0]
}

// colIndex 按表头名找列下标 (-1=找不到)
func colIndex(header []string, name string) int {
	for i, h := range header {
		if strings.TrimSpace(h) == name {
			return i
		}
	}
	return -1
}

func main() {
	excelPath := flag.String("excel", "", "人员导出 Excel 路径 (空=自动取桌面最新)")
	flag.Parse()

	path := *excelPath
	if path == "" {
		path = newestExport()
	}
	if path == "" {
		log.Fatalf("找不到 人员导出*.xlsx, 请用 -excel 指定")
	}
	log.Printf("使用 Excel: %s", path)

	db, err := loadDB()
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()

	_, _ = db.Exec(`ALTER TABLE hesi_employee_contract_company ADD COLUMN position VARCHAR(50) NULL COMMENT '岗位职级 (总裁/副总裁/集团总监/集团经理/主管和其他)'`)

	f, err := excelize.OpenFile(path)
	if err != nil {
		log.Fatalf("打开 Excel 失败: %v", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		log.Fatalf("Excel 无 sheet")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil || len(rows) < 2 {
		log.Fatalf("读取 sheet %s 失败或无数据: %v", sheets[0], err)
	}

	header := rows[0]
	cName := colIndex(header, "中文姓名")
	cSysName := colIndex(header, "系统姓名")
	cLevel := colIndex(header, "职级")
	if cLevel < 0 || (cName < 0 && cSysName < 0) {
		log.Fatalf("表头缺列: 中文姓名=%d 系统姓名=%d 职级=%d", cName, cSysName, cLevel)
	}

	type entry struct {
		name, level, tier string
	}
	var entries []entry
	unknownLevels := map[string]int{}
	for i, row := range rows {
		if i == 0 {
			continue
		}
		get := func(idx int) string {
			if idx >= 0 && idx < len(row) {
				return strings.TrimSpace(row[idx])
			}
			return ""
		}
		name := get(cName)
		if name == "" {
			name = get(cSysName) // 企业账号无中文姓名时兜底
		}
		level := get(cLevel)
		if name == "" || level == "" {
			continue
		}
		tier, ok := levelToTier[level]
		if !ok {
			tier = "主管和其他" // 新职级未映射 → 安全兜底为最低档
			unknownLevels[level]++
		}
		entries = append(entries, entry{name, level, tier})
	}
	log.Printf("Excel 解析: %d 条 (表头职级列=%q)", len(entries), header[cLevel])
	if len(unknownLevels) > 0 {
		log.Printf("⚠️ 出现未映射职级(已按'主管和其他'兜底, 请确认): %v", unknownLevels)
	}

	matched, ambiguous, missing := 0, 0, 0
	for _, e := range entries {
		var cnt int
		_ = db.QueryRow(`SELECT COUNT(*) FROM hesi_employee_contract_company WHERE hesi_name = ?`, e.name).Scan(&cnt)
		switch {
		case cnt == 1:
			if _, err := db.Exec(`UPDATE hesi_employee_contract_company SET position = ? WHERE hesi_name = ?`, e.tier, e.name); err != nil {
				log.Printf("UPDATE %s 失败: %v", e.name, err)
				continue
			}
			matched++
		case cnt > 1:
			ambiguous++
		default:
			missing++
		}
	}
	log.Printf("Position 同步完成: 单匹配 %d / 重名跳过 %d / 合思无此人 %d", matched, ambiguous, missing)

	rows2, err := db.Query(`SELECT IFNULL(position,'(空)'), COUNT(*) FROM hesi_employee_contract_company GROUP BY position ORDER BY COUNT(*) DESC`)
	if err == nil {
		defer rows2.Close()
		log.Println("当前 position 分布:")
		for rows2.Next() {
			var p string
			var n int
			_ = rows2.Scan(&p, &n)
			log.Printf("  %s: %d", p, n)
		}
	}
}
