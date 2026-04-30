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

// bbrCell 业务报表单元格 — 同时含 budget / actual / achievement_rate / ratio
// （v0.59 跑哥要求："我看预算怎么没有展示出来"）
type bbrCell struct {
	Budget          *float64 `json:"budget,omitempty"`
	Actual          *float64 `json:"actual,omitempty"`
	AchievementRate *float64 `json:"achievementRate,omitempty"` // = actual/budget
	Ratio           *float64 `json:"ratio,omitempty"`            // 占比销售
}

type bbrChannelSeries struct {
	Channel string    `json:"channel"`
	Series  bbrSeries `json:"series"`
}

type bbrSeries struct {
	RangeTotal bbrCell            `json:"rangeTotal"`
	Cells      map[string]bbrCell `json:"cells"`
}

// bbrRow 业务报表行 — children 用于子渠道下钻（点电商展开看 TOC/TOB）
type bbrRow struct {
	Code       string             `json:"code"`
	Name       string             `json:"name"`
	Level      int                `json:"level"`
	Parent     string             `json:"parent"`
	Category   string             `json:"category"`
	Channel    string             `json:"channel"`
	SubChannel string             `json:"subChannel"`
	Total      bbrSeries          `json:"total"`
	ByChannel  []bbrChannelSeries `json:"byChannel,omitempty"`
	Children   []bbrRow           `json:"children,omitempty"`
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
		channelsCsv = "总|"
	}
	// channels 每项格式 "channel|subChannel"（subChannel 空表示一级渠道汇总）
	type chPair struct{ channel, subChannel string }
	rawList := splitCsv(channelsCsv)
	chPairs := make([]chPair, 0, len(rawList))
	for _, item := range rawList {
		parts := strings.SplitN(item, "|", 2)
		ch := strings.TrimSpace(parts[0])
		sc := ""
		if len(parts) == 2 {
			sc = strings.TrimSpace(parts[1])
		}
		if ch != "" {
			chPairs = append(chPairs, chPair{ch, sc})
		}
	}
	if len(chPairs) == 0 {
		chPairs = []chPair{{"总", ""}}
	}
	// 兼容 byChannel 维度：用 "channel|subChannel" 作字符串标识
	chKey := func(ch, sc string) string {
		if sc == "" {
			return ch
		}
		return ch + "|" + sc
	}
	channels := make([]string, len(chPairs))
	for i, p := range chPairs {
		channels[i] = chKey(p.channel, p.subChannel)
	}

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

	// 1.5 找全库最新 snapshot 作为科目模板骨架（跑哥 2026-04-30 要求）
	// 所有年份 row 用最新 snapshot 的 (channel, sub_channel, subject) 集合作骨架
	// 缺数据的 cell 留空，避免因早年缺科目导致行序不齐
	var tplYear, tplMonth int
	err = h.DB.QueryRow(`
		SELECT snapshot_year, snapshot_month FROM business_budget_report
		ORDER BY snapshot_year DESC, snapshot_month DESC LIMIT 1`).Scan(&tplYear, &tplMonth)
	if writeDatabaseError(w, err) {
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
	// (channel, sub_channel) tuple WHERE 条件
	chTupleConds := make([]string, len(chPairs))
	for i, p := range chPairs {
		chTupleConds[i] = "(channel=? AND sub_channel=?)"
		args = append(args, p.channel, p.subChannel)
	}
	chTupleClause := "(" + strings.Join(chTupleConds, " OR ") + ")"
	args = append(args, monthStart, monthEnd)

	query := fmt.Sprintf(`
		SELECT snapshot_year, channel, sub_channel, parent_subject, subject,
		       subject_level, subject_category, sort_order, period_month,
		       budget, actual, ratio_actual, achievement_rate
		FROM business_budget_report
		WHERE (%s) AND %s AND period_month BETWEEN ? AND ?
		ORDER BY channel, sub_channel, sort_order, period_month`,
		strings.Join(conds, " OR "), chTupleClause)
	dataRows, err := h.DB.Query(query, args...)
	if writeDatabaseError(w, err) {
		return
	}
	defer dataRows.Close()

	// 行结构：rowKey = (parent_subject, subject) 区分跨父级同名 subject
	// 例如"人工成本"在销售费用和管理费用下都有 — 必须分行展示，不能去重
	// (channel, sub_channel) pair 不再做行维度，直接当 byChannel 列展示
	type rowKey struct{ parent, subject string }
	type rowAcc struct {
		row    *bbrRow
		byChan map[string]*bbrChannelSeries
	}
	rowMap := map[rowKey]*rowAcc{}
	var order []rowKey

	// 骨架固定取 "总" sheet 的科目顺序和数量
	// SELECT 加 DISTINCT 避免 0-12 month 数据 row 重复返回 13 次同 subject
	// ORDER BY sort_order 保证按 xlsx 行序展示
	tplQuery := `
		SELECT DISTINCT subject, subject_level, parent_subject, subject_category, sort_order
		FROM business_budget_report
		WHERE snapshot_year=? AND snapshot_month=? AND channel='总' AND sub_channel=''
		ORDER BY sort_order`
	tplRows, err := h.DB.Query(tplQuery, tplYear, tplMonth)
	if writeDatabaseError(w, err) {
		return
	}
	for tplRows.Next() {
		var (
			parent, subject, cat string
			level, sortOrd       int
		)
		if writeDatabaseError(w, tplRows.Scan(&subject, &level, &parent, &cat, &sortOrd)) {
			tplRows.Close()
			return
		}
		rk := rowKey{parent, subject}
		if _, exists := rowMap[rk]; exists {
			continue
		}
		_ = sortOrd
		rowMap[rk] = &rowAcc{
			row: &bbrRow{
				Code:     subject,
				Name:     subject,
				Level:    level,
				Parent:   parent,
				Category: cat,
				Total:    bbrSeries{Cells: map[string]bbrCell{}},
			},
			byChan: map[string]*bbrChannelSeries{},
		}
		order = append(order, rk)
	}
	tplRows.Close()

	addCell := func(cells map[string]bbrCell, ym string, b, a, r, ar sql.NullFloat64) {
		c := cells[ym]
		if b.Valid {
			v := b.Float64
			if c.Budget != nil {
				v += *c.Budget
			}
			c.Budget = &v
		}
		if a.Valid {
			v := a.Float64
			if c.Actual != nil {
				v += *c.Actual
			}
			c.Actual = &v
		}
		if r.Valid {
			rv := r.Float64
			c.Ratio = &rv
		}
		if ar.Valid {
			arv := ar.Float64
			c.AchievementRate = &arv
		}
		// 单 channel 时直接用 budget/actual 算达成率
		if c.AchievementRate == nil && c.Budget != nil && *c.Budget != 0 && c.Actual != nil {
			d := *c.Actual / *c.Budget
			c.AchievementRate = &d
		}
		cells[ym] = c
	}
	addRange := func(rt *bbrCell, b, a sql.NullFloat64) {
		if b.Valid {
			v := b.Float64
			if rt.Budget != nil {
				v += *rt.Budget
			}
			rt.Budget = &v
		}
		if a.Valid {
			v := a.Float64
			if rt.Actual != nil {
				v += *rt.Actual
			}
			rt.Actual = &v
		}
		if rt.Budget != nil && *rt.Budget != 0 && rt.Actual != nil {
			d := *rt.Actual / *rt.Budget
			rt.AchievementRate = &d
		}
	}

	for dataRows.Next() {
		var (
			snapY, level, sortOrd, pm                int
			channel, subChannel, parent, subject, cat string
			budget, actual, ratio, ar                sql.NullFloat64
		)
		if writeDatabaseError(w, dataRows.Scan(&snapY, &channel, &subChannel, &parent, &subject,
			&level, &cat, &sortOrd, &pm, &budget, &actual, &ratio, &ar)) {
			return
		}
		if pm == 0 {
			continue
		}
		rk := rowKey{parent, subject}
		acc, exists := rowMap[rk]
		if !exists {
			_ = level
			_ = cat
			continue
		}
		ym := fmt.Sprintf("%d-%d", snapY, pm)
		// per-(channel|sub_channel) series — 用 chKey 字符串作维度
		ck := chKey(channel, subChannel)
		cs, csExists := acc.byChan[ck]
		if !csExists {
			cs = &bbrChannelSeries{Channel: ck, Series: bbrSeries{Cells: map[string]bbrCell{}}}
			acc.byChan[ck] = cs
		}
		addCell(cs.Series.Cells, ym, budget, actual, ratio, ar)
		addRange(&cs.Series.RangeTotal, budget, actual)
		// total
		addCell(acc.row.Total.Cells, ym, budget, actual, ratio, ar)
		addRange(&acc.row.Total.RangeTotal, budget, actual)
	}

	// 组装 rows：rowKey 已经按 subject 唯一，直接 map → byChannel + total
	rows := make([]bbrRow, 0, len(order))
	for _, rk := range order {
		acc := rowMap[rk]
		if len(channels) > 1 {
			for _, ck := range channels {
				if cs, ok := acc.byChan[ck]; ok {
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

// GetBusinessReportChannelsList 返回最新 snapshot 下所有 (channel, sub_channel) pair
// 前端用此渲染 grouped checkbox（一级渠道为分组，二级 sub_channel 平铺）
func (h *DashboardHandler) GetBusinessReportChannelsList(w http.ResponseWriter, r *http.Request) {
	var tplYear, tplMonth int
	err := h.DB.QueryRow(`
		SELECT snapshot_year, snapshot_month FROM business_budget_report
		ORDER BY snapshot_year DESC, snapshot_month DESC LIMIT 1`).Scan(&tplYear, &tplMonth)
	if writeDatabaseError(w, err) {
		return
	}
	rows, err := h.DB.Query(`
		SELECT channel, sub_channel, COUNT(DISTINCT subject) AS subj_count
		FROM business_budget_report
		WHERE snapshot_year=? AND snapshot_month=?
		GROUP BY channel, sub_channel
		ORDER BY MIN(sheet_order), channel, sub_channel`, tplYear, tplMonth)
	if writeDatabaseError(w, err) {
		return
	}
	defer rows.Close()
	type chItem struct {
		Channel    string `json:"channel"`
		SubChannel string `json:"subChannel"`
		Key        string `json:"key"`   // "channel|subChannel"
		Label      string `json:"label"` // 显示名："电商" / "电商-TOC"
		SubjCount  int    `json:"subjCount"`
	}
	type chGroup struct {
		Channel string   `json:"channel"`
		Items   []chItem `json:"items"`
	}
	groupMap := map[string]*chGroup{}
	var groupOrder []string
	for rows.Next() {
		var ch, sc string
		var cnt int
		if writeDatabaseError(w, rows.Scan(&ch, &sc, &cnt)) {
			return
		}
		key := ch
		label := ch
		if sc != "" {
			key = ch + "|" + sc
			label = ch + "-" + sc
		}
		g, ok := groupMap[ch]
		if !ok {
			g = &chGroup{Channel: ch}
			groupMap[ch] = g
			groupOrder = append(groupOrder, ch)
		}
		g.Items = append(g.Items, chItem{Channel: ch, SubChannel: sc, Key: key, Label: label, SubjCount: cnt})
	}
	groups := make([]chGroup, 0, len(groupOrder))
	for _, k := range groupOrder {
		groups = append(groups, *groupMap[k])
	}
	writeJSON(w, map[string]interface{}{
		"snapshotYear":  tplYear,
		"snapshotMonth": tplMonth,
		"groups":        groups,
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
