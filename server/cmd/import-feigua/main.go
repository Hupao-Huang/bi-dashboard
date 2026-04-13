package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"

	_ "github.com/go-sql-driver/mysql"
	"github.com/xuri/excelize/v2"
)

var db *sql.DB

var filePattern = regexp.MustCompile(`飞瓜_(\d{8})_飞瓜_(抖音|快手|小红书)_(达人数据|达人归属)\.xlsx$`)

func main() {
	startDate, endDate := "", ""
	if len(os.Args) >= 3 {
		startDate = os.Args[1]
		endDate = os.Args[2]
	}

	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	db, err = sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)

	dataRoot := `Z:\信息部\RPA_集团数据看板\飞瓜`
	if len(os.Args) >= 4 {
		dataRoot = os.Args[3]
	}
	dataRoot, err = importutil.ResolveDataRoot(dataRoot)
	if err != nil {
		log.Fatalf("数据目录不可用: %v", err)
	}

	years, err := os.ReadDir(dataRoot)
	if err != nil {
		log.Fatalf("读取数据目录失败: %v", err)
	}
	matchedDate := false
	for _, y := range years {
		if !y.IsDir() {
			continue
		}
		yearPath := filepath.Join(dataRoot, y.Name())
		dates, err := os.ReadDir(yearPath)
		if err != nil {
			log.Printf("读取年份目录失败 [%s]: %v", yearPath, err)
			continue
		}
		for _, d := range dates {
			if !d.IsDir() {
				continue
			}
			dateStr := d.Name()
			if startDate != "" && dateStr < startDate {
				continue
			}
			if endDate != "" && dateStr > endDate {
				continue
			}
			matchedDate = true
			// 飞瓜目录下还有一层"飞瓜"子目录
			subDir := filepath.Join(yearPath, dateStr, "飞瓜")
			if _, err := os.Stat(subDir); err != nil {
				continue
			}
			processDir(subDir, dateStr)
		}
	}
	if startDate != "" && endDate != "" && !matchedDate {
		log.Fatalf("未找到日期范围 %s-%s 的数据目录", startDate, endDate)
	}
	log.Println("飞瓜数据导入完成!")
}

func processDir(dir, dateStr string) {
	files, _ := os.ReadDir(dir)
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		matches := filePattern.FindStringSubmatch(f.Name())
		if matches == nil {
			continue
		}
		platform := matches[2]
		dataType := matches[3]
		fullPath := filepath.Join(dir, f.Name())

		var err error
		switch dataType {
		case "达人数据":
			err = importCreatorDaily(fullPath, dateStr, platform)
		case "达人归属":
			err = importCreatorRoster(fullPath, dateStr, platform)
		}

		if err != nil {
			log.Printf("导入失败 [%s]: %v", f.Name(), err)
		} else {
			log.Printf("导入成功 [%s]", f.Name())
		}
	}
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	if s == "" || s == "-" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func parseInt(s string) int {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	if s == "" || s == "-" {
		return 0
	}
	v, _ := strconv.Atoi(s)
	return v
}

func formatDate(d string) string {
	d = strings.TrimSpace(d)
	if len(d) == 8 {
		return d[:4] + "-" + d[4:6] + "-" + d[6:8]
	}
	return d
}

func ne(s string) interface{} {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" || s == "NaN" {
		return nil
	}
	return s
}

// ==================== 达人出单日数据 ====================

func importCreatorDaily(path, dateStr, platform string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("打开xlsx: %w", err)
	}
	defer f.Close()

	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil || len(rows) < 2 {
		return nil
	}

	header := rows[0]
	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.TrimSpace(h)] = i
	}

	statDate := formatDate(dateStr)
	count := 0

	for _, row := range rows[1:] {
		get := func(name string) string {
			idx, ok := colMap[name]
			if !ok || idx >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[idx])
		}

		creatorName := get("达人昵称")
		if creatorName == "" {
			continue
		}

		// 平台号字段名不同
		var creatorId string
		switch platform {
		case "抖音":
			creatorId = get("抖音号")
		case "快手":
			creatorId = get("快手号")
		case "小红书":
			creatorId = get("红书ID")
		}
		if creatorId == "" {
			creatorId = get("UID")
		}

		// 跟进人字段名很长
		follower := ""
		for k, idx := range colMap {
			if strings.Contains(k, "跟进人") && idx < len(row) {
				follower = strings.TrimSpace(row[idx])
				break
			}
		}

		_, err := db.Exec(`REPLACE INTO fg_creator_daily (
			stat_date, platform, creator_name, creator_id, uid,
			product_count, order_count, gmv, actual_orders,
			commission_base, commission, actual_amount, refund_orders, follower
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			statDate, platform, creatorName, ne(creatorId), ne(get("UID")),
			parseInt(get("出单商品数")), parseInt(get("成交订单数")),
			parseFloat(get("成交金额")), parseInt(get("实际订单数")),
			parseFloat(get("计佣金额")), parseFloat(get("支出佣金")),
			parseFloat(get("实际支付金额")), parseInt(get("退货订单数")),
			ne(follower),
		)
		if err != nil {
			log.Printf("  达人数据插入失败[%s]: %v", creatorName, err)
			continue
		}
		count++
	}
	log.Printf("  %s达人数据: %d条", platform, count)
	return nil
}

// ==================== 达人资源库 ====================

func importCreatorRoster(path, dateStr, platform string) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("打开xlsx: %w", err)
	}
	defer f.Close()

	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil || len(rows) < 2 {
		return nil
	}

	header := rows[0]
	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.TrimSpace(h)] = i
	}

	statDate := formatDate(dateStr)
	count := 0

	for _, row := range rows[1:] {
		get := func(name string) string {
			idx, ok := colMap[name]
			if !ok || idx >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[idx])
		}

		creatorName := get("达人昵称")
		if creatorName == "" {
			continue
		}

		var creatorId string
		switch platform {
		case "抖音":
			creatorId = get("抖音号")
		case "快手":
			creatorId = get("快手号")
		case "小红书":
			creatorId = get("小红书号")
		}
		if creatorId == "" {
			creatorId = get("UID")
		}

		// 标签：抖音有普通标签+数据标签，快手小红书只有标签
		tags := get("标签")
		if tags == "" {
			pt := get("普通标签")
			dt := get("数据标签")
			if pt != "" && dt != "" {
				tags = pt + "," + dt
			} else if pt != "" {
				tags = pt
			} else {
				tags = dt
			}
		}

		_, err := db.Exec(`INSERT INTO fg_creator_roster (
			stat_date, platform, creator_name, creator_id, uid, fans_count,
			resource_type, tags, contact_status, contact_name, sample_count,
			total_gmv, total_products, follower, follower_gmv,
			claim_time, contact_time, created_time
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
			stat_date=VALUES(stat_date), creator_name=VALUES(creator_name),
			fans_count=VALUES(fans_count), tags=VALUES(tags),
			contact_status=VALUES(contact_status), sample_count=VALUES(sample_count),
			total_gmv=VALUES(total_gmv), total_products=VALUES(total_products),
			follower=VALUES(follower), follower_gmv=VALUES(follower_gmv)`,
			statDate, platform, creatorName, ne(creatorId), ne(get("UID")),
			ne(get("粉丝数")), ne(get("资源类型")), ne(tags),
			ne(get("建联状态")), ne(get("联系人")),
			parseInt(get("累计寄样数")),
			parseFloat(get("累计GMV")), parseInt(get("累计成交商品")),
			ne(get("跟进人")), parseFloat(get("跟进人累计GMV")),
			ne(get("认领时间")), ne(get("建联时间")), ne(get("创建时间")),
		)
		if err != nil {
			log.Printf("  达人归属插入失败[%s]: %v", creatorName, err)
			continue
		}
		count++
	}
	log.Printf("  %s达人归属: %d条", platform, count)
	return nil
}
