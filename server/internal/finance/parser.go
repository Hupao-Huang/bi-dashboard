// Package finance 财务报表解析和导入，被 cmd/import-finance 和 handler/finance_report 复用
package finance

import (
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// SheetDeptMap sheet 名到渠道的映射
var SheetDeptMap = map[string]string{
	"考核利润汇总表":   "汇总",
	"1、电商":      "电商",
	"2、社媒":      "社媒",
	"2、抖音":      "社媒",
	"3、线下":      "线下",
	"4、分销":      "分销",
	"5、私域":      "私域",
	"5、 私域":     "私域",
	"6、国际零售业务": "国际零售",
	"7、即时零售":   "即时零售",
	"8、糙有力量":   "糙有力量",
	"中台部门-明细":  "中台",
}

// Level2CodeForName Level2 科目判定（前缀或全名匹配），返回匹配的 subject_code 和是否命中
func Level2CodeForName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	switch {
	case name == "GMV合计":
		return "GMV_TOTAL", true
	case name == "售退":
		return "RETURN", true
	case name == "营业额合计":
		return "REV_TOTAL", true
	case strings.HasPrefix(name, "一、营业收入"):
		return "REV_MAIN", true
	case strings.HasPrefix(name, "减：营业成本"):
		return "COST_MAIN", true
	case name == "营业毛利":
		return "PROFIT_GROSS", true
	case strings.HasPrefix(name, "减：销售费用"):
		return "SALES_EXP", true
	case name == "运营利润":
		return "PROFIT_OP", true
	case strings.HasPrefix(name, "减：管理费用"):
		return "MGMT_EXP", true
	case strings.HasPrefix(name, "减：研发费用"):
		return "RND_EXP", true
	case name == "利润总额" || name == "营业利润":
		return "PROFIT_TOTAL", true
	case strings.HasPrefix(name, "加：营业外收入"):
		return "NON_REV", true
	case strings.HasPrefix(name, "减：营业外支出"):
		return "NON_EXP", true
	case strings.HasPrefix(name, "其中：报废损失"):
		return "LOSS_SCRAP", true
	case name == "税金及附加":
		return "TAX_SURCHARGE", true
	case name == "所得税费用":
		return "TAX_INCOME", true
	case strings.HasPrefix(name, "二：净利润") || strings.HasPrefix(name, "二、净利润"):
		return "NET_PROFIT", true
	case strings.HasPrefix(name, "补充数据"):
		return "VAT_EXTRA", true
	}
	return "", false
}

// isContainerLevel2 判断该 Level 2 科目是否有子项（Excel 里子项行在它下面）
// 只有 COST_MAIN / SALES_EXP / MGMT_EXP / GMV_TOTAL 有子项
func isContainerLevel2(code string) bool {
	switch code {
	case "COST_MAIN", "SALES_EXP", "MGMT_EXP", "GMV_TOTAL":
		return true
	}
	return false
}

// cleanNumStr 去掉千分位逗号和空格，让 ParseFloat 能识别 Excel 格式化数字
func cleanNumStr(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	return s
}

// parsePercentOrFloat 解析数值。支持 "28.80%" → 0.288、"0.288" → 0.288、"1,234.56" → 1234.56
func parsePercentOrFloat(s string) (float64, error) {
	s = cleanNumStr(s)
	if strings.HasSuffix(s, "%") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err != nil {
			return 0, err
		}
		return v / 100, nil
	}
	return strconv.ParseFloat(s, 64)
}

// Level2CodeByDict 从字典按 name/aliases 查 Level 2 科目
func Level2CodeByDict(dict map[string]*DictEntry, name string) (string, bool) {
	name = strings.TrimSpace(name)
	for _, d := range dict {
		if d.Level != 2 {
			continue
		}
		if d.Name == name {
			return d.Code, true
		}
		for _, a := range d.Aliases {
			if a == name {
				return d.Code, true
			}
		}
	}
	return "", false
}

// Level1CodeForName Level1 分组行判定
func Level1CodeForName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	switch name {
	case "GMV数据":
		return "GMV_GROUP", true
	case "财务数据":
		return "FIN_GROUP", true
	}
	return "", false
}

// Level2Category 根据 Level2 code 返回科目分组
func Level2Category(code string) string {
	switch code {
	case "GMV_TOTAL", "RETURN", "REV_TOTAL":
		return "GMV"
	case "REV_MAIN":
		return "收入"
	case "COST_MAIN":
		return "成本"
	case "PROFIT_GROSS":
		return "毛利"
	case "SALES_EXP":
		return "销售费用"
	case "PROFIT_OP":
		return "运营利润"
	case "MGMT_EXP":
		return "管理费用"
	case "RND_EXP":
		return "研发费用"
	case "PROFIT_TOTAL":
		return "利润总额"
	case "NON_REV", "NON_EXP", "LOSS_SCRAP":
		return "营业外"
	case "TAX_SURCHARGE", "TAX_INCOME", "VAT_EXTRA":
		return "税费"
	case "NET_PROFIT":
		return "净利润"
	}
	return "其他"
}

// DictEntry 字典条目
type DictEntry struct {
	Code     string
	Name     string
	Category string
	Level    int
	Parent   string
	Aliases  []string
}

// LoadSubjectDict 从数据库加载科目字典
func LoadSubjectDict(db *sql.DB) (map[string]*DictEntry, error) {
	rows, err := db.Query(`SELECT subject_code, subject_name, subject_category, subject_level, parent_code, aliases FROM finance_subject_dict`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	dict := make(map[string]*DictEntry)
	for rows.Next() {
		var d DictEntry
		var aliasesJSON sql.NullString
		if err := rows.Scan(&d.Code, &d.Name, &d.Category, &d.Level, &d.Parent, &aliasesJSON); err != nil {
			return nil, err
		}
		if aliasesJSON.Valid && aliasesJSON.String != "" {
			_ = json.Unmarshal([]byte(aliasesJSON.String), &d.Aliases)
		}
		dict[d.Code] = &d
	}
	return dict, nil
}

// MatchLevel3 根据 parent_code + subject_name 查字典，返回 (subject_code, 实际parent_code)
// 三级匹配：
// 1) (parent, name) 精确
// 2) (parent, aliases) 模糊
// 3) 全局按 name/aliases 查所有 level=3 候选；若唯一则用候选的 parent（纠正 Excel 排序错误）
func MatchLevel3(dict map[string]*DictEntry, parentCode, name string) (string, string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ""
	}
	expected := parentCode + "." + name
	if d, ok := dict[expected]; ok {
		return d.Code, d.Parent
	}
	for _, d := range dict {
		if d.Parent != parentCode || d.Level != 3 {
			continue
		}
		if d.Name == name {
			return d.Code, d.Parent
		}
		for _, a := range d.Aliases {
			if a == name {
				return d.Code, d.Parent
			}
		}
	}
	// 全局 fallback：跨 parent 查同名科目
	var candidates []*DictEntry
	for _, d := range dict {
		if d.Level != 3 {
			continue
		}
		if d.Name == name {
			candidates = append(candidates, d)
			continue
		}
		for _, a := range d.Aliases {
			if a == name {
				candidates = append(candidates, d)
				break
			}
		}
	}
	if len(candidates) == 1 {
		return candidates[0].Code, candidates[0].Parent
	}
	return "", ""
}

// ParseYearFromFilename 从文件名推断年份（如 2026年财务管理报表.xlsx）
func ParseYearFromFilename(filename string) int {
	base := filepath.Base(filename)
	re := regexp.MustCompile(`(\d{4})\s*年`)
	m := re.FindStringSubmatch(base)
	if len(m) >= 2 {
		y, _ := strconv.Atoi(m[1])
		return y
	}
	return 0
}

// FileMD5 计算文件 MD5
func FileMD5(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := md5.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// UnmappedEntry 未匹配科目记录
type UnmappedEntry struct {
	Sheet      string `json:"sheet"`
	Department string `json:"department"`
	Subject    string `json:"subject"`
	Parent     string `json:"parent"`
}

// FinanceRow 一行财务数据（一个科目×一个月份的记录）
type FinanceRow struct {
	Year            int
	Month           int
	Department      string
	SubChannel      string
	SubjectCode     string
	SubjectName     string
	SubjectCategory string
	SubjectLevel    int
	ParentCode      string
	SortOrder       int
	Amount          float64
	Ratio           *float64
}

// ParseResult 解析结果
type ParseResult struct {
	Year             int             `json:"year"`
	Departments      []string        `json:"departments"`
	Rows             []FinanceRow    `json:"rows"`
	UnmappedSubjects []UnmappedEntry `json:"unmappedSubjects"`
	SheetCount       int             `json:"sheetCount"`
	RowCount         int             `json:"rowCount"`
	Mode             string          `json:"mode"` // "full" 全年覆盖 | "incremental" 按月精确替换
}

// ImportModeFull 累积版：按 (year, dept) 全删后重写，旧月份会被清理
const ImportModeFull = "full"

// ImportModeIncremental 增量版：只删除 Excel 里实际出现金额的 (year, dept, month)，未出现的月份保留旧值
const ImportModeIncremental = "incremental"

// ParseFile 解析 Excel 文件，返回结果（不写入数据库）
func ParseFile(fpath string, year int, dict map[string]*DictEntry) (*ParseResult, error) {
	f, err := excelize.OpenFile(fpath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	result := &ParseResult{Year: year}
	deptSet := map[string]bool{}

	for _, sheetName := range f.GetSheetList() {
		dept, ok := SheetDeptMap[sheetName]
		if !ok {
			continue
		}
		result.SheetCount++
		deptSet[dept] = true

		sheetRows, err := parseSheet(f, sheetName, dept, year, dict, &result.UnmappedSubjects)
		if err != nil {
			return nil, fmt.Errorf("sheet [%s] 解析失败: %w", sheetName, err)
		}
		result.Rows = append(result.Rows, sheetRows...)
	}

	for d := range deptSet {
		result.Departments = append(result.Departments, d)
	}

	result.Rows = RecomputeAggregateSubjects(result.Rows, dict, year)

	result.RowCount = len(result.Rows)
	return result, nil
}

// aggregateRule 父项 = SUM(子项) 的覆盖规则
type aggregateRule struct {
	ParentCode string
	ChildCodes []string
}

// aggregateRules 需要后端重算的父项清单
// 财务 xlsx 中这些父项是手填合计，导入时 SUM 子项覆盖，避免财务手算错
var aggregateRules = []aggregateRule{
	{
		ParentCode: "COST_MAIN.仓储物流费用",
		ChildCodes: []string{
			"COST_MAIN.物流费用",
			"COST_MAIN.临时工费用",
			"COST_MAIN.发货耗材成本",
		},
	},
}

// RecomputeAggregateSubjects 父项 amount = SUM(子项 amount)
//
// 跑哥 2026-04-30 决策："有就加，没有就不加"
// 这里"有/没有"指**字典 finance_subject_dict 里有没有 subject_code**:
//   - 字典里有此 subject_code（aggregateRules 已配置）→ 加 SUM（覆盖已有 row 或补建 row）
//   - 字典里没有 → 不加（不新增 subject_code，本函数也不会管）
//
// 父项 finance_report 数据 row 的处理:
//   - 已存在 → 用 SUM 覆盖 amount + ratio
//   - 不存在但子项存在 → 用 SUM 自动补一行（字典查 name/category）
//   - 子项有但缺值 → 当 0 处理
//
// 不新增 subject_code 字典项；只补 finance_report 数据 row
func RecomputeAggregateSubjects(rows []FinanceRow, dict map[string]*DictEntry, year int) []FinanceRow {
	type key struct {
		dept  string
		month int
	}

	for _, rule := range aggregateRules {
		childSet := map[string]bool{}
		for _, c := range rule.ChildCodes {
			childSet[c] = true
		}

		// 收集 SUM + REV_TOTAL + 子项是否出现
		sumMap := map[key]float64{}
		hasChild := map[key]bool{}
		revMap := map[key]float64{}
		for _, r := range rows {
			k := key{r.Department, r.Month}
			if childSet[r.SubjectCode] {
				sumMap[k] += r.Amount
				hasChild[k] = true
			}
			if r.SubjectCode == "REV_TOTAL" {
				revMap[k] = r.Amount
			}
		}

		// 已存在父项 row → 覆盖 amount + ratio
		existsParent := map[key]bool{}
		for i, r := range rows {
			if r.SubjectCode != rule.ParentCode {
				continue
			}
			k := key{r.Department, r.Month}
			existsParent[k] = true
			sum := sumMap[k]
			rows[i].Amount = sum
			if rev, ok := revMap[k]; ok && rev != 0 {
				ratio := sum / rev
				rows[i].Ratio = &ratio
			} else {
				rows[i].Ratio = nil
			}
		}

		// 父项 row 缺失但子项存在 → 自动补一行（不新增 subject_code，仅补 finance_report 数据）
		dictEntry := dict[rule.ParentCode]
		for k := range hasChild {
			if existsParent[k] {
				continue
			}
			subjectName := rule.ParentCode
			subjectCategory := ""
			parentCode := ""
			if dictEntry != nil {
				subjectName = dictEntry.Name
				subjectCategory = dictEntry.Category
				parentCode = dictEntry.Parent
			}
			sum := sumMap[k]
			var ratioPtr *float64
			if rev, ok := revMap[k]; ok && rev != 0 {
				ratio := sum / rev
				ratioPtr = &ratio
			}
			rows = append(rows, FinanceRow{
				Year:            year,
				Month:           k.month,
				Department:      k.dept,
				SubjectCode:     rule.ParentCode,
				SubjectName:     subjectName,
				SubjectCategory: subjectCategory,
				SubjectLevel:    3,
				ParentCode:      parentCode,
				SortOrder:       9999, // 父项排到本组末尾
				Amount:          sum,
				Ratio:           ratioPtr,
			})
		}
	}
	return rows
}

type colInfo struct {
	month    int
	ratioCol int
}

func parseSheet(f *excelize.File, sheetName, dept string, year int, dict map[string]*DictEntry, unmapped *[]UnmappedEntry) ([]FinanceRow, error) {
	allRows, _ := f.GetRows(sheetName)
	if len(allRows) < 3 {
		return nil, nil
	}

	// 自动检测 header 行：第 0~3 行内找 A 列 == "项目"
	headerIdx := -1
	for i := 0; i < 5 && i < len(allRows); i++ {
		if len(allRows[i]) > 0 && strings.TrimSpace(allRows[i][0]) == "项目" {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		return nil, nil
	}
	header := allRows[headerIdx]
	if len(header) > 30 {
		header = header[:30]
	}

	// 月份匹配：兼容 "N月" 和 "N月实际数" 格式
	monthRe := regexp.MustCompile(`^(\d{1,2})月`)
	colMap := map[int]*colInfo{}
	for i, cell := range header {
		c := strings.TrimSpace(cell)
		if c == "合计" {
			ci := &colInfo{month: 0, ratioCol: -1}
			if i+1 < len(header) && strings.TrimSpace(header[i+1]) == "占比销售" {
				ci.ratioCol = i + 1
			}
			colMap[i] = ci
			continue
		}
		if m := monthRe.FindStringSubmatch(c); m != nil {
			month, _ := strconv.Atoi(m[1])
			if month >= 1 && month <= 12 {
				ci := &colInfo{month: month, ratioCol: -1}
				if i+1 < len(header) && strings.TrimSpace(header[i+1]) == "占比销售" {
					ci.ratioCol = i + 1
				}
				colMap[i] = ci
			}
		}
	}
	if len(colMap) == 0 {
		return nil, nil
	}

	var rows []FinanceRow
	currentL1 := ""
	currentL2 := ""
	currentL2Cat := ""
	sortOrder := 0

	maxRi := len(allRows)
	if maxRi > 80 {
		maxRi = 80
	}
	for ri := headerIdx + 1; ri < maxRi; ri++ {
		if len(allRows[ri]) == 0 {
			continue
		}
		subject := strings.TrimSpace(allRows[ri][0])
		if subject == "" || subject == "项目" {
			continue
		}

		var level int
		var code, category, parent string

		if l1Code, ok := Level1CodeForName(subject); ok {
			currentL1 = l1Code
			currentL2 = ""
			currentL2Cat = ""
			sortOrder++
			continue
		}

		l2Code, l2ok := Level2CodeForName(subject)
		if !l2ok {
			if c, ok := Level2CodeByDict(dict, subject); ok {
				l2Code = c
				l2ok = true
			}
		}
		if l2ok {
			level = 2
			code = l2Code
			category = Level2Category(l2Code)
			parent = currentL1
			// 只有"有子项的 Level 2"才更新 currentL2，叶子 Level 2 不更新（让后续 level 3 继续归到前一个可接收子项的 level 2）
			if isContainerLevel2(l2Code) {
				currentL2 = l2Code
				currentL2Cat = category
			}
			if l2Code == "GMV_TOTAL" || l2Code == "RETURN" || l2Code == "REV_TOTAL" {
				parent = "GMV_GROUP"
			}
			// 兼容新格式：A3 直接是 "GMV合计"（旧格式 A3 是 "GMV数据" 分组标题）
			// 新格式没有显式的 Level1 分组行，遇到 GMV_TOTAL 就补设 currentL1，
			// 让后续 GMV 子项行能正确走 Level3 → GMV_SUB 分支
			if l2Code == "GMV_TOTAL" {
				currentL1 = "GMV_GROUP"
			}
		} else {
			level = 3
			if currentL1 == "GMV_GROUP" && (currentL2 == "" || currentL2 == "GMV_TOTAL") {
				code = "GMV_SUB"
				category = "GMV"
				parent = "GMV_TOTAL"
			} else if currentL2 == "" {
				*unmapped = append(*unmapped, UnmappedEntry{Sheet: sheetName, Department: dept, Subject: subject, Parent: ""})
				continue
			} else {
				matched, actualParent := MatchLevel3(dict, currentL2, subject)
				if matched == "" {
					*unmapped = append(*unmapped, UnmappedEntry{Sheet: sheetName, Department: dept, Subject: subject, Parent: currentL2})
					continue
				}
				code = matched
				parent = actualParent
				// 使用纠正后的 parent 推断 category
				if pd, ok := dict[actualParent]; ok {
					category = pd.Category
				} else {
					category = currentL2Cat
				}
			}
		}

		sortOrder++

		for ci, info := range colMap {
			if ci >= len(allRows[ri]) {
				continue
			}
			valStr := strings.TrimSpace(allRows[ri][ci])
			if valStr == "" || valStr == "#DIV/0!" || valStr == "#REF!" || valStr == "-" {
				continue
			}
			amt, err := strconv.ParseFloat(cleanNumStr(valStr), 64)
			if err != nil {
				continue
			}

			var ratioPtr *float64
			if info.ratioCol >= 0 && info.ratioCol < len(allRows[ri]) {
				rStr := strings.TrimSpace(allRows[ri][info.ratioCol])
				if rStr != "" && rStr != "#DIV/0!" && rStr != "#REF!" && rStr != "-" {
					if rv, err := parsePercentOrFloat(rStr); err == nil {
						ratioPtr = &rv
					}
				}
			}

			row := FinanceRow{
				Year:            year,
				Month:           info.month,
				Department:      dept,
				SubjectCode:     code,
				SubjectName:     subject,
				SubjectCategory: category,
				SubjectLevel:    level,
				ParentCode:      parent,
				SortOrder:       sortOrder,
				Amount:          amt,
				Ratio:           ratioPtr,
			}
			if code == "GMV_SUB" {
				row.SubChannel = subject
			}
			rows = append(rows, row)
		}
	}
	return rows, nil
}

// CollectDeptMonths 从解析结果提取每个部门实际出现金额的月份集合（含 month=0 合计列）
// 返回 map[department]map[month]bool
func CollectDeptMonths(result *ParseResult) map[string]map[int]bool {
	out := make(map[string]map[int]bool)
	for _, r := range result.Rows {
		if _, ok := out[r.Department]; !ok {
			out[r.Department] = make(map[int]bool)
		}
		out[r.Department][r.Month] = true
	}
	return out
}

// WriteResult 写入数据库：根据 result.Mode 决定 DELETE 范围
//   - ImportModeFull: 按 (year, dept) 全删（默认行为，保持向后兼容）
//   - ImportModeIncremental: 按 (year, dept, month) 精确删，未在 Excel 出现的月份保留旧值
func WriteResult(db *sql.DB, result *ParseResult) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	mode := result.Mode
	if mode == "" {
		mode = ImportModeFull
	}

	switch mode {
	case ImportModeIncremental:
		deptMonths := CollectDeptMonths(result)
		for dept, months := range deptMonths {
			for m := range months {
				if _, err := tx.Exec(`DELETE FROM finance_report WHERE year = ? AND department = ? AND month = ?`, result.Year, dept, m); err != nil {
					return fmt.Errorf("清理 %s/%d/%d 失败: %w", dept, result.Year, m, err)
				}
			}
		}
	default: // ImportModeFull
		for _, dept := range result.Departments {
			if _, err := tx.Exec(`DELETE FROM finance_report WHERE year = ? AND department = ?`, result.Year, dept); err != nil {
				return fmt.Errorf("清理 %s/%d 失败: %w", dept, result.Year, err)
			}
		}
	}

	stmt, err := tx.Prepare(`INSERT INTO finance_report
		(year, month, department, sub_channel, subject_code, subject_name, subject_category, subject_level, parent_code, sort_order, amount, ratio)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE subject_name=VALUES(subject_name), subject_category=VALUES(subject_category), subject_level=VALUES(subject_level), parent_code=VALUES(parent_code), sort_order=VALUES(sort_order), amount=VALUES(amount), ratio=VALUES(ratio)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range result.Rows {
		var ratioVal interface{}
		if r.Ratio != nil {
			ratioVal = *r.Ratio
		}
		if _, err := stmt.Exec(
			r.Year, r.Month, r.Department, r.SubChannel,
			r.SubjectCode, r.SubjectName, r.SubjectCategory, r.SubjectLevel, r.ParentCode,
			r.SortOrder, r.Amount, ratioVal,
		); err != nil {
			return fmt.Errorf("插入失败 [%s/%d/%d/%s/%s]: %w", r.Department, r.Year, r.Month, r.SubChannel, r.SubjectCode, err)
		}
	}

	return tx.Commit()
}

// DiffEntry 单个 (department, month) 的变更对比
type DiffEntry struct {
	Department string  `json:"department"`
	Month      int     `json:"month"`     // 0=年合计列，1-12=月份
	OldAmount  float64 `json:"oldAmount"` // 数据库现有营业额合计 (REV_TOTAL)
	NewAmount  float64 `json:"newAmount"` // Excel 新营业额合计
	Delta      float64 `json:"delta"`     // 差额
	DeltaPct   float64 `json:"deltaPct"`  // 变化百分比
	OldRows    int     `json:"oldRows"`   // 数据库现有行数（所有科目）
	NewRows    int     `json:"newRows"`   // Excel 新行数
	Action     string  `json:"action"`    // "new" / "update" / "delete" / "unchanged"
}

// DiffSummary 整次导入的预览汇总
type DiffSummary struct {
	Mode          string      `json:"mode"`
	Year          int         `json:"year"`
	Departments   []string    `json:"departments"`
	Entries       []DiffEntry `json:"entries"`
	Warnings      []string    `json:"warnings"`
	UnmappedCount int         `json:"unmappedCount"`
	NewRows       int         `json:"newRows"`      // 即将插入的总行数
	DeleteRows    int         `json:"deleteRows"`   // 即将删除的旧行数
	NetChange     int         `json:"netChange"`    // 净增减行数
}

// ComputeDiff 根据 result + mode 查库计算变更预览
func ComputeDiff(db *sql.DB, result *ParseResult) (*DiffSummary, error) {
	summary := &DiffSummary{
		Mode:          result.Mode,
		Year:          result.Year,
		Departments:   result.Departments,
		UnmappedCount: len(result.UnmappedSubjects),
		NewRows:       len(result.Rows),
	}

	// 1. 聚合 Excel 新数据：每个 (dept, month) 的 REV_TOTAL 金额 + 总行数
	type aggVal struct {
		rev  float64
		rows int
	}
	newByKey := make(map[string]*aggVal)
	for _, r := range result.Rows {
		key := fmt.Sprintf("%s|%d", r.Department, r.Month)
		v, ok := newByKey[key]
		if !ok {
			v = &aggVal{}
			newByKey[key] = v
		}
		v.rows++
		if r.SubjectCode == "REV_TOTAL" {
			v.rev = r.Amount
		}
	}

	// 2. 一次性查数据库当年所有 (dept, month) 的旧数据
	oldByKey := make(map[string]*aggVal)
	rows, err := db.Query(`
		SELECT department, month,
			SUM(CASE WHEN subject_code='REV_TOTAL' THEN amount ELSE 0 END) AS rev,
			COUNT(*) AS rows
		FROM finance_report
		WHERE year = ?
		GROUP BY department, month`, result.Year)
	if err != nil {
		return nil, fmt.Errorf("查询旧数据失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var d string
		var m int
		var rev sql.NullFloat64
		var rc int
		if err := rows.Scan(&d, &m, &rev, &rc); err != nil {
			return nil, err
		}
		oldByKey[fmt.Sprintf("%s|%d", d, m)] = &aggVal{rev: rev.Float64, rows: rc}
	}

	// 3. 决定要对比的 key 集合
	//   - incremental: 只对比 Excel 实际出现的 (dept, month)
	//   - full: 加上"被本次导入波及的部门" × 0..12 月（旧数据中有但 Excel 没有的 → 显示为 delete）
	keys := make(map[string]bool)
	for k := range newByKey {
		keys[k] = true
	}
	if result.Mode == ImportModeFull {
		for _, dept := range result.Departments {
			for m := 0; m <= 12; m++ {
				k := fmt.Sprintf("%s|%d", dept, m)
				if _, ok := oldByKey[k]; ok {
					keys[k] = true
				}
			}
		}
	}

	// 4. 生成 entries
	for k := range keys {
		parts := strings.SplitN(k, "|", 2)
		dept := parts[0]
		month, _ := strconv.Atoi(parts[1])
		oldV := oldByKey[k]
		newV := newByKey[k]

		var oldRev, newRev float64
		var oldRows, newRows int
		if oldV != nil {
			oldRev = oldV.rev
			oldRows = oldV.rows
		}
		if newV != nil {
			newRev = newV.rev
			newRows = newV.rows
		}

		delta := newRev - oldRev
		deltaPct := 0.0
		if oldRev != 0 {
			deltaPct = delta / oldRev * 100
		}

		action := "update"
		switch {
		case oldRows == 0 && newRows > 0:
			action = "new"
		case oldRows > 0 && newRows == 0:
			action = "delete"
		case oldRows > 0 && newRows > 0 && oldRev == newRev && oldRows == newRows:
			action = "unchanged"
		}

		summary.Entries = append(summary.Entries, DiffEntry{
			Department: dept,
			Month:      month,
			OldAmount:  oldRev,
			NewAmount:  newRev,
			Delta:      delta,
			DeltaPct:   deltaPct,
			OldRows:    oldRows,
			NewRows:    newRows,
			Action:     action,
		})

		// 异常告警：营收环比变化超阈值
		if action == "update" && oldRev > 0 && newRev > 0 {
			if deltaPct > 50 || deltaPct < -30 {
				summary.Warnings = append(summary.Warnings,
					fmt.Sprintf("[%s · %d月] 营业额变化 %.1f%%（旧 ¥%.0f → 新 ¥%.0f）", dept, month, deltaPct, oldRev, newRev))
			}
		}
	}

	// 5. 计算 DeleteRows + NetChange
	deletedKeys := make(map[string]bool)
	switch result.Mode {
	case ImportModeIncremental:
		for k := range newByKey {
			deletedKeys[k] = true
		}
	default: // full
		for _, dept := range result.Departments {
			for k := range oldByKey {
				if strings.HasPrefix(k, dept+"|") {
					deletedKeys[k] = true
				}
			}
		}
	}
	for k := range deletedKeys {
		if v, ok := oldByKey[k]; ok {
			summary.DeleteRows += v.rows
		}
	}
	summary.NetChange = summary.NewRows - summary.DeleteRows

	// 6. 排序：按部门 + 月份
	sort.Slice(summary.Entries, func(i, j int) bool {
		if summary.Entries[i].Department != summary.Entries[j].Department {
			return summary.Entries[i].Department < summary.Entries[j].Department
		}
		return summary.Entries[i].Month < summary.Entries[j].Month
	})

	// 7. 全局警告：未映射科目
	if summary.UnmappedCount > 0 {
		summary.Warnings = append(summary.Warnings,
			fmt.Sprintf("有 %d 项科目未映射，导入后这些行会被丢弃", summary.UnmappedCount))
	}

	return summary, nil
}

// LogImport 写入导入日志
func LogImport(db *sql.DB, fpath string, year int, result *ParseResult, userID int, status, errMsg string) error {
	md5Hex, size, _ := FileMD5(fpath)
	unmappedJSON, _ := json.Marshal(result.UnmappedSubjects)
	if len(result.UnmappedSubjects) == 0 {
		unmappedJSON = []byte("[]")
	}
	_, err := db.Exec(`INSERT INTO finance_import_log
		(year, filename, file_size, md5, sheet_count, row_count, unmapped_subjects, status, error_msg, user_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		year, filepath.Base(fpath), size, md5Hex,
		result.SheetCount, result.RowCount, string(unmappedJSON),
		status, errMsg, userID)
	return err
}
