package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/xuri/excelize/v2"
)

var baseDir = `Z:\信息部\RPA_集团数据看板\天猫超市`

// rpaShopToName 把 RPA 店铺文件夹原名映射到数据库 shop_name 简称
// 两店从 2025-12-31 起分别采集；注意"一盘货"文件夹名里也带"寄售"后缀，必须先匹配一盘货
func rpaShopToName(rpaDir string) string {
	switch {
	case strings.Contains(rpaDir, "一盘货"):
		return "天猫超市一盘货"
	case strings.Contains(rpaDir, "寄售"):
		return "天猫超市寄售"
	default:
		return "天猫超市"
	}
}

// toSQLDate 把 "20260419" 转成 "2026-04-19"；已是 ISO 的原样返回
func toSQLDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 8 && !strings.Contains(s, "-") {
		return s[:4] + "-" + s[4:6] + "-" + s[6:8]
	}
	return s
}

func main() {
	unlock := importutil.AcquireLock("import-tmallcs")
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

	startDate, endDate := "", ""
	if len(os.Args) >= 3 {
		startDate, endDate = os.Args[1], os.Args[2]
	}
	resolvedBaseDir, err := importutil.ResolveDataRoot(baseDir)
	if err != nil {
		log.Fatalf("数据目录不可用: %v", err)
	}

	total := map[string]int{}
	matchedDate := false

	for _, yearDir := range []string{"2025", "2026"} {
		yearPath := filepath.Join(resolvedBaseDir, yearDir)
		dateDirs, err := os.ReadDir(yearPath)
		if err != nil {
			continue
		}
		for _, dd := range dateDirs {
			if !dd.IsDir() {
				continue
			}
			dateStr := dd.Name()
			if startDate != "" && dateStr < startDate {
				continue
			}
			if endDate != "" && dateStr > endDate {
				continue
			}
			matchedDate = true
			sqlDate := dateStr[:4] + "-" + dateStr[4:6] + "-" + dateStr[6:8]

			shopDirs, err := os.ReadDir(filepath.Join(yearPath, dateStr))
			if err != nil {
				continue
			}
			for _, sd := range shopDirs {
				if !sd.IsDir() {
					continue
				}
				shopName := rpaShopToName(sd.Name())
				shopPath := filepath.Join(yearPath, dateStr, sd.Name())

				files, err := os.ReadDir(shopPath)
				if err != nil {
					continue
				}
				for _, f := range files {
					if !strings.HasSuffix(f.Name(), ".xlsx") {
						continue
					}
					name := f.Name()
					fpath := filepath.Join(shopPath, name)

					// 注意判断顺序：更具体的子串必须先匹配（"无界明细"先于"无界场景"，"智多星_明细"先于"智多星"）
					switch {
					case strings.Contains(name, "销售数据_经营概况"):
						cnt, _ := importBusinessOverview(db, fpath, sqlDate, shopName)
						total["shop_daily"] += cnt
					case strings.Contains(name, "销售数据_商品"):
						cnt, _ := importGoods(db, fpath, sqlDate, shopName)
						total["goods_daily"] += cnt
					case strings.Contains(name, "市场_行业热词"):
						cnt, _ := importIndustryKeyword(db, fpath)
						total["industry_keyword"] += cnt
					case strings.Contains(name, "市场排名数据_") || strings.Contains(name, "市场数据_排名"):
						category := "排名"
						if idx := strings.Index(name, "市场排名数据_"); idx >= 0 {
							rest := name[idx+len("市场排名数据_"):]
							if dotIdx := strings.Index(rest, "."); dotIdx > 0 {
								category = rest[:dotIdx]
							}
						}
						cnt, _ := importMarketRank(db, fpath, sqlDate, category)
						total["market_rank"] += cnt
					case strings.Contains(name, "推广_无界明细"):
						cnt, _ := importWujieDetail(db, fpath, sqlDate, shopName)
						total["wujie_detail"] += cnt
					case strings.Contains(name, "推广_无界场景"):
						cnt, _ := importWujieScene(db, fpath, sqlDate, shopName)
						total["wujie_scene"] += cnt
					case strings.Contains(name, "推广_智多星_明细"):
						cnt, _ := importSmartPlanDetail(db, fpath, shopName)
						total["smart_plan_detail"] += cnt
					case strings.Contains(name, "推广_智多星"):
						cnt, _ := importSmartPlan(db, fpath, shopName)
						total["smart_plan"] += cnt
					case strings.Contains(name, "推广_淘客"):
						cnt, _ := importTaoke(db, fpath, shopName)
						total["taoke"] += cnt
					}
				}
			}
			fmt.Printf("[%s] 完成\n", dateStr)
		}
	}
	if startDate != "" && endDate != "" && !matchedDate {
		log.Fatalf("未找到日期范围 %s-%s 的数据目录", startDate, endDate)
	}

	fmt.Println("\n导入完成:")
	for k, v := range total {
		fmt.Printf("  %s: %d 条\n", k, v)
	}
}

// ==================== 经营概况 (30 天滚动) ====================
// 文件结构: [日期 支付金额 子订单均价 客单价 IPVUV 支付子订单数 支付商品件数 支付转化率 支付用户数]
func importBusinessOverview(db *sql.DB, fpath, sqlDate, shopName string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 2 {
		return 0, nil
	}
	count := 0
	for i := 1; i < len(rows); i++ {
		d := rows[i]
		if len(d) < 9 {
			continue
		}
		statDate := toSQLDate(d[0])
		if statDate == "" {
			statDate = sqlDate // fallback 到 RPA 文件名日期
		}
		_, err = db.Exec(`REPLACE INTO op_tmall_cs_shop_daily
			(stat_date, shop_name, pay_amount, sub_order_avg_price, avg_price, ipv_uv,
			 pay_sub_orders, pay_qty, conv_rate, pay_users)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			statDate, shopName,
			toF(d, 1), toF(d, 2), toF(d, 3), toI(d, 4),
			toI(d, 5), toI(d, 6), toF(d, 7), toI(d, 8),
		)
		if err != nil {
			log.Printf("business_overview失败 [%s/%s]: %v", statDate, shopName, err)
			continue
		}
		count++
	}
	return count, nil
}

// ==================== 行业热词 (不分店) ====================
// 文件结构: [统计日期 统计维度 业务渠道 搜索词 搜索曝光热度 引导成交热度 引导成交人气 引导成交规模 引导转化指数 引导访问热度 引导访问人气]
func importIndustryKeyword(db *sql.DB, fpath string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 2 {
		return 0, nil
	}
	count := 0
	for i := 1; i < len(rows); i++ {
		d := rows[i]
		if len(d) < 11 {
			continue
		}
		statDate := toSQLDate(d[0])
		keyword := strings.TrimSpace(d[3])
		if statDate == "" || keyword == "" {
			continue
		}
		_, err = db.Exec(`REPLACE INTO op_tmall_cs_industry_keyword
			(stat_date, dimension, channel, keyword,
			 search_impression, trade_heat, trade_popularity, trade_scale, conv_index,
			 visit_heat, visit_popularity)
			VALUES (?,?,?,?, ?,?,?,?,?, ?,?)`,
			statDate, strings.TrimSpace(d[1]), strings.TrimSpace(d[2]), keyword,
			toF(d, 4), toF(d, 5), toF(d, 6), toF(d, 7), toF(d, 8),
			toF(d, 9), toF(d, 10),
		)
		if err != nil {
			continue
		}
		count++
	}
	return count, nil
}

// ==================== 市场排名 (不分店) ====================
// 文件结构: [品牌名 成交热度 成交人气 访问热度 访问人气 转化指数 成交热度环比 成交人气环比 访问热度环比 访问人气环比 转化指数环比 交易指数 交易指数环比]
func importMarketRank(db *sql.DB, fpath, date, category string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 2 {
		return 0, nil
	}
	count := 0
	for i := 1; i < len(rows); i++ {
		d := rows[i]
		if len(d) < 13 {
			continue
		}
		brand := strings.TrimSpace(d[0])
		if brand == "" {
			continue
		}
		_, err = db.Exec(`REPLACE INTO op_tmall_cs_market_rank
			(stat_date, category, brand_name,
			 trade_heat, trade_popularity, visit_heat, visit_popularity, conv_index,
			 trade_heat_chg, trade_popularity_chg, visit_heat_chg, visit_popularity_chg, conv_index_chg,
			 trade_index, trade_index_chg)
			VALUES (?,?,?, ?,?,?,?,?, ?,?,?,?,?, ?,?)`,
			date, category, brand,
			toF(d, 1), toF(d, 2), toF(d, 3), toF(d, 4), toF(d, 5),
			toF(d, 6), toF(d, 7), toF(d, 8), toF(d, 9), toF(d, 10),
			toF(d, 11), toF(d, 12),
		)
		if err != nil {
			continue
		}
		count++
	}
	return count, nil
}

// ==================== 无界-场景级 (单天，69 列) ====================
// 来源: 推广_无界场景数据.xlsx；stat_date 取 Excel "日期" 列
func importWujieScene(db *sql.DB, fpath, dateStr, shopName string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 2 {
		return 0, nil
	}
	colMap := indexHeader(rows[0])

	count := 0
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		get := func(name string) string { return getCol(row, colMap, name) }
		getF := func(name string) float64 { return parseF(get(name)) }
		getI := func(name string) int { return parseI(get(name)) }

		sceneID := get("场景ID")
		if sceneID == "" {
			continue
		}
		statDate := get("日期")
		if statDate == "" {
			statDate = dateStr
		} else {
			statDate = toSQLDate(statDate)
		}

		_, err := db.Exec(`REPLACE INTO op_tmall_cs_wujie_scene_daily (
			stat_date, shop_name, scene_id, scene_name, orig_scene_id, orig_scene_name,
			impressions, clicks, cost, click_rate, avg_click_cost, cpm,
			presale_total_amount, presale_total_count,
			presale_direct_amount, presale_direct_count,
			presale_indirect_amount, presale_indirect_count,
			direct_pay_amount, indirect_pay_amount, total_pay_amount, total_pay_count,
			direct_pay_count, indirect_pay_count,
			click_conv_rate, roi, roi_with_presale, total_pay_cost,
			total_cart, direct_cart, indirect_cart, cart_rate, cart_cost,
			goods_collect_count, shop_collect_count, shop_collect_cost,
			total_cart_collect, total_cart_collect_cost,
			goods_cart_collect, goods_cart_collect_cost,
			total_collect, goods_collect_cost, goods_collect_rate,
			direct_goods_collect, indirect_goods_collect,
			place_order_count, place_order_amount,
			coupon_claim_count,
			shop_money_recharge_count, shop_money_recharge_amount,
			wangwang_consult_count,
			guide_visit_count, guide_visit_users, guide_visit_potential,
			guide_visit_potential_rate, member_join_rate, member_join_count,
			guide_visit_rate, deep_visit_count, avg_page_views,
			new_customer_count, new_customer_rate,
			member_first_buy, member_pay_amount, member_pay_count,
			buyer_count, avg_pay_count_per_user, avg_pay_amount_per_user,
			natural_flow_pay_amount, natural_flow_impressions
		) VALUES (?,?,?,?,?,?, ?,?,?,?,?,?, ?,?, ?,?, ?,?, ?,?,?,?, ?,?, ?,?,?,?, ?,?,?,?,?, ?,?,?, ?,?, ?,?, ?,?,?, ?,?, ?,?, ?, ?,?, ?, ?,?,?, ?,?,?, ?,?,?, ?,?, ?,?,?, ?,?,?, ?,?)`,
			statDate, shopName, sceneID, get("场景名字"), get("原二级场景ID"), get("原二级场景名字"),
			getI("展现量"), getI("点击量"), getF("花费"), getF("点击率"), getF("平均点击花费"), getF("千次展现花费"),
			getF("总预售成交金额"), getI("总预售成交笔数"),
			getF("直接预售成交金额"), getI("直接预售成交笔数"),
			getF("间接预售成交金额"), getI("间接预售成交笔数"),
			getF("直接成交金额"), getF("间接成交金额"), getF("总成交金额"), getI("总成交笔数"),
			getI("直接成交笔数"), getI("间接成交笔数"),
			getF("点击转化率"), getF("投入产出比"), getF("含预售投产比"), getF("总成交成本"),
			getI("总购物车数"), getI("直接购物车数"), getI("间接购物车数"), getF("加购率"), getF("加购成本"),
			getI("收藏宝贝数"), getI("收藏店铺数"), getF("店铺收藏成本"),
			getI("总收藏加购数"), getF("总收藏加购成本"),
			getI("宝贝收藏加购数"), getF("宝贝收藏加购成本"),
			getI("总收藏数"), getF("宝贝收藏成本"), getF("宝贝收藏率"),
			getI("直接收藏宝贝数"), getI("间接收藏宝贝数"),
			getI("拍下订单笔数"), getF("拍下订单金额"),
			getI("优惠券领取量"),
			getI("购物金充值笔数"), getF("购物金充值金额"),
			getI("旺旺咨询量"),
			getI("引导访问量"), getI("引导访问人数"), getI("引导访问潜客数"),
			getF("引导访问潜客占比"), getF("入会率"), getI("入会量"),
			getF("引导访问率"), getI("深度访问量"), getF("平均访问页面数"),
			getI("成交新客数"), getF("成交新客占比"),
			getI("会员首购人数"), getF("会员成交金额"), getI("会员成交笔数"),
			getI("成交人数"), getF("人均成交笔数"), getF("人均成交金额"),
			getF("自然流量转化金额"), getI("自然流量曝光量"),
		)
		if err != nil {
			log.Printf("wujie_scene失败 [%s/%s/%s]: %v", statDate, shopName, sceneID, err)
			continue
		}
		count++
	}
	return count, nil
}

// ==================== 无界-商品级明细 (68 列) ====================
// 来源: 推广_无界明细数据.xlsx；stat_date 取 Excel "日期" 列
func importWujieDetail(db *sql.DB, fpath, dateStr, shopName string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 2 {
		return 0, nil
	}
	colMap := indexHeader(rows[0])

	count := 0
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		get := func(name string) string { return getCol(row, colMap, name) }
		getF := func(name string) float64 { return parseF(get(name)) }
		getI := func(name string) int { return parseI(get(name)) }

		productID := get("主体ID")
		if productID == "" {
			continue
		}
		statDate := get("日期")
		if statDate == "" {
			statDate = dateStr
		} else {
			statDate = toSQLDate(statDate)
		}

		_, err := db.Exec(`REPLACE INTO op_tmall_cs_wujie_detail_daily (
			stat_date, shop_name, product_id, entity_type, product_name,
			impressions, clicks, cost, click_rate, avg_click_cost, cpm,
			presale_total_amount, presale_total_count,
			presale_direct_amount, presale_direct_count,
			presale_indirect_amount, presale_indirect_count,
			direct_pay_amount, indirect_pay_amount, total_pay_amount, total_pay_count,
			direct_pay_count, indirect_pay_count,
			click_conv_rate, roi, roi_with_presale, total_pay_cost,
			total_cart, direct_cart, indirect_cart, cart_rate, cart_cost,
			goods_collect_count, shop_collect_count, shop_collect_cost,
			total_cart_collect, total_cart_collect_cost,
			goods_cart_collect, goods_cart_collect_cost,
			total_collect, goods_collect_cost, goods_collect_rate,
			direct_goods_collect, indirect_goods_collect,
			place_order_count, place_order_amount,
			coupon_claim_count,
			shop_money_recharge_count, shop_money_recharge_amount,
			wangwang_consult_count,
			guide_visit_count, guide_visit_users, guide_visit_potential,
			guide_visit_potential_rate, member_join_rate, member_join_count,
			guide_visit_rate, deep_visit_count, avg_page_views,
			new_customer_count, new_customer_rate,
			member_first_buy, member_pay_amount, member_pay_count,
			buyer_count, avg_pay_count_per_user, avg_pay_amount_per_user,
			natural_flow_pay_amount, natural_flow_impressions
		) VALUES (?,?,?,?,?, ?,?,?,?,?,?, ?,?, ?,?, ?,?, ?,?,?,?, ?,?, ?,?,?,?, ?,?,?,?,?, ?,?,?, ?,?, ?,?, ?,?,?, ?,?, ?,?, ?, ?,?, ?, ?,?,?, ?,?,?, ?,?,?, ?,?, ?,?,?, ?,?,?, ?,?)`,
			statDate, shopName, productID, get("主体类型"), get("主体名称"),
			getI("展现量"), getI("点击量"), getF("花费"), getF("点击率"), getF("平均点击花费"), getF("千次展现花费"),
			getF("总预售成交金额"), getI("总预售成交笔数"),
			getF("直接预售成交金额"), getI("直接预售成交笔数"),
			getF("间接预售成交金额"), getI("间接预售成交笔数"),
			getF("直接成交金额"), getF("间接成交金额"), getF("总成交金额"), getI("总成交笔数"),
			getI("直接成交笔数"), getI("间接成交笔数"),
			getF("点击转化率"), getF("投入产出比"), getF("含预售投产比"), getF("总成交成本"),
			getI("总购物车数"), getI("直接购物车数"), getI("间接购物车数"), getF("加购率"), getF("加购成本"),
			getI("收藏宝贝数"), getI("收藏店铺数"), getF("店铺收藏成本"),
			getI("总收藏加购数"), getF("总收藏加购成本"),
			getI("宝贝收藏加购数"), getF("宝贝收藏加购成本"),
			getI("总收藏数"), getF("宝贝收藏成本"), getF("宝贝收藏率"),
			getI("直接收藏宝贝数"), getI("间接收藏宝贝数"),
			getI("拍下订单笔数"), getF("拍下订单金额"),
			getI("优惠券领取量"),
			getI("购物金充值笔数"), getF("购物金充值金额"),
			getI("旺旺咨询量"),
			getI("引导访问量"), getI("引导访问人数"), getI("引导访问潜客数"),
			getF("引导访问潜客占比"), getF("入会率"), getI("入会量"),
			getF("引导访问率"), getI("深度访问量"), getF("平均访问页面数"),
			getI("成交新客数"), getF("成交新客占比"),
			getI("会员首购人数"), getF("会员成交金额"), getI("会员成交笔数"),
			getI("成交人数"), getF("人均成交笔数"), getF("人均成交金额"),
			getF("自然流量转化金额"), getI("自然流量曝光量"),
		)
		if err != nil {
			log.Printf("wujie_detail失败 [%s/%s/%s]: %v", statDate, shopName, productID, err)
			continue
		}
		count++
	}
	return count, nil
}

// ==================== 智多星-总览 (16 列) ====================
// 来源: 推广_智多星.xlsx，sheet="数据总览"；每文件通常一天一行
func importSmartPlan(db *sql.DB, fpath, shopName string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sheet := pickSheet(f, "数据总览")
	rows, _ := f.GetRows(sheet)
	if len(rows) < 2 {
		return 0, nil
	}
	colMap := indexHeader(rows[0])

	count := 0
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		get := func(name string) string { return getCol(row, colMap, name) }
		getF := func(name string) float64 { return parseF(get(name)) }
		getI := func(name string) int { return parseI(get(name)) }

		statDate := toSQLDate(get("日期"))
		if statDate == "" {
			continue
		}
		campaignScene := get("投放场景")
		if campaignScene == "" {
			campaignScene = "未知"
		}

		_, err := db.Exec(`REPLACE INTO op_tmall_cs_smart_plan_daily (
			stat_date, shop_name, convert_cycle, campaign_scene,
			cost, impressions, clicks, click_rate, avg_click_cost,
			cart_count, collect_count,
			direct_pay_count, total_pay_count, direct_pay_amount, total_pay_amount,
			pay_conv_rate, roi
		) VALUES (?,?,?,?, ?,?,?,?,?, ?,?, ?,?,?,?, ?,?)`,
			statDate, shopName, get("转化周期"), campaignScene,
			getF("消耗(元)"), getI("曝光量"), getI("点击量"), getF("点击率"), getF("点击成本"),
			getI("加购数"), getI("收藏数"),
			getI("直接成交笔数"), getI("总成交笔数"), getF("直接成交金额"), getF("总成交金额"),
			getF("成交转化率"), getF("ROI"),
		)
		if err != nil {
			log.Printf("smart_plan失败 [%s/%s]: %v", statDate, shopName, err)
			continue
		}
		count++
	}
	return count, nil
}

// ==================== 智多星-计划×商品明细 (20 列) ====================
// 来源: 推广_智多星_明细.xlsx，sheet="商品效果数据"
func importSmartPlanDetail(db *sql.DB, fpath, shopName string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sheet := pickSheet(f, "商品效果数据")
	rows, _ := f.GetRows(sheet)
	if len(rows) < 2 {
		return 0, nil
	}
	colMap := indexHeader(rows[0])

	count := 0
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		get := func(name string) string { return getCol(row, colMap, name) }
		getF := func(name string) float64 { return parseF(get(name)) }
		getI := func(name string) int { return parseI(get(name)) }

		statDate := toSQLDate(get("日期"))
		planID := get("计划id")
		productID := get("宝贝id")
		if statDate == "" || planID == "" || productID == "" {
			continue
		}

		_, err := db.Exec(`REPLACE INTO op_tmall_cs_smart_plan_detail_daily (
			stat_date, shop_name, plan_id, plan_name, product_id, product_name,
			convert_cycle, campaign_scene,
			cost, impressions, clicks, click_rate, avg_click_cost,
			cart_count, collect_count,
			direct_pay_count, total_pay_count, direct_pay_amount, total_pay_amount,
			pay_conv_rate, roi
		) VALUES (?,?,?,?,?,?, ?,?, ?,?,?,?,?, ?,?, ?,?,?,?, ?,?)`,
			statDate, shopName, planID, get("计划名称"), productID, get("宝贝名称"),
			get("转化周期"), get("投放场景"),
			getF("消耗(元)"), getI("曝光量"), getI("点击量"), getF("点击率"), getF("点击成本"),
			getI("加购数"), getI("收藏数"),
			getI("直接成交笔数"), getI("总成交笔数"), getF("直接成交金额"), getF("总成交金额"),
			getF("成交转化率"), getF("ROI"),
		)
		if err != nil {
			log.Printf("smart_plan_detail失败 [%s/%s/%s/%s]: %v", statDate, shopName, planID, productID, err)
			continue
		}
		count++
	}
	return count, nil
}

// ==================== 淘客诊断 (12 列，多天滚动) ====================
// 来源: 推广_淘客诊断.xlsx，sheet="data"；每行一个业务日期
func importTaoke(db *sql.DB, fpath, shopName string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sheet := pickSheet(f, "data")
	rows, _ := f.GetRows(sheet)
	if len(rows) < 2 {
		return 0, nil
	}
	colMap := indexHeader(rows[0])

	count := 0
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		get := func(name string) string { return getCol(row, colMap, name) }
		getF := func(name string) float64 { return parseF(get(name)) }
		getI := func(name string) int { return parseI(get(name)) }

		statDate := toSQLDate(get("日期"))
		if statDate == "" {
			continue
		}

		_, err := db.Exec(`REPLACE INTO op_tmall_cs_taoke_daily (
			stat_date, shop_name,
			taoke_pay_amount, taoke_pay_penetration,
			taoke_total_cost, taoke_roi,
			taoke_pay_users, taoke_pay_users_penetration,
			taoke_new_density, taoke_channel_avg_price,
			taoke_active_goods, overall_active_goods, taoke_goods_penetration
		) VALUES (?,?, ?,?, ?,?, ?,?, ?,?, ?,?,?)`,
			statDate, shopName,
			getF("淘客成交金额"), getF("淘客成交渗透"),
			getF("淘客总投入费用"), getF("淘客投入ROI"),
			getI("淘客支付用户数"), getF("淘客支付用户数渗透"),
			getF("淘客新客浓度"), getF("淘客渠道客单价"),
			getI("淘客动销商品数"), getI("整体动销商品数"), getF("淘客商品渗透率"),
		)
		if err != nil {
			log.Printf("taoke失败 [%s/%s]: %v", statDate, shopName, err)
			continue
		}
		count++
	}
	return count, nil
}

// ==================== 商品级销售 (24 列) ====================
// 来源: 销售数据_商品.xlsx，sheet="data"；stat_date 取 Excel "统计日期" 列
func importGoods(db *sql.DB, fpath, dateStr, shopName string) (int, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sheet := pickSheet(f, "data")
	rows, _ := f.GetRows(sheet)
	if len(rows) < 2 {
		return 0, nil
	}
	colMap := indexHeader(rows[0])

	count := 0
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		get := func(name string) string { return getCol(row, colMap, name) }
		getF := func(name string) float64 { return parseF(get(name)) }
		getI := func(name string) int { return parseI(get(name)) }

		productID := get("商品ID")
		if productID == "" {
			continue
		}
		skuID := get("SKUID")
		if skuID == "" || skuID == "0" {
			skuID = "0"
		}
		statDate := toSQLDate(get("统计日期"))
		if statDate == "" {
			statDate = dateStr
		}

		_, err := db.Exec(`REPLACE INTO op_tmall_cs_goods_daily (
			stat_date, shop_name, product_id, sku_id, sku_attr,
			product_name, product_image, category_l4, region, brand, supplier_name,
			pay_amount, pay_qty, pay_users,
			cart_users, cart_qty,
			refund_init_amount, refund_init_sub_orders, refund_init_qty,
			pay_amount_ex_refund, pay_sub_orders_ex_refund,
			refund_success_amount, refund_success_sub_orders, refund_success_qty,
			unit_price
		) VALUES (?,?,?,?,?, ?,?,?,?,?,?, ?,?,?, ?,?, ?,?,?, ?,?, ?,?,?, ?)`,
			statDate, shopName, productID, skuID, get("SKU属性"),
			get("商品名称"), get("商品图片"), get("四级类目"), get("区域"), get("品牌"), get("供应商名称"),
			getF("支付金额"), getI("支付商品件数"), getI("支付用户数"),
			getI("加购用户数"), getI("加购件数"),
			getF("发起退款金额"), getI("发起退款子订单数"), getI("发起退款商品件数"),
			getF("支付金额(剔退款)"), getI("支付子订单数(剔退款)"),
			getF("退款成功金额"), getI("退款成功子订单数"), getI("退款成功商品件数"),
			getF("件单价"),
		)
		if err != nil {
			log.Printf("goods失败 [%s/%s/%s/%s]: %v", statDate, shopName, productID, skuID, err)
			continue
		}
		count++
	}
	return count, nil
}

// ==================== 工具函数 ====================

func indexHeader(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, h := range header {
		m[strings.TrimSpace(h)] = i
	}
	return m
}

func getCol(row []string, m map[string]int, name string) string {
	idx, ok := m[name]
	if !ok || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func parseF(s string) float64 {
	s = strings.ReplaceAll(strings.ReplaceAll(s, ",", ""), "%", "")
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
func parseI(s string) int { return int(parseF(s)) }

// pickSheet 优先取指定名字的 sheet，找不到就返回第 0 个
func pickSheet(f *excelize.File, preferred string) string {
	for _, name := range f.GetSheetList() {
		if name == preferred {
			return name
		}
	}
	return f.GetSheetName(0)
}

// 兼容旧函数签名（保留以防有历史调用），新代码统一用 parseF/parseI
func toF(d []string, i int) float64 {
	if i >= len(d) {
		return 0
	}
	return parseF(d[i])
}
func toI(d []string, i int) int { return int(toF(d, i)) }
