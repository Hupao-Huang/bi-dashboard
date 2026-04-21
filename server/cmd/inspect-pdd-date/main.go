package main

import (
	"bi-dashboard/internal/importutil"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

func main() {
	root, err := importutil.ResolveDataRoot(`Z:\信息部\RPA_集团数据看板\拼多多`)
	if err != nil {
		// 试多个候选路径
		for _, p := range []string{
			`Z:\信息部\RPA_集团数据看板\拼多多`,
			`\\172.16.100.10\松鲜鲜资料库\信息部\RPA_集团数据看板\拼多多`,
		} {
			if info, e := os.Stat(p); e == nil && info.IsDir() {
				root = p
				err = nil
				break
			}
		}
		if err != nil {
			log.Fatalf("路径失败")
		}
	}
	fmt.Printf("根目录: %s\n", root)
	count := 0
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || count >= 3 {
			return nil
		}
		if !strings.HasSuffix(path, ".xlsx") {
			return nil
		}
		if !strings.Contains(path, "交易概况") && !strings.Contains(path, "商品概况") && !strings.Contains(path, "服务概况") {
			return nil
		}
		f, err := excelize.OpenFile(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		rows, _ := f.GetRows(f.GetSheetName(0))
		if len(rows) < 2 {
			return nil
		}
		fmt.Printf("FILE: %s\n", filepath.Base(path))
		fmt.Printf("  header[0]=%q\n", rows[0][0])
		fmt.Printf("  rows[1][0]=%q (len=%d)\n", rows[1][0], len(rows[1][0]))
		if len(rows) > 2 {
			fmt.Printf("  rows[2][0]=%q\n", rows[2][0])
		}
		count++
		return nil
	})
}
