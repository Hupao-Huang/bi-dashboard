package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (h *DashboardHandler) GetFinanceReport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	yearStart, _ := strconv.Atoi(strings.TrimSpace(q.Get("yearStart")))
	yearEnd, _ := strconv.Atoi(strings.TrimSpace(q.Get("yearEnd")))
	monthStart, _ := strconv.Atoi(strings.TrimSpace(q.Get("monthStart")))
	monthEnd, _ := strconv.Atoi(strings.TrimSpace(q.Get("monthEnd")))
	channelsStr := strings.TrimSpace(q.Get("channels"))

	// 兼容旧参数
	if yearStart == 0 {
		yearStart, _ = strconv.Atoi(strings.TrimSpace(q.Get("year")))
	}
	if yearEnd == 0 {
		yearEnd = yearStart
	}
	if monthStart == 0 {
		monthStart = 1
	}
	if monthEnd == 0 {
		monthEnd = 12
	}
	if channelsStr == "" {
		channelsStr = strings.TrimSpace(q.Get("department"))
	}
	if yearStart == 0 {
		writeError(w, 400, "yearStart 参数缺失")
		return
	}
	channels := trimStrings(strings.Split(channelsStr, ","))
	if len(channels) == 0 {
		channels = []string{"汇总"}
	}
	if monthStart < 1 {
		monthStart = 1
	}
	if monthEnd > 12 {
		monthEnd = 12
	}
	if monthStart > monthEnd {
		writeError(w, 400, "monthStart 不能大于 monthEnd")
		return
	}

	chanPH := placeholders(len(channels))

	// 1) 骨架：固定用最新年份（通常是 2026）的科目作为参考，切换年份/月份时科目列表稳定
	//    用字典的标准 name/category/level/parent
	skArgs := []interface{}{}
	for _, c := range channels {
		skArgs = append(skArgs, c)
	}
	skQuery := fmt.Sprintf(`SELECT fr.subject_code, d.subject_name, d.subject_category, d.subject_level, d.parent_code, fr.sub_channel, d.display_order
		FROM (SELECT DISTINCT subject_code, sub_channel FROM finance_report
		      WHERE year = (SELECT MAX(year) FROM finance_report) AND department IN (%s)) fr
		LEFT JOIN finance_subject_dict d ON d.subject_code = fr.subject_code
		ORDER BY d.display_order, fr.subject_code, fr.sub_channel`, chanPH)
	skRows, ok := queryRowsOrWriteError(w, h.DB, skQuery, skArgs...)
	if !ok {
		return
	}

	keyOf := func(code, sub string) string {
		if sub != "" {
			return code + "|" + sub
		}
		return code
	}
	ymKey := func(y, m int) string { return fmt.Sprintf("%d-%d", y, m) }

	type subjectMeta struct {
		code, name, category, parent, subChannel string
		level, sortOrder                         int
	}
	subjMetas := map[string]*subjectMeta{}
	subjOrder := []string{}

	for skRows.Next() {
		var code, subChannel string
		var name, category, parent sql.NullString
		var level, displayOrder sql.NullInt64
		if writeDatabaseError(w, skRows.Scan(&code, &name, &category, &level, &parent, &subChannel, &displayOrder)) {
			skRows.Close()
			return
		}
		k := keyOf(code, subChannel)
		if _, exists := subjMetas[k]; exists {
			continue
		}
		// GMV_SUB 的特殊处理：字典只有一条 "子渠道GMV"，但 sub_channel 不同应分别显示
		n := ""
		if name.Valid {
			n = name.String
		}
		if code == "GMV_SUB" && subChannel != "" {
			n = subChannel
		}
		subjMetas[k] = &subjectMeta{
			code: code, name: n, subChannel: subChannel,
			category:  nullStr(category),
			parent:    nullStr(parent),
			level:     int(level.Int64),
			sortOrder: int(displayOrder.Int64),
		}
		subjOrder = append(subjOrder, k)
	}
	skRows.Close()

	// 1.5) 补入 Level 1 分组行（GMV数据 / 财务数据），没数值但作为标题显示
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
			subjMetas[code] = &subjectMeta{code: code, name: name, category: category, parent: parent, level: level, sortOrder: displayOrder}
			subjOrder = append(subjOrder, code)
		}
		levelOneRows.Close()
	}

	// 按 sortOrder 稳定排序，确保 Level 1 分组行在正确位置
	for i := 0; i < len(subjOrder); i++ {
		for j := i + 1; j < len(subjOrder); j++ {
			if subjMetas[subjOrder[i]].sortOrder > subjMetas[subjOrder[j]].sortOrder {
				subjOrder[i], subjOrder[j] = subjOrder[j], subjOrder[i]
			}
		}
	}

	// 2) 实际数据：选中区间 + 选中渠道
	args := []interface{}{yearStart, yearEnd, monthStart, monthEnd}
	for _, c := range channels {
		args = append(args, c)
	}
	query := fmt.Sprintf(`SELECT year, month, department, sub_channel, subject_code, amount
		FROM finance_report WHERE year BETWEEN ? AND ? AND month BETWEEN ? AND ? AND department IN (%s)`, chanPH)
	rows, ok := queryRowsOrWriteError(w, h.DB, query, args...)
	if !ok {
		return
	}
	defer rows.Close()

	amounts := map[string]map[string]map[string]float64{}
	revByChanYM := map[string]map[string]float64{}
	yearMonths := map[string]bool{}

	for rows.Next() {
		var year, month int
		var dept, subChannel, code string
		var amount float64
		if writeDatabaseError(w, rows.Scan(&year, &month, &dept, &subChannel, &code, &amount)) {
			return
		}
		ym := ymKey(year, month)
		yearMonths[ym] = true

		k := keyOf(code, subChannel)
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

	// 生成 yearMonth 列表：区间内全部月份，无论是否有数据（空科目也要展示完整列）
	_ = yearMonths
	ymList := []string{}
	for y := yearStart; y <= yearEnd; y++ {
		for m := monthStart; m <= monthEnd; m++ {
			ymList = append(ymList, ymKey(y, m))
		}
	}

	// 组装返回行
	multi := len(channels) > 1
	// GMV 类科目和营业收入本身不算占比（跟 Excel 一致）
	skipRatio := func(code, category string) bool {
		return category == "GMV" || code == "REV_MAIN"
	}
	resultRows := make([]*FinReportRow, 0, len(subjOrder))
	for _, k := range subjOrder {
		m := subjMetas[k]
		row := &FinReportRow{
			Code: m.code, Name: m.name, Level: m.level, Parent: m.parent, Category: m.category,
			SubChannel: m.subChannel, SortOrder: m.sortOrder,
			Total: FinSeries{Cells: map[string]FinCell{}},
		}

		// 每个 ym 的总（跨渠道之和）
		var rangeAmt, rangeRev float64
		for _, ym := range ymList {
			var cellAmt float64
			var cellRev float64
			for _, ch := range channels {
				if amounts[k] != nil && amounts[k][ch] != nil {
					cellAmt += amounts[k][ch][ym]
				}
				if revByChanYM[ch] != nil {
					cellRev += revByChanYM[ch][ym]
				}
			}
			cell := FinCell{Amount: cellAmt}
			if cellRev != 0 && !skipRatio(m.code, m.category) {
				r := cellAmt / cellRev
				cell.Ratio = &r
			}
			row.Total.Cells[ym] = cell
			rangeAmt += cellAmt
			rangeRev += cellRev
		}
		row.Total.RangeTotal = FinCell{Amount: rangeAmt}
		if rangeRev != 0 && !skipRatio(m.code, m.category) {
			r := rangeAmt / rangeRev
			row.Total.RangeTotal.Ratio = &r
		}

		// 分渠道明细
		if multi {
			for _, ch := range channels {
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
						r := amt / rev
						cell.Ratio = &r
					}
					series.Cells[ym] = cell
					cRangeAmt += amt
					cRangeRev += rev
				}
				series.RangeTotal = FinCell{Amount: cRangeAmt}
				if cRangeRev != 0 && m.code != "REV_MAIN" {
					r := cRangeAmt / cRangeRev
					series.RangeTotal.Ratio = &r
				}
				row.ByChannel = append(row.ByChannel, FinChannelSeries{Channel: ch, Series: series})
			}
		}

		resultRows = append(resultRows, row)
	}

	writeJSON(w, map[string]interface{}{
		"yearStart":  yearStart,
		"yearEnd":    yearEnd,
		"monthStart": monthStart,
		"monthEnd":   monthEnd,
		"channels":   channels,
		"yearMonths": ymList,
		"rows":       resultRows,
	})
}

func (h *DashboardHandler) GetFinanceReportTrend(w http.ResponseWriter, r *http.Request) {
	subjects := trimStrings(strings.Split(strings.TrimSpace(r.URL.Query().Get("subjects")), ","))
	channels := trimStrings(strings.Split(strings.TrimSpace(r.URL.Query().Get("channels")), ","))
	yearStart, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("yearStart")))
	yearEnd, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("yearEnd")))
	if yearStart == 0 {
		yearStart = 2024
	}
	if yearEnd == 0 {
		yearEnd = time.Now().Year()
	}
	if len(subjects) == 0 || len(channels) == 0 {
		writeError(w, 400, "subjects / channels 不能为空")
		return
	}
	subjectPH := placeholders(len(subjects))
	chanPH := placeholders(len(channels))
	args := []interface{}{yearStart, yearEnd}
	for _, s := range subjects {
		args = append(args, s)
	}
	for _, c := range channels {
		args = append(args, c)
	}
	q := fmt.Sprintf(`SELECT year, month, department, subject_code, subject_name, SUM(amount) amount FROM finance_report WHERE year BETWEEN ? AND ? AND month BETWEEN 1 AND 12 AND subject_code IN (%s) AND department IN (%s) AND sub_channel='' GROUP BY year, month, department, subject_code, subject_name ORDER BY year, month`, subjectPH, chanPH)
	rows, ok := queryRowsOrWriteError(w, h.DB, q, args...)
	if !ok {
		return
	}
	defer rows.Close()
	type Point struct {
		Year        int     `json:"year"`
		Month       int     `json:"month"`
		Department  string  `json:"department"`
		SubjectCode string  `json:"subjectCode"`
		SubjectName string  `json:"subjectName"`
		Amount      float64 `json:"amount"`
	}
	var points []Point
	for rows.Next() {
		var p Point
		if writeDatabaseError(w, rows.Scan(&p.Year, &p.Month, &p.Department, &p.SubjectCode, &p.SubjectName, &p.Amount)) {
			return
		}
		points = append(points, p)
	}
	writeJSON(w, map[string]interface{}{
		"yearStart": yearStart, "yearEnd": yearEnd,
		"subjects": subjects, "channels": channels, "points": points,
	})
}

func (h *DashboardHandler) GetFinanceReportCompare(w http.ResponseWriter, r *http.Request) {
	year, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("year")))
	month, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("month")))
	if year == 0 {
		writeError(w, 400, "year 参数缺失")
		return
	}
	kpiCodes := []string{"GMV_TOTAL", "REV_MAIN", "COST_MAIN", "PROFIT_GROSS", "SALES_EXP", "MGMT_EXP", "PROFIT_OP", "NET_PROFIT"}
	ph := placeholders(len(kpiCodes))
	var q string
	var args []interface{}
	if month == 0 {
		args = []interface{}{year}
		for _, c := range kpiCodes {
			args = append(args, c)
		}
		q = fmt.Sprintf(`SELECT department, subject_code, subject_name, SUM(amount) FROM finance_report WHERE year=? AND month BETWEEN 1 AND 12 AND subject_code IN (%s) AND sub_channel='' GROUP BY department, subject_code, subject_name ORDER BY department, subject_code`, ph)
	} else {
		args = []interface{}{year, month}
		for _, c := range kpiCodes {
			args = append(args, c)
		}
		q = fmt.Sprintf(`SELECT department, subject_code, subject_name, amount FROM finance_report WHERE year=? AND month=? AND subject_code IN (%s) AND sub_channel='' ORDER BY department, subject_code`, ph)
	}
	rows, ok := queryRowsOrWriteError(w, h.DB, q, args...)
	if !ok {
		return
	}
	defer rows.Close()
	result := map[string]map[string]float64{}
	names := map[string]string{}
	for rows.Next() {
		var dept, code, name string
		var amount float64
		if writeDatabaseError(w, rows.Scan(&dept, &code, &name, &amount)) {
			return
		}
		if result[dept] == nil {
			result[dept] = map[string]float64{}
		}
		result[dept][code] = amount
		names[code] = name
	}
	writeJSON(w, map[string]interface{}{
		"year": year, "month": month, "kpiCodes": kpiCodes,
		"subjectNames": names, "data": result,
	})
}

func (h *DashboardHandler) GetFinanceReportStructure(w http.ResponseWriter, r *http.Request) {
	year, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("year")))
	month, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("month")))
	dept := strings.TrimSpace(r.URL.Query().Get("department"))
	if year == 0 || dept == "" {
		writeError(w, 400, "year / department 参数缺失")
		return
	}
	monthCond := "month = ?"
	if month == 0 {
		monthCond = "month BETWEEN 1 AND 12"
	}
	type Item struct {
		Code     string  `json:"code"`
		Name     string  `json:"name"`
		Category string  `json:"category"`
		Amount   float64 `json:"amount"`
	}
	fetch := func(parent string) ([]Item, error) {
		q := `SELECT subject_code, subject_name, subject_category, SUM(amount) FROM finance_report WHERE year=? AND department=? AND ` + monthCond + ` AND parent_code=? AND subject_level=3 AND sub_channel='' GROUP BY subject_code, subject_name, subject_category HAVING SUM(amount) != 0 ORDER BY SUM(amount) DESC`
		args := []interface{}{year, dept}
		if month != 0 {
			args = append(args, month)
		}
		args = append(args, parent)
		r2, err := h.DB.Query(q, args...)
		if err != nil {
			return nil, err
		}
		defer r2.Close()
		var items []Item
		for r2.Next() {
			var it Item
			if err := r2.Scan(&it.Code, &it.Name, &it.Category, &it.Amount); err != nil {
				return nil, err
			}
			items = append(items, it)
		}
		return items, nil
	}
	costs, err := fetch("COST_MAIN")
	if writeDatabaseError(w, err) {
		return
	}
	salesExp, err := fetch("SALES_EXP")
	if writeDatabaseError(w, err) {
		return
	}
	mgmtExp, err := fetch("MGMT_EXP")
	if writeDatabaseError(w, err) {
		return
	}
	waterfallCodes := []string{"GMV_TOTAL", "REV_MAIN", "PROFIT_GROSS", "PROFIT_OP", "NET_PROFIT"}
	ph := placeholders(len(waterfallCodes))
	qw := `SELECT subject_code, subject_name, SUM(amount) FROM finance_report WHERE year=? AND department=? AND ` + monthCond + ` AND subject_code IN (` + ph + `) AND sub_channel='' GROUP BY subject_code, subject_name`
	wArgs := []interface{}{year, dept}
	if month != 0 {
		wArgs = append(wArgs, month)
	}
	for _, c := range waterfallCodes {
		wArgs = append(wArgs, c)
	}
	wRows, err := h.DB.Query(qw, wArgs...)
	if writeDatabaseError(w, err) {
		return
	}
	defer wRows.Close()
	type Step struct {
		Code   string  `json:"code"`
		Name   string  `json:"name"`
		Amount float64 `json:"amount"`
	}
	stepMap := map[string]Step{}
	for wRows.Next() {
		var s Step
		if writeDatabaseError(w, wRows.Scan(&s.Code, &s.Name, &s.Amount)) {
			return
		}
		stepMap[s.Code] = s
	}
	waterfall := []Step{}
	for _, c := range waterfallCodes {
		if s, ok := stepMap[c]; ok {
			waterfall = append(waterfall, s)
		}
	}
	writeJSON(w, map[string]interface{}{
		"year": year, "month": month, "department": dept,
		"cost": costs, "salesExp": salesExp, "mgmtExp": mgmtExp, "waterfall": waterfall,
	})
}

func (h *DashboardHandler) GetFinanceSubjects(w http.ResponseWriter, r *http.Request) {
	rows, ok := queryRowsOrWriteError(w, h.DB, `SELECT subject_code, subject_name, subject_category, subject_level, parent_code, display_order FROM finance_subject_dict ORDER BY display_order, subject_code`)
	if !ok {
		return
	}
	defer rows.Close()
	type Sub struct {
		Code     string `json:"code"`
		Name     string `json:"name"`
		Category string `json:"category"`
		Level    int    `json:"level"`
		Parent   string `json:"parent"`
		Order    int    `json:"order"`
	}
	var subs []Sub
	for rows.Next() {
		var s Sub
		if writeDatabaseError(w, rows.Scan(&s.Code, &s.Name, &s.Category, &s.Level, &s.Parent, &s.Order)) {
			return
		}
		subs = append(subs, s)
	}
	writeJSON(w, map[string]interface{}{"subjects": subs})
}
