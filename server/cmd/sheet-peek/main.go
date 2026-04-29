// sheet-peek.exe <xlsx>: 打印 xlsx 的 sheet 列表 + 每个 sheet 前 6 行
package main

import (
	"fmt"
	"os"

	"github.com/xuri/excelize/v2"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: sheet-peek.exe <xlsx>")
		os.Exit(2)
	}
	f, err := excelize.OpenFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(1)
	}
	defer f.Close()
	fmt.Printf("📋 文件: %s\n", os.Args[1])
	sheets := f.GetSheetList()
	fmt.Printf("📑 共 %d 个 sheet\n", len(sheets))
	for _, s := range sheets {
		rows, _ := f.GetRows(s)
		fmt.Printf("\n=== Sheet: %s (%d 行) ===\n", s, len(rows))
		limit := 6
		if len(rows) < limit {
			limit = len(rows)
		}
		for i := 0; i < limit; i++ {
			r := rows[i]
			if len(r) > 12 {
				r = append(r[:12], fmt.Sprintf("...(共%d列)", len(rows[i])))
			}
			fmt.Printf("  Row %d: %v\n", i+1, r)
		}
	}
}
