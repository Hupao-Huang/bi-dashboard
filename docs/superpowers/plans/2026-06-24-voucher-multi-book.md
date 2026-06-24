# 凭证查询 多账簿合并查询 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 凭证查询页账簿从单选改多选,后台逐账簿查、合并成一张带「账簿」列的表;默认每本只拉前 20 条(快),提供「拉全」按钮拉完整。

**Architecture:** 用友 `queryVouchers` 接口一次只认一本账且全局限流 1.1s/次,所以多账簿在 handler 层 fan-out(逐账簿调用)再合并。核心合并逻辑抽成纯函数 `fanOutVouchers`(注入 fetch 函数),不依赖网络/DB,可单测。前端从服务端翻页改本地翻页。

**Tech Stack:** Go (net/http 标准库) + React + AntD;后端 `server/internal/handler/ys_voucher.go`,前端 `src/pages/finance/VoucherQuery.tsx`;复用 `internal/yonsuite` 的 `QueryVoucherList`/`QueryAccbookList`(不改)。

## Global Constraints

- 设计文档来源(口径唯一来源):`docs/superpowers/specs/2026-06-24-voucher-multi-book-design.md`。
- 用友 client 限流 1.1s/次且与所有 YS 同步**共用同一个 `*Client`**;`fanOutVouchers` 必须最小化调用次数,「拉全」必须有兜底闸。
- 默认参数(spec §7):`voucherPerBookLimit=20` / `voucherMaxCalls=30` / `voucherMaxRows=6000` / `voucherFullPageSize=200`。
- 19+ 位 long id 用 string 透传,不做数值化(`voucher.go` 已 `UseNumber()` 解析,handler 端 `ysMapStr` 取 string)。
- 权限不变:仍仅超管(路由不动)。
- 仓库约定:commit 用 `<type>(<scope>): 主题 — 详情`(不带版本号);改 `.tsx` 必须 `npm run build`;改 handler 必须 `cd server && go build`。
- **本计划范围 = 代码 + 测试 + 本地 build 通过为止。部署(build exe / 重启 bi-server / 发版 notice)需跑哥另行明说,不在本计划内**(memory `feedback_deploy_needs_explicit_approval`)。push 由跑哥控制。

---

## File Structure

| 文件 | 责任 | 改动 |
|------|------|------|
| `server/internal/handler/ys_voucher.go` | 凭证查询 handler + 账簿清单 | 改:`voucherRow` 加账簿字段;抽 `flattenVoucherRecords`;加 `fanOutVouchers`+常量+类型;抽 `accbooksCached`;重写 `GetVoucherList` 收多账簿 |
| `server/internal/handler/ys_voucher_fanout_test.go` | 合并逻辑单测 | 新建:flatten / fanOut(快/全/截断/单本报错)/ 400 |
| `server/internal/yonsuite/voucher.go` | YS 单账簿凭证查询 | **不改** |
| `server/internal/yonsuite/accbook.go` | YS 账簿清单 | **不改** |
| `src/pages/finance/VoucherQuery.tsx` | 凭证查询前端页 | 改:账簿多选 + 账簿列 + 本地翻页 + 拉全按钮 + 截断提示 |

---

## Task 1: 后端 — 抽 `flattenVoucherRecords` + `voucherRow` 加账簿字段

**Files:**
- Modify: `server/internal/handler/ys_voucher.go:63-79`(voucherRow 加字段)、`ys_voucher.go:136-177`(抽出 flatten)
- Test: `server/internal/handler/ys_voucher_fanout_test.go`(新建)

**Interfaces:**
- Produces: `func flattenVoucherRecords(recordList []map[string]interface{}, accbookCode, accbookName string) []voucherRow`;`voucherRow` 新增字段 `AccbookCode string` / `AccbookName string`。

- [ ] **Step 1: 写失败测试**

新建 `server/internal/handler/ys_voucher_fanout_test.go`:

```go
package handler

import (
	"testing"

	"bi-dashboard/internal/yonsuite"
)

func TestFlattenVoucherRecordsTagsAccbook(t *testing.T) {
	recs := []map[string]interface{}{
		{
			"header": map[string]interface{}{
				"id":              "1234567890123456789",
				"period":          "2026-06",
				"displayname":     "转-1",
				"vouchertype":     map[string]interface{}{"name": "转账凭证"},
				"description":     "计提工资",
				"totaldebit_org":  float64(1000),
				"totalcredit_org": float64(1000),
				"srcsystem":       "总账",
				"maker":           map[string]interface{}{"name": "张三"},
				"voucherstatus":   "04",
				"maketime":        "2026-06-30",
				"attachedbill":    "2",
			},
			"body": []interface{}{
				map[string]interface{}{
					"recordnumber":   "1",
					"description":    "计提工资",
					"accsubject":     map[string]interface{}{"code": "6601", "name": "管理费用"},
					"auxiliaryShow":  "行政部",
					"debit_original": float64(1000),
				},
			},
		},
	}
	rows := flattenVoucherRecords(recs, "ZJ001", "浙江松鲜鲜")
	if len(rows) != 1 {
		t.Fatalf("应抽平 1 行, got %d", len(rows))
	}
	r := rows[0]
	if r.AccbookCode != "ZJ001" || r.AccbookName != "浙江松鲜鲜" {
		t.Errorf("账簿标记错: code=%q name=%q", r.AccbookCode, r.AccbookName)
	}
	if r.ID != "1234567890123456789" {
		t.Errorf("19 位 id 应原样 string, got %q", r.ID)
	}
	if r.VoucherNo != "转-1" || r.VoucherType != "转账凭证" {
		t.Errorf("字号/类型错: %q %q", r.VoucherNo, r.VoucherType)
	}
	if r.Status != "已记账" {
		t.Errorf("状态 04 应→已记账, got %q", r.Status)
	}
	if r.TotalDebit != 1000 || r.TotalCredit != 1000 {
		t.Errorf("借贷合计错: %v %v", r.TotalDebit, r.TotalCredit)
	}
	if len(r.Lines) != 1 || r.Lines[0].SubjectName != "管理费用" {
		t.Errorf("分录错: %+v", r.Lines)
	}
	_ = yonsuite.Accbook{} // 确保 import 被用到(后续任务会真正用)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd server && go test ./internal/handler/ -run TestFlattenVoucherRecordsTagsAccbook -v`
Expected: 编译失败 `undefined: flattenVoucherRecords` 或 `r.AccbookCode undefined`。

- [ ] **Step 3: 改 `voucherRow` 加两字段**

`ys_voucher.go` 的 `voucherRow` 结构体(当前 63-79 行),在 `ID` 之前加两字段:

```go
type voucherRow struct {
	AccbookCode string        `json:"accbookCode"` // 账簿编码 (多账簿合并用)
	AccbookName string        `json:"accbookName"` // 账簿名称
	ID          string        `json:"id"`
	Period      string        `json:"period"`
	// ... 其余字段保持不变 ...
}
```

- [ ] **Step 4: 抽出 `flattenVoucherRecords`**

在 `ys_voucher.go` 加新函数(把 `GetVoucherList` 当前 136-177 行的 for 循环逻辑搬进来,加账簿入参):

```go
// flattenVoucherRecords 把用友 recordList 抽平成 voucherRow, 每行打上账簿标记
func flattenVoucherRecords(recordList []map[string]interface{}, accbookCode, accbookName string) []voucherRow {
	rows := make([]voucherRow, 0, len(recordList))
	for _, rec := range recordList {
		header := mapObj(rec, "header")
		row := voucherRow{
			AccbookCode: accbookCode,
			AccbookName: accbookName,
			ID:          ysMapStr(header, "id"),
			Period:      ysMapStr(header, "period"),
			VoucherNo:   ysMapStr(header, "displayname"),
			VoucherType: ysMapStr(mapObj(header, "vouchertype"), "name"),
			Description: ysMapStr(header, "description"),
			TotalDebit:  ysMapFloat(header, "totaldebit_org"),
			TotalCredit: ysMapFloat(header, "totalcredit_org"),
			SrcSystem:   ysMapStr(header, "srcsystem"),
			Maker:       ysMapStr(mapObj(header, "maker"), "name"),
			Auditor:     ysMapStr(mapObj(header, "auditor"), "name"),
			Tallyman:    ysMapStr(mapObj(header, "tallyman"), "name"),
			Status:      voucherStatusText(ysMapStr(header, "voucherstatus")),
			MakeTime:    ysMapStr(header, "maketime"),
			Attached:    ysMapStr(header, "attachedbill"),
		}
		if bodyArr, ok := rec["body"].([]interface{}); ok {
			for _, bi := range bodyArr {
				lm, ok := bi.(map[string]interface{})
				if !ok {
					continue
				}
				row.Lines = append(row.Lines, voucherLine{
					RecordNumber: ysMapStr(lm, "recordnumber"),
					Description:  ysMapStr(lm, "description"),
					SubjectCode:  ysMapStr(mapObj(lm, "accsubject"), "code"),
					SubjectName:  ysMapStr(mapObj(lm, "accsubject"), "name"),
					Auxiliary:    ysMapStr(lm, "auxiliaryShow"),
					Debit:        ysMapFloat(lm, "debit_original"),
					Credit:       ysMapFloat(lm, "credit_original"),
				})
			}
		}
		if row.Description == "" && len(row.Lines) > 0 {
			row.Description = row.Lines[0].Description
		}
		rows = append(rows, row)
	}
	return rows
}
```

然后把 `GetVoucherList` 里 136-177 行那段(`rows := make(...)` 到 for 结束)替换成调用(account name 暂传 ""，Task 3 会补 fan-out):

```go
	rows := flattenVoucherRecords(resp.Data.RecordList, body.AccbookCode, "")
```

- [ ] **Step 5: 运行测试确认通过 + 同包不回归**

Run: `cd server && go test ./internal/handler/ -run TestFlattenVoucherRecordsTagsAccbook -v`
Expected: PASS。
Run: `cd server && go build ./... && go vet ./internal/handler/`
Expected: 无错。

- [ ] **Step 6: 提交**

```bash
git add server/internal/handler/ys_voucher.go server/internal/handler/ys_voucher_fanout_test.go
git commit -m "refactor(voucher): 抽 flattenVoucherRecords + voucherRow 加账簿字段 — 为多账簿合并铺垫, 单账簿行为不变"
```

---

## Task 2: 后端 — `fanOutVouchers` 合并逻辑(纯函数 + 单测)

**Files:**
- Modify: `server/internal/handler/ys_voucher.go`(加常量、类型、`fanOutVouchers`)
- Test: `server/internal/handler/ys_voucher_fanout_test.go`(追加)

**Interfaces:**
- Consumes: `flattenVoucherRecords`(Task 1);`yonsuite.VoucherListResp`(含 `Data.RecordCount` / `Data.RecordList`)。
- Produces:
  - 常量 `voucherPerBookLimit=20` / `voucherMaxCalls=30` / `voucherMaxRows=6000` / `voucherFullPageSize=200`
  - `type voucherFetchPage func(accbookCode string, pageIndex, pageSize int) (*yonsuite.VoucherListResp, error)`
  - `type voucherBookMeta struct{ Code, Name string; RecordCount, Fetched int; Error string }`
  - `func fanOutVouchers(codes []string, nameOf map[string]string, full bool, fetch voucherFetchPage) (rows []voucherRow, meta []voucherBookMeta, truncated bool)`

- [ ] **Step 1: 写失败测试**(追加到 `ys_voucher_fanout_test.go`)

```go
// dummyRecords 造 n 条最小 record (fanOut 只数行数+读 RecordCount, 不关心字段)
func dummyRecords(n int) []map[string]interface{} {
	out := make([]map[string]interface{}, n)
	for i := range out {
		out[i] = map[string]interface{}{"header": map[string]interface{}{"id": "x"}}
	}
	return out
}

func resp(recordCount, n int) *yonsuite.VoucherListResp {
	r := &yonsuite.VoucherListResp{}
	r.Data.RecordCount = recordCount
	r.Data.RecordList = dummyRecords(n)
	return r
}

func TestFanOutVouchersFastMode(t *testing.T) {
	nameOf := map[string]string{"A": "甲账簿", "B": "乙账簿"}
	calls := 0
	fetch := func(code string, pi, ps int) (*yonsuite.VoucherListResp, error) {
		calls++
		if ps != voucherPerBookLimit {
			t.Errorf("快模式每本应只拉 %d 条, got pageSize=%d", voucherPerBookLimit, ps)
		}
		switch code {
		case "A":
			return resp(50, 20), nil // 甲账簿共 50, 只拉到 20
		case "B":
			return resp(5, 5), nil
		}
		return resp(0, 0), nil
	}
	rows, meta, truncated := fanOutVouchers([]string{"A", "B"}, nameOf, false, fetch)
	if calls != 2 {
		t.Errorf("快模式 2 本账应只调 2 次, got %d", calls)
	}
	if len(rows) != 25 {
		t.Errorf("合并行数应 20+5=25, got %d", len(rows))
	}
	if truncated {
		t.Error("快模式不应标 truncated")
	}
	if meta[0].RecordCount != 50 || meta[0].Fetched != 20 || meta[0].Name != "甲账簿" {
		t.Errorf("甲账簿 meta 错: %+v", meta[0])
	}
	if meta[1].RecordCount != 5 || meta[1].Fetched != 5 {
		t.Errorf("乙账簿 meta 错: %+v", meta[1])
	}
}

func TestFanOutVouchersFullModePaginates(t *testing.T) {
	nameOf := map[string]string{"A": "甲"}
	pages := map[int]int{1: 200, 2: 200, 3: 50} // 共 450
	fetch := func(code string, pi, ps int) (*yonsuite.VoucherListResp, error) {
		if ps != voucherFullPageSize {
			t.Errorf("拉全每页应 %d, got %d", voucherFullPageSize, ps)
		}
		return resp(450, pages[pi]), nil
	}
	rows, meta, truncated := fanOutVouchers([]string{"A"}, nameOf, true, fetch)
	if len(rows) != 450 {
		t.Errorf("拉全应翻页到 450, got %d", len(rows))
	}
	if truncated {
		t.Error("450 行未触顶, 不应 truncated")
	}
	if meta[0].Fetched != 450 || meta[0].RecordCount != 450 {
		t.Errorf("meta 错: %+v", meta[0])
	}
}

func TestFanOutVouchersCapTruncates(t *testing.T) {
	nameOf := map[string]string{"A": "甲"}
	fetch := func(code string, pi, ps int) (*yonsuite.VoucherListResp, error) {
		return resp(1000000, voucherFullPageSize), nil // 永远还有更多
	}
	rows, _, truncated := fanOutVouchers([]string{"A"}, nameOf, true, fetch)
	if !truncated {
		t.Error("超大账簿拉全应触顶 truncated")
	}
	if len(rows) > voucherMaxRows {
		t.Errorf("行数不应超过闸值 %d, got %d", voucherMaxRows, len(rows))
	}
}

func TestFanOutVouchersPerBookErrorIsolated(t *testing.T) {
	nameOf := map[string]string{"A": "甲", "B": "乙"}
	fetch := func(code string, pi, ps int) (*yonsuite.VoucherListResp, error) {
		if code == "A" {
			return nil, errVoucherTest
		}
		return resp(3, 3), nil
	}
	rows, meta, _ := fanOutVouchers([]string{"A", "B"}, nameOf, false, fetch)
	if len(rows) != 3 {
		t.Errorf("甲失败应只剩乙的 3 行, got %d", len(rows))
	}
	if meta[0].Error == "" {
		t.Error("甲账簿应记 error")
	}
	if meta[1].Error != "" || meta[1].Fetched != 3 {
		t.Errorf("乙账簿不应受影响: %+v", meta[1])
	}
}

var errVoucherTest = &voucherTestErr{}

type voucherTestErr struct{}

func (*voucherTestErr) Error() string { return "用友连接失败(测试)" }
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd server && go test ./internal/handler/ -run TestFanOutVouchers -v`
Expected: 编译失败 `undefined: fanOutVouchers` / `undefined: voucherPerBookLimit` 等。

- [ ] **Step 3: 实现常量 + 类型 + `fanOutVouchers`**

在 `ys_voucher.go` 加(放在 `voucherRow` 定义附近):

```go
// 多账簿合并参数 (spec §7)
const (
	voucherPerBookLimit = 20   // 快模式每本账拉前几条
	voucherFullPageSize = 200  // 拉全模式每页大小
	voucherMaxCalls     = 30   // 拉全总调用上限 (≈33s 封顶, 防堵死 YS 共享通道)
	voucherMaxRows      = 6000 // 拉全总行数上限
)

// voucherFetchPage 拉一本账一页凭证 (注入便于单测)
type voucherFetchPage func(accbookCode string, pageIndex, pageSize int) (*yonsuite.VoucherListResp, error)

// voucherBookMeta 每本账的拉取情况 (回前端用于截断提示/失败提示)
type voucherBookMeta struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	RecordCount int    `json:"recordCount"`     // 用友报的总条数
	Fetched     int    `json:"fetched"`         // 实际拉到的条数
	Error       string `json:"error,omitempty"` // 该本账查询失败信息
}

// fanOutVouchers 逐账簿调用 fetch 并合并。
// full=false: 每本只拉第 1 页 (前 voucherPerBookLimit 条)。
// full=true:  每本循环翻页拉到底, 受 voucherMaxCalls / voucherMaxRows 兜底闸约束。
// 单本账失败不中断其他账簿 (记入 meta.Error)。
// 行顺序 = 账簿勾选顺序 + 用友返回的自然顺序, 不额外排序 (合并表按账簿分组天然成立)。
func fanOutVouchers(codes []string, nameOf map[string]string, full bool, fetch voucherFetchPage) (rows []voucherRow, meta []voucherBookMeta, truncated bool) {
	calls := 0
	for _, code := range codes {
		name := nameOf[code]
		if name == "" {
			name = code // 账簿名查不到时退回编码
		}
		if !full {
			if calls >= voucherMaxCalls {
				truncated = true
				break
			}
			r, err := fetch(code, 1, voucherPerBookLimit)
			calls++
			if err != nil {
				meta = append(meta, voucherBookMeta{Code: code, Name: name, Error: err.Error()})
				continue
			}
			recs := flattenVoucherRecords(r.Data.RecordList, code, name)
			rows = append(rows, recs...)
			meta = append(meta, voucherBookMeta{Code: code, Name: name, RecordCount: r.Data.RecordCount, Fetched: len(recs)})
			continue
		}
		// full: 翻页拉到底
		fetched, recordCount, bookErr, pageIndex := 0, 0, "", 1
		for {
			if calls >= voucherMaxCalls || len(rows) >= voucherMaxRows {
				truncated = true
				break
			}
			r, err := fetch(code, pageIndex, voucherFullPageSize)
			calls++
			if err != nil {
				bookErr = err.Error()
				break
			}
			recordCount = r.Data.RecordCount
			recs := flattenVoucherRecords(r.Data.RecordList, code, name)
			rows = append(rows, recs...)
			fetched += len(recs)
			if len(recs) == 0 || fetched >= recordCount {
				break
			}
			pageIndex++
		}
		meta = append(meta, voucherBookMeta{Code: code, Name: name, RecordCount: recordCount, Fetched: fetched, Error: bookErr})
		if truncated {
			break
		}
	}
	return rows, meta, truncated
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd server && go test ./internal/handler/ -run TestFanOutVouchers -v`
Expected: 4 个 fanOut 测试全 PASS。

- [ ] **Step 5: 提交**

```bash
git add server/internal/handler/ys_voucher.go server/internal/handler/ys_voucher_fanout_test.go
git commit -m "feat(voucher): 加 fanOutVouchers 多账簿合并逻辑(快/拉全/兜底闸/单本失败隔离)+ 单测"
```

---

## Task 3: 后端 — 重写 `GetVoucherList` 收多账簿 + `accbooksCached` 复用

**Files:**
- Modify: `server/internal/handler/ys_voucher.go`(加 `accbooksCached`、重构 `GetVoucherAccbooks`、重写 `GetVoucherList`)
- Test: `server/internal/handler/ys_voucher_fanout_test.go`(追加 400 测试)

**Interfaces:**
- Consumes: `fanOutVouchers`(Task 2)、`accbookCache*` 全局缓存变量(`ys_voucher.go:15-19`)、`h.YS.QueryAccbookList()`、`h.YS.QueryVoucherList()`。
- Produces: `func (h *DashboardHandler) accbooksCached() ([]yonsuite.Accbook, error)`;`GetVoucherList` 新入参 `accbookCodes []string` + `full bool`,响应 `{list, books, truncated, full}`。

- [ ] **Step 1: 写失败测试**(追加)

```go
import (
	"net/http"             // 追加到文件顶部 import
	"net/http/httptest"
	"strings"
)

func TestGetVoucherListEmptyCodes400(t *testing.T) {
	// YS 非 nil(用 NewClient 构造但永不拨号), accbookCodes 为空应在调用用友前就 400
	h := &DashboardHandler{YS: yonsuite.NewClient("k", "s", "http://127.0.0.1:0")}
	req := httptest.NewRequest("POST", "/api/finance/voucher/list",
		strings.NewReader(`{"accbookCodes":[],"periodStart":"2026-06","periodEnd":"2026-06"}`))
	w := httptest.NewRecorder()
	h.GetVoucherList(w, req)
	if w.Code != 400 {
		t.Fatalf("空账簿应返回 400, got %d, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "请选择账簿") {
		t.Errorf("应提示请选择账簿, got %s", w.Body.String())
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd server && go test ./internal/handler/ -run TestGetVoucherListEmptyCodes400 -v`
Expected: FAIL（当前 handler 收的是 `accbookCode` 单值,`accbookCodes` 空时不会命中新校验;或断言信息不符)。

- [ ] **Step 3: 抽 `accbooksCached` + 重构 `GetVoucherAccbooks`**

`ys_voucher.go` 加 helper,并让 `GetVoucherAccbooks` 复用它:

```go
// accbooksCached 返回账簿清单 (24h 缓存, 空/过期则现拉)。多账簿名映射与下拉共用一份缓存。
func (h *DashboardHandler) accbooksCached() ([]yonsuite.Accbook, error) {
	accbookCacheMu.Lock()
	if accbookCache != nil && time.Since(accbookCacheTime) < 24*time.Hour {
		cached := accbookCache
		accbookCacheMu.Unlock()
		return cached, nil
	}
	accbookCacheMu.Unlock()

	list, err := h.YS.QueryAccbookList()
	if err != nil {
		return nil, err
	}
	accbookCacheMu.Lock()
	accbookCache = list
	accbookCacheTime = time.Now()
	accbookCacheMu.Unlock()
	return list, nil
}
```

把 `GetVoucherAccbooks`(22-49 行)的缓存读取/回填逻辑替换为:

```go
func (h *DashboardHandler) GetVoucherAccbooks(w http.ResponseWriter, r *http.Request) {
	if h.YS == nil {
		writeError(w, 503, "用友 YS 未配置")
		return
	}
	list, err := h.accbooksCached()
	if err != nil {
		writeServerError(w, 500, "查询账簿失败", err)
		return
	}
	writeJSON(w, list)
}
```

- [ ] **Step 4: 重写 `GetVoucherList`**

把 `GetVoucherList`(81-185 行)整体替换为:

```go
// GetVoucherList 实时查用友凭证 (POST), 支持多账簿合并
func (h *DashboardHandler) GetVoucherList(w http.ResponseWriter, r *http.Request) {
	if h.YS == nil {
		writeError(w, 503, "用友 YS 未配置")
		return
	}
	if r.Method != "POST" {
		writeError(w, 405, "method not allowed")
		return
	}

	var body struct {
		AccbookCodes  []string `json:"accbookCodes"`
		PeriodStart   string   `json:"periodStart"`
		PeriodEnd     string   `json:"periodEnd"`
		VoucherStatus string   `json:"voucherStatus"`
		BillcodeMin   int      `json:"billcodeMin"`
		BillcodeMax   int      `json:"billcodeMax"`
		Full          bool     `json:"full"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, 400, "请求参数解析失败")
		return
	}
	if len(body.AccbookCodes) == 0 {
		writeError(w, 400, "请选择账簿")
		return
	}

	// 账簿名映射 (拉不到也不致命, fanOut 会退回编码)
	nameOf := map[string]string{}
	if books, err := h.accbooksCached(); err == nil {
		for _, b := range books {
			nameOf[b.Code] = b.Name
		}
	}

	// fetch 闭包: 把账期/状态/凭证号过滤条件 baked 进去, 只暴露 (code, pageIndex, pageSize)
	fetch := func(code string, pageIndex, pageSize int) (*yonsuite.VoucherListResp, error) {
		req := &yonsuite.VoucherListReq{
			AccbookCode: code,
			PeriodStart: body.PeriodStart,
			PeriodEnd:   body.PeriodEnd,
			BillcodeMin: body.BillcodeMin,
			BillcodeMax: body.BillcodeMax,
		}
		req.Pager.PageIndex = pageIndex
		req.Pager.PageSize = pageSize
		if body.VoucherStatus != "" {
			req.VoucherStatusList = []string{body.VoucherStatus}
		}
		return h.YS.QueryVoucherList(req)
	}

	rows, meta, truncated := fanOutVouchers(body.AccbookCodes, nameOf, body.Full, fetch)

	writeJSON(w, map[string]interface{}{
		"list":      rows,
		"books":     meta,
		"truncated": truncated,
		"full":      body.Full,
	})
}
```

> 注:Task 1 里 `flattenVoucherRecords` 那个临时调用点已被本次整体替换吸收(flatten 现在只被 fanOut 调用)。

- [ ] **Step 5: 运行测试确认通过 + 全 handler 套件不回归**

Run: `cd server && go test ./internal/handler/ -run TestGetVoucherListEmptyCodes400 -v`
Expected: PASS。
Run: `cd server && go build ./... && go vet ./internal/handler/ && go test ./internal/handler/ -run Voucher`
Expected: build/vet 无错;凭证相关测试全 PASS。

- [ ] **Step 6: 提交**

```bash
git add server/internal/handler/ys_voucher.go server/internal/handler/ys_voucher_fanout_test.go
git commit -m "feat(voucher): GetVoucherList 改收 accbookCodes 多账簿 + full 拉全 — fan-out 合并, 账簿名缓存复用, 空账簿 400"
```

---

## Task 4: 前端 — 账簿多选 + 账簿列 + 本地翻页 + 拉全

**Files:**
- Modify: `src/pages/finance/VoucherQuery.tsx`

**Interfaces:**
- Consumes: 后端 `/api/finance/voucher/list` 新响应 `{list: VoucherRow[], books: BookMeta[], truncated: boolean, full: boolean}`,`VoucherRow` 新增 `accbookCode/accbookName`。

- [ ] **Step 1: 状态改多选**

`VoucherQuery.tsx` 改:
- `interface VoucherRow` 加 `accbookCode: string; accbookName: string;`(放最前)。
- 新增 `interface BookMeta { code: string; name: string; recordCount: number; fetched: number; error?: string; }`
- 第 56 行 `const [accbookCode, setAccbookCode] = useState<string>('');` → `const [accbookCodes, setAccbookCodes] = useState<string[]>([]);`
- 加 `const [books, setBooks] = useState<BookMeta[]>([]);` 和 `const [truncated, setTruncated] = useState(false);`
- 账簿下拉加载(70-81 行)默认改成数组:`if (def) setAccbookCodes([def.code]);`

- [ ] **Step 2: `fetchVouchers` 改造(加 full 参数、本地翻页、读 books/truncated)**

把 `fetchVouchers`(83-121 行)与 `onSearch`(123 行)替换为:

```tsx
  const fetchVouchers = useCallback((full: boolean) => {
    if (!accbookCodes.length) { message.warning('请先选择账簿'); return; }
    abortRef.current?.abort();
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setLoading(true);
    fetch(`${API_BASE}/api/finance/voucher/list`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      signal: ctrl.signal,
      body: JSON.stringify({
        accbookCodes,
        periodStart: period[0].format('YYYY-MM'),
        periodEnd: period[1].format('YYYY-MM'),
        voucherStatus: status,
        billcodeMin: billMin || 0,
        billcodeMax: billMax || 0,
        full,
      }),
    })
      .then(res => res.json())
      .then(res => {
        if (res.code !== 0 && res.code !== 200) {
          message.error(res.message || '查询失败');
          setRows([]); setBooks([]); setTruncated(false); setLoading(false); return;
        }
        const d = res.data || {};
        setRows(d.list || []);
        setBooks(d.books || []);
        setTruncated(!!d.truncated);
        setPageIndex(1);
        setLoading(false);
        setQueried(true);
        // 单本账失败提示
        (d.books || []).filter((b: BookMeta) => b.error).forEach((b: BookMeta) =>
          message.warning(`账簿「${b.name}」查询失败:${b.error}`));
      })
      .catch((e: any) => {
        if (e?.name !== 'AbortError') { message.error('查询失败'); setLoading(false); }
      });
  }, [accbookCodes, period, status, billMin, billMax]);

  const onSearch = () => fetchVouchers(false);

  const onFullPull = () => {
    const totalMore = books.reduce((s, b) => s + Math.max(0, b.recordCount - b.fetched), 0);
    Modal.confirm({
      title: '拉取全部凭证',
      content: `将逐本账簿拉取选中账簿在当前条件下的全部凭证${accbookCodes.length > 3 || totalMore > 500 ? ',账簿多/数据量大时可能要等几十秒' : ''}。继续?`,
      onOk: () => fetchVouchers(true),
    });
  };

  // 是否有账簿还有更多未显示 (快模式截断提示)
  const hasMore = books.some(b => b.recordCount > b.fetched);
```

- [ ] **Step 3: import 补 Modal + Alert**

第 2 行 antd import 加 `Modal` 和 `Alert`:

```tsx
import { Card, Table, Select, DatePicker, InputNumber, Button, Space, Tag, message, Typography, Modal, Alert } from 'antd';
```

- [ ] **Step 4: 账簿框改多选 + 加账簿列 + 拉全按钮 + 提示条**

账簿 `<Select>`(173-181 行)改:

```tsx
            <Select
              mode="multiple"
              showSearch
              allowClear
              maxTagCount="responsive"
              style={{ minWidth: 320, maxWidth: 560 }}
              placeholder="选择账簿(可多选)"
              value={accbookCodes}
              onChange={setAccbookCodes}
              optionFilterProp="label"
              options={accbooks.map(a => ({ label: `${a.code} ${a.name}`, value: a.code }))}
            />
```

查询按钮(211-213 行)后面加「拉全」按钮:

```tsx
          <Button type="primary" icon={<SearchOutlined />} onClick={onSearch} loading={loading}>
            查询
          </Button>
          <Button onClick={onFullPull} loading={loading} disabled={!queried}>
            拉全
          </Button>
```

`columns`(125 行)最前面加账簿列:

```tsx
  const columns = [
    { title: '账簿', dataIndex: 'accbookName', width: 160, ellipsis: true,
      render: (v: string) => v || '-' },
    { title: '账期', dataIndex: 'period', width: 90 },
    // ... 其余列保持不变 ...
  ];
```

`<div style={{ marginBottom: 16 }}>` 那块筛选区(169 行)下方、`<Table>` 上方,加提示条:

```tsx
      {queried && (hasMore || truncated) && (
        <Alert
          style={{ marginBottom: 12 }}
          type="warning"
          showIcon
          message={truncated
            ? '结果太多已截断,请缩小账期或减少账簿后重试'
            : '部分账簿还有更多凭证未显示,点「拉全」查看完整,或缩小账期/凭证号范围'}
        />
      )}
```

- [ ] **Step 5: Table 改本地翻页**

`rowKey="id"` 改成兼顾多账簿唯一(同号跨账簿不撞):

```tsx
        rowKey={(r) => `${r.accbookCode}-${r.id}`}
```

`pagination`(238-246 行)改为本地翻页(去掉 `total`/`onChange` 回后端):

```tsx
        pagination={{
          current: pageIndex,
          pageSize,
          showSizeChanger: true,
          pageSizeOptions: ['20', '50', '100'],
          showTotal: (t) => `共 ${t} 张凭证`,
          onChange: (p, ps) => { setPageIndex(p); setPageSize(ps); },
        }}
```

(删除 `recordCount` 相关 state 与 `setRecordCount` 调用;本地翻页由 antd 按 `dataSource` 长度自动算 total。)

- [ ] **Step 6: 类型检查 + 构建**

Run: `npx tsc --noEmit` (在 repo 根)
Expected: 0 报错。
Run: `npm run build`
Expected: build 成功(无 eslint error;`CI=true` 下 warning 也算 error,注意清理未用变量如 `recordCount`)。

- [ ] **Step 7: 提交**

```bash
git add src/pages/finance/VoucherQuery.tsx
git commit -m "feat(voucher): 凭证查询账簿改多选 — 合并表+账簿列+本地翻页+拉全按钮+截断/失败提示"
```

---

## Task 5: 验证(playwright 真点)

**Files:** 无(仅验证)

- [ ] **Step 1: 起前端预览**(若未起):`npm run build` 后用现有 serve;或本地 `npm start`。后端需 `bi-server` 在 8080 且 YS 已配置。

- [ ] **Step 2: playwright 实测清单**(用浏览器 MCP,超管登录后到 财务→凭证查询):
  - 多选 2 本账 + 选当月 → 点查询 → 表格出现「账簿」列,两本账的凭证都在,账簿名正确。
  - 某本账凭证 >20 → 顶部出现黄色提示条「还有更多…」。
  - 点「拉全」→ 弹确认 → 确认后完整拉取,提示条消失(或仍在但因触顶变截断文案)。
  - 翻页/改每页条数 → 本地即时翻,不再转圈打后端。
  - 展开某行 → 分录明细(借贷)正常。
  - 只选 1 本账 → 行为正常(回归)。
- [ ] **Step 3: 截图留证**(查询结果 + 账簿列 + 提示条 + 拉全后)。

- [ ] **Step 4: 跑哥验收**:确认无误后,由跑哥决定是否进入部署/发版(本计划不含部署)。

---

## Self-Review(已自查并修正)

**1. Spec 覆盖**:spec §4.1 两模式→Task 2 fanOut(快/全);§4.2 后端流程→Task 2+3;§4.3 前端(多选/账簿列/本地翻页/拉全/提示条/单本失败)→Task 4;§4.4 错误处理(空账簿400/单本失败隔离/触顶截断/Abort)→Task 2+3+4;§5 测试→Task 1/2/3 单测 + Task 5 playwright;§7 默认参数→Task 2 常量。覆盖完整。

**2. 占位符扫描**:无 TBD/TODO;每个代码步给出完整代码与命令。

**3. 类型一致性**:`fanOutVouchers(codes, nameOf, full, fetch)` 签名 Task 2 定义、Task 3 调用一致;`voucherFetchPage(code, pageIndex, pageSize)` 与 Task 3 fetch 闭包一致;`voucherBookMeta` 字段(code/name/recordCount/fetched/error)与前端 `BookMeta`(code/name/recordCount/fetched/error)一致;`voucherRow.AccbookCode/AccbookName` 的 JSON tag(accbookCode/accbookName)与前端 `VoucherRow.accbookCode/accbookName` 一致;常量名 Task 2 定义、各处引用一致。

**已知小偏差(对 spec 的合理收敛)**:spec §4.2 提到"sort rows by (账簿顺序, period, voucherNo)";实现采用 append 顺序(= 账簿勾选顺序 + 用友自然顺序),天然满足"按账簿分组",不额外排序,更简单确定。已在 `fanOutVouchers` 注释标明。
