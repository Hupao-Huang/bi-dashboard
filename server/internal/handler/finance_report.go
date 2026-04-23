package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"bi-dashboard/internal/finance"

	"github.com/xuri/excelize/v2"
)

var financeImportMu sync.Mutex

type FinCell struct {
	Amount float64  `json:"amount"`
	Ratio  *float64 `json:"ratio,omitempty"`
}

type FinSeries struct {
	RangeTotal FinCell            `json:"rangeTotal"`
	Cells      map[string]FinCell `json:"cells"` // key = "YYYY-M"
}

type FinChannelSeries struct {
	Channel string    `json:"channel"`
	Series  FinSeries `json:"series"`
}

type FinReportRow struct {
	Code       string             `json:"code"`
	Name       string             `json:"name"`
	Level      int                `json:"level"`
	Parent     string             `json:"parent"`
	Category   string             `json:"category"`
	SubChannel string             `json:"subChannel,omitempty"`
	SortOrder  int                `json:"sortOrder"`
	Total      FinSeries          `json:"total"`               // 跨选中渠道的总（电商+社媒之和）
	ByChannel  []FinChannelSeries `json:"byChannel,omitempty"` // 各渠道明细；仅在 channels>1 时返回
}

func sortIntsAsc(a []int) {
	for i := 0; i < len(a); i++ {
		for j := i + 1; j < len(a); j++ {
			if a[i] > a[j] {
				a[i], a[j] = a[j], a[i]
			}
		}
	}
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	s := strings.Repeat("?,", n)
	return s[:len(s)-1]
}

func nullStr(s sql.NullString) string {
	if s.Valid {
		return s.String
	}
	return ""
}

func trimStrings(a []string) []string {
	var r []string
	for _, s := range a {
		s = strings.TrimSpace(s)
		if s != "" {
			r = append(r, s)
		}
	}
	return r
}

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

func (h *DashboardHandler) GetFinanceImportLogs(w http.ResponseWriter, r *http.Request) {
	rows, ok := queryRowsOrWriteError(w, h.DB, `SELECT id, year, filename, file_size, md5, sheet_count, row_count, unmapped_subjects, status, error_msg, user_id, created_at FROM finance_import_log ORDER BY created_at DESC LIMIT 50`)
	if !ok {
		return
	}
	defer rows.Close()
	type Log struct {
		ID         int64           `json:"id"`
		Year       int             `json:"year"`
		Filename   string          `json:"filename"`
		FileSize   int64           `json:"fileSize"`
		MD5        string          `json:"md5"`
		SheetCount int             `json:"sheetCount"`
		RowCount   int             `json:"rowCount"`
		Unmapped   json.RawMessage `json:"unmappedSubjects"`
		Status     string          `json:"status"`
		ErrorMsg   string          `json:"errorMsg"`
		UserID     int             `json:"userId"`
		CreatedAt  time.Time       `json:"createdAt"`
	}
	var logs []Log
	for rows.Next() {
		var l Log
		var md5 sql.NullString
		var unmap sql.NullString
		var errMsg sql.NullString
		if writeDatabaseError(w, rows.Scan(&l.ID, &l.Year, &l.Filename, &l.FileSize, &md5, &l.SheetCount, &l.RowCount, &unmap, &l.Status, &errMsg, &l.UserID, &l.CreatedAt)) {
			return
		}
		if md5.Valid {
			l.MD5 = md5.String
		}
		if errMsg.Valid {
			l.ErrorMsg = errMsg.String
		}
		if unmap.Valid && unmap.String != "" {
			l.Unmapped = json.RawMessage(unmap.String)
		} else {
			l.Unmapped = json.RawMessage("[]")
		}
		logs = append(logs, l)
	}
	writeJSON(w, map[string]interface{}{"logs": logs})
}

func (h *DashboardHandler) ImportFinanceReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	if !financeImportMu.TryLock() {
		writeError(w, 409, "有其他导入任务进行中，请稍后再试")
		return
	}
	defer financeImportMu.Unlock()
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		writeError(w, 400, "解析表单失败: "+err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, 400, "请选择 xlsx 文件")
		return
	}
	defer file.Close()
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".xlsx" {
		writeError(w, 400, "仅支持 .xlsx 格式")
		return
	}
	year := finance.ParseYearFromFilename(header.Filename)
	if yv := strings.TrimSpace(r.FormValue("year")); yv != "" {
		if y, err := strconv.Atoi(yv); err == nil {
			year = y
		}
	}
	if year == 0 {
		writeError(w, 400, "无法推断年份，请用 YYYY年财务管理报表.xlsx 命名或传 year 参数")
		return
	}
	if year < 2000 || year > 2100 {
		writeError(w, 400, fmt.Sprintf("年份 %d 不合理，请检查文件名", year))
		return
	}
	tmpDir := filepath.Join(os.TempDir(), "bi-finance-import")
	os.MkdirAll(tmpDir, 0755)
	tmpPath := filepath.Join(tmpDir, fmt.Sprintf("upload-%d-%s", time.Now().UnixMilli(), filepath.Base(header.Filename)))
	dst, err := os.Create(tmpPath)
	if err != nil {
		writeError(w, 500, "创建临时文件失败")
		return
	}
	if _, err := io.Copy(dst, file); err != nil {
		dst.Close()
		writeError(w, 500, "保存文件失败")
		return
	}
	dst.Close()
	defer os.Remove(tmpPath)
	userID := 0
	if payload, ok := authPayloadFromContext(r); ok && payload != nil {
		userID = int(payload.User.ID)
	}
	dict, err := finance.LoadSubjectDict(h.DB)
	if err != nil {
		writeError(w, 500, "加载字典失败: "+err.Error())
		return
	}
	result, err := finance.ParseFile(tmpPath, year, dict)
	if err != nil {
		badResult := &finance.ParseResult{Year: year}
		_ = finance.LogImport(h.DB, tmpPath, year, badResult, userID, "failed", err.Error())
		writeError(w, 500, "解析失败: "+err.Error())
		return
	}
	if err := finance.WriteResult(h.DB, result); err != nil {
		_ = finance.LogImport(h.DB, tmpPath, year, result, userID, "failed", err.Error())
		writeError(w, 500, "入库失败: "+err.Error())
		return
	}
	status := "success"
	if len(result.UnmappedSubjects) > 0 {
		status = "partial"
	}
	_ = finance.LogImport(h.DB, tmpPath, year, result, userID, status, "")
	writeJSON(w, map[string]interface{}{
		"year": year, "sheetCount": result.SheetCount, "rowCount": result.RowCount,
		"departments": result.Departments, "unmapped": result.UnmappedSubjects, "status": status,
	})
}

// ExportFinanceReport GET /api/finance/report/export - 按当前筛选条件导出 xlsx
func (h *DashboardHandler) ExportFinanceReport(w http.ResponseWriter, r *http.Request) {
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
			YearStart  int             `json:"yearStart"`
			YearEnd    int             `json:"yearEnd"`
			MonthStart int             `json:"monthStart"`
			MonthEnd   int             `json:"monthEnd"`
			Channels   []string        `json:"channels"`
			YearMonths []string        `json:"yearMonths"`
			Rows       []FinReportRow  `json:"rows"`
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
		NumFmt:    0,
		CustomNumFmt: &amountFmt,
		Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"},
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

func (c *captureWriter) Header() http.Header       { return c.header }
func (c *captureWriter) Write(b []byte) (int, error) { c.body = append(c.body, b...); return len(b), nil }
func (c *captureWriter) WriteHeader(s int)         { c.statusCode = s }
