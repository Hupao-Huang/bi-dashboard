// diff-finance.exe <xlsx_path>
// 解析新 xlsx 并与数据库现有数据对比，输出差异报告（不入库）
package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"sort"

	"bi-dashboard/internal/config"
	"bi-dashboard/internal/finance"

	_ "github.com/go-sql-driver/mysql"
)

type rowKey struct {
	Year        int
	Month       int
	Department  string
	SubjectCode string
}

func (k rowKey) String() string {
	return fmt.Sprintf("%d-%02d %s/%s", k.Year, k.Month, k.Department, k.SubjectCode)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: diff-finance.exe <xlsx_path>")
		os.Exit(2)
	}
	fpath := os.Args[1]
	year := finance.ParseYearFromFilename(fpath)
	if year == 0 {
		log.Fatalf("无法从文件名推断年份")
	}

	cfg, err := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	dict, err := finance.LoadSubjectDict(db)
	if err != nil {
		log.Fatalf("dict: %v", err)
	}

	result, err := finance.ParseFile(fpath, year, dict)
	if err != nil {
		log.Fatalf("parse: %v", err)
	}
	fmt.Printf("\n📋 文件: %s\n", fpath)
	fmt.Printf("📅 年份: %d, sheet: %d, rows: %d, 未映射: %d\n",
		year, result.SheetCount, result.RowCount, len(result.UnmappedSubjects))

	rows, err := db.Query(`SELECT month, department, subject_code, subject_name, amount
		FROM finance_report WHERE year = ?`, year)
	if err != nil {
		log.Fatalf("query: %v", err)
	}
	defer rows.Close()

	dbMap := map[rowKey]float64{}
	dbName := map[rowKey]string{}
	for rows.Next() {
		var k rowKey
		var amt float64
		var name string
		k.Year = year
		if err := rows.Scan(&k.Month, &k.Department, &k.SubjectCode, &name, &amt); err != nil {
			continue
		}
		dbMap[k] = amt
		dbName[k] = name
	}

	newMap := map[rowKey]float64{}
	newName := map[rowKey]string{}
	for _, r := range result.Rows {
		k := rowKey{r.Year, r.Month, r.Department, r.SubjectCode}
		newMap[k] = r.Amount
		newName[k] = r.SubjectName
	}

	type diff struct {
		k       rowKey
		op      string
		oldAmt  float64
		newAmt  float64
		subject string
	}
	var diffs []diff
	added, changed, removed, unchanged := 0, 0, 0, 0

	for k, nv := range newMap {
		if ov, ok := dbMap[k]; !ok {
			added++
			diffs = append(diffs, diff{k, "新增", 0, nv, newName[k]})
		} else if absFloat(ov-nv) > 0.005 {
			changed++
			diffs = append(diffs, diff{k, "修改", ov, nv, newName[k]})
		} else {
			unchanged++
		}
	}
	for k, ov := range dbMap {
		if _, ok := newMap[k]; !ok {
			removed++
			diffs = append(diffs, diff{k, "删除", ov, 0, dbName[k]})
		}
	}

	fmt.Printf("\n=== %d 年差异汇总 ===\n", year)
	fmt.Printf("✅ 未变: %d\n", unchanged)
	fmt.Printf("🆕 新增: %d\n", added)
	fmt.Printf("✏️  修改: %d\n", changed)
	fmt.Printf("🗑️  删除: %d\n", removed)

	if len(diffs) == 0 {
		fmt.Println("\n🎯 无任何差异")
		return
	}

	sort.Slice(diffs, func(i, j int) bool {
		a, b := diffs[i].k, diffs[j].k
		if a.Department != b.Department {
			return a.Department < b.Department
		}
		if a.Month != b.Month {
			return a.Month < b.Month
		}
		return a.SubjectCode < b.SubjectCode
	})

	fmt.Printf("\n=== 详细差异（前 80 条）===\n")
	limit := len(diffs)
	if limit > 80 {
		limit = 80
	}
	for i := 0; i < limit; i++ {
		d := diffs[i]
		switch d.op {
		case "新增":
			fmt.Printf("[新增] %s %s = %.2f\n", d.k, d.subject, d.newAmt)
		case "删除":
			fmt.Printf("[删除] %s %s = %.2f\n", d.k, d.subject, d.oldAmt)
		case "修改":
			delta := d.newAmt - d.oldAmt
			fmt.Printf("[修改] %s %s: %.2f → %.2f (Δ %+.2f)\n", d.k, d.subject, d.oldAmt, d.newAmt, delta)
		}
	}
	if len(diffs) > 80 {
		fmt.Printf("\n…还有 %d 条差异未显示\n", len(diffs)-80)
	}

	deptMonthDelta := map[string]float64{}
	for _, d := range diffs {
		if d.op == "修改" {
			key := fmt.Sprintf("%s / %d月", d.k.Department, d.k.Month)
			deptMonthDelta[key] += (d.newAmt - d.oldAmt)
		}
	}
	if len(deptMonthDelta) > 0 {
		fmt.Printf("\n=== 按部门×月份 净变化（仅修改）===\n")
		var keys []string
		for k := range deptMonthDelta {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("  %s : Δ %+.2f\n", k, deptMonthDelta[k])
		}
	}
}

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
