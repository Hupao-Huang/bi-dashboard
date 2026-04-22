package main

// probe-header-variance: 扫描所有 RPA Excel，按"文件类型"分组，比对同一类型的
// header 在不同日期的样本里是否一致。列顺序或字段名变过的类型会被标记为 ⚠️
// 差异数量会明确报出，方便定位受影响的 import 函数。

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"
)

type headerVariant struct {
	header   []string
	samples  []string // 采用这种 header 的文件名（最多记 3 个）
	count    int
}

type typeGroup struct {
	fileType string
	variants []*headerVariant
}

func (g *typeGroup) addSample(header []string, sample string) {
	for _, v := range g.variants {
		if equalSlice(v.header, header) {
			v.count++
			if len(v.samples) < 3 {
				v.samples = append(v.samples, sample)
			}
			return
		}
	}
	g.variants = append(g.variants, &headerVariant{
		header:  append([]string{}, header...),
		samples: []string{sample},
		count:   1,
	})
}

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i]) != strings.TrimSpace(b[i]) {
			return false
		}
	}
	return true
}

func main() {
	root := `Z:\信息部\RPA_集团数据看板`
	if len(os.Args) >= 2 {
		root = os.Args[1]
	}

	groups := map[string]*typeGroup{}

	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		lower := strings.ToLower(path)
		if !(strings.HasSuffix(lower, ".xlsx") || strings.HasSuffix(lower, ".xls")) {
			return nil
		}
		// 跳过 0 平台模板
		if strings.Contains(path, "0平台模板") {
			return nil
		}
		// 跳过 .xls.xlsx 转换文件
		if strings.HasSuffix(lower, ".xls.xlsx") {
			return nil
		}
		typeKey := classifyFile(path)
		if typeKey == "" {
			return nil
		}
		// 每个类型最多读 30 个样本即可（避免超长扫描）
		g, ok := groups[typeKey]
		if !ok {
			g = &typeGroup{fileType: typeKey}
			groups[typeKey] = g
		}
		// 控制读取量：每种类型每种 header 变体最多记 3 个，总变体数超过 5 就 skip
		totalSamples := 0
		for _, v := range g.variants {
			totalSamples += v.count
		}
		if totalSamples >= 50 {
			return nil
		}

		f, err := excelize.OpenFile(path)
		if err != nil {
			return nil
		}
		sheets := f.GetSheetList()
		if len(sheets) > 0 {
			rows, _ := f.GetRows(sheets[0])
			header := findHeaderRow(rows)
			if header != nil {
				g.addSample(header, filepath.Base(path))
			}
		}
		f.Close()
		return nil
	})

	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 先输出有变异的（问题清单）
	fmt.Println("# ⚠️ Header 列顺序/字段名有变异的文件类型")
	fmt.Println()
	variantCount := 0
	for _, k := range keys {
		g := groups[k]
		if len(g.variants) <= 1 {
			continue
		}
		variantCount++
		fmt.Printf("## %s (%d 种 header 变体)\n", k, len(g.variants))
		for i, v := range g.variants {
			fmt.Printf("  变体 %d (出现 %d 次, 列数=%d):\n", i+1, v.count, len(v.header))
			fmt.Printf("    样本: %s\n", strings.Join(v.samples, ", "))
			// 打印前 8 列对比
			preview := make([]string, 0, 8)
			for j := 0; j < 8 && j < len(v.header); j++ {
				preview = append(preview, fmt.Sprintf("[%d]%s", j, strings.TrimSpace(v.header[j])))
			}
			fmt.Printf("    前8列: %s\n", strings.Join(preview, " | "))
		}
		fmt.Println()
	}

	if variantCount == 0 {
		fmt.Println("（无变异 — 所有文件类型 header 稳定）")
	}

	fmt.Println()
	fmt.Println("# ✅ Header 稳定的文件类型（一种变体）")
	fmt.Println()
	for _, k := range keys {
		g := groups[k]
		if len(g.variants) != 1 {
			continue
		}
		v := g.variants[0]
		fmt.Printf("- %s  (%d 样本, 列数=%d)\n", k, v.count, len(v.header))
	}
}

// findHeaderRow 在前 5 行里找真正的表头行
// 规则：跳过 meta 行（如"查询条件 | 广告主PIN: ...; 时间: ...; 品牌: ..."）。
// 表头特征：每个非空单元格都是短字符串 (< 30 字符)，没有"; "或"冒号+空格+内容"的长文本。
// 此外还要排除形如 ["表名", None, None, None] 的标题行，及 ["日期", "汇总", ...] 这类数据汇总行。
func findHeaderRow(rows [][]string) []string {
	for i := 0; i < 5 && i < len(rows); i++ {
		row := rows[i]
		if !looksLikeHeader(row) {
			continue
		}
		return row
	}
	if len(rows) > 0 {
		return rows[0]
	}
	return nil
}

func looksLikeHeader(row []string) bool {
	nonEmpty := 0
	for _, c := range row {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		nonEmpty++
		// meta 行特征：长文本 / 含"; "分号分隔项 / 含"冒号+空格+内容"如 "PIN: xxx"
		if len(c) > 30 {
			return false
		}
		if strings.Contains(c, "; ") {
			return false
		}
	}
	// 至少需要 3 个非空单元格才视为合法表头（排除标题行如 ["报表名称"]）
	return nonEmpty >= 3
}

func classifyFile(path string) string {
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

	base := filepath.Base(path)
	name := strings.TrimSuffix(strings.TrimSuffix(base, ".xlsx"), ".xls")
	segs := strings.Split(name, "_")
	if len(segs) < 4 {
		return platform + "_" + name
	}
	return platform + "_" + strings.Join(segs[3:], "_")
}
