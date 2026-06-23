# 业务报表 网页上传 Excel 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给财务页"业务报表"tab 的上传按钮做成真功能,支持网页自助上传 xlsx(全量/增量两模式 + 上传前 diff 预览),复刻财务报表已验证的两步流。

**Architecture:** 后端复用现成的 `business.ParseFile` 解析器;给 `business.WriteResult` 加全量/增量分支(用 `ParseResult` 字段而非改签名,CLI 向后兼容);新写 `business.ComputeDiff`(逐格比对、按渠道分组、明细截断)和 `ParseSnapshotFromFilename`;handler 仿 `finance_report_import.go` 写两步流(preview 解析+算diff+存token / confirm 凭token写库)。前端 `BusinessReport.tsx` 解绑 disabled 按钮,加两步 Modal。

**Tech Stack:** Go (net/http 标准库 + database/sql + excelize/v2 + go-sqlmock 测试) / React + TypeScript + antd / MySQL。

## Global Constraints

- Go 构建 CWD 必须是 `server/`:`cd server && go build -o bi-server.exe ./cmd/server`(repo root 会失败)。
- 改了 `import-business-report` 的 main.go 要重 build exe 拷到 `server/` 根(feedback_deploy_exe)。
- 改了 `.tsx` 必须 `npm run build`(serve -s build 静态部署,dev server 不算生产)。
- 数据库表/字段注释中文。`ON DUPLICATE KEY`/`REPLACE` 必须配 UNIQUE KEY(本表 UK=`uk_bbr`)。
- 真实 UK `uk_bbr` = (snapshot_year, snapshot_month, channel, sub_channel, parent_subject, subject, period_month)。diff row key 用 `channel|sub_channel|parent_subject|subject|period_month`。
- 一个 xlsx sheet = 一个 (channel, sub_channel)(`parseSheetName` 决定);经营指标/中后台 sheet 的 sub_channel="";增量删除粒度必须到 (channel, sub_channel)。
- 权限点新增 `finance.business_report:import`;接口用 `pageProtected` 保护。
- 业务红线(财务数据):上线前走 `/code-review` 二审;部署错峰 + 钉钉公告。

---

### Task 1: WriteResult 支持全量/增量 + 真实用户(business 包)

**Files:**
- Modify: `server/internal/business/parser.go`(加常量+字段+CollectChannelSubs+WriteResult mode 分支)
- Modify: `server/cmd/import-business-report/main.go`(显式设 ImportedBy,可选)
- Test: `server/internal/business/writeresult_mode_test.go`(新建)

**Interfaces:**
- Produces:
  - `const ImportModeFull = "full"` / `const ImportModeIncremental = "incremental"`
  - `ParseResult.Mode string`(""→按 full)/ `ParseResult.ImportedBy string`(""→"cli")
  - `func CollectChannelSubs(result *ParseResult) [][2]string`(返回去重的 [channel, subChannel] 组合)
  - `WriteResult(db *sql.DB, result *ParseResult) error`(签名不变,行为按 result.Mode 分支)

- [ ] **Step 1: 写失败测试 — 增量模式只删文件出现的 (channel, sub_channel)**

新建 `server/internal/business/writeresult_mode_test.go`:

```go
package business

import (
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestWriteResultIncrementalDeletesPerChannelSub(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	// 增量：逐 (channel, sub_channel) 精确删，本例文件只含 电商|TOC
	mock.ExpectExec(`DELETE FROM business_budget_report WHERE snapshot_year=\? AND snapshot_month=\? AND channel=\? AND sub_channel=\?`).
		WithArgs(2026, 4, "电商", "TOC").
		WillReturnResult(sqlmock.NewResult(0, 10))
	mock.ExpectExec(`INSERT INTO business_budget_report`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO business_budget_import_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err = WriteResult(db, &ParseResult{
		SnapshotYear: 2026, SnapshotMonth: 4, Year: 2026,
		Mode:        ImportModeIncremental,
		ImportedBy:  "tester",
		Rows: []BudgetRow{
			{SnapshotYear: 2026, SnapshotMonth: 4, Year: 2026, Channel: "电商", SubChannel: "TOC", Subject: "营业收入", PeriodMonth: 1},
		},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestWriteResultFullDeletesWholeSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM business_budget_report WHERE snapshot_year=\? AND snapshot_month=\?`).
		WithArgs(2026, 4).
		WillReturnResult(sqlmock.NewResult(0, 100))
	mock.ExpectExec(`INSERT INTO business_budget_report`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO business_budget_import_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err = WriteResult(db, &ParseResult{
		SnapshotYear: 2026, SnapshotMonth: 4, Year: 2026,
		Mode: ImportModeFull,
		Rows: []BudgetRow{
			{SnapshotYear: 2026, SnapshotMonth: 4, Year: 2026, Channel: "电商", SubChannel: "TOC", Subject: "营业收入", PeriodMonth: 1},
		},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

var _ = errors.New
var _ = regexp.MustCompile
```

- [ ] **Step 2: 运行测试,确认失败**

Run: `cd server && go test ./internal/business/ -run TestWriteResult -v`
Expected: 编译失败或 FAIL —— `ParseResult.Mode`/`ImportedBy` 未定义、增量 DELETE 未实现(现有 WriteResult 只会发整版 DELETE,增量用例的 per-channel-sub DELETE 不匹配)。

- [ ] **Step 3: 加常量 + ParseResult 字段**

在 `server/internal/business/parser.go` 的 `ParseResult` struct 末尾加两字段:

```go
type ParseResult struct {
	SnapshotYear   int
	SnapshotMonth  int
	Year           int
	SourceFile     string
	Channels       []string
	SheetsHandled  int
	SheetsSkipped  int
	Rows           []BudgetRow
	RowCount       int
	Mode           string // "full"(默认) / "incremental"
	ImportedBy     string // 导入人，默认 "cli"
}
```

在 ParseResult struct 定义之后加常量:

```go
// ImportModeFull 全量：整版快照删了重写（文件没有的渠道/子渠道也清掉），默认行为
const ImportModeFull = "full"

// ImportModeIncremental 增量：只删文件里出现的 (channel, sub_channel)，其他保留
const ImportModeIncremental = "incremental"

// CollectChannelSubs 收集本次结果出现的 (channel, sub_channel) 去重组合
func CollectChannelSubs(result *ParseResult) [][2]string {
	seen := map[string]bool{}
	out := make([][2]string, 0)
	for _, r := range result.Rows {
		k := r.Channel + "\x00" + r.SubChannel
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, [2]string{r.Channel, r.SubChannel})
	}
	return out
}
```

- [ ] **Step 4: 改 WriteResult 删除分支**

把 `server/internal/business/parser.go` 中 WriteResult 的"1. 删旧"段(现为单条整版 DELETE,约 line 573-579)替换为 mode 分支:

```go
	// 1. 删旧（按 mode）
	mode := result.Mode
	if mode == "" {
		mode = ImportModeFull
	}
	var deleted int64
	switch mode {
	case ImportModeIncremental:
		for _, cs := range CollectChannelSubs(result) {
			res, err := tx.Exec(
				`DELETE FROM business_budget_report WHERE snapshot_year=? AND snapshot_month=? AND channel=? AND sub_channel=?`,
				result.SnapshotYear, result.SnapshotMonth, cs[0], cs[1])
			if err != nil {
				return fmt.Errorf("delete (%s,%s): %w", cs[0], cs[1], err)
			}
			n, _ := res.RowsAffected()
			deleted += n
		}
	default: // ImportModeFull
		res, err := tx.Exec(
			`DELETE FROM business_budget_report WHERE snapshot_year=? AND snapshot_month=?`,
			result.SnapshotYear, result.SnapshotMonth)
		if err != nil {
			return fmt.Errorf("delete old snapshot: %w", err)
		}
		deleted, _ = res.RowsAffected()
	}
```

- [ ] **Step 5: 改 import_log 的 imported_by**

把 WriteResult 末尾写 log 那段(约 line 620-624)的硬编码 `"admin"` 改为 result.ImportedBy(空则 "cli"):

```go
	importedBy := result.ImportedBy
	if importedBy == "" {
		importedBy = "cli"
	}
	_, err = tx.Exec(`INSERT INTO business_budget_import_log
		(snapshot_year, snapshot_month, year, source_file, rows_inserted, rows_updated, rows_deleted, imported_by, status)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		result.SnapshotYear, result.SnapshotMonth, result.Year, result.SourceFile,
		totalInserted, 0, deleted, importedBy, "success")
```

- [ ] **Step 6: 运行测试,确认通过 + 全包回归**

Run: `cd server && go test ./internal/business/ -v`
Expected: PASS（新两例 + 现有 writeresult_test.go 全过；现有用例 Mode="" 走 full 分支,DELETE 整版,与旧行为一致）。

- [ ] **Step 7: CLI 显式标记来源(可选,锦上添花)**

在 `server/cmd/import-business-report/main.go` 调 WriteResult 前(:121 附近)设来源:

```go
	result.ImportedBy = "cli"
	if err := business.WriteResult(db, result); err != nil {
```

(注:不设也能编译运行,字段零值默认 full + "cli";此步只为日志区分来源。)

- [ ] **Step 8: 重建 CLI exe**

Run: `cd server && go build -o import-business-report.exe ./cmd/import-business-report`
Expected: 编译成功,exe 生成。

- [ ] **Step 9: Commit**

```bash
git add server/internal/business/parser.go server/internal/business/writeresult_mode_test.go server/cmd/import-business-report/main.go
git commit -m "feat(business-report): WriteResult 支持全量/增量 + 真实导入人(CLI向后兼容)"
```

---

### Task 2: ComputeDiff + DiffSummary(逐格比对、按渠道分组、明细截断)

**Files:**
- Modify: `server/internal/business/parser.go`(加 DiffSummary/DiffGroup/DiffCell 类型 + ComputeDiff)
- Test: `server/internal/business/computediff_test.go`(新建)

**Interfaces:**
- Consumes: `ParseResult`(Task 1)、`ImportModeFull/Incremental`(Task 1)
- Produces: `func ComputeDiff(db *sql.DB, result *ParseResult) (*DiffSummary, error)`,返回结构:
  - `DiffSummary{ Mode, SnapshotYear, SnapshotMonth string/int; IsNewSnapshot bool; TotalNew, TotalDeleted, TotalChanged int; Groups []DiffGroup }`
  - `DiffGroup{ Channel, SubChannel string; OldRows, NewRows, ChangedCells int; Action string; Cells []DiffCell; Truncated bool }`
  - `DiffCell{ ParentSubject, Subject string; PeriodMonth int; Field string; Old, New *float64 }`

- [ ] **Step 1: 写失败测试 — 覆盖已有快照时算出修改/新增**

新建 `server/internal/business/computediff_test.go`:

```go
package business

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestComputeDiffDetectsChangedCell(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// 库里现有该快照一行：电商|TOC|收入|营业收入|period 1，budget=100 actual=90
	rows := sqlmock.NewRows([]string{"channel", "sub_channel", "parent_subject", "subject", "period_month", "budget", "actual", "budget_year_start"}).
		AddRow("电商", "TOC", "收入", "营业收入", 1, 100.0, 90.0, nil)
	mock.ExpectQuery(`SELECT channel, sub_channel, parent_subject, subject, period_month, budget, actual, budget_year_start FROM business_budget_report WHERE snapshot_year=\? AND snapshot_month=\?`).
		WithArgs(2026, 4).
		WillReturnRows(rows)

	f110, f95 := 110.0, 95.0
	res := &ParseResult{
		SnapshotYear: 2026, SnapshotMonth: 4, Year: 2026, Mode: ImportModeFull,
		Rows: []BudgetRow{
			{Channel: "电商", SubChannel: "TOC", ParentSubject: "收入", Subject: "营业收入", PeriodMonth: 1, Budget: &f110, Actual: &f95},
		},
	}
	diff, err := ComputeDiff(db, res)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if diff.IsNewSnapshot {
		t.Error("快照已存在，IsNewSnapshot 应为 false")
	}
	if diff.TotalChanged != 1 {
		t.Errorf("应有 1 个变更格(budget 100→110, actual 90→95 算 1 行有变化 ChangedCells), got TotalChanged=%d", diff.TotalChanged)
	}
	if len(diff.Groups) != 1 || diff.Groups[0].Channel != "电商" || diff.Groups[0].SubChannel != "TOC" {
		t.Fatalf("应有 1 组 电商|TOC, got %+v", diff.Groups)
	}
}

func TestComputeDiffNewSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT channel, sub_channel, parent_subject, subject, period_month, budget, actual, budget_year_start FROM business_budget_report WHERE snapshot_year=\? AND snapshot_month=\?`).
		WithArgs(2026, 5).
		WillReturnRows(sqlmock.NewRows([]string{"channel", "sub_channel", "parent_subject", "subject", "period_month", "budget", "actual", "budget_year_start"}))

	f1 := 1.0
	res := &ParseResult{
		SnapshotYear: 2026, SnapshotMonth: 5, Year: 2026, Mode: ImportModeFull,
		Rows: []BudgetRow{{Channel: "电商", SubChannel: "TOC", ParentSubject: "收入", Subject: "营业收入", PeriodMonth: 1, Budget: &f1}},
	}
	diff, err := ComputeDiff(db, res)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !diff.IsNewSnapshot {
		t.Error("库里无此快照，IsNewSnapshot 应为 true")
	}
	if diff.TotalNew != 1 {
		t.Errorf("应有 1 个新增, got %d", diff.TotalNew)
	}
}
```

- [ ] **Step 2: 运行测试,确认失败**

Run: `cd server && go test ./internal/business/ -run TestComputeDiff -v`
Expected: 编译失败 —— `ComputeDiff`/`DiffSummary` 未定义。

- [ ] **Step 3: 实现 DiffSummary 类型 + ComputeDiff**

在 `server/internal/business/parser.go` 末尾追加:

```go
// DiffCell 单格变更
type DiffCell struct {
	ParentSubject string   `json:"parentSubject"`
	Subject       string   `json:"subject"`
	PeriodMonth   int      `json:"periodMonth"`
	Field         string   `json:"field"` // budget / actual / budget_year_start
	Old           *float64 `json:"old"`
	New           *float64 `json:"new"`
}

// DiffGroup 一个 (channel, sub_channel) 的变更汇总 + 截断明细
type DiffGroup struct {
	Channel      string     `json:"channel"`
	SubChannel   string     `json:"subChannel"`
	OldRows      int        `json:"oldRows"`
	NewRows      int        `json:"newRows"`
	ChangedCells int        `json:"changedCells"`
	Action       string     `json:"action"` // new/update/delete/unchanged
	Cells        []DiffCell `json:"cells"`
	Truncated    bool       `json:"truncated"`
}

// DiffSummary 整次导入预览
type DiffSummary struct {
	Mode          string      `json:"mode"`
	SnapshotYear  int         `json:"snapshotYear"`
	SnapshotMonth int         `json:"snapshotMonth"`
	IsNewSnapshot bool        `json:"isNewSnapshot"`
	TotalNew      int         `json:"totalNew"`
	TotalDeleted  int         `json:"totalDeleted"`
	TotalChanged  int         `json:"totalChanged"`
	Groups        []DiffGroup `json:"groups"`
}

const diffCellLimitPerGroup = 50

type bbrCell struct {
	budget, actual, yearStart *float64
}

func bbrRowKey(channel, subChannel, parent, subject string, period int) string {
	return channel + "\x00" + subChannel + "\x00" + parent + "\x00" + subject + "\x00" + strconv.Itoa(period)
}

func grpKey(channel, subChannel string) string { return channel + "\x00" + subChannel }

func fEq(a, b *float64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	d := *a - *b
	return d < 0.005 && d > -0.005
}

// ComputeDiff 查库比对，逐格算变更，按 (channel, sub_channel) 分组，每组明细截断到 diffCellLimitPerGroup
func ComputeDiff(db *sql.DB, result *ParseResult) (*DiffSummary, error) {
	summary := &DiffSummary{
		Mode:          result.Mode,
		SnapshotYear:  result.SnapshotYear,
		SnapshotMonth: result.SnapshotMonth,
	}

	// 1. 查库现有该快照
	oldRows := map[string]bbrCell{}        // rowKey -> 三值
	oldGrpCount := map[string]int{}        // grpKey -> 行数
	q, err := db.Query(`SELECT channel, sub_channel, parent_subject, subject, period_month, budget, actual, budget_year_start
		FROM business_budget_report WHERE snapshot_year=? AND snapshot_month=?`, result.SnapshotYear, result.SnapshotMonth)
	if err != nil {
		return nil, fmt.Errorf("查旧快照失败: %w", err)
	}
	defer q.Close()
	for q.Next() {
		var ch, sc, ps, sj string
		var pm int
		var bd, ac, ys *float64
		if err := q.Scan(&ch, &sc, &ps, &sj, &pm, &bd, &ac, &ys); err != nil {
			return nil, err
		}
		oldRows[bbrRowKey(ch, sc, ps, sj, pm)] = bbrCell{bd, ac, ys}
		oldGrpCount[grpKey(ch, sc)]++
	}
	if err := q.Err(); err != nil {
		return nil, err
	}
	summary.IsNewSnapshot = len(oldRows) == 0

	// 2. 新数据按 grp 聚合
	newGrpCount := map[string]int{}
	grpOrder := []string{}
	grpSeen := map[string]bool{}
	type cs struct{ channel, subChannel string }
	grpMeta := map[string]cs{}
	groups := map[string]*DiffGroup{}
	getGroup := func(ch, sc string) *DiffGroup {
		k := grpKey(ch, sc)
		if !grpSeen[k] {
			grpSeen[k] = true
			grpOrder = append(grpOrder, k)
			grpMeta[k] = cs{ch, sc}
			groups[k] = &DiffGroup{Channel: ch, SubChannel: sc}
		}
		return groups[k]
	}

	newKeys := map[string]bool{}
	for i := range result.Rows {
		r := &result.Rows[i]
		gk := grpKey(r.Channel, r.SubChannel)
		newGrpCount[gk]++
		g := getGroup(r.Channel, r.SubChannel)
		rk := bbrRowKey(r.Channel, r.SubChannel, r.ParentSubject, r.Subject, r.PeriodMonth)
		newKeys[rk] = true
		old, existed := oldRows[rk]
		// 逐字段比对 budget/actual/budget_year_start
		changed := false
		if !existed {
			summary.TotalNew++
			changed = true
			appendCell(g, r.ParentSubject, r.Subject, r.PeriodMonth, "budget", nil, r.Budget)
		} else {
			for _, fc := range []struct {
				field    string
				oldV, nV *float64
			}{
				{"budget", old.budget, r.Budget},
				{"actual", old.actual, r.Actual},
				{"budget_year_start", old.yearStart, r.BudgetYearStart},
			} {
				if !fEq(fc.oldV, fc.nV) {
					changed = true
					appendCell(g, r.ParentSubject, r.Subject, r.PeriodMonth, fc.field, fc.oldV, fc.nV)
				}
			}
			if changed {
				summary.TotalChanged++
			}
		}
		if changed {
			g.ChangedCells++
		}
	}

	// 3. 删除判定（full=全部旧；incremental=只本次出现的 grp 的旧）
	incremental := result.Mode == ImportModeIncremental
	for rk := range oldRows {
		if newKeys[rk] {
			continue
		}
		// rk 拆出 channel|sub_channel 判断该 grp 本次是否出现
		// incremental 模式下，未出现的 grp 旧行保留，不算删
		// （rk 前两段即 channel,subChannel；用前缀匹配 newGrpCount）
		// 简化：full 全算删；incremental 只有 newGrpCount 有的 grp 才算删
		// 为避免再解析 rk，重新查 grp：用 oldGrpAffected 集合
		_ = incremental
	}
	summary.TotalDeleted = computeDeleted(oldRows, newKeys, newGrpCount, incremental)

	// 4. 组装 group 行数 + action + 截断标记
	for _, k := range grpOrder {
		g := groups[k]
		g.OldRows = oldGrpCount[k]
		g.NewRows = newGrpCount[k]
		switch {
		case g.OldRows == 0:
			g.Action = "new"
		case g.ChangedCells > 0:
			g.Action = "update"
		default:
			g.Action = "unchanged"
		}
		summary.Groups = append(summary.Groups, *g)
	}
	return summary, nil
}

func appendCell(g *DiffGroup, parent, subject string, pm int, field string, old, nv *float64) {
	if len(g.Cells) >= diffCellLimitPerGroup {
		g.Truncated = true
		return
	}
	g.Cells = append(g.Cells, DiffCell{ParentSubject: parent, Subject: subject, PeriodMonth: pm, Field: field, Old: old, New: nv})
}

// computeDeleted 统计将被删除的旧行数
//   full: 所有 old 里 new 没有的 key
//   incremental: 只统计本次出现的 (channel,sub_channel) 组里、old 有 new 没有的 key
func computeDeleted(oldRows map[string]bbrCell, newKeys map[string]bool, newGrpCount map[string]int, incremental bool) int {
	n := 0
	for rk := range oldRows {
		if newKeys[rk] {
			continue
		}
		if incremental {
			// rk = channel\x00subChannel\x00parent\x00subject\x00period；取前两段拼 grpKey
			parts := splitN2(rk)
			if newGrpCount[parts] == 0 {
				continue // 该 grp 本次没出现，旧行保留，不算删
			}
		}
		n++
	}
	return n
}

// splitN2 取 rk 的前两段(channel\x00subChannel)拼成 grpKey
func splitN2(rk string) string {
	first := strings.IndexByte(rk, '\x00')
	if first < 0 {
		return rk
	}
	second := strings.IndexByte(rk[first+1:], '\x00')
	if second < 0 {
		return rk
	}
	return rk[:first+1+second]
}
```

- [ ] **Step 4: 运行测试,确认通过**

Run: `cd server && go test ./internal/business/ -run TestComputeDiff -v`
Expected: PASS（两例）。

- [ ] **Step 5: 全包回归**

Run: `cd server && go test ./internal/business/ -v && cd server && go vet ./internal/business/`
Expected: 全 PASS + vet 干净。

- [ ] **Step 6: Commit**

```bash
git add server/internal/business/parser.go server/internal/business/computediff_test.go
git commit -m "feat(business-report): ComputeDiff 逐格比对+按渠道分组+明细截断"
```

---

### Task 3: ParseSnapshotFromFilename(文件名抠年月)

**Files:**
- Modify: `server/internal/business/parser.go`
- Test: `server/internal/business/snapshot_filename_test.go`(新建)

**Interfaces:**
- Produces: `func ParseSnapshotFromFilename(name string) (year, month int)`(抠不出返回 0,0)

- [ ] **Step 1: 写失败测试**

新建 `server/internal/business/snapshot_filename_test.go`:

```go
package business

import "testing"

func TestParseSnapshotFromFilename(t *testing.T) {
	cases := []struct {
		name             string
		wantY, wantM int
	}{
		{"2026年04月业务预决算报表.xlsx", 2026, 4},
		{"2026年4月业务预决算报表.xlsx", 2026, 4},
		{"2025年12月业务预决算报表.xlsx", 2025, 12},
		{"/tmp/upload-123-2026年04月业务预决算报表.xlsx", 2026, 4},
		{"2026年业务预决算报表.xlsx", 0, 0}, // 无月份 → 0,0
		{"乱七八糟.xlsx", 0, 0},
	}
	for _, c := range cases {
		y, m := ParseSnapshotFromFilename(c.name)
		if y != c.wantY || m != c.wantM {
			t.Errorf("%s: got (%d,%d) want (%d,%d)", c.name, y, m, c.wantY, c.wantM)
		}
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `cd server && go test ./internal/business/ -run TestParseSnapshotFromFilename -v`
Expected: 编译失败 —— 函数未定义。

- [ ] **Step 3: 实现**

在 `server/internal/business/parser.go` 末尾加(包已 import regexp/strconv):

```go
var reSnapshotYM = regexp.MustCompile(`(\d{4})\s*年\s*(\d{1,2})\s*月`)

// ParseSnapshotFromFilename 从文件名抠快照年月，如 "2026年04月业务预决算报表.xlsx" → 2026, 4；抠不出返回 0,0
func ParseSnapshotFromFilename(name string) (year, month int) {
	m := reSnapshotYM.FindStringSubmatch(name)
	if m == nil {
		return 0, 0
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	if mo < 1 || mo > 12 {
		return 0, 0
	}
	return y, mo
}
```

- [ ] **Step 4: 运行确认通过**

Run: `cd server && go test ./internal/business/ -run TestParseSnapshotFromFilename -v`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add server/internal/business/parser.go server/internal/business/snapshot_filename_test.go
git commit -m "feat(business-report): ParseSnapshotFromFilename 文件名抠快照年月"
```

---

### Task 4: handler 两步流(preview + confirm)

**Files:**
- Create: `server/internal/handler/business_report_import.go`
- Test: `server/internal/handler/business_report_import_test.go`
- Reference(只读参照,勿改): `server/internal/handler/finance_report_import.go`

**Interfaces:**
- Consumes: `business.ParseFile` / `business.WriteResult` / `business.ComputeDiff` / `business.ParseSnapshotFromFilename` / `business.ImportModeFull/Incremental`(Task 1-3)
- Produces:
  - `func (h *DashboardHandler) ImportBusinessReportPreview(w, r)` — POST 表单 file/mode/可选 snapshotMonth
  - `func (h *DashboardHandler) ImportBusinessReportConfirm(w, r)` — POST JSON {token}

- [ ] **Step 1: 写失败测试 — 非 xlsx 拒绝 + 缺月份拒绝**

新建 `server/internal/handler/business_report_import_test.go`:

```go
package handler

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func multipartBody(t *testing.T, filename, mode string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("file", filename)
	fw.Write([]byte("dummy"))
	w.WriteField("mode", mode)
	w.Close()
	return body, w.FormDataContentType()
}

func TestImportBusinessReportPreviewRejectsNonXlsx(t *testing.T) {
	h := &DashboardHandler{}
	body, ct := multipartBody(t, "2026年04月业务预决算报表.csv", "full")
	req := httptest.NewRequest(http.MethodPost, "/api/finance/business-report/import/preview", body)
	req.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()
	h.ImportBusinessReportPreview(rr, req)
	if rr.Code != 400 {
		t.Errorf("非 xlsx 应 400, got %d", rr.Code)
	}
}

func TestImportBusinessReportPreviewRejectsNoMonth(t *testing.T) {
	h := &DashboardHandler{}
	body, ct := multipartBody(t, "2026年业务预决算报表.xlsx", "full") // 文件名无月份且不传 snapshotMonth
	req := httptest.NewRequest(http.MethodPost, "/api/finance/business-report/import/preview", body)
	req.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()
	h.ImportBusinessReportPreview(rr, req)
	if rr.Code != 400 {
		t.Errorf("无快照月份应 400, got %d", rr.Code)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `cd server && go test ./internal/handler/ -run TestImportBusinessReport -v`
Expected: 编译失败 —— handler 方法未定义。

- [ ] **Step 3: 实现 handler(仿 finance_report_import.go)**

新建 `server/internal/handler/business_report_import.go`:

```go
package handler

import (
	"crypto/rand"
	"encoding/hex"
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

	"bi-dashboard/internal/business"
)

var businessImportMu sync.Mutex

const businessPreviewTTL = 30 * time.Minute

func businessPreviewDir() string {
	d := filepath.Join(os.TempDir(), "bi-business-import")
	os.MkdirAll(d, 0755)
	return d
}

func newBusinessToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

type businessPreviewPayload struct {
	SnapshotYear  int                  `json:"snapshotYear"`
	SnapshotMonth int                  `json:"snapshotMonth"`
	Mode          string               `json:"mode"`
	Filename      string               `json:"filename"`
	UserID        int                  `json:"userID"`
	UploadedAt    time.Time            `json:"uploadedAt"`
	Result        *business.ParseResult `json:"result"`
}

// ImportBusinessReportPreview 第一步：上传+解析+算diff+存token
// POST /api/finance/business-report/import/preview  表单: file, mode(full|incremental), 可选 snapshotMonth
func (h *DashboardHandler) ImportBusinessReportPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
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
	if strings.ToLower(filepath.Ext(header.Filename)) != ".xlsx" {
		writeError(w, 400, "仅支持 .xlsx 格式")
		return
	}

	mode := strings.TrimSpace(r.FormValue("mode"))
	if mode == "" {
		mode = business.ImportModeFull
	}
	if mode != business.ImportModeFull && mode != business.ImportModeIncremental {
		writeError(w, 400, "mode 必须是 full 或 incremental")
		return
	}

	year, month := business.ParseSnapshotFromFilename(header.Filename)
	if mv := strings.TrimSpace(r.FormValue("snapshotMonth")); mv != "" {
		if m, e := strconv.Atoi(mv); e == nil && m >= 1 && m <= 12 {
			month = m
			if year == 0 {
				if ym := business.ParseSnapshotFromFilename(header.Filename); ym != 0 {
					year = ym
				}
			}
		}
	}
	// 年份兜底：文件名只有年的情况
	if year == 0 {
		if yv := reYearOnly.FindStringSubmatch(header.Filename); yv != nil {
			year, _ = strconv.Atoi(yv[1])
		}
	}
	if year < 2020 || year > 2050 || month < 1 || month > 12 {
		writeError(w, 400, "无法确定快照年月,请用「YYYY年MM月业务预决算报表.xlsx」命名或手选月份")
		return
	}

	tmpPath := filepath.Join(businessPreviewDir(), fmt.Sprintf("upload-%d-%s", time.Now().UnixMilli(), filepath.Base(header.Filename)))
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

	result, err := business.ParseFile(tmpPath, year, month, year)
	if err != nil {
		writeServerError(w, 500, "Excel 解析失败", err)
		return
	}
	result.Mode = mode
	result.SourceFile = header.Filename // 存原名,不是临时路径

	diff, err := business.ComputeDiff(h.DB, result)
	if err != nil {
		writeServerError(w, 500, "计算变更预览失败", err)
		return
	}

	userID := 0
	if payload, ok := authPayloadFromContext(r); ok && payload != nil {
		userID = int(payload.User.ID)
	}

	token := newBusinessToken()
	payload := &businessPreviewPayload{
		SnapshotYear: year, SnapshotMonth: month, Mode: mode,
		Filename: header.Filename, UserID: userID, UploadedAt: time.Now(), Result: result,
	}
	cacheBytes, _ := json.Marshal(payload)
	if err := os.WriteFile(filepath.Join(businessPreviewDir(), token+".json"), cacheBytes, 0600); err != nil {
		writeServerError(w, 500, "缓存预览失败", err)
		return
	}
	go cleanupExpiredBusinessPreviews()

	writeJSON(w, map[string]interface{}{
		"token":         token,
		"snapshotYear":  year,
		"snapshotMonth": month,
		"mode":          mode,
		"filename":      header.Filename,
		"rowCount":      result.RowCount,
		"diff":          diff,
		"expiresAt":     time.Now().Add(businessPreviewTTL).Format(time.RFC3339),
	})
}

// ImportBusinessReportConfirm 第二步：凭 token 写库
// POST /api/finance/business-report/import/confirm  JSON {token}
func (h *DashboardHandler) ImportBusinessReportConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	if !businessImportMu.TryLock() {
		writeError(w, 409, "有其他导入任务进行中，请稍后再试")
		return
	}
	defer businessImportMu.Unlock()

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		writeError(w, 400, "缺少 token")
		return
	}
	for _, c := range req.Token {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			writeError(w, 400, "token 格式非法")
			return
		}
	}
	if len(req.Token) != 32 {
		writeError(w, 400, "token 长度非法")
		return
	}

	cachePath := filepath.Join(businessPreviewDir(), req.Token+".json")
	cacheBytes, err := os.ReadFile(cachePath)
	if err != nil {
		writeError(w, 404, "预览已过期或不存在，请重新上传")
		return
	}
	var payload businessPreviewPayload
	if err := json.Unmarshal(cacheBytes, &payload); err != nil {
		writeServerError(w, 500, "缓存损坏", err)
		return
	}
	if time.Since(payload.UploadedAt) > businessPreviewTTL {
		os.Remove(cachePath)
		writeError(w, 410, "预览已过期（30分钟），请重新上传")
		return
	}
	if payload.Result == nil {
		writeError(w, 500, "缓存内容异常")
		return
	}
	payload.Result.Mode = payload.Mode
	if uname := businessUsername(h, payload.UserID); uname != "" {
		payload.Result.ImportedBy = uname
	}

	if err := business.WriteResult(h.DB, payload.Result); err != nil {
		writeServerError(w, 500, "入库失败", err)
		return
	}
	os.Remove(cachePath)

	writeJSON(w, map[string]interface{}{
		"snapshotYear":  payload.SnapshotYear,
		"snapshotMonth": payload.SnapshotMonth,
		"mode":          payload.Mode,
		"rowCount":      payload.Result.RowCount,
	})
}

func cleanupExpiredBusinessPreviews() {
	entries, err := os.ReadDir(businessPreviewDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if time.Since(info.ModTime()) > businessPreviewTTL {
			os.Remove(filepath.Join(businessPreviewDir(), e.Name()))
		}
	}
}

// businessUsername 取用户名作 imported_by（查不到返回空，WriteResult 会回落 "cli"）
func businessUsername(h *DashboardHandler, userID int) string {
	if userID == 0 {
		return ""
	}
	var name string
	if err := h.DB.QueryRow(`SELECT username FROM users WHERE id=?`, userID).Scan(&name); err != nil {
		return ""
	}
	return name
}
```

- [ ] **Step 4: 加文件名"只有年"的兜底正则**

在 `business_report_import.go` 顶部 import 后加(用于年份兜底):

```go
var reYearOnly = regexpMustCompileYear()
```

并在文件末尾加(避免与 business 包正则重复,handler 包内自带):

```go
func regexpMustCompileYear() *regexp.Regexp { return regexp.MustCompile(`(\d{4})\s*年`) }
```

记得在 import 块加 `"regexp"`。

- [ ] **Step 5: 运行测试,确认通过**

Run: `cd server && go test ./internal/handler/ -run TestImportBusinessReport -v`
Expected: PASS（两例:非xlsx 400 / 无月份 400）。

> 注:happy-path（真解析+写库）依赖真实 xlsx + DB,放到 Task 6 后的手测(本步只覆盖入参校验分支,符合 handler 现有测试惯例)。

- [ ] **Step 6: 全 handler 包回归 + vet**

Run: `cd server && go test ./internal/handler/ 2>&1 | tail -5 && cd server && go vet ./internal/handler/`
Expected: 全 PASS + vet 干净。

- [ ] **Step 7: Commit**

```bash
git add server/internal/handler/business_report_import.go server/internal/handler/business_report_import_test.go
git commit -m "feat(business-report): 上传两步流 handler(preview+confirm)"
```

---

### Task 5: 注册路由 + 权限点

**Files:**
- Modify: `server/cmd/server/main.go`(2 条路由)
- Modify: `server/internal/handler/auth_seed.go`(权限点)
- Reference: `server/cmd/server/main.go:614-615`(财务报表导入路由)/ `auth_seed.go:72-73`(财务权限点)

**Interfaces:**
- Consumes: `h.ImportBusinessReportPreview` / `h.ImportBusinessReportConfirm`(Task 4)

- [ ] **Step 1: 加权限点**

在 `server/internal/handler/auth_seed.go` 找到 `finance.report:import` 那行(:73),其后加:

```go
		{Code: "finance.business_report:import", Name: "财务-业务报表导入", Type: "action"},
```

- [ ] **Step 2: 注册路由**

在 `server/cmd/server/main.go` 找到 business-report 现有两条 GET 路由(:626-627),其后加:

```go
	mux.HandleFunc("/api/finance/business-report/import/preview", pageProtected("finance.business_report:import", h.ImportBusinessReportPreview))
	mux.HandleFunc("/api/finance/business-report/import/confirm", pageProtected("finance.business_report:import", h.ImportBusinessReportConfirm))
```

- [ ] **Step 3: 编译 + 启动自检**

Run: `cd server && go build -o bi-server.exe ./cmd/server`
Expected: 编译成功。

- [ ] **Step 4: Commit**

```bash
git add server/cmd/server/main.go server/internal/handler/auth_seed.go
git commit -m "feat(business-report): 注册上传两接口 + finance.business_report:import 权限点"
```

---

### Task 6: 前端 BusinessReport.tsx 上传 Modal

**Files:**
- Modify: `src/pages/finance/BusinessReport.tsx`
- Reference: `src/pages/finance/Report.tsx:199-318`(财务两步 Modal 模式)

**Interfaces:**
- Consumes: 后端 `/api/finance/business-report/import/preview` 和 `/import/confirm`(Task 4-5)

- [ ] **Step 1: 解绑 disabled 按钮 + 加权限 + state**

在 `BusinessReport.tsx` 组件顶部(useAuth 之后)加权限与导入 state(参照 Report.tsx:157, 200-210):

```tsx
const canImport = !!session && (session.isSuperAdmin || session.permissions.includes('finance.business_report:import'));

const [importModal, setImportModal] = useState<{
  open: boolean; step: 1 | 2; mode: 'full' | 'incremental';
  file: File | null; snapshotMonth: number | null; preview: any | null; loading: boolean;
}>({ open: false, step: 1, mode: 'full', file: null, snapshotMonth: null, preview: null, loading: false });

const closeImportModal = () => setImportModal({ open: false, step: 1, mode: 'full', file: null, snapshotMonth: null, preview: null, loading: false });
const openImportModal = () => setImportModal({ open: true, step: 1, mode: 'full', file: null, snapshotMonth: null, preview: null, loading: false });
```

找到现有 `<Button icon={<UploadOutlined />} disabled>上传 Excel</Button>`(BusinessReport.tsx:138-140),替换为:

```tsx
{canImport && (
  <Button type="primary" icon={<UploadOutlined />} onClick={openImportModal}>上传 Excel</Button>
)}
```

- [ ] **Step 2: 加 uploadProps + doPreview + doConfirm**

在组件内加(参照 Report.tsx:212-280;文件名抠月份失败时靠 snapshotMonth 下拉兜底):

```tsx
const uploadProps: UploadProps = {
  name: 'file', accept: '.xlsx', maxCount: 1, showUploadList: false,
  beforeUpload: (file) => { setImportModal((s) => ({ ...s, file })); return Upload.LIST_IGNORE; },
};

const doPreview = async () => {
  if (!importModal.file) { message.error('请选择文件'); return; }
  setImportModal((s) => ({ ...s, loading: true }));
  const form = new FormData();
  form.append('file', importModal.file);
  form.append('mode', importModal.mode);
  if (importModal.snapshotMonth) form.append('snapshotMonth', String(importModal.snapshotMonth));
  try {
    const res = await fetch(`${API_BASE}/api/finance/business-report/import/preview`, { method: 'POST', credentials: 'include', body: form });
    const json = await res.json();
    if (json.code !== 200) { message.error(json.msg || '预览失败'); setImportModal((s) => ({ ...s, loading: false })); return; }
    setImportModal((s) => ({ ...s, step: 2, preview: json.data, loading: false }));
  } catch (e: any) { message.error('预览失败：' + e.message); setImportModal((s) => ({ ...s, loading: false })); }
};

const doConfirm = async () => {
  if (!importModal.preview?.token) return;
  setImportModal((s) => ({ ...s, loading: true }));
  try {
    const res = await fetch(`${API_BASE}/api/finance/business-report/import/confirm`, {
      method: 'POST', credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token: importModal.preview.token }),
    });
    const json = await res.json();
    if (json.code !== 200) { message.error(json.msg || '导入失败'); setImportModal((s) => ({ ...s, loading: false })); return; }
    message.success(`导入成功：${json.data.snapshotYear}年${json.data.snapshotMonth}月 共 ${json.data.rowCount} 条（${json.data.mode === 'incremental' ? '增量' : '全量覆盖'}）`);
    closeImportModal();
    // 触发页面重新拉数(调用本页已有的 fetch 函数,名称按实际)
    window.location.reload();
  } catch (e: any) { message.error('导入失败：' + e.message); setImportModal((s) => ({ ...s, loading: false })); }
};
```

> 注:`window.location.reload()` 是保底刷新;若本页已有数据加载函数(如 `fetchData`),改调它更平滑——实现时按 BusinessReport.tsx 实际的加载函数名替换。

- [ ] **Step 3: 加导入 Modal JSX**

在组件 return 的根节点内(表格之后)加 Modal(第1步选模式+文件+月份兜底,第2步展示 diff 摘要+分组):

```tsx
<Modal
  title="上传业务报表 Excel"
  open={importModal.open}
  onCancel={closeImportModal}
  footer={importModal.step === 1
    ? [<Button key="c" onClick={closeImportModal}>取消</Button>,
       <Button key="p" type="primary" loading={importModal.loading} onClick={doPreview}>预览</Button>]
    : [<Button key="b" onClick={() => setImportModal((s) => ({ ...s, step: 1, preview: null }))}>上一步</Button>,
       <Button key="ok" type="primary" loading={importModal.loading} onClick={doConfirm}>确认导入</Button>]}
  width={720}
>
  {importModal.step === 1 ? (
    <Space direction="vertical" style={{ width: '100%' }}>
      <div>导入模式：
        <Radio.Group value={importModal.mode} onChange={(e) => setImportModal((s) => ({ ...s, mode: e.target.value }))}>
          <Radio value="full">全量(整版覆盖)</Radio>
          <Radio value="incremental">增量(只更新文件内的子渠道)</Radio>
        </Radio.Group>
      </div>
      <div>快照月份(文件名带「YYYY年MM月」可不填,否则手选)：
        <Select allowClear placeholder="自动从文件名识别" style={{ width: 160 }}
          value={importModal.snapshotMonth ?? undefined}
          onChange={(v) => setImportModal((s) => ({ ...s, snapshotMonth: v ?? null }))}
          options={Array.from({ length: 12 }, (_, i) => ({ label: `${i + 1}月`, value: i + 1 }))} />
      </div>
      <Upload {...uploadProps}><Button icon={<UploadOutlined />}>选择 xlsx 文件</Button></Upload>
      {importModal.file && <div>已选：{importModal.file.name}</div>}
    </Space>
  ) : (
    <Space direction="vertical" style={{ width: '100%' }}>
      <div>
        {importModal.preview?.diff?.isNewSnapshot ? '🆕 新增快照' : '⚠️ 覆盖已有快照'}
        {' '}{importModal.preview?.snapshotYear}年{importModal.preview?.snapshotMonth}月,
        共 {importModal.preview?.rowCount} 行;
        新增 {importModal.preview?.diff?.totalNew}、修改 {importModal.preview?.diff?.totalChanged}、删除 {importModal.preview?.diff?.totalDeleted}
      </div>
      <Table
        size="small"
        rowKey={(g: any) => `${g.channel}|${g.subChannel}`}
        dataSource={importModal.preview?.diff?.groups || []}
        pagination={false}
        columns={[
          { title: '渠道', dataIndex: 'channel' },
          { title: '子渠道', dataIndex: 'subChannel', render: (v: string) => v || '—' },
          { title: '动作', dataIndex: 'action' },
          { title: '旧行', dataIndex: 'oldRows' },
          { title: '新行', dataIndex: 'newRows' },
          { title: '变更格', dataIndex: 'changedCells' },
        ]}
        expandable={{
          expandedRowRender: (g: any) => (
            <div style={{ maxHeight: 200, overflow: 'auto' }}>
              {(g.cells || []).map((c: any, i: number) => (
                <div key={i}>{c.parentSubject}/{c.subject} {c.periodMonth}月 {c.field}: {c.old ?? '无'} → {c.new ?? '无'}</div>
              ))}
              {g.truncated && <div>…(明细已截断,仅显示前 50 条)</div>}
            </div>
          ),
        }}
      />
    </Space>
  )}
</Modal>
```

确保文件顶部 import 含:`Modal, Button, Upload, Radio, Select, Space, Table, message, UploadProps`(antd)和 `UploadOutlined`(@ant-design/icons)——缺哪个补哪个。

- [ ] **Step 4: 编译检查**

Run: `npx tsc --noEmit -p tsconfig.json`
Expected: 0 报错(若报缺 import,补齐对应 antd 组件)。

- [ ] **Step 5: 前端 build**

Run: `npm run build`
Expected: build 成功(CRA;`CI=true` 下 eslint warning 会当 error,清理未用 import)。

- [ ] **Step 6: Commit**

```bash
git add src/pages/finance/BusinessReport.tsx
git commit -m "feat(business-report): 前端上传 Excel 两步 Modal(全量/增量+diff预览)"
```

---

### Task 7: 端到端手测 + 二审(交付前必做)

**Files:** 无代码改动,验收。

- [ ] **Step 1: 部署到本地实测环境**

```bash
cd server && go build -o bi-server.exe ./cmd/server
# 重启:只 kill 8080 PID(见 CLAUDE.md 重启法,清代理 env)
```
前端已 build。

- [ ] **Step 2: 真实 xlsx 手测(playwright 或浏览器)**

用一份真实「2026年MM月业务预决算报表.xlsx」:
1. 全量上传一个**新月份快照** → 预览显示"🆕 新增快照" → 确认 → 查库 `SELECT COUNT(*) FROM business_budget_report WHERE snapshot_year=2026 AND snapshot_month=MM` 行数对、展示页能切到该快照。
2. 对**已有快照**全量重传 → 预览显示"⚠️ 覆盖" + diff 分组 → 确认 → 行数与预览一致。
3. **增量**只传一个 (渠道,子渠道) 的文件 → 确认 → SQL 核对:**同渠道其他子渠道 + 其他渠道数据均未变**(这是问题2回归点)。
4. 特殊 sheet(经营指标/中后台)随真实文件一起导 → 不报错、不被误删。
5. 无 `finance.business_report:import` 权限的账号 → 看不到/点不动上传按钮。

记录每步 SQL 输出/截图作为证据(feedback_test_and_verify:build 通过≠完成)。

- [ ] **Step 3: /code-review 二审(财务红线)**

业务报表属财务数据 + 改了写库逻辑 → 跑 `/code-review` 二审,重点看:增量删除粒度、diff 比对 key 与 UK 对齐、token/锁/TTL、并发安全。过了再上线。

- [ ] **Step 4: 上线(等跑哥明确"上线")**

错峰 + 钉钉公告 + 旧 exe 备份 `bi-server.old.exe` 可回滚(见 feedback_deploy_needs_explicit_approval:上线须跑哥明说)。

---

## 自检(写完计划回看 spec)

- **Spec 覆盖**:文件名抠月+兜底(T3+T4)/ 全量增量(T1)/ 逐格diff摘要分组截断(T2+T6)/ 独立权限(T5)/ 两步流token锁TTL(T4)/ CLI兼容(T1)/ 特殊sheet(T1增量按 (channel,sub_channel) 含 sub_channel=""+T7手测)/ 前端字段重写(T6)—— 均有任务覆盖。
- **占位符**:无 TBD;前端"调本页加载函数"已注明按实际函数名替换(给了 reload 保底)。
- **类型一致**:`ImportModeFull/Incremental`、`ParseResult.Mode/ImportedBy`、`CollectChannelSubs`、`ComputeDiff`/`DiffSummary`/`DiffGroup`/`DiffCell` 在 T1/T2 定义,T4/T6 一致引用。
- **风险点**:`ComputeDiff` 的 `splitN2`/`computeDeleted` 在 incremental 删除判定用 rk 前缀,实现时确认 `\x00` 分隔一致(T2 测试已含 full;建议实现时补一个 incremental 删除判定的单测)。
