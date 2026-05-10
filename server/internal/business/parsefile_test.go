package business

// parsefile_test.go — ParseFile / parseKPISheet / parseBackOfficeSheet 主路径
// 用 excelize 在 tmp 目录造 minimal valid .xlsx, 触发 layoutFull/layoutCompact/layoutMinimal/KPI/BackOffice 各路径.
//
// 已 Read parser.go:
//   - ParseFile (line 78): 入口, snapshot/year 校验 + 遍历 GetSheetList
//   - parseSheetName (216): "1、电商" / "总" / "5.1xxx" / "线下-华东" / 大区名
//   - detectLayout (315): "项目"+"合计-预算"+"1月-预算" → Compact, +"预算-年初" → Full
//   - parseRowPeriod (422): pm=0 cols[1-7] (Full), pm=1+ cols[8+] step 4
//   - parseKPISheet (764): subject=col[1], pm=0 budget=col[2]/actual=col[3]/ar=col[4]
//   - parseBackOfficeSheet (665): col[0]=subject, col[1]=年度合计, col[2,3]=1月预算/实际, ...

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

// helper: 写一个 xlsx 文件到 tmp, 返回路径
func writeTestXlsx(t *testing.T, builder func(f *excelize.File)) string {
	t.Helper()
	f := excelize.NewFile()
	builder(f)
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("save xlsx: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close xlsx: %v", err)
	}
	return path
}

// ---------- 参数校验 ----------

func TestParseFileBadSnapshotYear(t *testing.T) {
	_, err := ParseFile("/dummy.xlsx", 2010, 4, 2026)
	if err == nil {
		t.Error("snapshotYear=2010 (越界) 应返 err")
	}
}

func TestParseFileBadSnapshotMonth(t *testing.T) {
	_, err := ParseFile("/dummy.xlsx", 2026, 13, 2026)
	if err == nil {
		t.Error("snapshotMonth=13 应返 err")
	}
}

func TestParseFileBadYear(t *testing.T) {
	_, err := ParseFile("/dummy.xlsx", 2026, 4, 2010)
	if err == nil {
		t.Error("year=2010 应返 err")
	}
}

func TestParseFileNotFound(t *testing.T) {
	_, err := ParseFile("/path/that/does/not/exist.xlsx", 2026, 4, 2026)
	if err == nil {
		t.Error("不存在文件应返 err")
	}
}

// ---------- ParseFile layoutFull happy path ----------

// 造一个最小 layoutFull xlsx: sheet "1、电商" + Row 2 表头 + Row 3 数据
func TestParseFileMinimalLayoutFull(t *testing.T) {
	path := writeTestXlsx(t, func(f *excelize.File) {
		// excelize 默认有 Sheet1, 先重命名
		f.SetSheetName("Sheet1", "1、电商")

		// Row 1-2 任意 (跳过)
		f.SetCellStr("1、电商", "A1", "电商部门预决算")
		f.SetCellStr("1、电商", "A2", "")

		// Row 3 (index 2) 表头, 触发 layoutFull
		f.SetCellStr("1、电商", "A3", "项目")
		f.SetCellStr("1、电商", "B3", "预算-年初")
		f.SetCellStr("1、电商", "C3", "占比-年初")
		f.SetCellStr("1、电商", "D3", "合计-预算")
		f.SetCellStr("1、电商", "E3", "占比-预算")
		f.SetCellStr("1、电商", "F3", "合计-实际")
		f.SetCellStr("1、电商", "G3", "占比-实际")
		f.SetCellStr("1、电商", "H3", "达成率")
		f.SetCellStr("1、电商", "I3", "1月-预算")

		// Row 4 (index 3) 数据行: 一、营业收入 (level 1)
		f.SetCellStr("1、电商", "A4", "一、营业收入")
		f.SetCellStr("1、电商", "B4", "1000000") // BudgetYearStart
		f.SetCellStr("1、电商", "D4", "100000")  // Budget (合计)
		f.SetCellStr("1、电商", "F4", "95000")   // Actual
		f.SetCellStr("1、电商", "H4", "95%")     // AchievementRate

		// Row 5 数据行: GMV数据 group header (跑哥定义白名单)
		f.SetCellStr("1、电商", "A5", "GMV数据")

		// Row 6 数据行: GMV合计 (level 2)
		f.SetCellStr("1、电商", "A6", "GMV合计")
		f.SetCellStr("1、电商", "D6", "200000")
	})

	result, err := ParseFile(path, 2026, 4, 2026)
	if err != nil {
		t.Fatalf("ParseFile err: %v", err)
	}

	if result.Year != 2026 {
		t.Errorf("Year=%d want 2026", result.Year)
	}
	if result.SheetsHandled == 0 {
		t.Error("应识别至少 1 个 sheet")
	}
	if len(result.Rows) == 0 {
		t.Error("应解析至少 1 行")
	}
	// 验证电商 channel 出现
	hasEcommerce := false
	for _, c := range result.Channels {
		if c == "电商" {
			hasEcommerce = true
		}
	}
	if !hasEcommerce {
		t.Errorf("Channels 应含电商, got %v", result.Channels)
	}
}

// ---------- parseKPISheet (经营指标 sheet) ----------

func TestParseFileKPISheet(t *testing.T) {
	path := writeTestXlsx(t, func(f *excelize.File) {
		f.SetSheetName("Sheet1", "经营指标")

		// Row 1-3 表头任意
		f.SetCellStr("经营指标", "A1", "经营指标")
		f.SetCellStr("经营指标", "A2", "")
		f.SetCellStr("经营指标", "A3", "序号")
		f.SetCellStr("经营指标", "B3", "指标项目")
		f.SetCellStr("经营指标", "C3", "预算数")

		// Row 4 (index 3): 数据行
		f.SetCellStr("经营指标", "A4", "1")
		f.SetCellStr("经营指标", "B4", "营收增长率")
		f.SetCellStr("经营指标", "C4", "10000000") // budget
		f.SetCellStr("经营指标", "D4", "8500000")   // actual
		f.SetCellStr("经营指标", "E4", "85%")       // achievement_rate
		f.SetCellStr("经营指标", "F4", "800000")    // 1月 actual
	})

	result, err := ParseFile(path, 2026, 4, 2026)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.SheetsHandled == 0 {
		t.Error("KPI sheet 应被识别")
	}
	hasKPI := false
	for _, c := range result.Channels {
		if c == "经营指标" {
			hasKPI = true
		}
	}
	if !hasKPI {
		t.Errorf("应含'经营指标' channel, got %v", result.Channels)
	}
	if len(result.Rows) == 0 {
		t.Error("KPI 应至少解析 1 行 (营收增长率)")
	}
}

// ---------- parseBackOfficeSheet (中后台合计) ----------

func TestParseFileBackOfficeSheet(t *testing.T) {
	path := writeTestXlsx(t, func(f *excelize.File) {
		f.SetSheetName("Sheet1", "中后台合计")

		// Row 1-5 任意 (源码 line 669 i=5 起)
		for i := 1; i <= 5; i++ {
			f.SetCellStr("中后台合计", "A"+string(rune('0'+i)), "")
		}
		f.SetCellStr("中后台合计", "A1", "中后台合计")

		// Row 6 (index 5): 数据行
		f.SetCellStr("中后台合计", "A6", "品牌费用") // 分组 header (白名单)

		// Row 7 (index 6): 数据科目
		f.SetCellStr("中后台合计", "A7", "广告费")
		f.SetCellStr("中后台合计", "B7", "120000") // 全年合计-实际
		f.SetCellStr("中后台合计", "C7", "10000")  // 1月-预算
		f.SetCellStr("中后台合计", "D7", "9500")   // 1月-实际
	})

	result, err := ParseFile(path, 2026, 4, 2026)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.SheetsHandled == 0 {
		t.Error("中后台合计应被识别")
	}
	hasBackOffice := false
	for _, c := range result.Channels {
		if c == "中后台" {
			hasBackOffice = true
		}
	}
	if !hasBackOffice {
		t.Errorf("应含'中后台' channel, got %v", result.Channels)
	}
}

// ---------- 不识别的 sheet 应跳过 ----------

func TestParseFileSkipsUnknownSheet(t *testing.T) {
	path := writeTestXlsx(t, func(f *excelize.File) {
		// 不匹配任何模式的 sheet 名
		f.SetSheetName("Sheet1", "随便写的内容XYZ")
		f.SetCellStr("随便写的内容XYZ", "A1", "x")
	})

	result, err := ParseFile(path, 2026, 4, 2026)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.SheetsSkipped == 0 {
		t.Errorf("无法识别的 sheet 应记录 SheetsSkipped, got %d", result.SheetsSkipped)
	}
	if len(result.Rows) != 0 {
		t.Errorf("不识别 sheet 不应产数据, got %d rows", len(result.Rows))
	}
}

// ---------- 数据 too short 跳过 ----------

func TestParseFileSheetTooShortRows(t *testing.T) {
	path := writeTestXlsx(t, func(f *excelize.File) {
		f.SetSheetName("Sheet1", "1、电商")
		f.SetCellStr("1、电商", "A1", "x") // 只有 1 row, < 4 → 跳过
	})

	result, err := ParseFile(path, 2026, 4, 2026)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.SheetsHandled != 0 {
		t.Errorf("rows < 4 应跳过, got SheetsHandled=%d", result.SheetsHandled)
	}
}

// ---------- 工具函数: 测试用辅助 (未读写) ----------

func TestWriteTestXlsxHelperWorks(t *testing.T) {
	// sanity: 验证 helper 自身工作 (避免 false negative)
	path := writeTestXlsx(t, func(f *excelize.File) {
		f.SetCellStr("Sheet1", "A1", "test")
	})
	if _, err := os.Stat(path); err != nil {
		t.Errorf("xlsx 应存在: %v", err)
	}
}
