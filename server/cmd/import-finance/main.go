// import-finance.exe <xlsx_path> [year]
// 从文件名推断年份（YYYY年财务管理报表.xlsx），可用第二参数覆盖
package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"

	"bi-dashboard/internal/config"
	"bi-dashboard/internal/finance"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: import-finance.exe <xlsx_path> [year]")
		os.Exit(2)
	}
	fpath := os.Args[1]

	year := finance.ParseYearFromFilename(fpath)
	if len(os.Args) >= 3 {
		if y, err := strconv.Atoi(os.Args[2]); err == nil {
			year = y
		}
	}
	if year == 0 {
		log.Fatalf("无法从文件名推断年份，请用第二参数指定 year")
	}

	if _, err := os.Stat(fpath); err != nil {
		log.Fatalf("文件不存在: %s", fpath)
	}

	cfg, err := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	defer db.Close()

	dict, err := finance.LoadSubjectDict(db)
	if err != nil {
		log.Fatalf("加载科目字典失败: %v", err)
	}
	log.Printf("已加载科目字典 %d 条", len(dict))

	log.Printf("开始解析: %s (年份: %d)", fpath, year)
	result, err := finance.ParseFile(fpath, year, dict)
	if err != nil {
		_ = finance.LogImport(db, fpath, year, &finance.ParseResult{Year: year}, 0, "failed", err.Error())
		log.Fatalf("解析失败: %v", err)
	}
	log.Printf("解析完成：sheet %d, rows %d, 未映射 %d", result.SheetCount, result.RowCount, len(result.UnmappedSubjects))
	for _, u := range result.UnmappedSubjects {
		log.Printf("  [未映射] %s/%s parent=%s subject=%s", u.Sheet, u.Department, u.Parent, u.Subject)
	}

	log.Printf("写入数据库中...渠道: %v", result.Departments)
	if err := finance.WriteResult(db, result); err != nil {
		_ = finance.LogImport(db, fpath, year, result, 0, "failed", err.Error())
		log.Fatalf("入库失败: %v", err)
	}

	status := "success"
	if len(result.UnmappedSubjects) > 0 {
		status = "partial"
	}
	if err := finance.LogImport(db, fpath, year, result, 0, status, ""); err != nil {
		log.Printf("写入日志失败: %v", err)
	}
	log.Printf("完成✓  %d 年入库 %d 条", year, result.RowCount)
}
