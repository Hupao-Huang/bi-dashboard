// import-business-report.exe: 导入业务预决算报表 xlsx
//
// 用法:
//   import-business-report.exe --snapshot 2026-04 --year 2026 --xlsx <path>
//   import-business-report.exe --snapshot 2025-12 --year 2025 --xlsx /path/2025年业务预决算报表.xlsx
//
// 默认行为:
//   - 按 (snapshot_year, snapshot_month) 全删 + 重写
//   - --year 默认等于 snapshot 年份
//   - 不带 --year 时从 snapshot 推断
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
	"time"

	_ "github.com/go-sql-driver/mysql"

	"bi-dashboard/internal/business"
	"bi-dashboard/internal/importutil"
)

type config struct {
	Database struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		User     string `json:"user"`
		Password string `json:"password"`
		DBName   string `json:"dbname"`
	} `json:"database"`
}

func loadConfig(path string) (*config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c.Database.Port == 0 {
		c.Database.Port = 3306
	}
	return &c, nil
}

func main() {
	unlock := importutil.AcquireLock("import-business-report")
	defer unlock()

	snapshot := flag.String("snapshot", "", "快照日期 YYYY-MM (财务出此报表的时间点)，如 2026-04")
	year := flag.Int("year", 0, "报表覆盖的业务年份；不填则用 snapshot 年份")
	xlsxPath := flag.String("xlsx", "", "xlsx 文件路径")
	configPath := flag.String("config", "config.json", "config.json 路径")
	dryRun := flag.Bool("dry-run", false, "只解析不写库")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "import-business-report.exe --snapshot 2026-04 --year 2026 --xlsx <path>")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *snapshot == "" || *xlsxPath == "" {
		flag.Usage()
		os.Exit(2)
	}

	sy, sm, err := parseSnapshot(*snapshot)
	if err != nil {
		log.Fatalf("snapshot 参数错误: %v", err)
	}
	if *year == 0 {
		*year = sy
	}

	t0 := time.Now()
	log.Printf("📥 解析 xlsx: %s (snapshot=%d-%02d, year=%d)", *xlsxPath, sy, sm, *year)
	result, err := business.ParseFile(*xlsxPath, sy, sm, *year)
	if err != nil {
		log.Fatalf("解析失败: %v", err)
	}
	log.Printf("📊 解析完成: %d 行 / %d sheet 处理 / %d sheet 跳过 / %d channels: %v",
		result.RowCount, result.SheetsHandled, result.SheetsSkipped, len(result.Channels), result.Channels)

	if *dryRun {
		log.Printf("🛑 dry-run 模式，前 5 行:")
		for i, r := range result.Rows {
			if i >= 5 {
				break
			}
			log.Printf("  [%d] %s/%s | %s (L%d, %s) | period=%d | budget=%v actual=%v",
				r.SortOrder, r.Channel, r.SubChannel, r.Subject, r.SubjectLevel, r.SubjectCategory,
				r.PeriodMonth, fmtPtr(r.Budget), fmtPtr(r.Actual))
		}
		log.Printf("✅ dry-run 退出，未写库")
		return
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("读 config: %v", err)
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local&interpolateParams=true",
		cfg.Database.User, cfg.Database.Password, cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("DB open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("DB ping: %v", err)
	}

	if err := business.WriteResult(db, result); err != nil {
		log.Fatalf("写库失败: %v", err)
	}
	log.Printf("✅ 写入成功，耗时 %v", time.Since(t0))
}

func parseSnapshot(s string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(s), "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("格式应为 YYYY-MM")
	}
	y, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("年份非数字: %s", parts[0])
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("月份非数字: %s", parts[1])
	}
	if y < 2020 || y > 2050 {
		return 0, 0, fmt.Errorf("年份超出 2020-2050: %d", y)
	}
	if m < 1 || m > 12 {
		return 0, 0, fmt.Errorf("月份超出 1-12: %d", m)
	}
	return y, m, nil
}

func fmtPtr(p *float64) string {
	if p == nil {
		return "NULL"
	}
	return fmt.Sprintf("%.2f", *p)
}
