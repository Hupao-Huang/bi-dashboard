package main

// 小红书运营数据导入器：笔记数据明细 + 商品销售明细。
// 目录结构: Z:\信息部\RPA_集团数据看板\小红书\<年>\<YYYYMMDD>\<店铺>\
//   小红书_<日期>_<店铺>_笔记_数据明细.xlsx
//   小红书_<日期>_<店铺>_商品_销售明细.xlsx
// 注意：这两类文件是"某天的全量快照"，数据行里没有逐行业务日期，
//       所以 stat_date 取目录/文件名日期（与抖音商品日报同款做法）。
// 用法: import-xhs.exe [startDate endDate]   日期格式 YYYYMMDD，不传则全量扫描。

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/xuri/excelize/v2"
)

var baseDir = `Z:\信息部\RPA_集团数据看板\小红书`

func main() {
	unlock := importutil.AcquireLock("import-xhs")
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

	if err := ensureTables(db); err != nil {
		log.Fatalf("建表失败: %v", err)
	}

	startDate, endDate := "", ""
	if len(os.Args) >= 3 {
		startDate, endDate = os.Args[1], os.Args[2]
	}
	resolvedBaseDir, err := importutil.ResolveDataRoot(baseDir)
	if err != nil {
		log.Fatalf("数据目录不可用: %v", err)
	}

	totalNote, totalGoods := 0, 0

	// 第一层：年份目录（4 位数字，如 2026），跳过 png/记录.xlsx 等非年份项
	yearEntries, err := os.ReadDir(resolvedBaseDir)
	if err != nil {
		log.Fatalf("读取小红书目录失败: %v", err)
	}
	for _, ye := range yearEntries {
		if !ye.IsDir() || len(ye.Name()) != 4 {
			continue
		}
		yearPath := filepath.Join(resolvedBaseDir, ye.Name())
		dateDirs, err := os.ReadDir(yearPath)
		if err != nil {
			continue
		}
		// 第二层：日期目录（8 位 YYYYMMDD）
		for _, dd := range dateDirs {
			if !dd.IsDir() || len(dd.Name()) != 8 {
				continue
			}
			dateStr := dd.Name()
			if startDate != "" && dateStr < startDate {
				continue
			}
			if endDate != "" && dateStr > endDate {
				continue
			}
			sqlDate := dateStr[:4] + "-" + dateStr[4:6] + "-" + dateStr[6:8]
			datePath := filepath.Join(yearPath, dateStr)

			// 第三层：店铺目录
			shopDirs, err := os.ReadDir(datePath)
			if err != nil {
				continue
			}
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
					if !strings.HasSuffix(name, ".xlsx") {
						continue
					}
					fpath := filepath.Join(shopPath, name)
					switch {
					case strings.Contains(name, "_笔记_数据明细"):
						totalNote += importNoteDaily(db, fpath, sqlDate, shopName)
					case strings.Contains(name, "_商品_销售明细"):
						totalGoods += importGoodsDaily(db, fpath, sqlDate, shopName)
					}
				}
			}
		}
	}

	fmt.Printf("\n小红书导入完成:\n  笔记明细: %d 条\n  商品销售明细: %d 条\n", totalNote, totalGoods)
}

func ensureTables(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS op_xhs_note_daily (
		id BIGINT PRIMARY KEY AUTO_INCREMENT,
		stat_date DATE NOT NULL COMMENT '统计日期(取RPA目录/文件名日期,即数据快照日)',
		shop_name VARCHAR(128) NOT NULL COMMENT '店铺名',
		note_title VARCHAR(500) DEFAULT '' COMMENT '笔记标题',
		note_id VARCHAR(64) NOT NULL COMMENT '笔记ID',
		note_url VARCHAR(500) DEFAULT '' COMMENT '笔记链接',
		author_name VARCHAR(128) DEFAULT '' COMMENT '作者昵称',
		author_xhs_id VARCHAR(64) DEFAULT '' COMMENT '作者小红书ID',
		note_create_time VARCHAR(32) DEFAULT '' COMMENT '笔记创建时间(原值)',
		note_type VARCHAR(32) DEFAULT '' COMMENT '笔记类型(图文/视频)',
		related_product_name VARCHAR(500) DEFAULT '' COMMENT '关联商品名称',
		related_product_id VARCHAR(64) DEFAULT '' COMMENT '关联商品ID',
		video_duration_sec DECIMAL(10,2) DEFAULT 0 COMMENT '视频时长(秒)',
		pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '笔记支付金额',
		pay_order_count INT DEFAULT 0 COMMENT '笔记支付订单数',
		product_click_pv INT DEFAULT 0 COMMENT '笔记商品点击次数',
		product_click_rate_pv DECIMAL(12,6) DEFAULT 0 COMMENT '笔记商品点击率(PV)',
		product_click_uv INT DEFAULT 0 COMMENT '笔记商品点击人数',
		pay_uv INT DEFAULT 0 COMMENT '笔记支付人数',
		pay_conv_rate_pv DECIMAL(12,6) DEFAULT 0 COMMENT '笔记支付转化率(PV)',
		pay_conv_rate_uv DECIMAL(12,6) DEFAULT 0 COMMENT '笔记支付转化率(UV)',
		refund_amount_by_refund DECIMAL(14,2) DEFAULT 0 COMMENT '笔记退款金额(退款时间)',
		refund_order_by_refund INT DEFAULT 0 COMMENT '笔记退款订单数(退款时间)',
		refund_uv_by_refund INT DEFAULT 0 COMMENT '笔记退款人数(退款时间)',
		refund_amount_by_pay DECIMAL(14,2) DEFAULT 0 COMMENT '笔记退款金额(支付时间)',
		refund_rate_by_pay DECIMAL(12,6) DEFAULT 0 COMMENT '笔记退款率(支付时间)',
		refund_order_by_pay INT DEFAULT 0 COMMENT '笔记退款订单数(支付时间)',
		add_cart_qty INT DEFAULT 0 COMMENT '笔记加购件数',
		to_shop_home_pv INT DEFAULT 0 COMMENT '引流店铺主页次数',
		to_shop_home_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '引流店铺主页支付金额',
		to_live_pv INT DEFAULT 0 COMMENT '引流直播间次数',
		to_live_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '引流直播间支付金额',
		read_count INT DEFAULT 0 COMMENT '笔记阅读数',
		like_count INT DEFAULT 0 COMMENT '点赞次数',
		collect_count INT DEFAULT 0 COMMENT '收藏次数',
		comment_count INT DEFAULT 0 COMMENT '评论次数',
		share_count INT DEFAULT 0 COMMENT '分享次数',
		follow_count INT DEFAULT 0 COMMENT '笔记点击关注次数',
		danmu_count INT DEFAULT 0 COMMENT '弹幕次数',
		avg_read_duration DECIMAL(14,4) DEFAULT 0 COMMENT '平均阅读时长(观播时长)',
		finish_rate_pv DECIMAL(12,6) DEFAULT 0 COMMENT '完播率(PV)',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		UNIQUE KEY uk_xhs_note (stat_date, shop_name, note_id),
		KEY idx_xhs_note_date (stat_date)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='小红书笔记数据明细(每日每店快照)'`); err != nil {
		return err
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS op_xhs_goods_daily (
		id BIGINT PRIMARY KEY AUTO_INCREMENT,
		stat_date DATE NOT NULL COMMENT '统计日期(取RPA目录/文件名日期,即数据快照日)',
		shop_name VARCHAR(128) NOT NULL COMMENT '店铺名',
		product_id VARCHAR(64) NOT NULL COMMENT '商品ID',
		product_name VARCHAR(500) DEFAULT '' COMMENT '商品名称',
		business_type VARCHAR(32) NOT NULL DEFAULT '' COMMENT '经营方式(全部/自营/带货)',
		carrier VARCHAR(32) NOT NULL DEFAULT '' COMMENT '载体(商卡/全部/笔记/直播)',
		is_channel_goods VARCHAR(16) DEFAULT '' COMMENT '是否渠道商品',
		category_l1 VARCHAR(128) DEFAULT '' COMMENT '一级品类',
		category_l2 VARCHAR(128) DEFAULT '' COMMENT '二级品类',
		brand VARCHAR(128) DEFAULT '' COMMENT '品牌',
		visitor_count INT DEFAULT 0 COMMENT '商品访客数',
		view_count INT DEFAULT 0 COMMENT '商品浏览量',
		add_cart_uv INT DEFAULT 0 COMMENT '新增加购人数',
		add_cart_qty INT DEFAULT 0 COMMENT '新增加购件数',
		add_wishlist_uv INT DEFAULT 0 COMMENT '新增加入心愿单人数',
		pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '支付金额',
		pay_buyer_count INT DEFAULT 0 COMMENT '支付买家数',
		pay_order_count INT DEFAULT 0 COMMENT '支付订单数',
		pay_qty INT DEFAULT 0 COMMENT '支付件数',
		pay_conv_rate DECIMAL(12,6) DEFAULT 0 COMMENT '支付转化率',
		avg_order_amount DECIMAL(14,4) DEFAULT 0 COMMENT '客单价',
		refund_amount_by_refund DECIMAL(14,2) DEFAULT 0 COMMENT '退款金额(退款时间)',
		refund_buyer_by_refund INT DEFAULT 0 COMMENT '退款买家数(退款时间)',
		refund_order_by_refund INT DEFAULT 0 COMMENT '退款订单数(退款时间)',
		pay_conv_rate_pv DECIMAL(12,6) DEFAULT 0 COMMENT '支付转化率(PV)',
		refund_amount_by_pay DECIMAL(14,2) DEFAULT 0 COMMENT '退款金额(支付时间)',
		refund_rate_by_pay DECIMAL(12,6) DEFAULT 0 COMMENT '退款率(支付时间)',
		refund_order_by_pay INT DEFAULT 0 COMMENT '退款订单数(支付时间)',
		pre_ship_refund_rate DECIMAL(12,6) DEFAULT 0 COMMENT '发货前退款率(支付时间)',
		post_ship_refund_rate DECIMAL(12,6) DEFAULT 0 COMMENT '发货后退款率(支付时间)',
		net_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '退款后支付金额(支付时间)',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		UNIQUE KEY uk_xhs_goods (stat_date, shop_name, product_id, business_type, carrier),
		KEY idx_xhs_goods_date (stat_date)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='小红书商品销售明细(每日每店快照)'`); err != nil {
		return err
	}
	return nil
}

// importNoteDaily 导入"笔记数据明细"xlsx（38 业务字段）
func importNoteDaily(db *sql.DB, path, sqlDate, shopName string) int {
	f, err := excelize.OpenFile(path)
	if err != nil {
		log.Printf("打开失败 %s: %v", filepath.Base(path), err)
		return 0
	}
	defer f.Close()

	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 2 {
		return 0
	}
	colMap := make(map[string]int)
	for i, h := range rows[0] {
		colMap[strings.TrimSpace(h)] = i
	}

	count := 0
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		get := func(name string) string {
			idx, ok := colMap[name]
			if !ok || idx >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[idx])
		}
		getF := func(name string) float64 { return importutil.ParseFloat(get(name)) }
		getI := func(name string) int { return importutil.ParseInt(get(name)) }

		noteID := get("笔记ID")
		if noteID == "" {
			continue
		}

		if _, err := db.Exec(`REPLACE INTO op_xhs_note_daily (
			stat_date, shop_name, note_title, note_id, note_url,
			author_name, author_xhs_id, note_create_time, note_type,
			related_product_name, related_product_id, video_duration_sec,
			pay_amount, pay_order_count, product_click_pv, product_click_rate_pv,
			product_click_uv, pay_uv, pay_conv_rate_pv, pay_conv_rate_uv,
			refund_amount_by_refund, refund_order_by_refund, refund_uv_by_refund,
			refund_amount_by_pay, refund_rate_by_pay, refund_order_by_pay,
			add_cart_qty, to_shop_home_pv, to_shop_home_pay_amount,
			to_live_pv, to_live_pay_amount,
			read_count, like_count, collect_count, comment_count, share_count,
			follow_count, danmu_count, avg_read_duration, finish_rate_pv
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			sqlDate, shopName, get("笔记标题"), noteID, get("笔记链接"),
			get("作者昵称"), get("作者xhs_ID"), get("笔记创建时间"), get("笔记类型"),
			get("关联商品名称"), get("关联商品ID"), getF("视频时长（秒）"),
			getF("笔记支付金额"), getI("笔记支付订单数"), getI("笔记商品点击次数"), getF("笔记商品点击率（PV）"),
			getI("笔记商品点击人数"), getI("笔记支付人数"), getF("笔记支付转化率（PV）"), getF("笔记支付转化率（UV）"),
			getF("笔记退款金额（退款时间）"), getI("笔记退款订单数（退款时间）"), getI("笔记退款人数（退款时间）"),
			getF("笔记退款金额（支付时间）"), getF("笔记退款率（支付时间）"), getI("笔记退款订单数（支付时间）"),
			getI("笔记加购件数"), getI("引流店铺主页次数"), getF("引流店铺主页支付金额"),
			getI("引流直播间次数"), getF("引流直播间支付金额"),
			getI("笔记阅读数"), getI("点赞次数"), getI("收藏次数"), getI("评论次数"), getI("分享次数"),
			getI("笔记点击关注次数"), getI("弹幕次数"), getF("平均阅读时长（观播时长）"), getF("完播率（PV）"),
		); err != nil {
			log.Printf("笔记写入失败 shop=%s note=%s: %v", shopName, noteID, err)
			continue
		}
		count++
	}
	return count
}

// importGoodsDaily 导入"商品销售明细"xlsx（29 业务字段）
func importGoodsDaily(db *sql.DB, path, sqlDate, shopName string) int {
	f, err := excelize.OpenFile(path)
	if err != nil {
		log.Printf("打开失败 %s: %v", filepath.Base(path), err)
		return 0
	}
	defer f.Close()

	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 2 {
		return 0
	}
	colMap := make(map[string]int)
	for i, h := range rows[0] {
		colMap[strings.TrimSpace(h)] = i
	}

	count := 0
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		get := func(name string) string {
			idx, ok := colMap[name]
			if !ok || idx >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[idx])
		}
		getF := func(name string) float64 { return importutil.ParseFloat(get(name)) }
		getI := func(name string) int { return importutil.ParseInt(get(name)) }

		productID := get("商品ID")
		if productID == "" {
			continue
		}

		if _, err := db.Exec(`REPLACE INTO op_xhs_goods_daily (
			stat_date, shop_name, product_id, product_name, business_type,
			carrier, is_channel_goods, category_l1, category_l2, brand,
			visitor_count, view_count, add_cart_uv, add_cart_qty, add_wishlist_uv,
			pay_amount, pay_buyer_count, pay_order_count, pay_qty, pay_conv_rate, avg_order_amount,
			refund_amount_by_refund, refund_buyer_by_refund, refund_order_by_refund,
			pay_conv_rate_pv, refund_amount_by_pay, refund_rate_by_pay, refund_order_by_pay,
			pre_ship_refund_rate, post_ship_refund_rate, net_pay_amount
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			sqlDate, shopName, productID, get("商品NAME"), get("经营方式"),
			get("载体"), get("是否渠道商品"), get("一级品类"), get("二级品类"), get("品牌"),
			getI("商品访客数"), getI("商品浏览量"), getI("新增加购人数"), getI("新增加购件数"), getI("新增加入心愿单人数"),
			getF("支付金额"), getI("支付买家数"), getI("支付订单数"), getI("支付件数"), getF("支付转化率"), getF("客单价"),
			getF("退款金额（退款时间）"), getI("退款买家数（退款时间）"), getI("退款订单数（退款时间）"),
			getF("支付转化率（PV）"), getF("退款金额（支付时间）"), getF("退款率（支付时间）"), getI("退款订单数（支付时间）"),
			getF("发货前退款率（支付时间）"), getF("发货后退款率（支付时间）"), getF("退款后支付金额（支付时间）"),
		); err != nil {
			log.Printf("商品写入失败 shop=%s product=%s: %v", shopName, productID, err)
			continue
		}
		count++
	}
	return count
}
