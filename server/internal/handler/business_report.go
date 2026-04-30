package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// BusinessReportHandler 业务预决算报表 API（v0.58）
//
// 4 个 GET API:
//   GET /api/finance/business-report/snapshots - 列出已有 snapshot 列表
//   GET /api/finance/business-report/detail?snapshot=YYYY-MM&channel=&sub_channel= - 单 sheet 完整数据
//   GET /api/finance/business-report/overview?snapshot=YYYY-MM&period_month= - 全部 channels 当 period 数据汇总
//   GET /api/finance/business-report/trend?snapshot=YYYY-MM&channel=&sub_channel=&subject= - 单科目月度趋势

type budgetSnapshot struct {
	SnapshotYear  int    `json:"snapshotYear"`
	SnapshotMonth int    `json:"snapshotMonth"`
	Year          int    `json:"year"`
	Label         string `json:"label"` // "2026-04 (覆盖 2026 年)"
	RowCount      int    `json:"rowCount"`
	ChannelCount  int    `json:"channelCount"`
}

func (h *DashboardHandler) GetBusinessReportSnapshots(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(`
		SELECT snapshot_year, snapshot_month, year, COUNT(*) AS rc, COUNT(DISTINCT channel) AS cc
		FROM business_budget_report
		GROUP BY snapshot_year, snapshot_month, year
		ORDER BY snapshot_year DESC, snapshot_month DESC`)
	if writeDatabaseError(w, err) {
		return
	}
	defer rows.Close()

	var out []budgetSnapshot
	for rows.Next() {
		var s budgetSnapshot
		if writeDatabaseError(w, rows.Scan(&s.SnapshotYear, &s.SnapshotMonth, &s.Year, &s.RowCount, &s.ChannelCount)) {
			return
		}
		s.Label = formatSnapshotLabel(s.SnapshotYear, s.SnapshotMonth, s.Year)
		out = append(out, s)
	}
	writeJSON(w, map[string]interface{}{"snapshots": out})
}

type budgetCell struct {
	Subject         string   `json:"subject"`
	SubjectLevel    int      `json:"subjectLevel"`
	SubjectCategory string   `json:"subjectCategory"`
	ParentSubject   string   `json:"parentSubject"`
	SortOrder       int      `json:"sortOrder"`
	BudgetYearStart *float64 `json:"budgetYearStart,omitempty"`
	RatioYearStart  *float64 `json:"ratioYearStart,omitempty"`
	BudgetTotal     *float64 `json:"budgetTotal,omitempty"`
	RatioBudget     *float64 `json:"ratioBudget,omitempty"`
	ActualTotal     *float64 `json:"actualTotal,omitempty"`
	RatioActual     *float64 `json:"ratioActual,omitempty"`
	AchievementRate *float64 `json:"achievementRate,omitempty"`
	Months          []budgetMonth `json:"months"` // length 12
}

type budgetMonth struct {
	Month       int      `json:"month"` // 1-12
	Budget      *float64 `json:"budget,omitempty"`
	RatioBudget *float64 `json:"ratioBudget,omitempty"`
	Actual      *float64 `json:"actual,omitempty"`
	RatioActual *float64 `json:"ratioActual,omitempty"`
}

func (h *DashboardHandler) GetBusinessReportDetail(w http.ResponseWriter, r *http.Request) {
	sy, sm, ok := parseSnapshotParam(w, r)
	if !ok {
		return
	}
	channel := strings.TrimSpace(r.URL.Query().Get("channel"))
	subChannel := strings.TrimSpace(r.URL.Query().Get("sub_channel"))
	if channel == "" {
		http.Error(w, `{"code":400,"msg":"channel required"}`, http.StatusBadRequest)
		return
	}

	rows, err := h.DB.Query(`
		SELECT subject, subject_level, subject_category, parent_subject, sort_order, period_month,
		       budget_year_start, ratio_year_start, budget, ratio_budget, actual, ratio_actual, achievement_rate
		FROM business_budget_report
		WHERE snapshot_year=? AND snapshot_month=? AND channel=? AND sub_channel=?
		ORDER BY sort_order, period_month`,
		sy, sm, channel, subChannel)
	if writeDatabaseError(w, err) {
		return
	}
	defer rows.Close()

	cellMap := map[int]*budgetCell{} // sort_order → cell
	var order []int
	for rows.Next() {
		var (
			subject, category, parent string
			level, sortOrder, pm      int
			bys, rys, b, rb, a, ra, ar sql.NullFloat64
		)
		if writeDatabaseError(w, rows.Scan(&subject, &level, &category, &parent, &sortOrder, &pm,
			&bys, &rys, &b, &rb, &a, &ra, &ar)) {
			return
		}
		c, exists := cellMap[sortOrder]
		if !exists {
			c = &budgetCell{
				Subject:         subject,
				SubjectLevel:    level,
				SubjectCategory: category,
				ParentSubject:   parent,
				SortOrder:       sortOrder,
				Months:          make([]budgetMonth, 12),
			}
			for i := 0; i < 12; i++ {
				c.Months[i].Month = i + 1
			}
			cellMap[sortOrder] = c
			order = append(order, sortOrder)
		}
		if pm == 0 {
			c.BudgetYearStart = nullToPtr(bys)
			c.RatioYearStart = nullToPtr(rys)
			c.BudgetTotal = nullToPtr(b)
			c.RatioBudget = nullToPtr(rb)
			c.ActualTotal = nullToPtr(a)
			c.RatioActual = nullToPtr(ra)
			c.AchievementRate = nullToPtr(ar)
		} else if pm >= 1 && pm <= 12 {
			m := &c.Months[pm-1]
			m.Budget = nullToPtr(b)
			m.RatioBudget = nullToPtr(rb)
			m.Actual = nullToPtr(a)
			m.RatioActual = nullToPtr(ra)
		}
	}

	cells := make([]*budgetCell, 0, len(order))
	for _, so := range order {
		cells = append(cells, cellMap[so])
	}

	// 列出该 snapshot 下此 channel 的所有 sub_channel（前端切换 tab 用）
	subs, _ := h.queryDistinctSubChannels(sy, sm, channel)

	writeJSON(w, map[string]interface{}{
		"snapshotYear":  sy,
		"snapshotMonth": sm,
		"channel":       channel,
		"subChannel":    subChannel,
		"subChannels":   subs,
		"cells":         cells,
	})
}

func (h *DashboardHandler) queryDistinctSubChannels(sy, sm int, channel string) ([]string, error) {
	rows, err := h.DB.Query(`
		SELECT DISTINCT sub_channel FROM business_budget_report
		WHERE snapshot_year=? AND snapshot_month=? AND channel=?
		ORDER BY sub_channel`, sy, sm, channel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

type channelOverview struct {
	Channel         string   `json:"channel"`
	SubChannel      string   `json:"subChannel"`
	Subject         string   `json:"subject"` // 默认 GMV合计
	BudgetTotal     *float64 `json:"budgetTotal,omitempty"`
	ActualTotal     *float64 `json:"actualTotal,omitempty"`
	AchievementRate *float64 `json:"achievementRate,omitempty"`
}

func (h *DashboardHandler) GetBusinessReportOverview(w http.ResponseWriter, r *http.Request) {
	sy, sm, ok := parseSnapshotParam(w, r)
	if !ok {
		return
	}
	subjectFilter := strings.TrimSpace(r.URL.Query().Get("subject"))
	if subjectFilter == "" {
		subjectFilter = "GMV合计"
	}

	rows, err := h.DB.Query(`
		SELECT channel, sub_channel, subject, budget, actual, achievement_rate
		FROM business_budget_report
		WHERE snapshot_year=? AND snapshot_month=? AND subject=? AND period_month=0
		ORDER BY channel, sub_channel`,
		sy, sm, subjectFilter)
	if writeDatabaseError(w, err) {
		return
	}
	defer rows.Close()

	var out []channelOverview
	for rows.Next() {
		var (
			channel, subChannel, subject string
			b, a, ar                     sql.NullFloat64
		)
		if writeDatabaseError(w, rows.Scan(&channel, &subChannel, &subject, &b, &a, &ar)) {
			return
		}
		out = append(out, channelOverview{
			Channel:         channel,
			SubChannel:      subChannel,
			Subject:         subject,
			BudgetTotal:     nullToPtr(b),
			ActualTotal:     nullToPtr(a),
			AchievementRate: nullToPtr(ar),
		})
	}
	writeJSON(w, map[string]interface{}{
		"snapshotYear":  sy,
		"snapshotMonth": sm,
		"subject":       subjectFilter,
		"channels":      out,
	})
}

func (h *DashboardHandler) GetBusinessReportTrend(w http.ResponseWriter, r *http.Request) {
	sy, sm, ok := parseSnapshotParam(w, r)
	if !ok {
		return
	}
	channel := strings.TrimSpace(r.URL.Query().Get("channel"))
	subChannel := strings.TrimSpace(r.URL.Query().Get("sub_channel"))
	subject := strings.TrimSpace(r.URL.Query().Get("subject"))
	if channel == "" || subject == "" {
		http.Error(w, `{"code":400,"msg":"channel and subject required"}`, http.StatusBadRequest)
		return
	}
	rows, err := h.DB.Query(`
		SELECT period_month, budget, actual
		FROM business_budget_report
		WHERE snapshot_year=? AND snapshot_month=? AND channel=? AND sub_channel=? AND subject=?
		  AND period_month BETWEEN 1 AND 12
		ORDER BY period_month`,
		sy, sm, channel, subChannel, subject)
	if writeDatabaseError(w, err) {
		return
	}
	defer rows.Close()
	type point struct {
		Month  int      `json:"month"`
		Budget *float64 `json:"budget,omitempty"`
		Actual *float64 `json:"actual,omitempty"`
	}
	pts := make([]point, 0, 12)
	for rows.Next() {
		var (
			pm   int
			b, a sql.NullFloat64
		)
		if writeDatabaseError(w, rows.Scan(&pm, &b, &a)) {
			return
		}
		pts = append(pts, point{Month: pm, Budget: nullToPtr(b), Actual: nullToPtr(a)})
	}
	writeJSON(w, map[string]interface{}{
		"snapshotYear":  sy,
		"snapshotMonth": sm,
		"channel":       channel,
		"subChannel":    subChannel,
		"subject":       subject,
		"points":        pts,
	})
}

// ---- helpers ----

func parseSnapshotParam(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	s := strings.TrimSpace(r.URL.Query().Get("snapshot"))
	if s == "" {
		http.Error(w, `{"code":400,"msg":"snapshot required (YYYY-MM)"}`, http.StatusBadRequest)
		return 0, 0, false
	}
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		http.Error(w, `{"code":400,"msg":"snapshot format YYYY-MM"}`, http.StatusBadRequest)
		return 0, 0, false
	}
	y, e1 := strconv.Atoi(parts[0])
	m, e2 := strconv.Atoi(parts[1])
	if e1 != nil || e2 != nil || y < 2020 || y > 2050 || m < 1 || m > 12 {
		http.Error(w, `{"code":400,"msg":"snapshot range 2020-2050, 1-12"}`, http.StatusBadRequest)
		return 0, 0, false
	}
	return y, m, true
}

func formatSnapshotLabel(sy, sm, y int) string {
	base := strconv.Itoa(sy) + "-" + zeroPad(sm)
	if sy != y {
		base += " (覆盖 " + strconv.Itoa(y) + " 年)"
	}
	return base
}

func zeroPad(n int) string {
	s := strconv.Itoa(n)
	if len(s) == 1 {
		return "0" + s
	}
	return s
}

func nullToPtr(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	v := n.Float64
	return &v
}

// ============= ReportData 兼容 API（v0.59 跑哥要求"和财务报表一样"）=============
//
// GET /api/finance/business-report?yearStart=2024&yearEnd=2026&monthStart=1&monthEnd=12&channels=总,电商,私域
//
// 返回结构跟 finance_report 的 ReportData 一致：
//   { channels, yearMonths, rows: [{ code, name, level, parent, category, subChannel, total{rangeTotal, cells{}}, byChannel[]}] }
//
// 每个 (year, month) 取该 year 已有的最新 snapshot 数据：
//   yearStart=2024..2026 → 自动用 (2024-12, 2025-12, 2026-04) 三份 snapshot
//   yearMonths = ["2024-1", ..., "2024-12", "2025-1", ..., "2026-12"]

type bbrCell struct {
	Amount float64  `json:"amount"`
	Ratio  *float64 `json:"ratio,omitempty"`
}

type bbrChannelSeries struct {
	Channel string `json:"channel"`
	Series  bbrSeries `json:"series"`
}

type bbrSeries struct {
	RangeTotal bbrCell            `json:"rangeTotal"`
	Cells      map[string]bbrCell `json:"cells"`
}

type bbrRow struct {
	Code       string             `json:"code"`
	Name       string             `json:"name"`
	Level      int                `json:"level"`
	Parent     string             `json:"parent"`
	Category   string             `json:"category"`
	SubChannel string             `json:"subChannel"`
	Total      bbrSeries          `json:"total"`
	ByChannel  []bbrChannelSeries `json:"byChannel,omitempty"`
}

func (h *DashboardHandler) GetBusinessReportFinanceLike(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	yearStart, _ := strconv.Atoi(q.Get("yearStart"))
	yearEnd, _ := strconv.Atoi(q.Get("yearEnd"))
	monthStart, _ := strconv.Atoi(q.Get("monthStart"))
	monthEnd, _ := strconv.Atoi(q.Get("monthEnd"))
	if yearStart == 0 || yearEnd == 0 {
		yearStart, yearEnd = 2024, 2026
	}
	if monthStart < 1 || monthStart > 12 {
		monthStart = 1
	}
	if monthEnd < 1 || monthEnd > 12 {
		monthEnd = 12
	}
	if yearStart > yearEnd {
		yearStart, yearEnd = yearEnd, yearStart
	}
	channelsCsv := strings.TrimSpace(q.Get("channels"))
	if channelsCsv == "" {
		channelsCsv = "总"
	}
	channels := splitCsv(channelsCsv)

	// 1. 找 [yearStart..yearEnd] 范围内每年最新 snapshot
	snapRows, err := h.DB.Query(`
		SELECT snapshot_year, MAX(snapshot_month) AS sm
		FROM business_budget_report
		WHERE snapshot_year BETWEEN ? AND ?
		GROUP BY snapshot_year
		ORDER BY snapshot_year`, yearStart, yearEnd)
	if writeDatabaseError(w, err) {
		return
	}
	type yearSnap struct{ year, month int }
	var snaps []yearSnap
	for snapRows.Next() {
		var ys yearSnap
		if writeDatabaseError(w, snapRows.Scan(&ys.year, &ys.month)) {
			snapRows.Close()
			return
		}
		snaps = append(snaps, ys)
	}
	snapRows.Close()
	if len(snaps) == 0 {
		writeJSON(w, map[string]interface{}{
			"channels":   channels,
			"yearMonths": []string{},
			"rows":       []bbrRow{},
		})
		return
	}

	// 2. 构造 yearMonths 列表 (空 month 也保留方便对比，但只显示有数据的年)
	yearMonths := []string{}
	for _, ys := range snaps {
		for m := monthStart; m <= monthEnd; m++ {
			yearMonths = append(yearMonths, fmt.Sprintf("%d-%d", ys.year, m))
		}
	}

	// 3. 一次性 fetch 所有数据（按 snapshot OR 拼）
	conds := []string{}
	args := []interface{}{}
	for _, ys := range snaps {
		conds = append(conds, "(snapshot_year=? AND snapshot_month=?)")
		args = append(args, ys.year, ys.month)
	}
	chPH := strings.TrimSuffix(strings.Repeat("?,", len(channels)), ",")
	for _, c := range channels {
		args = append(args, c)
	}
	args = append(args, monthStart, monthEnd)

	query := fmt.Sprintf(`
		SELECT snapshot_year, channel, sub_channel, parent_subject, subject,
		       subject_level, subject_category, sort_order, period_month, actual, ratio_actual
		FROM business_budget_report
		WHERE (%s) AND channel IN (%s) AND period_month BETWEEN ? AND ?
		ORDER BY channel, sub_channel, sort_order, period_month`,
		strings.Join(conds, " OR "), chPH)
	dataRows, err := h.DB.Query(query, args...)
	if writeDatabaseError(w, err) {
		return
	}
	defer dataRows.Close()

	// 行 key: (subject, sub_channel) — 跟财务报表 keyOf 风格一致
	type rowKey struct{ subject, subChannel string }
	type rowAcc struct {
		row    *bbrRow
		byChan map[string]*bbrChannelSeries // channel → series
	}
	rowMap := map[rowKey]*rowAcc{}
	var order []rowKey

	for dataRows.Next() {
		var (
			snapY, level, sortOrd, pm                int
			channel, subChannel, parent, subject, cat string
			actual, ratio                            sql.NullFloat64
		)
		if writeDatabaseError(w, dataRows.Scan(&snapY, &channel, &subChannel, &parent, &subject,
			&level, &cat, &sortOrd, &pm, &actual, &ratio)) {
			return
		}
		// 跳过 period_month=0（合计列我们用 rangeTotal 计算，避免重复）
		if pm == 0 {
			continue
		}
		rk := rowKey{subject, subChannel}
		acc, exists := rowMap[rk]
		if !exists {
			acc = &rowAcc{
				row: &bbrRow{
					Code:       subject,
					Name:       subject,
					Level:      level,
					Parent:     parent,
					Category:   cat,
					SubChannel: subChannel,
					Total: bbrSeries{
						Cells: map[string]bbrCell{},
					},
				},
				byChan: map[string]*bbrChannelSeries{},
			}
			rowMap[rk] = acc
			order = append(order, rk)
		}
		ym := fmt.Sprintf("%d-%d", snapY, pm)
		amount := 0.0
		if actual.Valid {
			amount = actual.Float64
		}
		var rPtr *float64
		if ratio.Valid {
			rv := ratio.Float64
			rPtr = &rv
		}

		// per-channel
		cs, csExists := acc.byChan[channel]
		if !csExists {
			cs = &bbrChannelSeries{Channel: channel, Series: bbrSeries{Cells: map[string]bbrCell{}}}
			acc.byChan[channel] = cs
		}
		cell := bbrCell{Amount: amount, Ratio: rPtr}
		cs.Series.Cells[ym] = cell
		cs.Series.RangeTotal = bbrCell{Amount: cs.Series.RangeTotal.Amount + amount, Ratio: nil}

		// total 跨 channel 累加（如果只 1 个 channel，total = byChannel[0]）
		t := acc.row.Total.Cells[ym]
		t.Amount += amount
		acc.row.Total.Cells[ym] = t
		acc.row.Total.RangeTotal = bbrCell{Amount: acc.row.Total.RangeTotal.Amount + amount, Ratio: nil}
	}

	// 组装 rows
	rows := make([]bbrRow, 0, len(order))
	for _, rk := range order {
		acc := rowMap[rk]
		if len(channels) > 1 {
			for _, ch := range channels {
				if cs, ok := acc.byChan[ch]; ok {
					acc.row.ByChannel = append(acc.row.ByChannel, *cs)
				}
			}
		}
		rows = append(rows, *acc.row)
	}

	writeJSON(w, map[string]interface{}{
		"channels":   channels,
		"yearMonths": yearMonths,
		"rows":       rows,
	})
}

func splitCsv(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
