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
	"github.com/xuri/excelize/v2"
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
				// 优先 csv，没有则回退 xlsx
				if path := pickFile(shopPath, "_笔记_标准投数据"); path != "" {
					total += importFile(db, path, fileDate, shopName)
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
		like_count BIGINT DEFAULT 0 COMMENT '点赞',
		comment_count BIGINT DEFAULT 0 COMMENT '评论',
		collect_count BIGINT DEFAULT 0 COMMENT '收藏',
		follow_count BIGINT DEFAULT 0 COMMENT '关注',
		share_count BIGINT DEFAULT 0 COMMENT '分享',
		interaction_count BIGINT DEFAULT 0 COMMENT '互动量',
		avg_interaction_cost DECIMAL(12,4) DEFAULT 0 COMMENT '平均互动成本',
		action_btn_click_count BIGINT DEFAULT 0 COMMENT '行动按钮点击量',
		action_btn_click_rate DECIMAL(12,4) DEFAULT 0 COMMENT '行动按钮点击率(%)',
		screenshot_count BIGINT DEFAULT 0 COMMENT '截图',
		save_image_count BIGINT DEFAULT 0 COMMENT '保存图片',
		search_widget_click_count BIGINT DEFAULT 0 COMMENT '搜索组件点击量',
		search_widget_click_conv_rate DECIMAL(12,4) DEFAULT 0 COMMENT '搜索组件点击转化率(%)',
		avg_post_search_read_notes DECIMAL(12,4) DEFAULT 0 COMMENT '平均搜索后阅读笔记篇数',
		post_search_read_count BIGINT DEFAULT 0 COMMENT '搜后阅读量',
		reservation_count BIGINT DEFAULT 0 COMMENT '预约人次',
		live_reservation_count BIGINT DEFAULT 0 COMMENT '直播预约人次',
		live_reservation_cost DECIMAL(12,4) DEFAULT 0 COMMENT '直播预约成本',
		reserve_reach_live_impression BIGINT DEFAULT 0 COMMENT '预约触达人群开播曝光人次',
		reserve_reach_live_view BIGINT DEFAULT 0 COMMENT '预约触达人群开播观看人次',
		reserve_reach_live_order BIGINT DEFAULT 0 COMMENT '预约触达人群直播间下单人次',
		live_direct_goods_visitor_cost DECIMAL(12,4) DEFAULT 0 COMMENT '直播间直接商品访客量成本',
		live_direct_goods_visitor BIGINT DEFAULT 0 COMMENT '直播间直接商品访客量',
		live_direct_goods_addcart BIGINT DEFAULT 0 COMMENT '直播间直接商品加购量',
		live_direct_goods_addcart_cost DECIMAL(12,4) DEFAULT 0 COMMENT '直播间直接商品加购量成本',
		goods_visitor_7d BIGINT DEFAULT 0 COMMENT '7日总商品访客量',
		goods_visitor_7d_cost DECIMAL(12,4) DEFAULT 0 COMMENT '7日总商品访客成本',
		goods_addcart_7d BIGINT DEFAULT 0 COMMENT '7日总商品加购量',
		goods_addcart_7d_cost DECIMAL(12,4) DEFAULT 0 COMMENT '7日总商品加购成本',
		video_play_count BIGINT DEFAULT 0 COMMENT '视频播放量',
		video_5s_play_count BIGINT DEFAULT 0 COMMENT '视频5s播放量',
		order_7d_count_conv BIGINT DEFAULT 0 COMMENT '7日总下单订单量(转化时间)',
		order_7d_cost_conv DECIMAL(12,4) DEFAULT 0 COMMENT '7日总下单订单成本(转化时间)',
		order_7d_amount_conv DECIMAL(14,2) DEFAULT 0 COMMENT '7日总下单金额(转化时间)',
		order_7d_roi_conv DECIMAL(12,4) DEFAULT 0 COMMENT '7日总下单ROI(转化时间)',
		pay_7d_order_count_conv BIGINT DEFAULT 0 COMMENT '7日总支付订单量(转化时间)',
		pay_7d_order_cost_conv DECIMAL(12,4) DEFAULT 0 COMMENT '7日总支付订单成本(转化时间)',
		pay_7d_amount_conv DECIMAL(14,2) DEFAULT 0 COMMENT '7日总支付金额(转化时间)',
		pay_7d_roi_conv DECIMAL(12,4) DEFAULT 0 COMMENT '7日总支付ROI(转化时间)',
		direct_pay_order_count BIGINT DEFAULT 0 COMMENT '直接支付订单量',
		direct_pay_order_cost DECIMAL(12,4) DEFAULT 0 COMMENT '直接支付订单成本',
		direct_pay_gmv DECIMAL(14,2) DEFAULT 0 COMMENT '直接支付订单gmv',
		direct_pay_roi DECIMAL(12,4) DEFAULT 0 COMMENT '直接支付ROI',
		goods_direct_order_count BIGINT DEFAULT 0 COMMENT '商品直接下单订单量',
		goods_direct_order_cost DECIMAL(12,4) DEFAULT 0 COMMENT '商品直接下单订单成本',
		goods_direct_order_amount DECIMAL(14,2) DEFAULT 0 COMMENT '商品直接下单金额',
		goods_direct_order_roi DECIMAL(12,4) DEFAULT 0 COMMENT '商品直接下单ROI',
		reserve_reach_live_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '预约触达人群直播间直接支付金额',
		reserve_reach_live_pay_order_count BIGINT DEFAULT 0 COMMENT '预约触达人群直播间直接支付订单量',
		reserve_reach_pay_7d_amount DECIMAL(14,2) DEFAULT 0 COMMENT '预约触达人群7日总支付金额',
		reserve_reach_pay_7d_order_count BIGINT DEFAULT 0 COMMENT '预约触达人群7日支付订单量',
		reserve_reach_pay_15d_amount DECIMAL(14,2) DEFAULT 0 COMMENT '预约触达人群15日总支付金额',
		reserve_reach_pay_15d_order_count BIGINT DEFAULT 0 COMMENT '预约触达人群15日总支付订单量',
		shop_newcust_goods_visit BIGINT DEFAULT 0 COMMENT '店铺新客商品访问量',
		shop_newcust_pay_order_count BIGINT DEFAULT 0 COMMENT '店铺新客支付订单量',
		shop_newcust_pay_roi DECIMAL(12,4) DEFAULT 0 COMMENT '店铺新客支付ROI',
		shop_newcust_pay_people BIGINT DEFAULT 0 COMMENT '店铺新客支付人数',
		shop_newcust_pay_7d_order_count BIGINT DEFAULT 0 COMMENT '店铺新客7日支付订单量',
		shop_newcust_pay_7d_amount DECIMAL(14,2) DEFAULT 0 COMMENT '店铺新客7日支付金额',
		shop_newcust_order_roi DECIMAL(12,4) DEFAULT 0 COMMENT '店铺新客下单ROI',
		shop_newcust_order_people BIGINT DEFAULT 0 COMMENT '店铺新客下单人数',
		shop_newcust_repur_7d_order_count BIGINT DEFAULT 0 COMMENT '店铺新客首购及7日复购支付订单量',
		shop_newcust_repur_7d_amount DECIMAL(14,2) DEFAULT 0 COMMENT '店铺新客首购及7日复购支付金额',
		shop_newcust_repur_7d_roi DECIMAL(12,4) DEFAULT 0 COMMENT '店铺新客首购及7日复购支付ROI',
		shop_newcust_repur_15d_order_count BIGINT DEFAULT 0 COMMENT '店铺新客首购及15日复购支付订单量',
		shop_newcust_repur_15d_amount DECIMAL(14,2) DEFAULT 0 COMMENT '店铺新客首购及15日复购支付金额',
		shop_newcust_repur_15d_roi DECIMAL(12,4) DEFAULT 0 COMMENT '店铺新客首购及15日复购支付ROI',
		shop_newcust_repur_60d_order_count BIGINT DEFAULT 0 COMMENT '店铺新客首购及60日复购支付订单量',
		shop_newcust_repur_60d_amount DECIMAL(14,2) DEFAULT 0 COMMENT '店铺新客首购及60日复购支付金额',
		shop_newcust_repur_60d_roi DECIMAL(12,4) DEFAULT 0 COMMENT '店铺新客首购及60日复购支付ROI',
		live_newcust_direct_pay_order_count BIGINT DEFAULT 0 COMMENT '直播间新客直接支付订单量',
		live_newcust_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '直播间新客支付金额',
		live_newcust_direct_pay_roi DECIMAL(12,4) DEFAULT 0 COMMENT '直播间新客直接支付ROI',
		live_newcust_repur_7d_order_count BIGINT DEFAULT 0 COMMENT '直播间新客首购及7日复购支付订单量',
		live_newcust_repur_7d_amount DECIMAL(14,2) DEFAULT 0 COMMENT '直播间新客首购及7日复购支付金额',
		live_newcust_repur_7d_roi DECIMAL(12,4) DEFAULT 0 COMMENT '直播间新客首购及7日复购支付ROI',
		live_newcust_repur_15d_order_count BIGINT DEFAULT 0 COMMENT '直播间新客首购及15日复购支付订单量',
		live_newcust_repur_15d_amount DECIMAL(14,2) DEFAULT 0 COMMENT '直播间新客首购及15日复购支付金额',
		live_newcust_repur_15d_roi DECIMAL(12,4) DEFAULT 0 COMMENT '直播间新客首购及15日复购支付ROI',
		live_newcust_repur_30d_order_count BIGINT DEFAULT 0 COMMENT '直播间新客首购及30日复购支付订单量',
		live_newcust_repur_30d_amount DECIMAL(14,2) DEFAULT 0 COMMENT '直播间新客首购及30日复购支付金额',
		live_newcust_repur_30d_roi DECIMAL(12,4) DEFAULT 0 COMMENT '直播间新客首购及30日复购支付ROI',
		live_newcust_repur_60d_order_count BIGINT DEFAULT 0 COMMENT '直播间新客首购及60日复购支付订单量',
		live_newcust_repur_60d_amount DECIMAL(14,2) DEFAULT 0 COMMENT '直播间新客首购及60日复购支付金额',
		live_newcust_repur_60d_roi DECIMAL(12,4) DEFAULT 0 COMMENT '直播间新客首购及60日复购支付ROI',
		live_newcust_reservation_count BIGINT DEFAULT 0 COMMENT '直播间新客预约人次',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		UNIQUE KEY uk_xhs_cf (stat_date, shop_name, note_id),
		KEY idx_xhs_cf_date (stat_date)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='小红书乘风信息流投流数据(每日每店每笔记)'`)
	return err
}

// pickFile 在 dir 里按 marker 选数据文件：优先 .csv，没有则 .xlsx，都没返回 ""
func pickFile(dir, marker string) string {
	files, _ := os.ReadDir(dir)
	var csvP, xlsxP string
	for _, fl := range files {
		name := fl.Name()
		if fl.IsDir() || strings.HasPrefix(name, "~$") || !strings.Contains(name, marker) {
			continue
		}
		if strings.HasSuffix(name, ".csv") {
			csvP = filepath.Join(dir, name)
		} else if strings.HasSuffix(name, ".xlsx") {
			xlsxP = filepath.Join(dir, name)
		}
	}
	if csvP != "" {
		return csvP
	}
	return xlsxP
}

// readTable 读 csv 或 xlsx 成 [][]string(表头+数据行)；xlsx 走 GetRows
func readTable(path string) ([][]string, bool) {
	if strings.HasSuffix(path, ".csv") {
		f, err := os.Open(path)
		if err != nil {
			log.Printf("打开失败 %s: %v", filepath.Base(path), err)
			return nil, false
		}
		defer f.Close()
		rd := csv.NewReader(f)
		rd.FieldsPerRecord = -1
		rows, err := rd.ReadAll()
		if err != nil {
			log.Printf("解析CSV失败 %s: %v", filepath.Base(path), err)
			return nil, false
		}
		return rows, true
	}
	f, err := excelize.OpenFile(path)
	if err != nil {
		log.Printf("打开xlsx失败 %s: %v", filepath.Base(path), err)
		return nil, false
	}
	defer f.Close()
	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil {
		log.Printf("读xlsx失败 %s: %v", filepath.Base(path), err)
		return nil, false
	}
	return rows, true
}

func importFile(db *sql.DB, path, fileDate, shopName string) int {
	rows, ok := readTable(path)
	if !ok || len(rows) < 2 {
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
			pay_7d_conv_rate, shop_newcust_repurchase_order_count, shop_newcust_repurchase_amount, shop_newcust_repurchase_roi,
			like_count, comment_count, collect_count, follow_count, share_count, interaction_count,
			avg_interaction_cost, action_btn_click_count, action_btn_click_rate, screenshot_count, save_image_count, search_widget_click_count,
			search_widget_click_conv_rate, avg_post_search_read_notes, post_search_read_count, reservation_count, live_reservation_count, live_reservation_cost,
			reserve_reach_live_impression, reserve_reach_live_view, reserve_reach_live_order, live_direct_goods_visitor_cost, live_direct_goods_visitor, live_direct_goods_addcart,
			live_direct_goods_addcart_cost, goods_visitor_7d, goods_visitor_7d_cost, goods_addcart_7d, goods_addcart_7d_cost, video_play_count,
			video_5s_play_count, order_7d_count_conv, order_7d_cost_conv, order_7d_amount_conv, order_7d_roi_conv, pay_7d_order_count_conv,
			pay_7d_order_cost_conv, pay_7d_amount_conv, pay_7d_roi_conv, direct_pay_order_count, direct_pay_order_cost, direct_pay_gmv,
			direct_pay_roi, goods_direct_order_count, goods_direct_order_cost, goods_direct_order_amount, goods_direct_order_roi, reserve_reach_live_pay_amount,
			reserve_reach_live_pay_order_count, reserve_reach_pay_7d_amount, reserve_reach_pay_7d_order_count, reserve_reach_pay_15d_amount, reserve_reach_pay_15d_order_count, shop_newcust_goods_visit,
			shop_newcust_pay_order_count, shop_newcust_pay_roi, shop_newcust_pay_people, shop_newcust_pay_7d_order_count, shop_newcust_pay_7d_amount, shop_newcust_order_roi,
			shop_newcust_order_people, shop_newcust_repur_7d_order_count, shop_newcust_repur_7d_amount, shop_newcust_repur_7d_roi, shop_newcust_repur_15d_order_count, shop_newcust_repur_15d_amount,
			shop_newcust_repur_15d_roi, shop_newcust_repur_60d_order_count, shop_newcust_repur_60d_amount, shop_newcust_repur_60d_roi, live_newcust_direct_pay_order_count, live_newcust_pay_amount,
			live_newcust_direct_pay_roi, live_newcust_repur_7d_order_count, live_newcust_repur_7d_amount, live_newcust_repur_7d_roi, live_newcust_repur_15d_order_count, live_newcust_repur_15d_amount,
			live_newcust_repur_15d_roi, live_newcust_repur_30d_order_count, live_newcust_repur_30d_amount, live_newcust_repur_30d_roi, live_newcust_repur_60d_order_count, live_newcust_repur_60d_amount,
			live_newcust_repur_60d_roi, live_newcust_reservation_count
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
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
			getI("点赞"), getI("评论"), getI("收藏"), getI("关注"), getI("分享"),
			getI("互动量"), getF("平均互动成本"), getI("行动按钮点击量"), getF("行动按钮点击率"), getI("截图"),
			getI("保存图片"), getI("搜索组件点击量"), getF("搜索组件点击转化率"), getF("平均搜索后阅读笔记篇数"), getI("搜后阅读量"),
			getI("预约人次"), getI("直播预约人次"), getF("直播预约成本"), getI("预约触达人群开播曝光人次"), getI("预约触达人群开播观看人次"),
			getI("预约触达人群直播间下单人次"), getF("直播间直接商品访客量成本"), getI("直播间直接商品访客量"), getI("直播间直接商品加购量"), getF("直播间直接商品加购量成本"),
			getI("7日总商品访客量"), getF("7日总商品访客成本"), getI("7日总商品加购量"), getF("7日总商品加购成本"), getI("视频播放量"),
			getI("视频5s播放量"), getI("7日总下单订单量(转化时间)"), getF("7日总下单订单成本(转化时间)"), getF("7日总下单金额(转化时间)"), getF("7日总下单ROI(转化时间)"),
			getI("7日总支付订单量(转化时间)"), getF("7日总支付订单成本(转化时间)"), getF("7日总支付金额(转化时间)"), getF("7日总支付ROI(转化时间)"), getI("直接支付订单量"),
			getF("直接支付订单成本"), getF("直接支付订单gmv"), getF("直接支付Roi"), getI("商品直接下单订单量"), getF("商品直接下单订单成本"),
			getF("商品直接下单金额"), getF("商品直接下单ROI"), getF("预约触达人群直播间直接支付金额"), getI("预约触达人群直播间直接支付订单量"), getF("预约触达人群7日总支付金额"),
			getI("预约触达人群7日支付订单量"), getF("预约触达人群15日总支付金额"), getI("预约触达人群15日总支付订单量"), getI("店铺新客商品访问量"), getI("店铺新客支付订单量"),
			getF("店铺新客支付ROI"), getI("店铺新客支付人数"), getI("店铺新客7日支付订单量"), getF("店铺新客7日支付金额"), getF("店铺新客下单Roi"),
			getI("店铺新客下单人数"), getI("店铺新客首购及7日复购支付订单量"), getF("店铺新客首购及7日复购支付金额"), getF("店铺新客首购及7日复购支付ROI"), getI("店铺新客首购及15日复购支付订单量"),
			getF("店铺新客首购及15日复购支付金额"), getF("店铺新客首购及15日复购支付ROI"), getI("店铺新客首购及60日复购支付订单量"), getF("店铺新客首购及60日复购支付金额"), getF("店铺新客首购及60日复购支付ROI"),
			getI("直播间新客直接支付订单量"), getF("直播间新客支付金额"), getF("直播间新客直接支付ROI"), getI("直播间新客首购及7日复购支付订单量"), getF("直播间新客首购及7日复购支付金额"),
			getF("直播间新客首购及7日复购支付ROI"), getI("直播间新客首购及15日复购支付订单量"), getF("直播间新客首购及15日复购支付金额"), getF("直播间新客首购及15日复购支付ROI"), getI("直播间新客首购及30日复购支付订单量"),
			getF("直播间新客首购及30日复购支付金额"), getF("直播间新客首购及30日复购支付ROI"), getI("直播间新客首购及60日复购支付订单量"), getF("直播间新客首购及60日复购支付金额"), getF("直播间新客首购及60日复购支付ROI"),
			getI("直播间新客预约人次"),
		); err != nil {
			log.Printf("写入失败 shop=%s note=%s: %v", shopName, noteID, err)
			continue
		}
		count++
	}
	return count
}
