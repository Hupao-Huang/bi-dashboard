package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"
)

// 扫描所有 RPA Excel 文件，按"文件类型"分组，每组取一个样本
// 输出每种文件类型：header[0], header[1], rows[1][0], rows[1][1], rows[2][0]
// 用来判断该类型 Excel 第 0 列是否是日期以及格式

var fileDateRe = regexp.MustCompile(`\d{8}`)

type typeSample struct {
	fileType     string
	sampleFile   string
	header0      string
	header1      string
	row1col0     string
	row1col1     string
	totalRows    int
	sheetName    string
	isXlsx       bool
	dateColIdx   int    // header 里找到的日期列索引（-1 表示未找到）
	dateColName  string // 找到的日期列名
	firstBizRow  int    // 第一个业务日期行索引
	firstBizVal  string // 第一个业务日期值
	firstBizCtx  string // 业务行完整内容（前 5 列）
}

var dateColCandidates = []string{"日期", "统计日期", "统计时间", "时间", "数据日期", "stat_date", "点击时间"}

var summaryLabels = map[string]bool{
	"全部": true, "合计": true, "汇总": true, "总计": true, "总值": true,
	"均值": true, "总": true, "-": true, "": true, "合计值": true, "日期": true, "汇总值": true,
}

func main() {
	root := `Z:\信息部\RPA_集团数据看板`
	if len(os.Args) >= 2 {
		root = os.Args[1]
	}

	samples := map[string]*typeSample{}

	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		lower := strings.ToLower(path)
		isXlsx := strings.HasSuffix(lower, ".xlsx") || strings.HasSuffix(lower, ".xls")
		isJSON := strings.HasSuffix(lower, ".json")
		if !isXlsx && !isJSON {
			return nil
		}

		typeKey := classifyFile(path)
		if typeKey == "" {
			return nil
		}
		if _, ok := samples[typeKey]; ok {
			return nil // 每种类型只取第一个样本
		}

		ts := &typeSample{
			fileType:   typeKey,
			sampleFile: filepath.Base(path),
			isXlsx:     isXlsx,
		}
		if isXlsx {
			f, err := excelize.OpenFile(path)
			if err != nil {
				ts.header0 = fmt.Sprintf("[打开失败: %v]", err)
				samples[typeKey] = ts
				return nil
			}
			sheets := f.GetSheetList()
			ts.dateColIdx = -1
			ts.firstBizRow = -1
			if len(sheets) > 0 {
				ts.sheetName = sheets[0]
				rows, _ := f.GetRows(sheets[0])
				ts.totalRows = len(rows)
				if len(rows) > 0 {
					header := rows[0]
					if len(header) > 0 {
						ts.header0 = header[0]
					}
					if len(header) > 1 {
						ts.header1 = header[1]
					}
					// 在 header 里找日期列
					for i, h := range header {
						hTrim := strings.TrimSpace(h)
						for _, cand := range dateColCandidates {
							if hTrim == cand {
								ts.dateColIdx = i
								ts.dateColName = cand
								break
							}
						}
						if ts.dateColIdx >= 0 {
							break
						}
					}
				}
				if len(rows) > 1 {
					if len(rows[1]) > 0 {
						ts.row1col0 = rows[1][0]
					}
					if len(rows[1]) > 1 {
						ts.row1col1 = rows[1][1]
					}
				}
				// 扫 1~15 行找第一个非汇总标签的业务日期
				col := 0
				if ts.dateColIdx > 0 {
					col = ts.dateColIdx
				}
				for i := 1; i < len(rows) && i < 15; i++ {
					if col >= len(rows[i]) {
						continue
					}
					v := strings.TrimSpace(rows[i][col])
					if summaryLabels[v] {
						continue
					}
					// 看起来像日期（含 "-" "." "/" 或全数字 8 位）
					looksLikeDate := strings.ContainsAny(v, "-./") || (len(v) == 8 && isDigits(v))
					if looksLikeDate {
						ts.firstBizRow = i
						ts.firstBizVal = v
						// 记录前 5 列上下文
						ctx := []string{}
						for j := 0; j < 5 && j < len(rows[i]); j++ {
							ctx = append(ctx, rows[i][j])
						}
						ts.firstBizCtx = strings.Join(ctx, " | ")
						break
					}
				}
			}
			f.Close()
		} else {
			ts.header0 = "[JSON 文件]"
		}
		samples[typeKey] = ts
		return nil
	})

	keys := make([]string, 0, len(samples))
	for k := range samples {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Println("# RPA Excel/JSON 首行样本")
	fmt.Println()
	for _, k := range keys {
		s := samples[k]
		if !s.isXlsx {
			continue
		}
		// 只显示有意义的样本（能打开）
		if strings.HasPrefix(s.header0, "[打开失败") {
			continue
		}
		fmt.Printf("## %s\n", k)
		fmt.Printf("  文件: %s\n", s.sampleFile)
		fmt.Printf("  sheet=%s, 行数=%d\n", s.sheetName, s.totalRows)
		fmt.Printf("  header[0] = %q, header[1] = %q\n", s.header0, s.header1)
		if s.dateColIdx >= 0 {
			fmt.Printf("  ⭐ 日期列: header[%d] = %q\n", s.dateColIdx, s.dateColName)
		} else {
			fmt.Printf("  ❌ 无日期列\n")
		}
		if s.firstBizRow >= 0 {
			fmt.Printf("  首个业务行: row[%d] = %q\n", s.firstBizRow, s.firstBizVal)
			fmt.Printf("    上下文: %s\n", s.firstBizCtx)
		} else {
			fmt.Printf("  首个业务行: 未找到日期数据（前 15 行扫描）\n")
		}
		fmt.Println()
	}
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
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
	name = strings.TrimSuffix(name, ".json")
	segs := strings.Split(name, "_")
	if len(segs) < 4 {
		return platform + "_" + name
	}
	return platform + "_" + strings.Join(segs[3:], "_")
}
