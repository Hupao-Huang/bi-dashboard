package handler

// finance_report_query_sections.go — GetFinanceReport 的 section helper。
// 把原 273 行的 GetFinanceReport 拆成: 参数解析 / 科目骨架 / 实际数据 / 组装 四段 helper,
// 主函数只留编排。行为保持: 见 finance_report_golden_test.go 逐字节对拍。纯 DB 只读, 无外部 API。

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// finReportParams 收口 GetFinanceReport 的请求参数 (已校验 + 补默认 + 算好渠道占位符)。
type finReportParams struct {
	yearStart, yearEnd   int
	monthStart, monthEnd int
	channels             []string
	chanPH               string // 渠道 IN 占位符 (?,?,...)
}

// finSubjectMeta 科目元信息 (原为 GetFinanceReport 函数内局部类型)。
type finSubjectMeta struct {
	code, name, category, parent, subChannel string
	level, sortOrder                         int
}

// finKeyOf 科目行的唯一键: 有子渠道则 code|sub, 否则 code。
func finKeyOf(code, sub string) string {
	if sub != "" {
		return code + "|" + sub
	}
	return code
}

// finYmKey 年月键 "y-m"。
func finYmKey(y, m int) string { return fmt.Sprintf("%d-%d", y, m) }

// finSkipRatio GMV 类科目和营业收入本身不算占比 (跟 Excel 一致)。
func finSkipRatio(code, category string) bool {
	return category == "GMV" || code == "REV_MAIN"
}

// parseFinReportParams 解析 + 校验请求参数; 出错时已写响应并返回 false。
func parseFinReportParams(w http.ResponseWriter, r *http.Request) (finReportParams, bool) {
	q := r.URL.Query()
	var p finReportParams
	p.yearStart, _ = strconv.Atoi(strings.TrimSpace(q.Get("yearStart")))
	p.yearEnd, _ = strconv.Atoi(strings.TrimSpace(q.Get("yearEnd")))
	p.monthStart, _ = strconv.Atoi(strings.TrimSpace(q.Get("monthStart")))
	p.monthEnd, _ = strconv.Atoi(strings.TrimSpace(q.Get("monthEnd")))
	channelsStr := strings.TrimSpace(q.Get("channels"))

	// 兼容旧参数
	if p.yearStart == 0 {
		p.yearStart, _ = strconv.Atoi(strings.TrimSpace(q.Get("year")))
	}
	if p.yearEnd == 0 {
		p.yearEnd = p.yearStart
	}
	if p.monthStart == 0 {
		p.monthStart = 1
	}
	if p.monthEnd == 0 {
		p.monthEnd = 12
	}
	if channelsStr == "" {
		channelsStr = strings.TrimSpace(q.Get("department"))
	}
	if p.yearStart == 0 {
		writeError(w, 400, "yearStart 参数缺失")
		return p, false
	}
	p.channels = trimStrings(strings.Split(channelsStr, ","))
	if len(p.channels) == 0 {
		p.channels = []string{"汇总"}
	}
	if p.monthStart < 1 {
		p.monthStart = 1
	}
	if p.monthEnd > 12 {
		p.monthEnd = 12
	}
	if p.monthStart > p.monthEnd {
		writeError(w, 400, "monthStart 不能大于 monthEnd")
		return p, false
	}
	p.chanPH = placeholders(len(p.channels))
	return p, true
}

// loadFinSubjectMeta 构建科目骨架: 用最新年份的科目作参考 (切年份/月份时科目列表稳定) +
// 补 Level1 分组标题行 + 按 display_order 稳定排序。出错时已写响应并返回 false。
func (h *DashboardHandler) loadFinSubjectMeta(w http.ResponseWriter, r *http.Request, channels []string, chanPH string) (map[string]*finSubjectMeta, []string, bool) {
	// 1) 骨架: 固定用最新年份 (通常 2026) 的科目作为参考, 用字典的标准 name/category/level/parent
	skArgs := []interface{}{}
	for _, c := range channels {
		skArgs = append(skArgs, c)
	}
	skQuery := fmt.Sprintf(`SELECT fr.subject_code, d.subject_name, d.subject_category, d.subject_level, d.parent_code, fr.sub_channel, d.display_order
		FROM (SELECT DISTINCT subject_code, sub_channel FROM finance_report
		      WHERE year = (SELECT MAX(year) FROM finance_report) AND department IN (%s)) fr
		LEFT JOIN finance_subject_dict d ON d.subject_code = fr.subject_code
		ORDER BY d.display_order, fr.subject_code, fr.sub_channel`, chanPH)
	skRows, ok := queryRowsOrWriteError(w, r, h.DB, skQuery, skArgs...)
	if !ok {
		return nil, nil, false
	}

	subjMetas := map[string]*finSubjectMeta{}
	subjOrder := []string{}

	for skRows.Next() {
		var code, subChannel string
		var name, category, parent sql.NullString
		var level, displayOrder sql.NullInt64
		if writeDatabaseError(w, skRows.Scan(&code, &name, &category, &level, &parent, &subChannel, &displayOrder)) {
			skRows.Close()
			return nil, nil, false
		}
		k := finKeyOf(code, subChannel)
		if _, exists := subjMetas[k]; exists {
			continue
		}
		// GMV_SUB 的特殊处理: 字典只有一条 "子渠道GMV", 但 sub_channel 不同应分别显示
		n := ""
		if name.Valid {
			n = name.String
		}
		if code == "GMV_SUB" && subChannel != "" {
			n = subChannel
		}
		subjMetas[k] = &finSubjectMeta{
			code: code, name: n, subChannel: subChannel,
			category:  nullStr(category),
			parent:    nullStr(parent),
			level:     int(level.Int64),
			sortOrder: int(displayOrder.Int64),
		}
		subjOrder = append(subjOrder, k)
	}
	skRows.Close()

	// 1.5) 补入 Level 1 分组行 (GMV数据 / 财务数据), 没数值但作为标题显示
	levelOneRows, err := h.DB.Query(`SELECT subject_code, subject_name, subject_category, subject_level, parent_code, display_order FROM finance_subject_dict WHERE subject_level = 1 ORDER BY display_order`)
	if err == nil {
		for levelOneRows.Next() {
			var code, name, category, parent string
			var level, displayOrder int
			if err := levelOneRows.Scan(&code, &name, &category, &level, &parent, &displayOrder); err != nil {
				continue
			}
			if _, exists := subjMetas[code]; exists {
				continue
			}
			subjMetas[code] = &finSubjectMeta{code: code, name: name, category: category, parent: parent, level: level, sortOrder: displayOrder}
			subjOrder = append(subjOrder, code)
		}
		levelOneRows.Close()
	}

	// 按 sortOrder 稳定排序, 确保 Level 1 分组行在正确位置
	for i := 0; i < len(subjOrder); i++ {
		for j := i + 1; j < len(subjOrder); j++ {
			if subjMetas[subjOrder[i]].sortOrder > subjMetas[subjOrder[j]].sortOrder {
				subjOrder[i], subjOrder[j] = subjOrder[j], subjOrder[i]
			}
		}
	}

	return subjMetas, subjOrder, true
}

// loadFinAmounts 查选中区间+渠道的实际数据, 返回 amounts[key][dept][ym] 和 营业收入 revByChanYM[dept][ym]。
// 出错时已写响应并返回 false。
func (h *DashboardHandler) loadFinAmounts(w http.ResponseWriter, r *http.Request, p finReportParams) (map[string]map[string]map[string]float64, map[string]map[string]float64, bool) {
	args := []interface{}{p.yearStart, p.yearEnd, p.monthStart, p.monthEnd}
	for _, c := range p.channels {
		args = append(args, c)
	}
	query := fmt.Sprintf(`SELECT year, month, department, sub_channel, subject_code, amount
		FROM finance_report WHERE year BETWEEN ? AND ? AND month BETWEEN ? AND ? AND department IN (%s)`, p.chanPH)
	rows, ok := queryRowsOrWriteError(w, r, h.DB, query, args...)
	if !ok {
		return nil, nil, false
	}
	defer rows.Close()

	amounts := map[string]map[string]map[string]float64{}
	revByChanYM := map[string]map[string]float64{}

	for rows.Next() {
		var year, month int
		var dept, subChannel, code string
		var amount float64
		if writeDatabaseError(w, rows.Scan(&year, &month, &dept, &subChannel, &code, &amount)) {
			return nil, nil, false
		}
		ym := finYmKey(year, month)

		k := finKeyOf(code, subChannel)
		if amounts[k] == nil {
			amounts[k] = map[string]map[string]float64{}
		}
		if amounts[k][dept] == nil {
			amounts[k][dept] = map[string]float64{}
		}
		amounts[k][dept][ym] = amount

		if code == "REV_MAIN" && subChannel == "" {
			if revByChanYM[dept] == nil {
				revByChanYM[dept] = map[string]float64{}
			}
			revByChanYM[dept][ym] = amount
		}
	}
	return amounts, revByChanYM, true
}

// finYmList 生成区间内全部年月列表 (无论是否有数据, 空科目也要展示完整列)。
func finYmList(p finReportParams) []string {
	ymList := []string{}
	for y := p.yearStart; y <= p.yearEnd; y++ {
		for m := p.monthStart; m <= p.monthEnd; m++ {
			ymList = append(ymList, finYmKey(y, m))
		}
	}
	return ymList
}

// buildFinReportRows 组装返回行: 每科目算跨渠道合计 Total (含占比), 多渠道时再拆 ByChannel 明细。
func buildFinReportRows(p finReportParams, subjOrder []string, subjMetas map[string]*finSubjectMeta,
	amounts map[string]map[string]map[string]float64, revByChanYM map[string]map[string]float64, ymList []string) []*FinReportRow {
	multi := len(p.channels) > 1
	resultRows := make([]*FinReportRow, 0, len(subjOrder))
	for _, k := range subjOrder {
		m := subjMetas[k]
		row := &FinReportRow{
			Code: m.code, Name: m.name, Level: m.level, Parent: m.parent, Category: m.category,
			SubChannel: m.subChannel, SortOrder: m.sortOrder,
			Total: FinSeries{Cells: map[string]FinCell{}},
		}

		// 每个 ym 的总 (跨渠道之和)
		var rangeAmt, rangeRev float64
		for _, ym := range ymList {
			var cellAmt float64
			var cellRev float64
			for _, ch := range p.channels {
				if amounts[k] != nil && amounts[k][ch] != nil {
					cellAmt += amounts[k][ch][ym]
				}
				if revByChanYM[ch] != nil {
					cellRev += revByChanYM[ch][ym]
				}
			}
			cell := FinCell{Amount: cellAmt}
			if cellRev != 0 && !finSkipRatio(m.code, m.category) {
				ratio := cellAmt / cellRev
				cell.Ratio = &ratio
			}
			row.Total.Cells[ym] = cell
			rangeAmt += cellAmt
			rangeRev += cellRev
		}
		row.Total.RangeTotal = FinCell{Amount: rangeAmt}
		if rangeRev != 0 && !finSkipRatio(m.code, m.category) {
			ratio := rangeAmt / rangeRev
			row.Total.RangeTotal.Ratio = &ratio
		}

		// 分渠道明细
		if multi {
			for _, ch := range p.channels {
				series := FinSeries{Cells: map[string]FinCell{}}
				var cRangeAmt, cRangeRev float64
				for _, ym := range ymList {
					var amt float64
					if amounts[k] != nil && amounts[k][ch] != nil {
						amt = amounts[k][ch][ym]
					}
					var rev float64
					if revByChanYM[ch] != nil {
						rev = revByChanYM[ch][ym]
					}
					cell := FinCell{Amount: amt}
					if rev != 0 && m.code != "REV_MAIN" {
						ratio := amt / rev
						cell.Ratio = &ratio
					}
					series.Cells[ym] = cell
					cRangeAmt += amt
					cRangeRev += rev
				}
				series.RangeTotal = FinCell{Amount: cRangeAmt}
				if cRangeRev != 0 && m.code != "REV_MAIN" {
					ratio := cRangeAmt / cRangeRev
					series.RangeTotal.Ratio = &ratio
				}
				row.ByChannel = append(row.ByChannel, FinChannelSeries{Channel: ch, Series: series})
			}
		}

		resultRows = append(resultRows, row)
	}
	return resultRows
}
