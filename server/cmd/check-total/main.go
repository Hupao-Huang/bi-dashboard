package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"github.com/xuri/excelize/v2"
)

func main() {
	// 主播分析
	f1, _ := excelize.OpenFile(`Z:\信息部\RPA_集团数据看板\抖音\2026\20260401\松鲜鲜官方旗舰店\抖音_20260401_松鲜鲜官方旗舰店_自营_主播分析.xlsx`)
	if f1 != nil {
		rows, _ := f1.GetRows(f1.GetSheetName(0))
		fmt.Printf("主播分析: %d行\n表头: %s\n第1行: %s\n\n", len(rows), strings.Join(rows[0], " | "), strings.Join(rows[1], " | "))
		f1.Close()
	}
	// 推广视频素材
	f2, _ := excelize.OpenFile(`Z:\信息部\RPA_集团数据看板\抖音\2026\20260401\松鲜鲜官方旗舰店\抖音_20260401_松鲜鲜官方旗舰店_自营_推广视频素材.xlsx`)
	if f2 != nil {
		rows, _ := f2.GetRows(f2.GetSheetName(0))
		fmt.Printf("推广视频素材: %d行\n表头: %s\n\n", len(rows), strings.Join(rows[0], " | "))
		f2.Close()
	}
	// 分销推素材
	files, _ := filepath.Glob(`Z:\信息部\RPA_集团数据看板\抖音分销\2026\20260401\*\*推素材*`)
	if len(files) > 0 {
		f3, _ := excelize.OpenFile(files[0])
		if f3 != nil {
			rows, _ := f3.GetRows(f3.GetSheetName(0))
			fmt.Printf("分销推素材(%s): %d行\n表头: %s\n", filepath.Base(files[0]), len(rows), strings.Join(rows[0], " | "))
			f3.Close()
		}
	}
}
