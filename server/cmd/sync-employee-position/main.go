// sync-employee-position 从 SSC 职位表 Excel 同步员工岗位职级到 hesi_employee_contract_company
// 用途: 规则 7-3 住宿标准按职级判定 (总裁/副总裁/集团总监/集团经理/集团主管及其他员工)
// Excel 路径: 桌面 ssc确认职位表(1)11.28.xlsx, 列 11 (索引 10) = "岗位职级"
// 匹配方式: 用姓名 join hesi_employee_contract_company.hesi_name (无 staff_id 列)
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
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

func main() {
	excelPath := flag.String("excel", `C:\Users\Administrator\Desktop\ssc确认职位表(1)11.28.xlsx`, "职位表 Excel 路径")
	flag.Parse()

	db, err := loadDB()
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()

	// 确保 position 列存在 (幂等)
	_, _ = db.Exec(`ALTER TABLE hesi_employee_contract_company ADD COLUMN position VARCHAR(50) NULL COMMENT '岗位职级 (总裁/副总裁/集团总监/集团经理/主管和其他)'`)

	f, err := excelize.OpenFile(*excelPath)
	if err != nil {
		log.Fatalf("打开 Excel 失败: %v", err)
	}
	defer f.Close()

	rows, err := f.GetRows("Sheet1")
	if err != nil {
		log.Fatalf("读取 Sheet1 失败: %v", err)
	}

	type entry struct {
		name     string
		position string
	}
	var entries []entry
	for i, row := range rows {
		if i == 0 {
			continue
		}
		if len(row) < 11 {
			continue
		}
		name := strings.TrimSpace(row[1])
		position := strings.TrimSpace(row[10])
		if name == "" || position == "" {
			continue
		}
		entries = append(entries, entry{name, position})
	}
	log.Printf("Excel 解析: %d 条非空员工岗位职级", len(entries))

	// 按姓名 update hesi_employee_contract_company.position
	matched, ambiguous, missing := 0, 0, 0
	for _, e := range entries {
		var cnt int
		_ = db.QueryRow(`SELECT COUNT(*) FROM hesi_employee_contract_company WHERE hesi_name = ?`, e.name).Scan(&cnt)
		switch {
		case cnt == 1:
			_, err := db.Exec(`UPDATE hesi_employee_contract_company SET position = ? WHERE hesi_name = ?`, e.position, e.name)
			if err != nil {
				log.Printf("UPDATE %s 失败: %v", e.name, err)
				continue
			}
			matched++
		case cnt > 1:
			log.Printf("姓名重复 %s (合思 %d 条), 跳过 — 待人工对账", e.name, cnt)
			ambiguous++
		default:
			missing++
		}
	}
	log.Printf("Position 同步完成: 单匹配 %d / 重名 %d / 合思没此人 %d", matched, ambiguous, missing)

	// 分布统计
	rows2, err := db.Query(`SELECT position, COUNT(*) FROM hesi_employee_contract_company WHERE position IS NOT NULL GROUP BY position`)
	if err != nil {
		log.Fatalf("统计失败: %v", err)
	}
	defer rows2.Close()
	log.Println("当前 position 分布:")
	for rows2.Next() {
		var p string
		var n int
		_ = rows2.Scan(&p, &n)
		log.Printf("  %s: %d", p, n)
	}
}
