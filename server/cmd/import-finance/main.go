package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"bi-dashboard/internal/config"

	_ "github.com/go-sql-driver/mysql"
	"github.com/xuri/excelize/v2"
)

var sheetDeptMap = map[string]string{
	"考核利润汇总表": "汇总",
	"1、电商":     "电商",
	"2、社媒":     "社媒",
	"3、线下":     "线下",
	"4、分销":     "分销",
	"5、私域":     "私域",
	"6、国际零售业务": "国际零售",
	"7、即时零售":   "即时零售",
	"8、糙有力量":   "糙有力量",
	"中台部门":     "中台",
	"电商":       "电商",
	"社媒":       "社媒",
	"线下":       "线下",
	"分销":       "分销",
	"私域":       "私域",
}

var categoryKeywords = []struct {
	keyword  string
	category string
}{
	{"GMV", "GMV"},
	{"营业收入", "收入"},
	{"营业成本", "成本"},
	{"营业毛利", "毛利"},
	{"仓储物流费用", "成本"},
	{"销售费用", "销售费用"},
	{"运营利润", "运营利润"},
	{"管理费用", "管理费用"},
	{"研发费用", "研发费用"},
	{"利润总额", "利润总额"},
	{"营业利润", "利润总额"},
	{"营业外收入", "营业外"},
	{"营业外支出", "营业外"},
	{"净利润", "净利润"},
}

func main() {
	cfg, _ := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	dir := `C:\Users\Administrator\Desktop\财务报表`
	files := map[string]int{
		"25年12月财务报表.xlsx":    2025,
		"26年1-33月财务报表.xlsx": 2026,
	}

	for filename, year := range files {
		fpath := filepath.Join(dir, filename)
		if _, err := os.Stat(fpath); err != nil {
			log.Printf("文件不存在: %s", fpath)
			continue
		}
		log.Printf("处理 %s (年份: %d)", filename, year)
		count := importFile(db, fpath, year)
		log.Printf("  导入 %d 条记录", count)
	}
}

func colName(col int) string {
	name := ""
	for col >= 0 {
		name = string(rune('A'+col%26)) + name
		col = col/26 - 1
	}
	return name
}

func importFile(db *sql.DB, fpath string, year int) int {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		log.Printf("打开文件失败: %v", err)
		return 0
	}
	defer f.Close()

	total := 0
	for _, sheetName := range f.GetSheetList() {
		dept, ok := sheetDeptMap[sheetName]
		if !ok {
			continue
		}

		allRows, _ := f.GetRows(sheetName)
		if len(allRows) < 3 {
			continue
		}

		headerRow := 2
		monthCols := map[int]int{}
		for ci := 0; ci < 30; ci++ {
			cell, _ := f.GetCellValue(sheetName, colName(ci)+fmt.Sprintf("%d", headerRow))
			cell = strings.TrimSpace(cell)
			for m := 1; m <= 12; m++ {
				if cell == fmt.Sprintf("%d月", m) {
					monthCols[ci] = m
					break
				}
			}
		}
		if len(monthCols) == 0 {
			continue
		}

		sheetCount := 0
		currentCategory := ""
		sortOrder := 0
		maxRow := len(allRows) + 5
		if maxRow > 80 {
			maxRow = 80
		}
		for ri := headerRow + 1; ri <= maxRow; ri++ {
			subject, _ := f.GetCellValue(sheetName, "A"+fmt.Sprintf("%d", ri))
			subject = strings.TrimSpace(subject)
			if subject == "" || subject == "项目" {
				continue
			}

			for _, kw := range categoryKeywords {
				if strings.Contains(subject, kw.keyword) {
					currentCategory = kw.category
					break
				}
			}

			sortOrder++
			for ci, month := range monthCols {
				cellRef := colName(ci) + fmt.Sprintf("%d", ri)
				valStr, _ := f.GetCellValue(sheetName, cellRef)
				valStr = strings.TrimSpace(valStr)
				if valStr == "" || valStr == "#DIV/0!" || valStr == "#REF!" || valStr == "-" {
					continue
				}
				val, err := strconv.ParseFloat(valStr, 64)
				if err != nil || val == 0 {
					continue
				}

				_, err = db.Exec(`
					INSERT INTO finance_report (year, month, department, subject, subject_category, sort_order, amount)
					VALUES (?, ?, ?, ?, ?, ?, ?)
					ON DUPLICATE KEY UPDATE
						subject_category=VALUES(subject_category), sort_order=VALUES(sort_order), amount=VALUES(amount)`,
					year, month, dept, subject, currentCategory, sortOrder, val)
				if err != nil {
					log.Printf("  插入失败 [%s][%d月][%s]: %v", dept, month, subject, err)
					continue
				}
				sheetCount++
				total++
			}
		}
		log.Printf("  [%s] %s → %d 条", sheetName, dept, sheetCount)
	}
	return total
}
