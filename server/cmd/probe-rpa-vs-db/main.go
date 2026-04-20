package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"bi-dashboard/internal/config"

	_ "github.com/go-sql-driver/mysql"
)

// 文件名规则：平台_YYYYMMDD_店铺_类型描述.ext
var filePattern = regexp.MustCompile(`^([^_]+)_(\d{8})_(.+?)_(.+?)\.(xlsx|xls|json|csv)$`)

type fileKey struct {
	platform string
	filetype string
}

type fileBucket struct {
	count   int
	dates   map[string]bool
	shops   map[string]bool
	example string
}

func main() {
	cfg, err := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	if err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	buckets := map[fileKey]*fileBucket{}

	for _, plat := range []string{"京东", "京东自营", "抖音", "抖音分销", "拼多多", "唯品会", "天猫超市", "天猫", "快手", "小红书", "飞瓜"} {
		root := filepath.Join(`Z:\信息部\RPA_集团数据看板`, plat)
		if _, err := os.Stat(root); err != nil {
			continue
		}
		filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			name := d.Name()
			m := filePattern.FindStringSubmatch(name)
			if m == nil {
				return nil
			}
			plat2 := m[1]
			dateStr := m[2]
			// m[3] 可能包含 "_"，不一定全是店铺名，需要根据目录判断
			shop := filepath.Base(filepath.Dir(path))
			// m[3]+m[4] 里包含 shop 和文件类型 — shop是父目录名
			// 文件类型 = 去除 shop_ 前缀后的剩余部分
			remainAll := m[3] + "_" + m[4]
			ft := remainAll
			if strings.HasPrefix(remainAll, shop+"_") {
				ft = strings.TrimPrefix(remainAll, shop+"_")
			}
			// 只取扩展名之前
			key := fileKey{platform: plat2, filetype: ft}
			if buckets[key] == nil {
				buckets[key] = &fileBucket{
					dates: map[string]bool{},
					shops: map[string]bool{},
					example: name,
				}
			}
			buckets[key].count++
			sqlDate := dateStr[:4] + "-" + dateStr[4:6] + "-" + dateStr[6:8]
			buckets[key].dates[sqlDate] = true
			buckets[key].shops[shop] = true
			return nil
		})
	}

	// 查所有 op_* 和 fg_* 表
	tblRows, _ := db.Query(`SELECT TABLE_NAME FROM information_schema.TABLES
		WHERE TABLE_SCHEMA='bi_dashboard'
		  AND TABLE_NAME NOT LIKE '\_unused%'
		  AND (TABLE_NAME LIKE 'op\_%' OR TABLE_NAME LIKE 'fg\_%')
		ORDER BY TABLE_NAME`)
	tables := []string{}
	for tblRows.Next() {
		var t string
		tblRows.Scan(&t)
		tables = append(tables, t)
	}
	tblRows.Close()

	type tblStat struct {
		rows    int
		dates   int
		minDate string
		maxDate string
		shops   int
	}
	tblStats := map[string]tblStat{}
	for _, t := range tables {
		var st tblStat
		var minD, maxD *string
		// 所有 op_ 表都有 stat_date
		_ = db.QueryRow(fmt.Sprintf("SELECT COUNT(*),COUNT(DISTINCT stat_date),MIN(stat_date),MAX(stat_date) FROM %s", t)).Scan(&st.rows, &st.dates, &minD, &maxD)
		if minD != nil {
			st.minDate = (*minD)[:10]
		}
		if maxD != nil {
			st.maxDate = (*maxD)[:10]
		}
		// 店铺列：大部分用 shop_name，抖音分销用 account_name
		col := "shop_name"
		if strings.Contains(t, "douyin_dist") {
			col = "account_name"
		}
		db.QueryRow(fmt.Sprintf("SELECT COUNT(DISTINCT %s) FROM %s", col, t)).Scan(&st.shops)
		tblStats[t] = st
	}

	fmt.Println("===== RPA 文件类型分布 =====")
	fmt.Printf("%-10s %-50s %8s %8s %5s  %s\n", "平台", "文件类型", "文件数", "日期数", "店铺", "示例")
	keys := []fileKey{}
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].platform != keys[j].platform {
			return keys[i].platform < keys[j].platform
		}
		return keys[i].filetype < keys[j].filetype
	})
	for _, k := range keys {
		b := buckets[k]
		fmt.Printf("%-10s %-50s %8d %8d %5d  %s\n", k.platform, k.filetype, b.count, len(b.dates), len(b.shops), b.example)
	}

	fmt.Println("\n===== 数据库表统计 =====")
	fmt.Printf("%-40s %10s %8s %12s %12s %8s\n", "表名", "行数", "日期数", "最早", "最新", "店铺数")
	for _, t := range tables {
		st := tblStats[t]
		fmt.Printf("%-40s %10d %8d %12s %12s %8d\n", t, st.rows, st.dates, st.minDate, st.maxDate, st.shops)
	}
}
