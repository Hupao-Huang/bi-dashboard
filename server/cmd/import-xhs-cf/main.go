package main

// 小红书乘风(信息流投流)数据导入器。
// 目录: Z:\信息部\RPA_集团数据看板\小红书乘风\<年>\<YYYYMMDD>\<店铺>\
//   小红书乘风_<日期>_<店铺>_笔记_标准投数据.csv  (同名还有 .xlsx，只读 csv)
// 口径：
//   - stat_date 取行内"时间"列(业务日)，不是目录日(导出日，投流数据有延迟)。
//   - 首行"合计NNN条记录"汇总行跳过(笔记/素材ID="-")。
//   - 率类带"%"，去掉后存数值(如 13.83 表示 13.83%)。
//   - 用 encoding/csv 解析(标题含逗号也不怕)。
// 用法: import-xhs-cf.exe [startDate endDate]  日期 YYYYMMDD(按目录日过滤)，不传则全量。

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"database/sql"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var baseDir = `Z:\信息部\RPA_集团数据看板\小红书乘风`

func main() {
	unlock := importutil.AcquireLock("import-xhs-cf")
	defer unlock()

	cfg, err := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()

	if err := ensureTable(db); err != nil {
		log.Fatalf("建表失败: %v", err)
	}

	startDate, endDate := "", ""
	if len(os.Args) >= 3 {
		startDate, endDate = os.Args[1], os.Args[2]
	}
	root, err := importutil.ResolveDataRoot(baseDir)
	if err != nil {
		log.Fatalf("数据目录不可用: %v", err)
	}

	total := 0
	yearEntries, err := os.ReadDir(root)
	if err != nil {
		log.Fatalf("读取小红书乘风目录失败: %v", err)
	}
	for _, ye := range yearEntries {
		if !ye.IsDir() || len(ye.Name()) != 4 {
			continue
		}
		yearPath := filepath.Join(root, ye.Name())
		dateDirs, _ := os.ReadDir(yearPath)
		for _, dd := range dateDirs {
			if !dd.IsDir() || len(dd.Name()) != 8 {
				continue
			}
			dirDate := dd.Name()
			if startDate != "" && dirDate < startDate {
				continue
			}
			if endDate != "" && dirDate > endDate {
				continue
			}
			fileDate := dirDate[:4] + "-" + dirDate[4:6] + "-" + dirDate[6:8]
			datePath := filepath.Join(yearPath, dirDate)
			shopDirs, _ := os.ReadDir(datePath)
			for _, sd := range shopDirs {
				if !sd.IsDir() {
					continue
				}
				shopName := sd.Name()
				shopPath := filepath.Join(datePath, shopName)
				files, _ := os.ReadDir(shopPath)
				for _, fl := range files {
					if fl.IsDir() {
						continue
					}
					name := fl.Name()
					if strings.HasPrefix(name, "~$") {
						continue
					}
					if !strings.HasSuffix(name, ".csv") || !strings.Contains(name, "_笔记_标准投数据") {
						continue
					}
					total += importFile(db, filepath.Join(shopPath, name), fileDate, shopName)
				}
			}
		}
	}
	fmt.Printf("\n小红书乘风导入完成: %d 条\n", total)
}

func ensureTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS op_xhs_chengfeng_daily (
		id BIGINT PRIMARY KEY AUTO_INCREMENT,
		stat_date DATE NOT NULL COMMENT '业务日期(取CSV时间列)',
		file_date DATE NOT NULL COMMENT '导出文件日期(目录日,投流数据有延迟)',
		shop_name VARCHAR(128) NOT NULL COMMENT '店铺名(目录名)',
		note_id VARCHAR(64) NOT NULL COMMENT '笔记/素材ID',
		note_url VARCHAR(500) DEFAULT '' COMMENT '笔记/素材链接',
		note_title VARCHAR(500) DEFAULT '' COMMENT '笔记标题',
		cost DECIMAL(14,2) DEFAULT 0 COMMENT '消费',
		impression BIGINT DEFAULT 0 COMMENT '展现量',
		click_rate DECIMAL(12,4) DEFAULT 0 COMMENT '点击率(%)',
		video_5s_finish_rate DECIMAL(12,4) DEFAULT 0 COMMENT '视频5s完播率(%)',
		click_count BIGINT DEFAULT 0 COMMENT '点击量',
		shop_newcust_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '店铺新客支付金额',
		avg_click_cost DECIMAL(12,4) DEFAULT 0 COMMENT '平均点击成本',
		avg_cpm DECIMAL(12,4) DEFAULT 0 COMMENT '平均千次展示费用',
		newcust_cost DECIMAL(14,2) DEFAULT 0 COMMENT '新客消耗',
		newcust_impression BIGINT DEFAULT 0 COMMENT '新客曝光',
		newcust_click BIGINT DEFAULT 0 COMMENT '新客点击',
		newcust_click_rate DECIMAL(12,4) DEFAULT 0 COMMENT '新客点击率(%)',
		newcust_avg_click_cost DECIMAL(12,4) DEFAULT 0 COMMENT '新客平均点击成本',
		newcust_avg_cpm DECIMAL(12,4) DEFAULT 0 COMMENT '新客平均千次展示费用',
		platform_subsidy_amount DECIMAL(14,2) DEFAULT 0 COMMENT '平台额外补贴金额',
		subsidy_driven_gmv DECIMAL(14,2) DEFAULT 0 COMMENT '补贴撬动全店交易额',
		live_direct_order_count BIGINT DEFAULT 0 COMMENT '直播间直接下单订单量',
		live_direct_order_cost DECIMAL(12,4) DEFAULT 0 COMMENT '直播间直接下单订单成本',
		live_direct_order_amount DECIMAL(14,2) DEFAULT 0 COMMENT '直播间直接下单金额',
		live_direct_order_roi DECIMAL(12,4) DEFAULT 0 COMMENT '直播间直接下单ROI',
		live_direct_pay_order_count BIGINT DEFAULT 0 COMMENT '直播间直接支付订单量',
		live_direct_pay_order_cost DECIMAL(12,4) DEFAULT 0 COMMENT '直播间直接支付订单成本',
		live_direct_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '直播间直接支付金额',
		live_direct_pay_roi DECIMAL(12,4) DEFAULT 0 COMMENT '直播间直接支付ROI',
		live_view_count BIGINT DEFAULT 0 COMMENT '直播间观看次数',
		live_view_cost DECIMAL(12,4) DEFAULT 0 COMMENT '直播间观看成本',
		live_avg_stay_duration DECIMAL(12,4) DEFAULT 0 COMMENT '直播间平均停留时长',
		live_new_fans BIGINT DEFAULT 0 COMMENT '直播间新增粉丝数量',
		live_5s_view_count BIGINT DEFAULT 0 COMMENT '直播间5s观看次数',
		live_5s_view_cost DECIMAL(12,4) DEFAULT 0 COMMENT '直播间5s观看成本',
		live_comment_count BIGINT DEFAULT 0 COMMENT '直播间评论次数',
		live_30s_view_count BIGINT DEFAULT 0 COMMENT '直播间30s观看次数',
		live_30s_view_cost DECIMAL(12,4) DEFAULT 0 COMMENT '直播间30s观看成本',
		goods_1d_pay_order_count BIGINT DEFAULT 0 COMMENT '商品1日支付订单量',
		goods_1d_pay_order_cost DECIMAL(12,4) DEFAULT 0 COMMENT '商品1日支付订单成本',
		goods_1d_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '商品1日支付金额',
		goods_1d_pay_roi DECIMAL(12,4) DEFAULT 0 COMMENT '商品1日支付ROI',
		order_7d_count BIGINT DEFAULT 0 COMMENT '7日总下单订单量',
		order_7d_cost DECIMAL(12,4) DEFAULT 0 COMMENT '7日总下单订单成本',
		order_7d_amount DECIMAL(14,2) DEFAULT 0 COMMENT '7日总下单金额',
		order_7d_roi DECIMAL(12,4) DEFAULT 0 COMMENT '7日总下单ROI',
		pay_7d_order_count BIGINT DEFAULT 0 COMMENT '7日总支付订单量',
		pay_7d_order_cost DECIMAL(12,4) DEFAULT 0 COMMENT '7日总支付订单成本',
		pay_7d_amount DECIMAL(14,2) DEFAULT 0 COMMENT '7日总支付金额',
		pay_7d_roi DECIMAL(12,4) DEFAULT 0 COMMENT '7日总支付ROI',
		first_screen_impression BIGINT DEFAULT 0 COMMENT '首屏展现',
		first_screen_click BIGINT DEFAULT 0 COMMENT '首屏点击',
		first_screen_cost DECIMAL(14,2) DEFAULT 0 COMMENT '首屏消耗',
		first_screen_impression_pct DECIMAL(12,4) DEFAULT 0 COMMENT '首屏展现占比(%)',
		first_screen_click_pct DECIMAL(12,4) DEFAULT 0 COMMENT '首屏点击占比(%)',
		first_screen_cost_pct DECIMAL(12,4) DEFAULT 0 COMMENT '首屏消耗占比(%)',
		pay_7d_conv_rate DECIMAL(12,4) DEFAULT 0 COMMENT '7日支付转化率(%)',
		shop_newcust_repurchase_order_count BIGINT DEFAULT 0 COMMENT '店铺新客首购及30日复购支付订单量',
		shop_newcust_repurchase_amount DECIMAL(14,2) DEFAULT 0 COMMENT '店铺新客首购及30日复购支付金额',
		shop_newcust_repurchase_roi DECIMAL(12,4) DEFAULT 0 COMMENT '店铺新客首购及30日复购支付ROI',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		UNIQUE KEY uk_xhs_cf (stat_date, shop_name, note_id),
		KEY idx_xhs_cf_date (stat_date)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='小红书乘风信息流投流数据(每日每店每笔记)'`)
	return err
}

func importFile(db *sql.DB, path, fileDate, shopName string) int {
	f, err := os.Open(path)
	if err != nil {
		log.Printf("打开失败 %s: %v", filepath.Base(path), err)
		return 0
	}
	defer f.Close()

	rd := csv.NewReader(f)
	rd.FieldsPerRecord = -1 // 容忍行列数差异
	rows, err := rd.ReadAll()
	if err != nil {
		log.Printf("解析CSV失败 %s: %v", filepath.Base(path), err)
		return 0
	}
	if len(rows) < 2 {
		return 0
	}
	colMap := make(map[string]int)
	for i, h := range rows[0] {
		// 剥掉 UTF-8 BOM(EF BB BF), 否则首列表头键会带 BOM 匹配不上
		if len(h) >= 3 && h[0] == 0xEF && h[1] == 0xBB && h[2] == 0xBF {
			h = h[3:]
		}
		colMap[strings.TrimSpace(h)] = i
	}

	count := 0
	for _, row := range rows[1:] {
		get := func(name string) string {
			idx, ok := colMap[name]
			if !ok || idx >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[idx])
		}
		// 去掉百分号再解析数值
		getF := func(name string) float64 {
			return importutil.ParseFloat(strings.TrimSuffix(get(name), "%"))
		}
		getI := func(name string) int64 {
			return int64(getF(name)) // 计数列可能带 .0，统一过 float 再截断
		}

		// 跳过"合计"汇总行 + 业务日不是合法日期的行
		t := get("时间")
		bizDate, err := time.Parse("2006-01-02", t)
		if err != nil {
			continue
		}
		noteID := get("笔记/素材ID")
		if noteID == "" || noteID == "-" {
			continue
		}
		statDate := bizDate.Format("2006-01-02")

		if _, err := db.Exec(`REPLACE INTO op_xhs_chengfeng_daily (
			stat_date, file_date, shop_name, note_id, note_url, note_title,
			cost, impression, click_rate, video_5s_finish_rate, click_count,
			shop_newcust_pay_amount, avg_click_cost, avg_cpm, newcust_cost, newcust_impression,
			newcust_click, newcust_click_rate, newcust_avg_click_cost, newcust_avg_cpm,
			platform_subsidy_amount, subsidy_driven_gmv,
			live_direct_order_count, live_direct_order_cost, live_direct_order_amount, live_direct_order_roi,
			live_direct_pay_order_count, live_direct_pay_order_cost, live_direct_pay_amount, live_direct_pay_roi,
			live_view_count, live_view_cost, live_avg_stay_duration, live_new_fans,
			live_5s_view_count, live_5s_view_cost, live_comment_count, live_30s_view_count, live_30s_view_cost,
			goods_1d_pay_order_count, goods_1d_pay_order_cost, goods_1d_pay_amount, goods_1d_pay_roi,
			order_7d_count, order_7d_cost, order_7d_amount, order_7d_roi,
			pay_7d_order_count, pay_7d_order_cost, pay_7d_amount, pay_7d_roi,
			first_screen_impression, first_screen_click, first_screen_cost,
			first_screen_impression_pct, first_screen_click_pct, first_screen_cost_pct,
			pay_7d_conv_rate, shop_newcust_repurchase_order_count, shop_newcust_repurchase_amount, shop_newcust_repurchase_roi
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			statDate, fileDate, shopName, noteID, get("笔记/素材链接"), get("笔记标题"),
			getF("消费"), getI("展现量"), getF("点击率"), getF("视频5s完播率"), getI("点击量"),
			getF("店铺新客支付金额"), getF("平均点击成本"), getF("平均千次展示费用"), getF("新客消耗"), getI("新客曝光"),
			getI("新客点击"), getF("新客点击率"), getF("新客平均点击成本"), getF("新客平均千次展示费用"),
			getF("平台额外补贴金额"), getF("补贴撬动全店交易额"),
			getI("直播间直接下单订单量"), getF("直播间直接下单订单成本"), getF("直播间直接下单金额"), getF("直播间直接下单ROI"),
			getI("直播间直接支付订单量"), getF("直播间直接支付订单成本"), getF("直播间直接支付金额"), getF("直播间直接支付ROI"),
			getI("直播间观看次数"), getF("直播间观看成本"), getF("直播间平均停留时长"), getI("直播间新增粉丝数量"),
			getI("直播间5s观看次数"), getF("直播间5s观看成本"), getI("直播间评论次数"), getI("直播间30s观看次数"), getF("直播间30s观看成本"),
			getI("商品1日支付订单量"), getF("商品1日支付订单成本"), getF("商品1日支付金额"), getF("商品1日支付ROI"),
			getI("7日总下单订单量"), getF("7日总下单订单成本"), getF("7日总下单金额"), getF("7日总下单ROI"),
			getI("7日总支付订单量"), getF("7日总支付订单成本"), getF("7日总支付金额"), getF("7日总支付ROI"),
			getI("首屏展现"), getI("首屏点击"), getF("首屏消耗"),
			getF("首屏展现占比"), getF("首屏点击占比"), getF("首屏消耗占比"),
			getF("7日支付转化率"), getI("店铺新客首购及30日复购支付订单量"), getF("店铺新客首购及30日复购支付金额"), getF("店铺新客首购及30日复购支付ROI"),
		); err != nil {
			log.Printf("写入失败 shop=%s note=%s: %v", shopName, noteID, err)
			continue
		}
		count++
	}
	return count
}
