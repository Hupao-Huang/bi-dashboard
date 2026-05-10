package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/xuri/excelize/v2"
)

// ExportFinanceReport GET /api/finance/report/export - 按当前筛选条件导出 xlsx
func (h *DashboardHandler) ExportFinanceReport(w http.ResponseWriter, r *http.Request) {
	h.logAudit(r, "export", "财务报表导出", map[string]interface{}{"query": r.URL.RawQuery})
	// 复用 GetFinanceReport 的参数解析和数据聚合，但输出改为 xlsx
	rw := &captureWriter{header: http.Header{}}
	h.GetFinanceReport(rw, r)
	if rw.statusCode >= 400 {
		w.WriteHeader(rw.statusCode)
		w.Write(rw.body)
		return
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			YearStart  int            `json:"yearStart"`
			YearEnd    int            `json:"yearEnd"`
			MonthStart int            `json:"monthStart"`
			MonthEnd   int            `json:"monthEnd"`
			Channels   []string       `json:"channels"`
			YearMonths []string       `json:"yearMonths"`
			Rows       []FinReportRow `json:"rows"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rw.body, &resp); err != nil {
		writeError(w, 500, "序列化失败")
		return
	}
	d := resp.Data
	multi := len(d.Channels) > 1

	xf := excelize.NewFile()
	defer xf.Close()
	sheet := "财务报表"
	xf.SetSheetName("Sheet1", sheet)

	// 样式：金额千分位、占比百分比、表头加粗、分组行灰底、高亮行蓝色加粗
	amountFmt := "#,##0.00;[Red]-#,##0.00"
	ratioFmt := "0.00%"
	amountStyle, _ := xf.NewStyle(&excelize.Style{
		NumFmt:       0,
		CustomNumFmt: &amountFmt,
		Alignment:    &excelize.Alignment{Horizontal: "right", Vertical: "center"},
	})
	ratioStyle, _ := xf.NewStyle(&excelize.Style{
		CustomNumFmt: &ratioFmt,
		Alignment:    &excelize.Alignment{Horizontal: "right", Vertical: "center"},
		Font:         &excelize.Font{Color: "64748B", Size: 10},
	})
	headerStyle, _ := xf.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"1E40AF"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "FFFFFF", Style: 1},
			{Type: "right", Color: "FFFFFF", Style: 1},
			{Type: "top", Color: "FFFFFF", Style: 1},
			{Type: "bottom", Color: "FFFFFF", Style: 1},
		},
	})
	subjectStyle, _ := xf.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Horizontal: "left", Vertical: "center"},
	})
	groupStyle, _ := xf.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "0F172A"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"E2E8F0"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "left", Vertical: "center"},
	})
	highlightAmount, _ := xf.NewStyle(&excelize.Style{
		CustomNumFmt: &amountFmt,
		Alignment:    &excelize.Alignment{Horizontal: "right", Vertical: "center"},
		Font:         &excelize.Font{Bold: true, Color: "1E40AF"},
		Fill:         excelize.Fill{Type: "pattern", Color: []string{"EFF6FF"}, Pattern: 1},
	})
	highlightSubject, _ := xf.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Horizontal: "left", Vertical: "center"},
		Font:      &excelize.Font{Bold: true, Color: "1E40AF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"EFF6FF"}, Pattern: 1},
	})

	highlightCodes := map[string]bool{
		"GMV_TOTAL": true, "REV_MAIN": true, "COST_MAIN": true, "PROFIT_GROSS": true,
		"PROFIT_OP": true, "NET_PROFIT": true, "PROFIT_TOTAL": true,
	}

	// 表头（两行：第一行分组，第二行子列）
	type colSpec struct {
		kind string // amount/ratio
	}
	var colSpecs []colSpec
	// 建表头并合并
	writeHeader := func() {
		// 第一行 group header
		xf.SetCellValue(sheet, "A1", "科目")
		xf.MergeCell(sheet, "A1", "A2")
		col := 2
		writeGroup := func(groupLabel string) {
			// 金额 + 占比 [+ 每渠道 金额+占比]
			subCount := 2
			if multi {
				subCount += len(d.Channels) * 2
			}
			startCol, _ := excelize.ColumnNumberToName(col)
			endCol, _ := excelize.ColumnNumberToName(col + subCount - 1)
			xf.SetCellValue(sheet, fmt.Sprintf("%s1", startCol), groupLabel)
			if subCount > 1 {
				xf.MergeCell(sheet, fmt.Sprintf("%s1", startCol), fmt.Sprintf("%s1", endCol))
			}
			// 第二行子列名
			titleTotal := "金额"
			if multi {
				titleTotal = "总"
			}
			c1, _ := excelize.ColumnNumberToName(col)
			xf.SetCellValue(sheet, fmt.Sprintf("%s2", c1), titleTotal)
			colSpecs = append(colSpecs, colSpec{kind: "amount"})
			col++
			c2, _ := excelize.ColumnNumberToName(col)
			xf.SetCellValue(sheet, fmt.Sprintf("%s2", c2), "占比")
			colSpecs = append(colSpecs, colSpec{kind: "ratio"})
			col++
			if multi {
				for _, ch := range d.Channels {
					cc, _ := excelize.ColumnNumberToName(col)
					xf.SetCellValue(sheet, fmt.Sprintf("%s2", cc), ch)
					colSpecs = append(colSpecs, colSpec{kind: "amount"})
					col++
					rc, _ := excelize.ColumnNumberToName(col)
					xf.SetCellValue(sheet, fmt.Sprintf("%s2", rc), "占比")
					colSpecs = append(colSpecs, colSpec{kind: "ratio"})
					col++
				}
			}
		}
		writeGroup("区间合计")
		for _, ym := range d.YearMonths {
			writeGroup(ym)
		}
	}
	writeHeader()

	totalCols := 1 + len(colSpecs)
	lastColName, _ := excelize.ColumnNumberToName(totalCols)

	// 表头样式
	xf.SetCellStyle(sheet, "A1", fmt.Sprintf("%s2", lastColName), headerStyle)
	xf.SetRowHeight(sheet, 1, 24)
	xf.SetRowHeight(sheet, 2, 22)

	// 列宽
	xf.SetColWidth(sheet, "A", "A", 28)
	for i := 0; i < len(colSpecs); i++ {
		cn, _ := excelize.ColumnNumberToName(i + 2)
		if colSpecs[i].kind == "amount" {
			xf.SetColWidth(sheet, cn, cn, 16)
		} else {
			xf.SetColWidth(sheet, cn, cn, 10)
		}
	}

	// 数据行
	for ri, row := range d.Rows {
		rowIdx := ri + 3
		isHL := highlightCodes[row.Code] && row.Level == 2
		// 科目列
		xf.SetCellValue(sheet, fmt.Sprintf("A%d", rowIdx), displayName(row))
		if row.Level == 1 {
			// 分组行整行灰底
			xf.MergeCell(sheet, fmt.Sprintf("A%d", rowIdx), fmt.Sprintf("%s%d", lastColName, rowIdx))
			xf.SetCellStyle(sheet, fmt.Sprintf("A%d", rowIdx), fmt.Sprintf("%s%d", lastColName, rowIdx), groupStyle)
			continue
		}
		if isHL {
			xf.SetCellStyle(sheet, fmt.Sprintf("A%d", rowIdx), fmt.Sprintf("A%d", rowIdx), highlightSubject)
		} else {
			xf.SetCellStyle(sheet, fmt.Sprintf("A%d", rowIdx), fmt.Sprintf("A%d", rowIdx), subjectStyle)
		}

		col := 2
		setCell := func(c FinCell) {
			cn, _ := excelize.ColumnNumberToName(col)
			ref := fmt.Sprintf("%s%d", cn, rowIdx)
			if c.Amount != 0 {
				xf.SetCellValue(sheet, ref, c.Amount)
			}
			if isHL {
				xf.SetCellStyle(sheet, ref, ref, highlightAmount)
			} else {
				xf.SetCellStyle(sheet, ref, ref, amountStyle)
			}
			col++

			rn, _ := excelize.ColumnNumberToName(col)
			rref := fmt.Sprintf("%s%d", rn, rowIdx)
			if c.Ratio != nil && !isGmvCategory(row.Category) {
				xf.SetCellValue(sheet, rref, *c.Ratio)
			}
			xf.SetCellStyle(sheet, rref, rref, ratioStyle)
			col++
		}
		setCell(row.Total.RangeTotal)
		if multi {
			for _, ch := range d.Channels {
				cs := findChannelSeries(row.ByChannel, ch)
				if cs != nil {
					setCell(cs.RangeTotal)
				} else {
					col += 2
				}
			}
		}
		for _, ym := range d.YearMonths {
			setCell(row.Total.Cells[ym])
			if multi {
				for _, ch := range d.Channels {
					cs := findChannelSeries(row.ByChannel, ch)
					if cs != nil {
						setCell(cs.Cells[ym])
					} else {
						col += 2
					}
				}
			}
		}
	}

	// 冻结前 2 行 + 首列
	xf.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      1,
		YSplit:      2,
		TopLeftCell: "B3",
		ActivePane:  "bottomRight",
	})

	filename := fmt.Sprintf("财务报表_%d-%d_%d-%dM.xlsx", d.YearStart, d.YearEnd, d.MonthStart, d.MonthEnd)
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename*=UTF-8''%s`, urlEscape(filename)))
	xf.Write(w)
}

func displayName(row FinReportRow) string {
	if row.SubChannel != "" {
		return "· " + row.SubChannel
	}
	return row.Name
}

func isGmvCategory(cat string) bool { return cat == "GMV" }

func findChannelSeries(list []FinChannelSeries, ch string) *FinSeries {
	for i := range list {
		if list[i].Channel == ch {
			return &list[i].Series
		}
	}
	return nil
}

func urlEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x80 && (r == '-' || r == '_' || r == '.' || r == '~' ||
			(r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			b.WriteRune(r)
		} else {
			buf := make([]byte, 4)
			n := 0
			if r < 0x80 {
				buf[0] = byte(r)
				n = 1
			} else if r < 0x800 {
				buf[0] = 0xC0 | byte(r>>6)
				buf[1] = 0x80 | byte(r)&0x3F
				n = 2
			} else if r < 0x10000 {
				buf[0] = 0xE0 | byte(r>>12)
				buf[1] = 0x80 | byte(r>>6)&0x3F
				buf[2] = 0x80 | byte(r)&0x3F
				n = 3
			} else {
				buf[0] = 0xF0 | byte(r>>18)
				buf[1] = 0x80 | byte(r>>12)&0x3F
				buf[2] = 0x80 | byte(r>>6)&0x3F
				buf[3] = 0x80 | byte(r)&0x3F
				n = 4
			}
			for i := 0; i < n; i++ {
				b.WriteString(fmt.Sprintf("%%%02X", buf[i]))
			}
		}
	}
	return b.String()
}

// captureWriter 拦截 GetFinanceReport 的输出，用于导出复用
type captureWriter struct {
	header     http.Header
	body       []byte
	statusCode int
}

func (c *captureWriter) Header() http.Header         { return c.header }
func (c *captureWriter) Write(b []byte) (int, error) { c.body = append(c.body, b...); return len(b), nil }
func (c *captureWriter) WriteHeader(s int)           { c.statusCode = s }
