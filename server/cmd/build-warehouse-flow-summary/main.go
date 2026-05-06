// build-warehouse-flow-summary 物化预聚合表构建
//
// 用法：
//   ./build-warehouse-flow-summary --ym=2026-04             # 单月
//   ./build-warehouse-flow-summary --start=2025-01 --end=2026-04  # 月份范围
//   ./build-warehouse-flow-summary --all                     # 全部 trade_YYYYMM 表
//   ./build-warehouse-flow-summary                           # 默认当月（每日定时跑）
//
// 策略: 单月 DELETE WHERE ym=? + INSERT (幂等)
// SQL 与 handler 中 GetWarehouseFlowOverview/Matrix 完全对齐(口径一致)
package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// planWarehouses 与 server/internal/handler/supply_chain.go 保持一致
// 改这里要同步改 supply_chain.go
var planWarehouses = []string{
	"南京委外成品仓-公司仓-委外",
	"天津委外仓-公司仓-外仓",
	"西安仓库成品-公司仓-外仓",
	"松鲜鲜&大地密码云仓",
	"长沙委外成品仓-公司仓-外仓",
	"安徽郎溪成品-公司仓-自营",
	"南京分销虚拟仓-公司仓-外仓",
}

// provinceNormSQL 与 server/internal/handler/warehouse_flow.go 一致
const provinceNormSQL = `CASE
	WHEN t.state IN ('北京','北京市') THEN '北京市'
	WHEN t.state IN ('上海','上海市') THEN '上海市'
	WHEN t.state IN ('天津','天津市') THEN '天津市'
	WHEN t.state IN ('重庆','重庆市') THEN '重庆市'
	WHEN t.state LIKE '广东%' THEN '广东省'
	WHEN t.state LIKE '广西%' THEN '广西壮族自治区'
	WHEN t.state LIKE '宁夏%' THEN '宁夏回族自治区'
	WHEN t.state LIKE '内蒙古%' THEN '内蒙古自治区'
	WHEN t.state LIKE '新疆%' THEN '新疆维吾尔自治区'
	WHEN t.state LIKE '西藏%' THEN '西藏自治区'
	WHEN t.state LIKE '香港%' THEN '香港特别行政区'
	WHEN t.state LIKE '澳门%' THEN '澳门特别行政区'
	WHEN t.state LIKE '台湾%' THEN '台湾省'
	WHEN t.state IN ('江苏','江苏省') THEN '江苏省'
	WHEN t.state IN ('浙江','浙江省') THEN '浙江省'
	WHEN t.state IN ('山东','山东省') THEN '山东省'
	WHEN t.state IN ('福建','福建省') THEN '福建省'
	WHEN t.state IN ('湖南','湖南省') THEN '湖南省'
	WHEN t.state IN ('湖北','湖北省') THEN '湖北省'
	WHEN t.state IN ('河南','河南省') THEN '河南省'
	WHEN t.state IN ('河北','河北省') THEN '河北省'
	WHEN t.state IN ('山西','山西省') THEN '山西省'
	WHEN t.state IN ('陕西','陕西省') THEN '陕西省'
	WHEN t.state IN ('四川','四川省') THEN '四川省'
	WHEN t.state IN ('安徽','安徽省') THEN '安徽省'
	WHEN t.state IN ('江西','江西省') THEN '江西省'
	WHEN t.state IN ('辽宁','辽宁省') THEN '辽宁省'
	WHEN t.state IN ('吉林','吉林省') THEN '吉林省'
	WHEN t.state IN ('黑龙江','黑龙江省') THEN '黑龙江省'
	WHEN t.state IN ('云南','云南省') THEN '云南省'
	WHEN t.state IN ('贵州','贵州省') THEN '贵州省'
	WHEN t.state IN ('甘肃','甘肃省') THEN '甘肃省'
	WHEN t.state IN ('青海','青海省') THEN '青海省'
	WHEN t.state IN ('海南','海南省') THEN '海南省'
	ELSE t.state
END`

func main() {
	ymFlag := flag.String("ym", "", "单月 YYYY-MM")
	startFlag := flag.String("start", "", "起始月 YYYY-MM")
	endFlag := flag.String("end", "", "结束月 YYYY-MM")
	allFlag := flag.Bool("all", false, "全部 trade_YYYYMM 表")
	flag.Parse()

	unlock := importutil.AcquireLock("build-warehouse-flow-summary")
	defer unlock()

	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()

	yms := resolveYms(db, *ymFlag, *startFlag, *endFlag, *allFlag)
	if len(yms) == 0 {
		log.Fatalf("没有要处理的月份")
	}

	fmt.Printf("将构建物化表: %v\n\n", yms)
	totalRows := int64(0)
	for _, ym := range yms {
		rows, dur := buildOne(db, ym)
		fmt.Printf("[%s] %d 行, 耗时 %v\n", ym, rows, dur)
		totalRows += rows
	}
	fmt.Printf("\n总计: %d 行\n", totalRows)
}

// resolveYms 决定要处理哪些月份
func resolveYms(db *sql.DB, single, start, end string, all bool) []string {
	if all {
		return listAllTradeMonths(db)
	}
	if start != "" && end != "" {
		return monthRange(start, end)
	}
	if single != "" {
		return []string{single}
	}
	// 默认: 当月
	return []string{time.Now().Format("2006-01")}
}

func listAllTradeMonths(db *sql.DB) []string {
	rs, err := db.Query(`SELECT TABLE_NAME FROM information_schema.TABLES
		WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME REGEXP '^trade_[0-9]{6}$'
		ORDER BY TABLE_NAME`)
	if err != nil {
		log.Fatalf("列表 trade 表失败: %v", err)
	}
	defer rs.Close()
	var months []string
	for rs.Next() {
		var n string
		rs.Scan(&n)
		s := strings.TrimPrefix(n, "trade_")
		if len(s) == 6 {
			months = append(months, s[:4]+"-"+s[4:])
		}
	}
	return months
}

func monthRange(start, end string) []string {
	t1, err := time.Parse("2006-01", start)
	if err != nil {
		log.Fatalf("起始月格式错误: %v", err)
	}
	t2, err := time.Parse("2006-01", end)
	if err != nil {
		log.Fatalf("结束月格式错误: %v", err)
	}
	var months []string
	for !t1.After(t2) {
		months = append(months, t1.Format("2006-01"))
		t1 = t1.AddDate(0, 1, 0)
	}
	return months
}

// buildOne 构建单月物化数据(DELETE+INSERT 幂等)
func buildOne(db *sql.DB, ym string) (int64, time.Duration) {
	tStart := time.Now()
	t, err := time.Parse("2006-01", ym)
	if err != nil {
		log.Fatalf("[%s] 月份格式错误: %v", ym, err)
	}
	yyyymm := t.Format("200601")
	tradeT := "trade_" + yyyymm
	pkgT := "trade_package_" + yyyymm

	// 检查源表存在
	var exists int
	db.QueryRow(`SELECT COUNT(*) FROM information_schema.TABLES
		WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?`, tradeT).Scan(&exists)
	if exists == 0 {
		log.Fatalf("[%s] 源表 %s 不存在", ym, tradeT)
	}

	// 7 仓白名单 placeholders
	whPh := strings.Repeat("?,", len(planWarehouses))
	whPh = whPh[:len(whPh)-1]
	args := make([]interface{}, 0, len(planWarehouses)+1)
	args = append(args, ym)
	for _, w := range planWarehouses {
		args = append(args, w)
	}

	// DELETE 旧数据
	_, err = db.Exec(`DELETE FROM warehouse_flow_summary WHERE ym = ?`, ym)
	if err != nil {
		log.Fatalf("[%s] 删除旧数据失败: %v", ym, err)
	}

	// 聚合 INSERT
	insSQL := fmt.Sprintf(`
		INSERT INTO warehouse_flow_summary
			(ym, shop_name, warehouse_name, province, orders, packages)
		SELECT
			? AS ym,
			IFNULL(t.shop_name, '') AS shop_name,
			t.warehouse_name,
			(%s) AS province,
			COUNT(DISTINCT t.trade_id) AS orders,
			COUNT(DISTINCT CONCAT(t.trade_id, '|', p.logistic_no)) AS packages
		FROM %s t
		LEFT JOIN %s p ON p.trade_id = t.trade_id
		WHERE t.trade_status_explain NOT LIKE '%%取消%%'
		  AND t.state IS NOT NULL AND t.state != ''
		  AND t.trade_type NOT IN (8, 12)
		  AND t.warehouse_name IN (%s)
		GROUP BY shop_name, warehouse_name, province`,
		provinceNormSQL, tradeT, pkgT, whPh)

	res, err := db.Exec(insSQL, args...)
	if err != nil {
		log.Fatalf("[%s] 聚合插入失败: %v", ym, err)
	}
	rows, _ := res.RowsAffected()
	return rows, time.Since(tStart)
}
