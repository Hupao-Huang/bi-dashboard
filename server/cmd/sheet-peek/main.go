// sheet-peek.exe <xlsx> [--rows N] [--sheet "name"]: 打印 xlsx 的 sheet 列表 + 行内容
// 默认每 sheet 前 6 行；--rows N 改为前 N 行（0 = 全部）；--sheet 限定单个 sheet
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/xuri/excelize/v2"
)

func main() {
	rowsLimit := flag.Int("rows", 6, "rows per sheet to print (0=all)")
	onlySheet := flag.String("sheet", "", "only print this sheet")
	maxCols := flag.Int("cols", 12, "max columns per row before truncating")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: sheet-peek.exe <xlsx> [--rows N] [--sheet \"name\"] [--cols N]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}
	f, err := excelize.OpenFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(1)
	}
	defer f.Close()
	fmt.Printf("📋 文件: %s\n", flag.Arg(0))
	sheets := f.GetSheetList()
	fmt.Printf("📑 共 %d 个 sheet\n", len(sheets))
	for _, s := range sheets {
		if *onlySheet != "" && s != *onlySheet {
			continue
		}
		rows, _ := f.GetRows(s)
		fmt.Printf("\n=== Sheet: %s (%d 行) ===\n", s, len(rows))
		limit := *rowsLimit
		if limit == 0 || limit > len(rows) {
			limit = len(rows)
		}
		for i := 0; i < limit; i++ {
			r := rows[i]
			if *maxCols > 0 && len(r) > *maxCols {
				r = append(r[:*maxCols], fmt.Sprintf("...(共%d列)", len(rows[i])))
			}
			fmt.Printf("  Row %d: %v\n", i+1, r)
		}
	}
}
