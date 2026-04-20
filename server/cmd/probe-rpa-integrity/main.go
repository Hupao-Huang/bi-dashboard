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

// 排除：跑哥说天猫超市先不管，且天猫平台也不管
// 抖音分销特殊（多天数据，stat_date来自excel列），也跳过对比
var skipPlatforms = map[string]bool{
	"天猫超市": true, "天猫": true, "抖音分销": true,
}

// 表 → (关键字, 平台)。一表多文件类型取并集
type tableSpec struct {
	table     string
	shopCol   string
	patterns  []struct {
		platform string
		keyword  string
	}
}

var specs = []tableSpec{
	// ===== 京东 =====
	{table: "op_jd_shop_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"京东", "_销售数据.xlsx"},
	}},
	{table: "op_jd_affiliate_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"京东", "_推广_京东联盟"},
	}},
	{table: "op_jd_campaign_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"京东", "_推广_京准通全站"},
		{"京东", "_推广_京准通非全站"},
	}},
	{table: "op_jd_customer_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"京东", "_客户数据_洞察"},
	}},
	{table: "op_jd_customer_type_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"京东", "_客户数据_新老客"},
	}},
	{table: "op_jd_promo_sku_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"京东", "_营销数据_便宜包邮"},
	}},
	{table: "op_jd_promo_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"京东", "_营销数据_百亿补贴"},
		{"京东", "_营销数据_秒杀活动"},
	}},
	{table: "op_jd_industry_keyword", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"京东", "_行业_交易榜"},
	}},
	{table: "op_jd_industry_rank", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"京东", "_行业_热搜榜"},
		{"京东", "_行业_飙升榜"},
	}},
	// ===== 京东自营 =====
	{table: "op_jd_cs_workload_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"京东自营", "_客服_服务工作量"},
	}},
	{table: "op_jd_cs_sales_perf_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"京东自营", "_客服_销售绩效"},
	}},
	// ===== 抖音 =====
	{table: "op_douyin_live_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"抖音", "_自营_直播数据"},
	}},
	{table: "op_douyin_goods_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"抖音", "_自营_商品数据"},
	}},
	{table: "op_douyin_ad_live_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"抖音", "_自营_推广直播间画面"},
	}},
	{table: "op_douyin_ad_material_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"抖音", "_自营_推广视频素材"},
	}},
	{table: "op_douyin_channel_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"抖音", "_自营_渠道分析"},
	}},
	{table: "op_douyin_funnel_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"抖音", "_自营_转化漏斗"},
	}},
	{table: "op_douyin_anchor_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"抖音", "_自营_主播分析"},
	}},
	{table: "op_douyin_cs_feige_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"抖音", "_飞鸽_客服表现"},
	}},
	// ===== 拼多多 =====
	{table: "op_pdd_shop_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"拼多多", "_销售数据_交易概况"},
	}},
	{table: "op_pdd_goods_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"拼多多", "_销售数据_商品概况"},
	}},
	{table: "op_pdd_goods_detail", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"拼多多", "_销售数据_商品数据"},
	}},
	{table: "op_pdd_service_overview", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"拼多多", "_销售数据_服务概况"},
	}},
	{table: "op_pdd_video_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"拼多多", "_推广数据_多多视频"},
	}},
	{table: "op_pdd_campaign_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"拼多多", "_推广数据_商品推广"},
		{"拼多多", "_推广数据_明星店铺"},
		{"拼多多", "_推广数据_直播推广"},
	}},
	{table: "op_pdd_cs_service_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"拼多多", "_客服_服务数据"},
	}},
	{table: "op_pdd_cs_sales_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"拼多多", "_客服_销售数据"},
	}},
	// ===== 唯品会 =====
	{table: "op_vip_shop_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"唯品会", "_销售数据_经营"},
	}},
	{table: "op_vip_cancel", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"唯品会", "_销售数据_取消金额"},
	}},
	{table: "op_vip_targetmax", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"唯品会", "_推广_TargetMax"},
	}},
	{table: "op_vip_weixiangke", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"唯品会", "_推广_唯享客"},
	}},
	// ===== 快手 =====
	{table: "op_kuaishou_cs_assessment_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"快手", "_客服_考核数据"},
	}},
	// ===== 小红书 =====
	{table: "op_xhs_cs_analysis_daily", shopCol: "shop_name", patterns: []struct{ platform, keyword string }{
		{"小红书", "_客服数据_客服分析"},
	}},
	// ===== 飞瓜 =====
	{table: "fg_creator_roster", shopCol: "platform", patterns: []struct{ platform, keyword string }{
		{"飞瓜", "_达人归属"},
	}},
	{table: "fg_creator_daily", shopCol: "platform", patterns: []struct{ platform, keyword string }{
		{"飞瓜", "_达人数据"},
	}},
}

var filePattern = regexp.MustCompile(`_(\d{8})_`)

type key struct {
	date string
	shop string
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

	totalMissing := 0
	totalExtra := 0
	for _, spec := range specs {
		// 1. 收集 RPA 文件的 (date, shop) 集合
		rpa := map[key]int{}
		for _, p := range spec.patterns {
			root := filepath.Join(`Z:\信息部\RPA_集团数据看板`, p.platform)
			filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				name := d.Name()
				if !strings.Contains(name, p.keyword) {
					return nil
				}
				// 跳过老Excel格式 .xls(仅要 .xlsx/.json/.csv)
				if strings.HasSuffix(name, ".xls") {
					return nil
				}
				m := filePattern.FindStringSubmatch(name)
				if m == nil {
					return nil
				}
				dateStr := m[1]
				sqlDate := dateStr[:4] + "-" + dateStr[4:6] + "-" + dateStr[6:8]
				shop := filepath.Base(filepath.Dir(path))
				rpa[key{sqlDate, shop}]++
				return nil
			})
		}
		// 2. 收集数据库 (stat_date, shop) 集合
		dbSet := map[key]int{}
		q := fmt.Sprintf("SELECT DATE_FORMAT(stat_date,'%%Y-%%m-%%d'), %s, COUNT(*) FROM %s GROUP BY 1,2", spec.shopCol, spec.table)
		rows, qerr := db.Query(q)
		if qerr != nil {
			fmt.Printf("[%s] 查询失败: %v\n", spec.table, qerr)
			continue
		}
		for rows.Next() {
			var d, s string
			var cnt int
			rows.Scan(&d, &s, &cnt)
			dbSet[key{d, s}] = cnt
		}
		rows.Close()

		// 3. 差集
		missing := []key{} // RPA 有文件但 DB 没数据
		extra := []key{}   // DB 有数据但 RPA 无文件
		for k := range rpa {
			if _, ok := dbSet[k]; !ok {
				missing = append(missing, k)
			}
		}
		for k := range dbSet {
			if _, ok := rpa[k]; !ok {
				extra = append(extra, k)
			}
		}
		sort.Slice(missing, func(i, j int) bool {
			if missing[i].date != missing[j].date {
				return missing[i].date < missing[j].date
			}
			return missing[i].shop < missing[j].shop
		})
		sort.Slice(extra, func(i, j int) bool {
			if extra[i].date != extra[j].date {
				return extra[i].date < extra[j].date
			}
			return extra[i].shop < extra[j].shop
		})

		// 4. 找重复（RPA多个文件同day+shop，或db多行同day+shop）
		rpaDups := 0
		for _, c := range rpa {
			if c > 1 {
				rpaDups++
			}
		}

		// 5. 汇总
		marker := "✅"
		if len(missing) > 0 {
			marker = "🔴"
		}
		fmt.Printf("\n===== [%s] RPA组合=%d, DB组合=%d, 缺失=%d, 多余=%d, RPA同组合重复=%d %s =====\n",
			spec.table, len(rpa), len(dbSet), len(missing), len(extra), rpaDups, marker)
		if len(missing) > 0 && len(missing) <= 15 {
			fmt.Println("  缺失(RPA有文件但表无数据):")
			for _, k := range missing {
				fmt.Printf("    %s / %s\n", k.date, k.shop)
			}
		} else if len(missing) > 15 {
			// 聚合按 shop 显示
			shopMissCount := map[string]int{}
			for _, k := range missing {
				shopMissCount[k.shop]++
			}
			fmt.Println("  缺失汇总（按店铺）:")
			for s, c := range shopMissCount {
				fmt.Printf("    %s: 缺 %d 天\n", s, c)
			}
			// 只显示前5条
			fmt.Println("  缺失样本前5:")
			for i := 0; i < 5 && i < len(missing); i++ {
				fmt.Printf("    %s / %s\n", missing[i].date, missing[i].shop)
			}
		}
		if len(extra) > 0 && len(extra) <= 10 {
			fmt.Println("  多余(表有数据但RPA无文件):")
			for _, k := range extra {
				fmt.Printf("    %s / %s\n", k.date, k.shop)
			}
		} else if len(extra) > 10 {
			fmt.Printf("  多余: %d 条（前3）:\n", len(extra))
			for i := 0; i < 3; i++ {
				fmt.Printf("    %s / %s\n", extra[i].date, extra[i].shop)
			}
		}
		totalMissing += len(missing)
		totalExtra += len(extra)
	}

	fmt.Printf("\n\n【全局汇总】缺失=%d 多余=%d\n", totalMissing, totalExtra)
}
