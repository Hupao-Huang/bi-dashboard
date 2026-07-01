// import-report-maps.exe: 导入销售日报的两张维表
//   dim_sales_channel_map  ← RPA映射表.xlsx / 销售渠道映射(A=销售渠道I店铺, C=销售渠道II渠道)
//   dim_goods_pack_spec    ← RPA映射表.xlsx / 箱规映射(货品编号→箱规) + 箱规拖规.xlsx(名称→箱规+托规)
// 用法: import-report-maps.exe [--rpa=RPA映射表.xlsx] [--pallet=箱规拖规.xlsx] [--config=config.json]
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

	"bi-dashboard/internal/importutil"
)

type Config struct {
	Database struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		User     string `json:"user"`
		Password string `json:"password"`
		Dbname   string `json:"dbname"`
	} `json:"database"`
}

func main() {
	unlock := importutil.AcquireLock("import-report-maps")
	defer unlock()

	rpaPath := flag.String("rpa", `C:\Users\Administrator\Desktop\RPA映射表.xlsx`, "含『销售渠道映射』『箱规映射』的 xlsx")
	palletPath := flag.String("pallet", `C:\Users\Administrator\Desktop\箱规拖规.xlsx`, "含 名称|箱规|托规 的 xlsx")
	configPath := flag.String("config", `C:\Users\Administrator\bi-dashboard\server\config.json`, "配置")
	flag.Parse()

	db, err := connectDB(*configPath)
	if err != nil {
		log.Fatal("连数据库失败:", err)
	}
	defer db.Close()

	if err := ensureTables(db); err != nil {
		log.Fatal("建表失败:", err)
	}
	fmt.Printf("✅ 渠道映射 upsert %d 条\n", importChannelMap(db, *rpaPath))
	fmt.Printf("✅ 箱规 upsert %d 条\n", importBoxSpec(db, *rpaPath))
	fmt.Printf("✅ 箱规托规(主力品) upsert %d 条\n", importPalletSpec(db, *palletPath))
	fmt.Println("🎯 完成")
}

func ensureTables(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS dim_sales_channel_map (
			shop_name VARCHAR(200) NOT NULL COMMENT '店铺名(= 销售渠道I,对 trade.shop_name)',
			channel VARCHAR(50) NOT NULL COMMENT '渠道(抖音/天猫/拼多多/京东/唯品会/分销/私域/线下/新零售/其它)',
			platform VARCHAR(20) NOT NULL COMMENT '平台(社媒/电商/其他)',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
			PRIMARY KEY(shop_name)
		) COMMENT='销售日报-渠道映射' COLLATE=utf8mb4_0900_ai_ci`,
		`CREATE TABLE IF NOT EXISTS dim_goods_pack_spec (
			goods_no VARCHAR(64) NOT NULL COMMENT '货品编码(对 trade_goods.goods_no / goods.goods_no)',
			box_qty DECIMAL(14,4) NULL COMMENT '箱规=每箱单瓶数',
			pallet_box_qty DECIMAL(14,4) NULL COMMENT '托规=每托箱数',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
			PRIMARY KEY(goods_no)
		) COMMENT='销售日报-箱规托规' COLLATE=utf8mb4_0900_ai_ci`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

// importChannelMap 读『销售渠道映射』(A=销售渠道I店铺, B=渠道分组, C=销售渠道II细渠道)
// importChannelMap 读『销售渠道映射』(A=销售渠道I店铺, B=渠道分组, C=销售渠道II细渠道)
// shop_name=A, channel=C, platform=platformOf(C)
// 2026-07-01 起渠道映射改吉客云驱动(前端列 sales_channel 店铺 + 业务手动配): 这里只导「吉客云 sales_channel 里还有的店铺」,
// 不再灌入吉客云已删/停用的死店铺(否则污染回来)。此 CLI 主要用于首次初始化, 日常维护走页面。
func importChannelMap(db *sql.DB, path string) int {
	// 先拉吉客云现有店铺集合
	valid := map[string]bool{}
	vrows, err := db.Query(`SELECT DISTINCT channel_name FROM sales_channel WHERE channel_name<>''`)
	if err != nil {
		log.Fatalf("查 sales_channel: %v", err)
	}
	for vrows.Next() {
		var name string
		if err := vrows.Scan(&name); err != nil {
			log.Fatalf("scan sales_channel: %v", err)
		}
		valid[name] = true
	}
	vrows.Close()

	f, err := excelize.OpenFile(path)
	if err != nil {
		log.Fatalf("打开 %s 失败: %v", path, err)
	}
	defer f.Close()
	rows, _ := f.GetRows("销售渠道映射")
	cnt, skipped := 0, 0
	for i, r := range rows {
		if i == 0 {
			continue
		}
		shop := safeCell(r, 0)
		chII := safeCell(r, 2)
		if shop == "" || chII == "" {
			continue
		}
		if !valid[shop] { // 吉客云已没有的店铺, 跳过不导
			skipped++
			continue
		}
		if _, err := db.Exec(`INSERT INTO dim_sales_channel_map(shop_name,channel,platform) VALUES(?,?,?)
			ON DUPLICATE KEY UPDATE channel=VALUES(channel), platform=VALUES(platform)`,
			shop, chII, platformOf(chII)); err != nil {
			log.Fatalf("upsert 渠道 %q: %v", shop, err)
		}
		cnt++
	}
	if skipped > 0 {
		log.Printf("渠道映射: 跳过 %d 条吉客云已没有的店铺", skipped)
	}
	return cnt
}

// importBoxSpec 读『箱规映射』(A=货品编号, B=名称, C=箱规),编码归一后 upsert box_qty
func importBoxSpec(db *sql.DB, path string) int {
	f, err := excelize.OpenFile(path)
	if err != nil {
		log.Fatalf("打开 %s 失败: %v", path, err)
	}
	defer f.Close()
	rows, _ := f.GetRows("箱规映射")
	cnt := 0
	for i, r := range rows {
		if i == 0 {
			continue
		}
		no := normalizeGoodsNo(safeCell(r, 0))
		box := safeCell(r, 2)
		if no == "" || box == "" {
			continue
		}
		if _, err := db.Exec(`INSERT INTO dim_goods_pack_spec(goods_no,box_qty) VALUES(?,?)
			ON DUPLICATE KEY UPDATE box_qty=VALUES(box_qty)`, no, box); err != nil {
			log.Fatalf("upsert 箱规 %q: %v", no, err)
		}
		cnt++
	}
	return cnt
}

// importPalletSpec 读箱规拖规(Sheet1: A=名称, B=箱规, C=托规),名称→goods_no(经 goods 表),
// **只补 pallet_box_qty(托规), 不碰 box_qty(箱规)** —— 箱规以 RPA映射表为准(跑哥定, 箱规拖规里的箱规会误覆盖)。
func importPalletSpec(db *sql.DB, path string) int {
	f, err := excelize.OpenFile(path)
	if err != nil {
		log.Fatalf("打开 %s 失败: %v", path, err)
	}
	defer f.Close()
	rows, _ := f.GetRows("Sheet1")
	cnt := 0
	for i, r := range rows {
		if i == 0 {
			continue
		}
		name := safeCell(r, 0)
		if name == "" {
			continue
		}
		pallet := safeCell(r, 2)
		if pallet == "" {
			continue
		}
		var no string
		err := db.QueryRow(`SELECT goods_no FROM goods WHERE goods_name=? AND is_delete=0 LIMIT 1`, name).Scan(&no)
		if err == sql.ErrNoRows {
			log.Printf("⚠️  托规表名称对不上 goods, 跳过: %q", name)
			continue
		} else if err != nil {
			log.Fatalf("查 goods %q: %v", name, err)
		}
		// 只写托规: 若该 goods_no 还没箱规行则插一条(箱规留 NULL, 后续 RPA映射表箱规导入会补); 有则只更托规。
		if _, err := db.Exec(`INSERT INTO dim_goods_pack_spec(goods_no,pallet_box_qty) VALUES(?,?)
			ON DUPLICATE KEY UPDATE pallet_box_qty=VALUES(pallet_box_qty)`,
			no, pallet); err != nil {
			log.Fatalf("upsert 托规 %q: %v", no, err)
		}
		cnt++
	}
	return cnt
}

func safeCell(r []string, idx int) string {
	if idx < 0 || idx >= len(r) {
		return ""
	}
	return strings.TrimSpace(r[idx])
}

func connectDB(configPath string) (*sql.DB, error) {
	bs, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(bs, &cfg); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		cfg.Database.User, cfg.Database.Password,
		cfg.Database.Host, cfg.Database.Port, cfg.Database.Dbname)
	return sql.Open("mysql", dsn)
}
