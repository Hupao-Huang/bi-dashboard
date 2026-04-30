// Package business 业务预决算报表 xlsx 解析器
//
// 输入：财务出的 YYYY年业务预决算报表.xlsx，每月一份快照
// 输出：[]BudgetRow，按 (snapshot_year, snapshot_month, channel, sub_channel, subject, period_month) 写入 business_budget_report
//
// Sheet 命名约定:
//   - "总" → channel="总", sub_channel=""
//   - "经营指标" → 跳过（结构特殊，单独处理，目前不入库）
//   - "1、电商" / "2、私域" / "3、分销" / "4、社媒" / "5、线下" / "6、国际零售业务" / "中后台合计"
//     → channel 取去序号去后缀名（电商/私域/分销/社媒/线下/国际零售/中后台），sub_channel=""
//   - "电商-TOC" / "电商—TOC" / "分销-礼品" / "社媒-自营" / "社媒-小红书" / "社媒-视频号" / "社媒-外包"
//     → channel="电商|分销|社媒", sub_channel="TOC|礼品|自营|小红书|..."
//   - 大区 sheet（华南/华东/华北/华中/西南/西北/东北/重客/山东/母婴/新零售）
//     → channel="线下", sub_channel=该大区名（trim 尾空格）
//
// 列结构（Row 3 表头，共 56 列）:
//   [0] 项目
//   [1-7]  预算-年初/占比/合计-预算/占比/合计-实际/占比/达成率
//   [8-11] 1月-预算/占比/1月/占比
//   [12-15] 2月-预算/占比/2月/占比
//   ... 12 月，每月 4 列
//
// 行结构:
//   - 分组 header（"GMV数据"/"财务数据"）：单列内容，更新 current_category，不入库
//   - 验证行（项目=" 验证"）：跳过
//   - 普通科目行：解析 0-12 月数据，每个 period 入一行
package business

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// BudgetRow 一行业务预决算数据，对应 business_budget_report 表
type BudgetRow struct {
	SnapshotYear    int
	SnapshotMonth   int
	Year            int
	Channel         string
	SubChannel      string
	SheetOrder      int // sheet 在 xlsx 中的排序索引（按 GetSheetList 返回顺序）
	Subject         string // trim 后的科目名
	SubjectLevel    int    // 1/2/3
	SubjectCategory string // GMV数据 / 财务数据 / ""
	ParentSubject   string
	SortOrder       int
	PeriodMonth     int      // 0=合计/年初 1-12=月份
	BudgetYearStart *float64 // period_month=0 才有
	RatioYearStart  *float64
	Budget          *float64
	RatioBudget     *float64
	Actual          *float64
	RatioActual     *float64
	AchievementRate *float64 // period_month=0 才有
}

// ParseResult 解析结果
type ParseResult struct {
	SnapshotYear   int
	SnapshotMonth  int
	Year           int
	SourceFile     string
	Channels       []string // 出现的所有 channel
	SheetsHandled  int
	SheetsSkipped  int
	Rows           []BudgetRow
	RowCount       int
}

// ParseFile 解析单个 xlsx 文件
func ParseFile(fpath string, snapshotYear, snapshotMonth, year int) (*ParseResult, error) {
	if snapshotYear < 2020 || snapshotYear > 2050 || snapshotMonth < 1 || snapshotMonth > 12 {
		return nil, fmt.Errorf("snapshot 参数非法: %d-%d", snapshotYear, snapshotMonth)
	}
	if year < 2020 || year > 2050 {
		return nil, fmt.Errorf("year 参数非法: %d", year)
	}
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	defer f.Close()

	result := &ParseResult{
		SnapshotYear:  snapshotYear,
		SnapshotMonth: snapshotMonth,
		Year:          year,
		SourceFile:    fpath,
	}
	channelSet := map[string]bool{}

	for sheetIdx, sheetName := range f.GetSheetList() {
		t := strings.TrimSpace(sheetName)
		// 经营指标 sheet（KPI 表，结构特殊）
		if strings.Contains(t, "经营指标") {
			rows, err := f.GetRows(sheetName)
			if err == nil && len(rows) >= 4 {
				newRows := parseKPISheet(rows, snapshotYear, snapshotMonth, year, sheetIdx)
				if len(newRows) > 0 {
					result.Rows = append(result.Rows, newRows...)
					channelSet["经营指标"] = true
					result.SheetsHandled++
					continue
				}
			}
			result.SheetsSkipped++
			continue
		}
		// 中后台合计 sheet（无合计/占比/达成率，仅 12 月 budget+actual）
		if t == "中后台合计" {
			rows, err := f.GetRows(sheetName)
			if err == nil && len(rows) >= 6 {
				newRows := parseBackOfficeSheet(rows, snapshotYear, snapshotMonth, year, sheetIdx)
				if len(newRows) > 0 {
					result.Rows = append(result.Rows, newRows...)
					channelSet["中后台"] = true
					result.SheetsHandled++
					continue
				}
			}
			result.SheetsSkipped++
			continue
		}
		channel, subChannel, ok := parseSheetName(sheetName)
		if !ok {
			result.SheetsSkipped++
			continue
		}
		rows, err := f.GetRows(sheetName)
		if err != nil || len(rows) < 4 {
			result.SheetsSkipped++
			continue
		}
		// 检测列布局（layoutFull/Compact/Unknown）
		layout := detectLayout(rows[2])
		if layout == layoutUnknown {
			result.SheetsSkipped++
			continue
		}
		channelSet[channel] = true
		result.SheetsHandled++

		// 解析主体行
		var currentCategory string
		var level1Parent string
		for i := 3; i < len(rows); i++ {
			r := rows[i]
			if len(r) == 0 {
				continue
			}
			rawSubject := r[0]
			subject := strings.TrimSpace(rawSubject)
			if subject == "" {
				continue
			}
			// 验证行
			if strings.HasPrefix(subject, "验证") {
				continue
			}
			// 分组 header（只有第 0 列有值，其他列空或 0）
			if isGroupHeader(r) {
				currentCategory = subject
				continue
			}
			level := detectLevel(rawSubject, subject)
			if level == 1 {
				level1Parent = subject
			}
			parent := ""
			if level >= 2 {
				parent = level1Parent
			}
			// 解析每个 period 的数据
			for pm := 0; pm <= 12; pm++ {
				br := parseRowPeriod(r, pm, layout)
				if !hasAnyValue(br) {
					continue
				}
				br.SnapshotYear = snapshotYear
				br.SnapshotMonth = snapshotMonth
				br.Year = year
				br.Channel = channel
				br.SubChannel = subChannel
				br.SheetOrder = sheetIdx
				br.Subject = subject
				br.SubjectLevel = level
				br.SubjectCategory = currentCategory
				br.ParentSubject = parent
				br.SortOrder = i
				br.PeriodMonth = pm
				result.Rows = append(result.Rows, br)
			}
		}
	}

	for c := range channelSet {
		result.Channels = append(result.Channels, c)
	}
	result.RowCount = len(result.Rows)
	return result, nil
}

// parseSheetName sheet 名 → (channel, sub_channel, ok)
// 不可识别的 sheet 返回 ok=false 跳过
//
// channel 父级序号约定（来自 xlsx 中的"X、xxx"前缀，跑哥维护）:
//   5、线下 → channel="线下"，5.1/5.2 子项归此 channel
//   其他序号 1/2/3/4/6/7/8 一级渠道
func parseSheetName(s string) (string, string, bool) {
	t := strings.TrimSpace(s)
	if t == "" {
		return "", "", false
	}
	// 总
	if t == "总" {
		return "总", "", true
	}
	// 中后台合计
	if t == "中后台合计" {
		return "中后台", "", true
	}
	// "X.Y子渠道-供应商" 格式：当前已知 5.1/5.2 归线下
	if m := reLevel2NumPrefix.FindStringSubmatch(t); m != nil {
		parentNum := m[1]
		body := strings.TrimSpace(m[2])
		if parentNum == "5" {
			return "线下", body, true
		}
		// 其他父级序号未约定，先按 channel 处理（保留全名作为标识）
		return body, "", true
	}
	// 一级渠道：1、电商 / 2、私域 / ... / 8、糙能
	if m := reLevel1Channel.FindStringSubmatch(t); m != nil {
		base := strings.TrimSpace(m[1])
		base = normalizeChannel(base)
		if !strings.ContainsAny(base, "-—") {
			return base, "", true
		}
	}
	// 国际零售业务（兜底，"6、国际零售业务" 已经被 reLevel1Channel 命中后归一化）
	if strings.Contains(t, "国际零售") {
		return "国际零售", "", true
	}
	// 二级子渠道：电商-TOC / 电商—TOC / 分销-礼品 / 社媒-自营 / ...
	for _, sep := range []string{"—", "-"} {
		if idx := strings.Index(t, sep); idx > 0 {
			parent := strings.TrimSpace(t[:idx])
			child := strings.TrimSpace(t[idx+len(sep):])
			parent = stripLevel1Prefix(parent)
			parent = normalizeChannel(parent)
			child = strings.TrimSpace(child)
			child = strings.Trim(child, "()（）")
			if parent != "" && child != "" {
				return parent, child, true
			}
		}
	}
	// 大区 sheet → 线下
	if isOfflineRegion(t) {
		return "线下", strings.TrimSpace(t), true
	}
	return "", "", false
}

var reLevel1Channel = regexp.MustCompile(`^[0-9一二三四五六七八九十]+[、.]?\s*(.+)$`)
var reLevel2NumPrefix = regexp.MustCompile(`^(\d+)\.\d+\s*(.+)$`)

func stripLevel1Prefix(s string) string {
	if m := reLevel1Channel.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1])
	}
	return strings.TrimSpace(s)
}

// normalizeChannel 标准化一级渠道名（处理"国际零售业务"→"国际零售"等）
func normalizeChannel(s string) string {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "国际零售") {
		return "国际零售"
	}
	return s
}

func isOfflineRegion(s string) bool {
	t := strings.TrimSpace(s)
	for _, r := range []string{"华南", "华东", "华北", "华中", "西南", "西北", "东北", "重客", "山东", "母婴", "新零售"} {
		if t == r {
			return true
		}
	}
	return false
}

// sheetLayout 标识 sheet 列布局
//   layoutFull: 56+ 列（2026/2025）— 含 预算-年初 + 合计-预算 + 合计-实际 + 达成率 + 12 月每月 4 列
//   layoutCompact: 53 列（2024）— 无 预算-年初 + 无 达成率，只有 合计-预算 + 合计-实际 + 12 月每月 4 列
//   layoutUnknown: 不识别（如 2023 7 列极简版），跳过
type sheetLayout int

const (
	layoutUnknown sheetLayout = iota
	layoutFull                // 2026/2025: 56+ 列，含 预算-年初 + 达成率
	layoutCompact             // 2024: 53 列，无 预算-年初 + 无 达成率
	layoutMinimal             // 2023: 7 列，仅 合计+11月+12月（早期版本，无预算）
)

// detectLayout 根据 Row 3 表头识别 sheet 列布局
func detectLayout(r []string) sheetLayout {
	if len(r) < 4 {
		return layoutUnknown
	}
	hasItem, hasBudgetStart, hasBudgetTotal, hasMonth1Budget, hasMonth11, hasTotalOnly := false, false, false, false, false, false
	for _, c := range r {
		ct := strings.TrimSpace(c)
		switch {
		case ct == "项目":
			hasItem = true
		case strings.Contains(ct, "预算-年初"):
			hasBudgetStart = true
		case strings.Contains(ct, "合计-预算"):
			hasBudgetTotal = true
		case strings.Contains(ct, "1月-预算"):
			hasMonth1Budget = true
		case ct == "11月":
			hasMonth11 = true
		case ct == "合计":
			hasTotalOnly = true
		}
	}
	if !hasItem {
		return layoutUnknown
	}
	if hasBudgetTotal && hasMonth1Budget {
		if hasBudgetStart {
			return layoutFull
		}
		return layoutCompact
	}
	// 2023 极简：项目 + 合计 + 11月 + 12月
	if hasTotalOnly && hasMonth11 {
		return layoutMinimal
	}
	return layoutUnknown
}

// isValidHeader Row 3 是否标准业务报表表头
func isValidHeader(r []string) bool {
	return detectLayout(r) != layoutUnknown
}

// isGroupHeader 分组 header 行：只识别已知的"GMV数据"/"财务数据" + 中后台 sheet 的"品牌费用/管理费用/财务费用"
// 跑哥 2026-04-30 反馈：旧版本"全 cell 为 0 即 group header"会误判全 0 数据科目（如"样品费用"）
// 必须用白名单严判，避免误吞数据行
func isGroupHeader(r []string) bool {
	if len(r) == 0 {
		return false
	}
	first := strings.TrimSpace(r[0])
	if first == "" {
		return false
	}
	// 总/各渠道 sheet 的分组 header
	if first == "GMV数据" || first == "财务数据" {
		return true
	}
	// 中后台合计 sheet 的分组 header
	if first == "品牌费用" || first == "管理费用" || first == "财务费用" || first == "人数" || first == "人均薪酬" {
		return true
	}
	return false
}

// detectLevel 根据原始 subject 字符串识别层级
//   level 1: "一、" / "二、" / "三、" 前缀；"减：" 前缀；"营业毛利"/"运营利润"/"利润总额"/"净利润"
//   level 3: 单字 S/A/B/C/其他/新品（SKU 分级）；"A、" / "B、" / "C、" 前缀
//   level 2: 其他
func detectLevel(raw, trimmed string) int {
	// SKU 分级：单一字符
	switch trimmed {
	case "S", "A", "B", "C", "其他", "新品":
		return 3
	}
	// "A、xxx" / "B、xxx" / "C、xxx" → level 3
	if reLevel3Prefix.MatchString(trimmed) {
		return 3
	}
	// "一、" / "二、" / "三、" 前缀
	if reLevel1NumPrefix.MatchString(trimmed) {
		return 1
	}
	// "减：" 前缀
	if strings.HasPrefix(trimmed, "减：") || strings.HasPrefix(trimmed, "减:") {
		return 1
	}
	// 计算项关键词
	for _, kw := range []string{"营业毛利", "运营利润", "利润总额", "净利润"} {
		if trimmed == kw || strings.Contains(trimmed, kw) {
			return 1
		}
	}
	// 默认 level 2
	return 2
}

var reLevel1NumPrefix = regexp.MustCompile(`^[一二三四五六七八九十][、：:]`)
var reLevel3Prefix = regexp.MustCompile(`^[A-D][、.]`)

// parseRowPeriod 提取一行的某个 period 数据
//   layoutFull (2026/2025):
//     period 0: cols [1-7] 预算-年初/占比/合计-预算/占比/合计-实际/占比/达成率
//     period N (1-12): cols [8 + (N-1)*4 .. +3] 预算/占比/实际/占比
//   layoutCompact (2024):
//     period 0: cols [1-4] 合计-预算/占比/合计-实际/占比 (无年初/达成率)
//     period N (1-12): cols [5 + (N-1)*4 .. +3] 预算/占比/实际/占比
func parseRowPeriod(r []string, pm int, layout sheetLayout) BudgetRow {
	br := BudgetRow{}
	if layout == layoutFull {
		if pm == 0 {
			br.BudgetYearStart = parseNum(safeCol(r, 1))
			br.RatioYearStart = parsePct(safeCol(r, 2))
			br.Budget = parseNum(safeCol(r, 3))
			br.RatioBudget = parsePct(safeCol(r, 4))
			br.Actual = parseNum(safeCol(r, 5))
			br.RatioActual = parsePct(safeCol(r, 6))
			br.AchievementRate = parsePct(safeCol(r, 7))
			return br
		}
		base := 8 + (pm-1)*4
		br.Budget = parseNum(safeCol(r, base))
		br.RatioBudget = parsePct(safeCol(r, base+1))
		br.Actual = parseNum(safeCol(r, base+2))
		br.RatioActual = parsePct(safeCol(r, base+3))
		return br
	}
	if layout == layoutCompact {
		if pm == 0 {
			br.Budget = parseNum(safeCol(r, 1))
			br.RatioBudget = parsePct(safeCol(r, 2))
			br.Actual = parseNum(safeCol(r, 3))
			br.RatioActual = parsePct(safeCol(r, 4))
			// 无年初预算 / 无达成率，但合计/实际有则计算达成率
			if br.Budget != nil && *br.Budget != 0 && br.Actual != nil {
				ar := *br.Actual / *br.Budget
				br.AchievementRate = &ar
			}
			return br
		}
		base := 5 + (pm-1)*4
		br.Budget = parseNum(safeCol(r, base))
		br.RatioBudget = parsePct(safeCol(r, base+1))
		br.Actual = parseNum(safeCol(r, base+2))
		br.RatioActual = parsePct(safeCol(r, base+3))
		return br
	}
	// layoutMinimal (2023): 项目 + 合计 + 11月 + 12月，无预算
	switch pm {
	case 0:
		br.Actual = parseNum(safeCol(r, 1))
		br.RatioActual = parsePct(safeCol(r, 2))
	case 11:
		br.Actual = parseNum(safeCol(r, 3))
		br.RatioActual = parsePct(safeCol(r, 4))
	case 12:
		br.Actual = parseNum(safeCol(r, 5))
		br.RatioActual = parsePct(safeCol(r, 6))
	}
	return br
}

func safeCol(r []string, i int) string {
	if i < 0 || i >= len(r) {
		return ""
	}
	return r[i]
}

// parseNum 解析金额字符串：去千位逗号，处理 #REF!/#DIV/0!/空 → nil
func parseNum(s string) *float64 {
	t := strings.TrimSpace(s)
	if t == "" {
		return nil
	}
	if isExcelError(t) {
		return nil
	}
	// 去千位逗号
	t = strings.ReplaceAll(t, ",", "")
	t = strings.ReplaceAll(t, " ", "")
	// 去括号负数 (123.45) → -123.45
	if strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") {
		t = "-" + t[1:len(t)-1]
	}
	v, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return nil
	}
	return &v
}

// parsePct 解析百分比字符串："61.98%" → 0.6198；裸数字也接受
func parsePct(s string) *float64 {
	t := strings.TrimSpace(s)
	if t == "" {
		return nil
	}
	if isExcelError(t) {
		return nil
	}
	t = strings.ReplaceAll(t, ",", "")
	t = strings.ReplaceAll(t, " ", "")
	pctMode := false
	if strings.HasSuffix(t, "%") {
		pctMode = true
		t = t[:len(t)-1]
	}
	if strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") {
		t = "-" + t[1:len(t)-1]
	}
	v, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return nil
	}
	if pctMode {
		v = v / 100
	}
	return &v
}

func isExcelError(t string) bool {
	if t == "" {
		return false
	}
	if t[0] != '#' {
		return false
	}
	for _, e := range []string{"#REF!", "#DIV/0!", "#VALUE!", "#N/A", "#NAME?", "#NUM!", "#NULL!"} {
		if t == e {
			return true
		}
	}
	return strings.HasPrefix(t, "#")
}

func hasAnyValue(br BudgetRow) bool {
	return br.BudgetYearStart != nil || br.RatioYearStart != nil ||
		br.Budget != nil || br.RatioBudget != nil ||
		br.Actual != nil || br.RatioActual != nil ||
		br.AchievementRate != nil
}

// WriteResult 写入数据库（按 snapshot 维度全删后重写）
//
// 写入策略：UNIQUE KEY 是 (snapshot_year, snapshot_month, channel, sub_channel, subject, period_month)
// 同一份 snapshot 重导时，先按 (snapshot_year, snapshot_month) 删旧，再批量 INSERT
// 不影响其他 snapshot 的数据（每月一份独立快照）
func WriteResult(db *sql.DB, result *ParseResult) error {
	if len(result.Rows) == 0 {
		return errors.New("空结果，无数据写入")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. 删旧
	delRes, err := tx.Exec(`DELETE FROM business_budget_report WHERE snapshot_year=? AND snapshot_month=?`,
		result.SnapshotYear, result.SnapshotMonth)
	if err != nil {
		return fmt.Errorf("delete old snapshot: %w", err)
	}
	deleted, _ := delRes.RowsAffected()

	// 2. 批量 INSERT
	const batchSize = 500
	stmt := `INSERT INTO business_budget_report
		(snapshot_year, snapshot_month, year, channel, sub_channel, sheet_order, subject, subject_level,
		 subject_category, parent_subject, sort_order, period_month,
		 budget_year_start, ratio_year_start, budget, ratio_budget,
		 actual, ratio_actual, achievement_rate)
		VALUES `
	rowVals := "(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)"

	rows := result.Rows
	totalInserted := int64(0)
	for i := 0; i < len(rows); i += batchSize {
		end := i + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[i:end]
		valsList := make([]string, 0, len(batch))
		args := make([]interface{}, 0, len(batch)*18)
		for _, r := range batch {
			valsList = append(valsList, rowVals)
			args = append(args, r.SnapshotYear, r.SnapshotMonth, r.Year, r.Channel, r.SubChannel, r.SheetOrder,
				r.Subject, r.SubjectLevel, r.SubjectCategory, r.ParentSubject, r.SortOrder, r.PeriodMonth,
				nullIfNil(r.BudgetYearStart), nullIfNil(r.RatioYearStart),
				nullIfNil(r.Budget), nullIfNil(r.RatioBudget),
				nullIfNil(r.Actual), nullIfNil(r.RatioActual),
				nullIfNil(r.AchievementRate))
		}
		query := stmt + strings.Join(valsList, ",")
		res, err := tx.Exec(query, args...)
		if err != nil {
			return fmt.Errorf("insert batch [%d, %d): %w", i, end, err)
		}
		ins, _ := res.RowsAffected()
		totalInserted += ins
	}

	// 3. 写日志
	_, err = tx.Exec(`INSERT INTO business_budget_import_log
		(snapshot_year, snapshot_month, year, source_file, rows_inserted, rows_updated, rows_deleted, imported_by, status)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		result.SnapshotYear, result.SnapshotMonth, result.Year, result.SourceFile,
		totalInserted, 0, deleted, "admin", "success")
	if err != nil {
		return fmt.Errorf("write import log: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func nullIfNil(p *float64) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

// FormatTimestamp 给日志用
func FormatTimestamp(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

// parseBackOfficeSheet 解析 "中后台合计" sheet
//
// 列结构 (实际，对账总 sheet 验证):
//   col[0]: 科目名
//   col[1]: 全年合计-实际 (Row 5 表头"2026年中台预算"是误导，实测是合计-实际)
//   col[2,3]: 1月-预算 / 1月-实际
//   col[4,5]: 2月-预算 / 2月-实际
//   ... col[24,25]: 12月-预算 / 12月-实际
//
// 落库:
//   period_month=0: actual=col[1] (合计-实际，无预算/达成率)
//   period_month=N (1-12): budget=col[2N], actual=col[2N+1]
//
// 行结构:
//   Row 6: "数据维度 全年" 跳过
//   Row 7+: "品牌费用"/"管理费用"/"财务费用" 分组 header (1 级科目)
//           然后是科目 + 12 月数据；"合计"/"利润"/"人数"/"人均薪酬" 计算或元数据
//   "验证"/"数据维度" 跳过
func parseBackOfficeSheet(rows [][]string, snapshotYear, snapshotMonth, year, sheetIdx int) []BudgetRow {
	var out []BudgetRow
	const channel = "中后台"
	var currentCategory, level1Parent string
	for i := 5; i < len(rows); i++ {
		r := rows[i]
		if len(r) == 0 {
			continue
		}
		rawSubject := r[0]
		subject := strings.TrimSpace(rawSubject)
		if subject == "" {
			continue
		}
		if strings.HasPrefix(subject, "验证") || strings.HasPrefix(subject, "数据维度") {
			continue
		}
		// 分组 header 判定：只有 col[0] 有值，其他全空/0
		if isGroupHeader(r) {
			currentCategory = subject
			level1Parent = subject
			continue
		}
		level := 2
		// "合计"/"利润" 是分组计算行：subject 拼上分组名避免跨分组撞 UK
		if subject == "合计" || subject == "利润" {
			if currentCategory != "" {
				subject = currentCategory + subject
			}
			level = 1
		}
		// "1.办公费用-样品费用" 类有点号编号 → level 3
		if reLevel3Prefix.MatchString(subject) || strings.HasPrefix(subject, "1.") {
			level = 3
		}
		parent := ""
		if level >= 2 {
			parent = level1Parent
		}
		// period_month=0: 全年合计-实际
		if total := parseNum(safeCol(r, 1)); total != nil {
			out = append(out, BudgetRow{
				SnapshotYear:    snapshotYear,
				SnapshotMonth:   snapshotMonth,
				Year:            year,
				Channel:         channel,
				SubChannel:      "",
				SheetOrder:      sheetIdx,
				Subject:         subject,
				SubjectLevel:    level,
				SubjectCategory: currentCategory,
				ParentSubject:   parent,
				SortOrder:       i,
				PeriodMonth:     0,
				Actual:          total,
			})
		}
		// 解析 12 月每月 budget+actual (从 col[2,3] 开始)
		for pm := 1; pm <= 12; pm++ {
			budgetCol := 2 + (pm-1)*2
			actualCol := budgetCol + 1
			br := BudgetRow{}
			br.Budget = parseNum(safeCol(r, budgetCol))
			br.Actual = parseNum(safeCol(r, actualCol))
			if br.Budget == nil && br.Actual == nil {
				continue
			}
			br.SnapshotYear = snapshotYear
			br.SnapshotMonth = snapshotMonth
			br.Year = year
			br.Channel = channel
			br.SubChannel = ""
			br.SheetOrder = sheetIdx
			br.Subject = subject
			br.SubjectLevel = level
			br.SubjectCategory = currentCategory
			br.ParentSubject = parent
			br.SortOrder = i
			br.PeriodMonth = pm
			out = append(out, br)
		}
	}
	return out
}

// parseKPISheet 解析 "经营指标" sheet (KPI 表)
//
// 列结构 (Row 2 表头):
//   col[0]: 序号
//   col[1]: 指标项目 (subject)
//   col[2]: 年度预算（"预算数"）
//   col[3]: 上年数 → 用作 actual 的代理（实际去年值，作展示对比）
//   col[4]: 增长率/额 → 存到 achievement_rate
//   col[5-16]: 12 月分月数据（按 row 3 显示是 1-12 月数字）
//   col[17+]: 备注或其他字段（暂忽略）
//
// 落库时：
//   period_month=0: budget=col[2], actual=col[3], achievement_rate=col[4]
//   period_month 1-12: actual=col[5+m-1] (单值，按位置当实际值)
func parseKPISheet(rows [][]string, snapshotYear, snapshotMonth, year, sheetIdx int) []BudgetRow {
	var out []BudgetRow
	const channel = "经营指标"
	for i := 3; i < len(rows); i++ {
		r := rows[i]
		if len(r) < 2 {
			continue
		}
		// col[0] 序号，col[1] 指标项目
		subject := strings.TrimSpace(safeCol(r, 1))
		// 多行单元格（含换行）：取首行
		if idx := strings.IndexAny(subject, "\n\r"); idx > 0 {
			subject = strings.TrimSpace(subject[:idx])
		}
		if subject == "" {
			continue
		}
		if strings.HasPrefix(subject, "验证") {
			continue
		}
		// period_month=0 行：budget=col[2], actual=col[3], achievement_rate=col[4]
		br0 := BudgetRow{
			SnapshotYear:    snapshotYear,
			SnapshotMonth:   snapshotMonth,
			Year:            year,
			Channel:         channel,
			SubChannel:      "",
			SheetOrder:      sheetIdx,
			Subject:         subject,
			SubjectLevel:    2, // KPI 都是同级指标
			SubjectCategory: "核心指标",
			ParentSubject:   "",
			SortOrder:       i,
			PeriodMonth:     0,
		}
		br0.Budget = parseNum(safeCol(r, 2))
		// 上年数和增长率根据指标类型可能是百分比，用 parsePct 兼容
		br0.Actual = parseNum(safeCol(r, 3))
		if br0.Actual == nil {
			br0.RatioYearStart = parsePct(safeCol(r, 3))
		}
		br0.AchievementRate = parsePct(safeCol(r, 4))
		if hasAnyValue(br0) {
			out = append(out, br0)
		}
		// period 1-12: 单值（实际数据）
		for pm := 1; pm <= 12; pm++ {
			col := 4 + pm // col[5..16]
			val := parseNum(safeCol(r, col))
			if val == nil {
				val = parsePct(safeCol(r, col))
			}
			if val == nil {
				continue
			}
			brM := BudgetRow{
				SnapshotYear:    snapshotYear,
				SnapshotMonth:   snapshotMonth,
				Year:            year,
				Channel:         channel,
				SubChannel:      "",
				Subject:         subject,
				SubjectLevel:    2,
				SubjectCategory: "核心指标",
				ParentSubject:   "",
				SortOrder:       i,
				PeriodMonth:     pm,
				Actual:          val,
			}
			out = append(out, brM)
		}
	}
	return out
}
