package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (h *DashboardHandler) GetFinanceReport(w http.ResponseWriter, r *http.Request) {
	p, ok := parseFinReportParams(w, r)
	if !ok {
		return
	}
	subjMetas, subjOrder, ok := h.loadFinSubjectMeta(w, r, p.channels, p.chanPH)
	if !ok {
		return
	}
	amounts, revByChanYM, ok := h.loadFinAmounts(w, r, p)
	if !ok {
		return
	}
	ymList := finYmList(p)
	resultRows := buildFinReportRows(p, subjOrder, subjMetas, amounts, revByChanYM, ymList)

	writeJSON(w, map[string]interface{}{
		"yearStart":  p.yearStart,
		"yearEnd":    p.yearEnd,
		"monthStart": p.monthStart,
		"monthEnd":   p.monthEnd,
		"channels":   p.channels,
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
	rows, ok := queryRowsOrWriteError(w, r, h.DB, q, args...)
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
	rows, ok := queryRowsOrWriteError(w, r, h.DB, q, args...)
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
	rows, ok := queryRowsOrWriteError(w, r, h.DB, `SELECT subject_code, subject_name, subject_category, subject_level, parent_code, display_order FROM finance_subject_dict ORDER BY display_order, subject_code`)
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
