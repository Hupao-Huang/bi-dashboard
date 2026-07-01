package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"bi-dashboard/internal/salesdaily"
	"context"
	"database/sql"
	"io"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	unlock := importutil.AcquireLock("build-sales-daily-report-summary")
	defer unlock()

	logFile, err := os.OpenFile(`C:\Users\Administrator\bi-dashboard\server\build-sales-daily-report-summary.log`, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		log.SetOutput(io.MultiWriter(logFile, os.Stdout))
		defer logFile.Close()
	}

	cfg, err := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(4)

	startDate, endDate := resolveDateRange()
	log.Printf("========== 开始构建销售日报快表 %s ~ %s ==========", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	if err := salesdaily.RebuildReportSummaryRange(context.Background(), db, startDate, endDate); err != nil {
		log.Fatalf("构建失败: %v", err)
	}
	log.Println("========== 销售日报快表构建完成 ==========")
}

func resolveDateRange() (time.Time, time.Time) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	end := now.AddDate(0, 0, -1)
	if start.After(end) {
		start = end
	}
	if s := firstEnv("REPORT_START_DATE", "COMBO_START_DATE"); s != "" {
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			log.Fatalf("REPORT_START_DATE 格式错误(应为 yyyy-MM-dd): %v", err)
		}
		start = d
	}
	if s := firstEnv("REPORT_END_DATE", "COMBO_END_DATE"); s != "" {
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			log.Fatalf("REPORT_END_DATE 格式错误(应为 yyyy-MM-dd): %v", err)
		}
		end = d
	}
	if start.After(end) {
		log.Fatalf("开始日期不能晚于结束日期: %s > %s", start.Format("2006-01-02"), end.Format("2006-01-02"))
	}
	return start, end
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if v := os.Getenv(name); v != "" {
			return v
		}
	}
	return ""
}
