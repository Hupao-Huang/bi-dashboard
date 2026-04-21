package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// 探测 RPA 文件"文件名日期 vs 文件内业务日期"的 delta（滞后天数）
// 输出每种文件类型的 delta 分布，一眼看出哪些需要改 stat_date 来源

var (
	fileDateRe = regexp.MustCompile(`(\d{8})`)
	// 汇总行标签（不算业务日期）
	summaryLabels = map[string]bool{
		"全部": true, "合计": true, "汇总": true, "总计": true, "总值": true,
		"均值": true, "总": true, "-": true, "": true, "合计值": true,
	}
)

func main() {
	root := `Z:\信息部\RPA_集团数据看板`
	if len(os.Args) >= 2 {
		root = os.Args[1]
	}

	// file type → delta 分布 → count
	stats := map[string]*typeStat{}

	// 遍历所有 xlsx
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".xlsx") {
			return nil
		}

		base := filepath.Base(path)
		// 提取文件名日期 (YYYYMMDD)
		m := fileDateRe.FindStringSubmatch(base)
		if m == nil {
			return nil
		}
		fileDate, err := time.Parse("20060102", m[1])
		if err != nil {
			return nil
		}

		// 归类：取 {平台}_{文件类型标签}，去除日期和店铺名
		typeKey := classifyFile(path, base)
		if typeKey == "" {
			return nil
		}

		// 读 Excel 第一数据行的第 0 列（或 header 里找日期列）
		bizDateStr := readFirstBusinessDate(path)
		st := stats[typeKey]
		if st == nil {
			st = &typeStat{
				deltaDistrib: map[int]int{},
				noDateRows:   0,
			}
			stats[typeKey] = st
		}
		st.total++
		if bizDateStr == "" {
			st.noDateRows++
			st.exampleNoDate = path
			return nil
		}
		bizDate, err := parseBizDate(bizDateStr)
		if err != nil {
			st.parseErr++
			st.exampleParseErr = fmt.Sprintf("%s (cell=%q)", path, bizDateStr)
			return nil
		}
		delta := int(fileDate.Sub(bizDate).Hours() / 24)
		st.deltaDistrib[delta]++
		if delta != 0 && st.exampleLag == "" {
			st.exampleLag = fmt.Sprintf("%s: 文件名=%s vs 内部=%s (delta=%d)", base, fileDate.Format("2006-01-02"), bizDate.Format("2006-01-02"), delta)
		}
		return nil
	})

	// 排序输出
	typeKeys := make([]string, 0, len(stats))
	for k := range stats {
		typeKeys = append(typeKeys, k)
	}
	sort.Strings(typeKeys)

	fmt.Println("# RPA 文件名日期 vs 内部业务日期 滞后分析")
	fmt.Println()
	fmt.Println("说明: delta = 文件名日期 - 内部业务日期")
	fmt.Println("  delta=0  → 无滞后（文件名日期=业务日期）")
	fmt.Println("  delta>0  → RPA 滞后 N 天（正值）")
	fmt.Println("  delta<0  → 未来日期（异常）")
	fmt.Println()
	fmt.Printf("%-50s %6s %8s %8s %8s %8s %8s %8s %6s %6s\n",
		"文件类型", "总数", "d=0", "d=1", "d=2", "d=3", "d=4+", "d<0", "无日期", "解析错")
	fmt.Println(strings.Repeat("-", 140))

	for _, k := range typeKeys {
		st := stats[k]
		d0 := st.deltaDistrib[0]
		d1 := st.deltaDistrib[1]
		d2 := st.deltaDistrib[2]
		d3 := st.deltaDistrib[3]
		dNeg := 0
		dBig := 0
		for d, c := range st.deltaDistrib {
			if d < 0 {
				dNeg += c
			}
			if d >= 4 {
				dBig += c
			}
		}
		marker := "  "
		// 标记需要改的类型：任一样本 delta != 0 说明文件内有真实日期且跟文件名不一致
		nonZero := st.total - d0 - st.noDateRows - st.parseErr
		if nonZero > 0 {
			marker = "⚠️"
		}
		fmt.Printf("%s %-48s %6d %8d %8d %8d %8d %8d %8d %6d %6d\n",
			marker, k, st.total, d0, d1, d2, d3, dBig, dNeg, st.noDateRows, st.parseErr)
	}

	fmt.Println()
	fmt.Println("# 样本示例")
	for _, k := range typeKeys {
		st := stats[k]
		if st.exampleLag != "" {
			fmt.Printf("  [滞后] %s: %s\n", k, st.exampleLag)
		}
		if st.exampleParseErr != "" {
			fmt.Printf("  [解析错] %s: %s\n", k, st.exampleParseErr)
		}
	}
}

type typeStat struct {
	total           int
	deltaDistrib    map[int]int // delta 天数 → 文件数
	noDateRows      int
	parseErr        int
	exampleLag      string
	exampleParseErr string
	exampleNoDate   string
}

// classifyFile 从路径 + 文件名提取文件类型 key
// 典型路径: Z:\信息部\RPA_集团数据看板\天猫\2026\2026-04-20\松鲜鲜调味品旗舰店\天猫_20260420_松鲜鲜调味品旗舰店_生意参谋_业绩询单.xlsx
// 返回: "天猫_生意参谋_业绩询单"
func classifyFile(path, base string) string {
	// 平台 = 路径里"RPA_集团数据看板"后的第一级目录
	idx := strings.Index(path, "RPA_集团数据看板")
	if idx < 0 {
		return ""
	}
	rest := path[idx+len("RPA_集团数据看板"):]
	rest = strings.Trim(rest, `\/`)
	parts := strings.Split(rest, `\`)
	if len(parts) < 1 {
		return ""
	}
	platform := parts[0]

	// 从文件名提取"文件类型"（去除日期和店铺名）
	name := strings.TrimSuffix(base, ".xlsx")
	name = strings.TrimSuffix(name, ".xls")
	// 按下划线拆
	segs := strings.Split(name, "_")
	if len(segs) < 4 {
		return platform + "_" + name
	}
	// 典型格式: 平台_日期_店铺_来源_类型[_类型2]
	// 取第 3 段之后拼接
	return platform + "_" + strings.Join(segs[3:], "_")
}

// readFirstBusinessDate 从 Excel 读第一个业务日期
// 策略：
//  1. 读 header，如果有"日期/统计日期/统计时间/时间/数据日期"列，用该列
//  2. 否则用第 0 列
//  3. 从数据行往下找，跳过汇总标签
func readFirstBusinessDate(path string) string {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return ""
	}
	rows, _ := f.GetRows(sheets[0])
	if len(rows) < 2 {
		return ""
	}

	// 找 header 里的日期列索引
	header := rows[0]
	dateCol := 0
	for i, h := range header {
		h = strings.TrimSpace(h)
		if h == "日期" || h == "统计日期" || h == "统计时间" || h == "时间" || h == "数据日期" || h == "stat_date" || h == "点击时间" {
			dateCol = i
			break
		}
	}

	// 有时有多行表头（比如接待评价是行 0/1 两行表头，行 2 才是数据）
	// 策略：从 rows[1] 开始，找第一个 dateCol 位置不在 summaryLabels 里的行
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if dateCol >= len(row) {
			continue
		}
		v := strings.TrimSpace(row[dateCol])
		if summaryLabels[v] {
			continue
		}
		if v == "" {
			continue
		}
		return v
	}
	return ""
}

// parseBizDate 兼容多种 Excel 日期格式
func parseBizDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "年", "-")
	s = strings.ReplaceAll(s, "月", "-")
	s = strings.ReplaceAll(s, "日", "")
	for _, layout := range []string{"2006-01-02", "2006-1-2", "20060102"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	// 补 0 再试
	parts := strings.Split(s, "-")
	if len(parts) == 3 {
		y, m, d := parts[0], parts[1], parts[2]
		if len(m) == 1 {
			m = "0" + m
		}
		if len(d) == 1 {
			d = "0" + d
		}
		if len(y) == 4 {
			return time.Parse("2006-01-02", y+"-"+m+"-"+d)
		}
	}
	return time.Time{}, fmt.Errorf("无法解析: %q", s)
}
