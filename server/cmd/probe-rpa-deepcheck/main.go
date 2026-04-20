package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"bi-dashboard/internal/config"

	_ "github.com/go-sql-driver/mysql"
	"github.com/xuri/excelize/v2"
)

type tableSpec struct {
	table    string
	shopCol  string
	patterns []struct {
		platform string
		keyword  string
	}
}

var specs = []tableSpec{
	{"op_jd_shop_daily", "shop_name", []struct{ platform, keyword string }{
		{"京东", "_销售数据.xlsx"},
	}},
	{"op_jd_affiliate_daily", "shop_name", []struct{ platform, keyword string }{
		{"京东", "_推广_京东联盟"},
	}},
	{"op_jd_campaign_daily", "shop_name", []struct{ platform, keyword string }{
		{"京东", "_推广_京准通全站"}, {"京东", "_推广_京准通非全站"},
	}},
	{"op_jd_customer_daily", "shop_name", []struct{ platform, keyword string }{
		{"京东", "_客户数据_洞察"},
	}},
	{"op_jd_customer_type_daily", "shop_name", []struct{ platform, keyword string }{
		{"京东", "_客户数据_新老客"},
	}},
	{"op_jd_industry_keyword", "shop_name", []struct{ platform, keyword string }{
		{"京东", "_行业_交易榜"},
	}},
	{"op_jd_industry_rank", "shop_name", []struct{ platform, keyword string }{
		{"京东", "_行业_热搜榜"}, {"京东", "_行业_飙升榜"},
	}},
	{"op_douyin_live_daily", "shop_name", []struct{ platform, keyword string }{
		{"抖音", "_自营_直播数据"},
	}},
	{"op_douyin_goods_daily", "shop_name", []struct{ platform, keyword string }{
		{"抖音", "_自营_商品数据"},
	}},
	{"op_douyin_ad_live_daily", "shop_name", []struct{ platform, keyword string }{
		{"抖音", "_自营_推广直播间画面"},
	}},
	{"op_douyin_ad_material_daily", "shop_name", []struct{ platform, keyword string }{
		{"抖音", "_自营_推广视频素材"},
	}},
	{"op_douyin_channel_daily", "shop_name", []struct{ platform, keyword string }{
		{"抖音", "_自营_渠道分析"},
	}},
	{"op_douyin_funnel_daily", "shop_name", []struct{ platform, keyword string }{
		{"抖音", "_自营_转化漏斗"},
	}},
	{"op_douyin_anchor_daily", "shop_name", []struct{ platform, keyword string }{
		{"抖音", "_自营_主播分析"},
	}},
	{"op_pdd_shop_daily", "shop_name", []struct{ platform, keyword string }{
		{"拼多多", "_销售数据_交易概况"},
	}},
	{"op_pdd_goods_daily", "shop_name", []struct{ platform, keyword string }{
		{"拼多多", "_销售数据_商品概况"},
	}},
	{"op_pdd_goods_detail", "shop_name", []struct{ platform, keyword string }{
		{"拼多多", "_销售数据_商品数据"},
	}},
	{"op_pdd_service_overview", "shop_name", []struct{ platform, keyword string }{
		{"拼多多", "_销售数据_服务概况"},
	}},
	{"op_pdd_video_daily", "shop_name", []struct{ platform, keyword string }{
		{"拼多多", "_推广数据_多多视频"},
	}},
	{"op_pdd_campaign_daily", "shop_name", []struct{ platform, keyword string }{
		{"拼多多", "_推广数据_商品推广"}, {"拼多多", "_推广数据_明星店铺"}, {"拼多多", "_推广数据_直播推广"},
	}},
	{"op_pdd_cs_sales_daily", "shop_name", []struct{ platform, keyword string }{
		{"拼多多", "_客服_销售数据"},
	}},
	{"op_vip_cancel", "shop_name", []struct{ platform, keyword string }{
		{"唯品会", "_销售数据_取消金额"},
	}},
	{"op_vip_targetmax", "shop_name", []struct{ platform, keyword string }{
		{"唯品会", "_推广_TargetMax"},
	}},
	{"op_vip_weixiangke", "shop_name", []struct{ platform, keyword string }{
		{"唯品会", "_推广_唯享客"},
	}},
}

var dateRe = regexp.MustCompile(`_(\d{8})_`)

type key struct{ date, shop string }

type fileInfo struct {
	path     string
	ext      string
	dataRows int // 实际有值的数据行数（排除表头和全空行）
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

	type verdict struct {
		codeBug    []key
		emptyFile  []key
		noFile     []key
	}
	results := map[string]*verdict{}

	for _, spec := range specs {
		// RPA 文件收集：key → file path
		rpa := map[key]fileInfo{}
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
				if strings.HasSuffix(name, ".xls") {
					return nil
				}
				m := dateRe.FindStringSubmatch(name)
				if m == nil {
					return nil
				}
				dateStr := m[1]
				sqlDate := dateStr[:4] + "-" + dateStr[4:6] + "-" + dateStr[6:8]
				shop := filepath.Base(filepath.Dir(path))
				k := key{sqlDate, shop}
				ext := filepath.Ext(name)
				// 同一 (date,shop) 可能有 xlsx+json+csv，任意一个有数据就算
				if cur, ok := rpa[k]; ok {
					if cur.dataRows > 0 {
						return nil
					}
					_ = cur
				}
				rpa[k] = fileInfo{path: path, ext: ext}
				return nil
			})
		}

		// DB
		dbSet := map[key]bool{}
		q := fmt.Sprintf("SELECT DATE_FORMAT(stat_date,'%%Y-%%m-%%d'), %s FROM %s GROUP BY 1,2", spec.shopCol, spec.table)
		rows, qerr := db.Query(q)
		if qerr != nil {
			continue
		}
		for rows.Next() {
			var d, s string
			rows.Scan(&d, &s)
			dbSet[key{d, s}] = true
		}
		rows.Close()

		v := &verdict{}
		// 遍历 RPA 没在 DB 的
		for k, fi := range rpa {
			if dbSet[k] {
				continue
			}
			// 判断文件数据
			rowsWithData := countDataRows(fi)
			if rowsWithData > 0 {
				v.codeBug = append(v.codeBug, k)
			} else {
				v.emptyFile = append(v.emptyFile, k)
			}
		}
		// DB 有但 RPA 文件不存在的
		for k := range dbSet {
			if _, ok := rpa[k]; !ok {
				v.noFile = append(v.noFile, k)
			}
		}
		results[spec.table] = v
	}

	// 输出
	for _, spec := range specs {
		v := results[spec.table]
		if v == nil {
			continue
		}
		totalIssue := len(v.codeBug) + len(v.emptyFile)
		if totalIssue == 0 && len(v.noFile) == 0 {
			fmt.Printf("%-35s ✅ 完全对齐\n", spec.table)
			continue
		}
		mark := "🟢"
		if len(v.codeBug) > 0 {
			mark = "🔴"
		}
		fmt.Printf("%-35s %s 代码bug=%d 空文件=%d DB多于RPA=%d\n",
			spec.table, mark, len(v.codeBug), len(v.emptyFile), len(v.noFile))
		// 显示代码bug的明细（紧要）
		if len(v.codeBug) > 0 && len(v.codeBug) <= 10 {
			sort.Slice(v.codeBug, func(i, j int) bool { return v.codeBug[i].date < v.codeBug[j].date })
			for _, k := range v.codeBug {
				fmt.Printf("    🔴 %s / %s\n", k.date, k.shop)
			}
		} else if len(v.codeBug) > 10 {
			sort.Slice(v.codeBug, func(i, j int) bool { return v.codeBug[i].date < v.codeBug[j].date })
			fmt.Printf("    🔴 前5个: ")
			for i := 0; i < 5; i++ {
				fmt.Printf("%s/%s  ", v.codeBug[i].date, v.codeBug[i].shop)
			}
			fmt.Println()
		}
	}
}

func countDataRows(fi fileInfo) int {
	switch fi.ext {
	case ".xlsx":
		f, err := excelize.OpenFile(fi.path)
		if err != nil {
			return 0
		}
		defer f.Close()
		rows, _ := f.GetRows(f.GetSheetName(0))
		cnt := 0
		for i, r := range rows {
			if i == 0 {
				continue
			}
			// 任一单元格非空即算有数据
			for _, c := range r {
				if strings.TrimSpace(c) != "" {
					cnt++
					break
				}
			}
		}
		return cnt
	case ".json":
		b, err := os.ReadFile(fi.path)
		if err != nil {
			return 0
		}
		// 粗略判断：长度>200字节且能解析为包含数据的JSON
		if len(b) < 100 {
			return 0
		}
		var v interface{}
		if err := json.Unmarshal(b, &v); err != nil {
			return 0
		}
		// JSON 有内容就算1行
		return 1
	case ".csv":
		b, err := os.ReadFile(fi.path)
		if err != nil {
			return 0
		}
		lines := strings.Split(strings.TrimSpace(string(b)), "\n")
		if len(lines) < 2 {
			return 0
		}
		// 数据行非空
		cnt := 0
		for i, l := range lines {
			if i == 0 {
				continue
			}
			if strings.TrimSpace(strings.ReplaceAll(l, ",", "")) != "" {
				cnt++
			}
		}
		return cnt
	}
	return 0
}
