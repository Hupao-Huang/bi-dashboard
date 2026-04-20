package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"
)

// 每种文件抽样多个，统计"真实业务日期"(过滤汇总行)的分布
// 聚合标签：全部/合计/汇总/总计/总值/均值/总计/-/空
var summaryLabels = map[string]bool{
	"全部": true, "合计": true, "汇总": true, "总计": true, "总值": true,
	"均值": true, "总":  true, "-":   true, "":      true, "合计值": true,
}

type fileStats struct {
	label         string
	sampledFiles  int
	multiDayFiles int            // 有>1个真实业务日期的文件数
	dateColName   string         // 找到的日期列名
	hasDateCol    int            // 多少样本有日期列
	dateDistrib   map[int]int    // 业务日期数 → 文件数
	exampleMulti  string         // 一个多天文件的路径
	exampleDates  []string       // 该文件的所有业务日期
	allDates      map[string]int // 所有出现过的业务日期及对应行数
}

func main() {
	targets := []struct {
		label   string
		pattern string
		rootDir string
	}{
		{"京东-客户数据-新老客", "客户数据_新老客", `Z:\信息部\RPA_集团数据看板\京东`},
		{"京东-客户数据-类型", "客户数据_类型", `Z:\信息部\RPA_集团数据看板\京东`},
		{"京东-店铺核心", "店铺数据_核心", `Z:\信息部\RPA_集团数据看板\京东`},
		{"京东-便宜包邮", "便宜包邮", `Z:\信息部\RPA_集团数据看板\京东`},
		{"京东-百亿", "百亿", `Z:\信息部\RPA_集团数据看板\京东`},
		{"京东-秒杀", "秒杀", `Z:\信息部\RPA_集团数据看板\京东`},
		{"京东-交易榜", "交易榜", `Z:\信息部\RPA_集团数据看板\京东`},
		{"京东-热搜榜", "热搜榜", `Z:\信息部\RPA_集团数据看板\京东`},
		{"京东-飙升榜", "飙升榜", `Z:\信息部\RPA_集团数据看板\京东`},
		{"京东-联盟", "京东联盟", `Z:\信息部\RPA_集团数据看板\京东`},
		{"京东-京准通全站", "京准通全站", `Z:\信息部\RPA_集团数据看板\京东`},
		{"京东-京准通非全站", "京准通非全站", `Z:\信息部\RPA_集团数据看板\京东`},
		{"京东自营-客服工作量", "客服_服务工作量", `Z:\信息部\RPA_集团数据看板\京东自营`},
		{"京东自营-销售绩效", "客服_销售绩效", `Z:\信息部\RPA_集团数据看板\京东自营`},
		{"抖音-直播", "自营_直播数据", `Z:\信息部\RPA_集团数据看板\抖音`},
		{"抖音-商品", "自营_商品数据", `Z:\信息部\RPA_集团数据看板\抖音`},
		{"抖音-推广直播间", "推广直播间", `Z:\信息部\RPA_集团数据看板\抖音`},
		{"抖音-推广视频素材", "推广视频素材", `Z:\信息部\RPA_集团数据看板\抖音`},
		{"抖音-主播分析", "主播分析", `Z:\信息部\RPA_集团数据看板\抖音`},
		{"抖音-飞鸽客服", "飞鸽", `Z:\信息部\RPA_集团数据看板\抖音`},
		{"快手-客服考核", "客服_考核数据", `Z:\信息部\RPA_集团数据看板\快手`},
		{"拼多多-交易概况", "销售数据_交易概况", `Z:\信息部\RPA_集团数据看板\拼多多`},
		{"拼多多-商品概况", "销售数据_商品概况", `Z:\信息部\RPA_集团数据看板\拼多多`},
		{"拼多多-商品推广", "商品推广", `Z:\信息部\RPA_集团数据看板\拼多多`},
		{"拼多多-明星店铺", "明星店铺", `Z:\信息部\RPA_集团数据看板\拼多多`},
		{"拼多多-直播推广", "直播推广", `Z:\信息部\RPA_集团数据看板\拼多多`},
		{"拼多多-服务概况", "销售数据_服务概况", `Z:\信息部\RPA_集团数据看板\拼多多`},
		{"拼多多-客服服务", "客服_服务数据", `Z:\信息部\RPA_集团数据看板\拼多多`},
		{"拼多多-客服销售", "客服_销售数据", `Z:\信息部\RPA_集团数据看板\拼多多`},
		{"天猫超市-市场排名", "市场排名", `Z:\信息部\RPA_集团数据看板\天猫超市`},
		{"天猫超市-活动", "活动数据", `Z:\信息部\RPA_集团数据看板\天猫超市`},
		{"天猫超市-品牌", "品牌数据", `Z:\信息部\RPA_集团数据看板\天猫超市`},
		{"天猫超市-行业关键词", "行业关键词", `Z:\信息部\RPA_集团数据看板\天猫超市`},
	}

	results := []*fileStats{}
	for _, t := range targets {
		files := findAll(t.rootDir, t.pattern, 50) // 每种文件最多抽50个样本
		st := &fileStats{
			label:       t.label,
			dateDistrib: map[int]int{},
			allDates:    map[string]int{},
		}
		for _, p := range files {
			biz := scanFileDates(p)
			if biz == nil {
				continue
			}
			st.sampledFiles++
			st.dateColName = biz.dateColName
			if biz.hasDateCol {
				st.hasDateCol++
			}
			st.dateDistrib[biz.businessDateCount]++
			for d, c := range biz.allBusinessDates {
				st.allDates[d] += c
			}
			if biz.businessDateCount > 1 {
				st.multiDayFiles++
				if st.exampleMulti == "" {
					st.exampleMulti = p
					dates := []string{}
					for d := range biz.allBusinessDates {
						dates = append(dates, d)
					}
					sort.Strings(dates)
					st.exampleDates = dates
				}
			}
		}
		results = append(results, st)
	}

	fmt.Println("【探测报告】")
	fmt.Printf("%-30s %10s %10s %10s %10s  %s\n", "文件类型", "样本数", "含日期列", "多天文件", "不同业务日", "示例多天文件")
	for _, r := range results {
		exMulti := ""
		if r.multiDayFiles > 0 {
			exMulti = fmt.Sprintf("  多天示例: %s dates=%v", filepath.Base(r.exampleMulti), r.exampleDates)
		}
		marker := "  "
		if r.multiDayFiles > 0 {
			marker = "⚠️"
		}
		fmt.Printf("%s %-30s %10d %10d %10d %10d%s\n", marker, r.label, r.sampledFiles, r.hasDateCol, r.multiDayFiles, len(r.allDates), exMulti)
	}
	fmt.Println()

	// 单独把有问题的列出来
	fmt.Println("【有多天数据的文件类型（疑似bug）】")
	for _, r := range results {
		if r.multiDayFiles > 0 {
			fmt.Printf("  [%s] 多天文件 %d/%d 示例=%s dates=%v\n", r.label, r.multiDayFiles, r.sampledFiles, filepath.Base(r.exampleMulti), r.exampleDates)
		}
	}
}

type bizDates struct {
	dateColName       string
	hasDateCol        bool
	businessDateCount int
	allBusinessDates  map[string]int
}

func scanFileDates(path string) *bizDates {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) < 2 {
		return nil
	}
	header := rows[0]
	dateCol := -1
	dateName := ""
	for i, h := range header {
		h = strings.TrimSpace(h)
		if h == "日期" || h == "统计日期" || h == "stat_date" || h == "数据日期" || h == "时间" || h == "统计时间" {
			dateCol = i
			dateName = h
			break
		}
	}
	result := &bizDates{
		dateColName:      dateName,
		hasDateCol:       dateCol >= 0,
		allBusinessDates: map[string]int{},
	}
	if dateCol < 0 {
		return result
	}
	for i := 1; i < len(rows); i++ {
		if dateCol < len(rows[i]) {
			v := strings.TrimSpace(rows[i][dateCol])
			if summaryLabels[v] {
				continue
			}
			// 简单判断：包含"-"或"/"的字符串（日期格式）
			if strings.Contains(v, "-") || strings.Contains(v, "/") {
				result.allBusinessDates[v]++
			}
		}
	}
	result.businessDateCount = len(result.allBusinessDates)
	return result
}

func findAll(root, pattern string, max int) []string {
	result := []string{}
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".xlsx") && strings.Contains(path, pattern) {
			result = append(result, path)
			if len(result) >= max {
				return filepath.SkipAll
			}
		}
		return nil
	})
	return result
}
