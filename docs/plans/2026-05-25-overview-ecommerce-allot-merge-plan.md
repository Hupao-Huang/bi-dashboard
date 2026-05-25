# 综合看板电商部 KPI 调拨合并 Implementation Plan (v1.74.3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 综合看板电商部 mini 卡片改为"销售 + 调拨 + 总额"3 数据拆解, 修正长期口径错误 (2 调拨渠道用销售单口径, 应该用调拨口径).

**Architecture:** 后端在 `dashboard_overview.go` 加 helper `loadEcommerceAllotAdjustment` 单独查 2 渠道双口径, 在 GetOverview 内对 ecommerce dept 做后处理 (减销售 + 加调拨, 主字给总和). DeptSummary 加 SalesAmt/AllotAmt 字段供前端拆解. 前端 `index.tsx` 改 mini 卡渲染 + 删 "不含调拨" Tooltip.

**Tech Stack:** Go 1.21 + net/http + sqlmock (测试), React + AntD (前端)

**Spec**: `docs/specs/2026-05-25-overview-ecommerce-allot-merge-design.md` (commit `503f31a`)

---

## Task 1: 后端 — DeptSummary struct 加 SalesAmt / AllotAmt 字段

**Files:**
- Modify: `server/internal/handler/dashboard_overview.go:42-49`

**Why first**: 后续任务 (helper / GetOverview / 单测) 都会用到这俩字段, 先定义结构.

- [ ] **Step 1: Edit DeptSummary struct**

把 `dashboard_overview.go:42-49` 改为:

```go
type DeptSummary struct {
    Department string  `json:"department"`
    Sales      float64 `json:"sales"`
    Qty        float64 `json:"qty"`
    Profit     float64 `json:"profit"`
    Cost       float64 `json:"cost"`
    SkuCount   int     `json:"skuCount"`
    SalesAmt   float64 `json:"salesAmt,omitempty"` // v1.74.3: 电商部排除 2 调拨渠道后的销售口径
    AllotAmt   float64 `json:"allotAmt,omitempty"` // v1.74.3: 电商部 2 调拨渠道的调拨口径
}
```

- [ ] **Step 2: Verify compile**

Run: `cd server && go build ./internal/handler/...`
Expected: No errors. (即使 helper 没写, 新字段只是 struct 加 field, 不依赖其他)

- [ ] **Step 3: Commit**

```bash
cd /c/Users/Administrator/bi-dashboard
git add server/internal/handler/dashboard_overview.go
git commit -m "refactor(overview): DeptSummary 加 SalesAmt/AllotAmt 字段 (v1.74.3 准备)"
```

---

## Task 2: 后端 — helper `loadEcommerceAllotAdjustment` (TDD)

**Files:**
- Create: `server/internal/handler/dashboard_overview_test.go`
- Modify: `server/internal/handler/dashboard_overview.go` (在 GetOverview 函数末尾追加 helper)

- [ ] **Step 1: Write failing test for happy path**

新建 `server/internal/handler/dashboard_overview_test.go`:

```go
package handler

import (
    "context"
    "regexp"
    "testing"

    "github.com/DATA-DOG/go-sqlmock"
)

func TestLoadEcommerceAllotAdjustment_HappyPath(t *testing.T) {
    db, mock, err := sqlmock.New()
    if err != nil {
        t.Fatalf("sqlmock: %v", err)
    }
    defer db.Close()

    // query 1: 2 渠道销售单口径 SUM
    mock.ExpectQuery(regexp.QuoteMeta("FROM sales_goods_summary")).
        WithArgs("2026-04-01", "2026-04-30", "1819610592561398400", "1819610591915475584").
        WillReturnRows(sqlmock.NewRows([]string{"sales"}).AddRow(5130000.0))

    // query 2: 2 渠道调拨 excel_amount SUM
    mock.ExpectQuery(regexp.QuoteMeta("FROM allocate_orders o")).
        WithArgs("2026-04-01", "2026-04-30").
        WillReturnRows(sqlmock.NewRows([]string{"amt"}).AddRow(7320000.0))

    h := &DashboardHandler{DB: db}
    salesExcluded, allotAmt, err := h.loadEcommerceAllotAdjustment(
        context.Background(), "2026-04-01", "2026-04-30", "", nil)
    if err != nil {
        t.Fatalf("unexpected err: %v", err)
    }
    if salesExcluded != 5130000.0 {
        t.Errorf("salesExcluded=%v, want 5130000", salesExcluded)
    }
    if allotAmt != 7320000.0 {
        t.Errorf("allotAmt=%v, want 7320000", allotAmt)
    }
    if err := mock.ExpectationsWereMet(); err != nil {
        t.Errorf("unmet expectations: %v", err)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && go test ./internal/handler/ -run TestLoadEcommerceAllotAdjustment_HappyPath -v`
Expected: FAIL with "undefined: (*DashboardHandler).loadEcommerceAllotAdjustment"

- [ ] **Step 3: Implement helper in dashboard_overview.go**

在 `dashboard_overview.go` 文件末尾追加 (最后一个 `}` 之后):

```go
// loadEcommerceAllotAdjustment v1.74.3: 加载电商部 2 调拨渠道的双口径金额
// 返回:
//   salesExcluded: 这 2 渠道在 sales_goods_summary 的销售单口径 SUM (要从 dept.sales 减掉)
//   allotAmt: 这 2 渠道在 allocate_details 的 excel_amount SUM (要加到 dept.sales)
// 兜底原则: 任一 query 失败返 err, 调用方决定 fallback 还是 fail (本设计 fallback)
func (h *DashboardHandler) loadEcommerceAllotAdjustment(
    ctx context.Context,
    start, end string,
    scopeCond string, scopeArgs []interface{},
) (salesExcluded, allotAmt float64, err error) {
    // 固定 2 渠道 ID (跟 special_channel.go 一致)
    const jdShopID = "1819610592561398400"   // ds-京东-清心湖自营
    const tmcsShopID = "1819610591915475584" // ds-天猫超市-寄售

    // query 1: 2 渠道销售单口径 (要从 dept.sales 减)
    salesArgs := append([]interface{}{start, end, jdShopID, tmcsShopID}, scopeArgs...)
    err = h.DB.QueryRowContext(ctx, `
        SELECT IFNULL(SUM(IFNULL(local_goods_amt, goods_amt)), 0)
        FROM sales_goods_summary
        WHERE stat_date BETWEEN ? AND ?
          AND shop_id IN (?, ?)`+scopeCond, salesArgs...).Scan(&salesExcluded)
    if err != nil {
        return 0, 0, fmt.Errorf("查 2 渠道销售单口径失败: %w", err)
    }

    // query 2: 2 渠道调拨口径 (要加进 dept.sales)
    err = h.DB.QueryRowContext(ctx, `
        SELECT IFNULL(SUM(d.excel_amount), 0)
        FROM allocate_orders o
        JOIN allocate_details d ON d.allocate_no = o.allocate_no
        WHERE o.stat_date BETWEEN ? AND ?
          AND o.channel_key IN ('京东', '猫超')`, start, end).Scan(&allotAmt)
    if err != nil {
        return 0, 0, fmt.Errorf("查 2 渠道调拨口径失败: %w", err)
    }

    return salesExcluded, allotAmt, nil
}
```

⚠️ 注意: `dashboard_overview.go` 现有 import 是 `"net/http"`, `"strings"`. 这次新增需要 `"context"` + `"fmt"`. Edit `import` 块加这俩.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd server && go test ./internal/handler/ -run TestLoadEcommerceAllotAdjustment_HappyPath -v`
Expected: PASS

- [ ] **Step 5: Add error test (query 1 fail)**

在 `dashboard_overview_test.go` 加测试:

```go
func TestLoadEcommerceAllotAdjustment_Query1Error(t *testing.T) {
    db, mock, _ := sqlmock.New()
    defer db.Close()

    mock.ExpectQuery(regexp.QuoteMeta("FROM sales_goods_summary")).
        WillReturnError(errors.New("connection lost"))

    h := &DashboardHandler{DB: db}
    _, _, err := h.loadEcommerceAllotAdjustment(
        context.Background(), "2026-04-01", "2026-04-30", "", nil)
    if err == nil {
        t.Fatal("expected err for query 1 failure, got nil")
    }
}
```

需要在文件顶部 import 加 `"errors"`.

- [ ] **Step 6: Run all tests, all pass**

Run: `cd server && go test ./internal/handler/ -run TestLoadEcommerceAllotAdjustment -v`
Expected: 2 PASS (HappyPath + Query1Error)

- [ ] **Step 7: Commit**

```bash
git add server/internal/handler/dashboard_overview.go server/internal/handler/dashboard_overview_test.go
git commit -m "feat(overview): helper loadEcommerceAllotAdjustment + 2 单测 (v1.74.3)"
```

---

## Task 3: 后端 — GetOverview 集成 helper + 修改 dept.Sales

**Files:**
- Modify: `server/internal/handler/dashboard_overview.go` (在 line 84 deptList 构建完之后插入)

- [ ] **Step 1: Write integration test (sqlmock 端到端)**

在 `dashboard_overview_test.go` 加:

```go
func TestGetOverviewWithAllotAdjustment_HappyPath(t *testing.T) {
    db, mock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
    defer db.Close()

    // 主 SQL: 各部门汇总, ecommerce dept 含 2 渠道销售口径
    deptRows := sqlmock.NewRows([]string{"dept", "sales", "qty", "profit", "cost", "sku_count"}).
        AddRow("ecommerce", 7094000.0, 1000, 1000000.0, 5000000.0, 50). // sales 含 2 渠道销售口径 ¥513 万
        AddRow("social", 500000.0, 100, 50000.0, 300000.0, 20)
    mock.ExpectQuery("SELECT CASE WHEN department IS NULL").
        WillReturnRows(deptRows)

    // trendRows (line 88 trend SQL)
    trendRows := sqlmock.NewRows([]string{"d", "dept", "sales", "qty"})
    mock.ExpectQuery("SELECT DATE_FORMAT").WillReturnRows(trendRows)

    // 其它 SQL (按 GetOverview 顺序 mock 全部, 此处简化, 见实际代码)
    // ...

    // helper query 1: 2 渠道销售口径
    mock.ExpectQuery("FROM sales_goods_summary").
        WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "1819610592561398400", "1819610591915475584").
        WillReturnRows(sqlmock.NewRows([]string{"x"}).AddRow(5130000.0))

    // helper query 2: 2 渠道调拨口径
    mock.ExpectQuery("FROM allocate_orders o").
        WillReturnRows(sqlmock.NewRows([]string{"x"}).AddRow(7320000.0))

    // (此测试简化版只验证 ecommerce dept 数据被修改, 完整 mock 见实施时增补)
    // TODO 实施时: 增补全部 query mock + req/resp 构造
}
```

⚠️ 这个集成测试比较复杂 (GetOverview 内有 5+ query), 实施时需要 mock 全部. 实施时可以简化: 抽出 `applyEcommerceAllotAdjustment(deptList, salesExcluded, allotAmt)` 子函数, 单测它即可 (推荐).

**改进版 (推荐 — 抽子函数单测)**:

```go
func TestApplyEcommerceAllotAdjustment(t *testing.T) {
    deptList := []DeptSummary{
        {Department: "ecommerce", Sales: 7094000.0, Qty: 1000},
        {Department: "social", Sales: 500000.0, Qty: 100},
    }
    applyEcommerceAllotAdjustment(deptList, 5130000.0, 7320000.0)

    // ecommerce dept 应该被修改
    if deptList[0].SalesAmt != 7094000.0-5130000.0 {
        t.Errorf("ecommerce.SalesAmt = %v, want %v", deptList[0].SalesAmt, 7094000.0-5130000.0)
    }
    if deptList[0].AllotAmt != 7320000.0 {
        t.Errorf("ecommerce.AllotAmt = %v, want 7320000", deptList[0].AllotAmt)
    }
    expectedSales := (7094000.0 - 5130000.0) + 7320000.0
    if deptList[0].Sales != expectedSales {
        t.Errorf("ecommerce.Sales = %v, want %v (新总和)", deptList[0].Sales, expectedSales)
    }

    // social dept 不应该被修改
    if deptList[1].Sales != 500000.0 {
        t.Errorf("social.Sales 不应该变, 实际 %v", deptList[1].Sales)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && go test ./internal/handler/ -run TestApplyEcommerceAllotAdjustment -v`
Expected: FAIL with "undefined: applyEcommerceAllotAdjustment"

- [ ] **Step 3: Implement applyEcommerceAllotAdjustment helper**

在 `dashboard_overview.go` 文件末尾 (loadEcommerceAllotAdjustment 之后) 追加:

```go
// applyEcommerceAllotAdjustment v1.74.3: 把 2 调拨渠道的口径换到 dept.Sales
// 找到 ecommerce dept, 计算:
//   SalesAmt = Sales - salesExcluded (其它电商渠道销售口径)
//   AllotAmt = allotAmt              (这 2 调拨渠道)
//   Sales = SalesAmt + AllotAmt      (新总和, 给顶部 totalSales / 右上角 tag 用)
// 兜底: SalesAmt < 0 钳到 0 (理论不应发生, 防数据异常)
func applyEcommerceAllotAdjustment(deptList []DeptSummary, salesExcluded, allotAmt float64) {
    for i, d := range deptList {
        if d.Department != "ecommerce" {
            continue
        }
        salesAmt := d.Sales - salesExcluded
        if salesAmt < 0 {
            salesAmt = 0
        }
        deptList[i].SalesAmt = salesAmt
        deptList[i].AllotAmt = allotAmt
        deptList[i].Sales = salesAmt + allotAmt
        break
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd server && go test ./internal/handler/ -run TestApplyEcommerceAllotAdjustment -v`
Expected: PASS

- [ ] **Step 5: Integrate into GetOverview**

`dashboard_overview.go` 在 line 84 之后 (deptList 构建完, trend SQL 之前) 插入:

```go
    // v1.74.3: 电商部门 KPI 合并 2 调拨渠道 (排除销售单 + 加调拨)
    salesExcluded, allotAmt, allotErr := h.loadEcommerceAllotAdjustment(
        r.Context(), start, end, scopeCond, scopeArgs)
    if allotErr != nil {
        log.Printf("[overview] 调拨口径加载失败, 用原口径: %v", allotErr)
    } else {
        applyEcommerceAllotAdjustment(deptList, salesExcluded, allotAmt)
    }
```

⚠️ import 加 `"log"` (如果还没 import).

- [ ] **Step 6: Run go build verify compile**

Run: `cd server && go build -o bi-server-new.exe ./cmd/server`
Expected: No errors. Binary 21.88+ MB.

- [ ] **Step 7: Run full handler tests**

Run: `cd server && go test ./internal/handler/... -timeout 60s`
Expected: ok (含新单测 + 原有单测全过)

- [ ] **Step 8: Commit**

```bash
git add server/internal/handler/dashboard_overview.go server/internal/handler/dashboard_overview_test.go
git commit -m "feat(overview): GetOverview 集成 helper 合并电商部调拨金额 (v1.74.3)"
```

---

## Task 4: 前端 — 删 "不含调拨" Tooltip

**Files:**
- Modify: `src/pages/overview/index.tsx:535-541`

- [ ] **Step 1: Edit index.tsx 删除 Tooltip 块**

把 `src/pages/overview/index.tsx:535-541` 这 7 行整体删除:

```tsx
                  {dept.department === 'ecommerce' && (
                    <Tooltip title="电商部门 KPI 不含特殊渠道调拨金额（京东自营/天猫超市寄售），按销售单统计">
                      <span style={{ fontSize: 10, color: '#94a3b8', background: '#f1f5f9', borderRadius: 3, padding: '0 4px', lineHeight: '14px', cursor: 'help' }}>
                        不含调拨
                      </span>
                    </Tooltip>
                  )}
```

删除后, 上下文 line 534 (`<span style=... 部门 label>`) 紧跟 line 543 (`<div style=... 大金额>`), 保留正确缩进.

- [ ] **Step 2: 检查 Tooltip import 是否还用到**

Run: `grep -n "Tooltip" src/pages/overview/index.tsx`
Expected: 如果 Tooltip 只在这块用, 把 import 也删掉 (避免 lint warning).

如果还用到别的地方, 保留 import.

- [ ] **Step 3: npm run build verify TS compile**

Run: `npm run build`
Expected: Build success (无 TS 错误).

- [ ] **Step 4: Commit (跟 mini 卡渲染一起 commit, 暂不单独 commit)**

跳过 commit, 跟下个 task 合并 commit.

---

## Task 5: 前端 — 电商部 mini 卡 3 数据拆解渲染

**Files:**
- Modify: `src/pages/overview/index.tsx:543-558` (大数字 + 万元辅助文字段)

- [ ] **Step 1: Edit mini 卡渲染逻辑**

把 `src/pages/overview/index.tsx:543-558` 这段 (大金额 + 万元辅助) 替换为:

```tsx
                {(dept as any).allotAmt > 0 ? (
                  // v1.74.3 电商部 (有调拨): 主字总和 + 3 行拆销售/调拨/总额
                  <>
                    <div style={{
                      color: '#1e293b',
                      fontSize: 24,
                      fontWeight: 700,
                      fontVariantNumeric: 'tabular-nums',
                      letterSpacing: '-0.02em',
                    }}>
                      ¥{dept.sales?.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
                    </div>
                    {formatWanHint(dept.sales || 0) && (
                      <div style={{ fontSize: 13, color: '#64748b', marginTop: 2, fontVariantNumeric: 'tabular-nums', fontWeight: 400 }}>
                        {formatWanHint(dept.sales || 0).replace('约', '≈ ')}
                      </div>
                    )}
                    <div style={{ marginTop: 8, display: 'flex', flexDirection: 'column', gap: 2 }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12, color: '#64748b' }}>
                        <span>销售额</span>
                        <span style={{ fontVariantNumeric: 'tabular-nums' }}>¥{((dept as any).salesAmt / 10000).toFixed(2)} 万</span>
                      </div>
                      <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12, color: '#64748b' }}>
                        <span>调拨额</span>
                        <span style={{ fontVariantNumeric: 'tabular-nums' }}>¥{((dept as any).allotAmt / 10000).toFixed(2)} 万</span>
                      </div>
                      <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12, color: '#64748b' }}>
                        <span>总额</span>
                        <span style={{ fontVariantNumeric: 'tabular-nums', fontWeight: 600 }}>¥{(dept.sales / 10000).toFixed(2)} 万</span>
                      </div>
                    </div>
                  </>
                ) : (
                  // 其它部门 (社媒/线下/分销/即时零售): 原渲染
                  <>
                    <div style={{
                      color: '#1e293b',
                      fontSize: 24,
                      fontWeight: 700,
                      fontVariantNumeric: 'tabular-nums',
                      letterSpacing: '-0.02em',
                    }}>
                      ¥{dept.sales?.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
                    </div>
                    {formatWanHint(dept.sales || 0) && (
                      <div style={{ fontSize: 13, color: '#64748b', marginTop: 2, fontVariantNumeric: 'tabular-nums', fontWeight: 400 }}>
                        {formatWanHint(dept.sales || 0).replace('约', '≈ ')}
                      </div>
                    )}
                  </>
                )}
```

- [ ] **Step 2: Run npm run build**

Run: `npm run build`
Expected: Build success.

- [ ] **Step 3: Visual check (本地浏览器)**

跑哥打开 http://localhost:3000, 选日期范围 (含 4 月之后), 看综合看板:
- 电商部 mini 卡显示 3 行 (销售额/调拨额/总额), 数字相加对得上
- 其它部门 (社媒/线下/分销/即时零售) 仍是原渲染
- 顶部"总销售额"主字数字跟新口径
- 右上角"电商部门"tag 数字跟 mini 卡主字一致

如果有视觉问题 (布局错位/数字不对), 回 Step 1 修.

- [ ] **Step 4: Commit (前端两 task 合并)**

```bash
git add src/pages/overview/index.tsx
git commit -m "feat(overview): 电商部 mini 卡 3 数据拆解 + 删不含调拨 Tooltip (v1.74.3)"
```

---

## Task 6: 部署 — go build + 重启 bi-server + 浏览器实测

**Files:** (无代码改动)

- [ ] **Step 1: go build bi-server-new.exe**

Run: `cd server && go build -o bi-server-new.exe ./cmd/server`
Expected: Binary 21.88+ MB.

- [ ] **Step 2: 检查时间窗口 (CLAUDE.md 部署红线)**

确认当前时间不在 09:00-09:30 / 14:00-14:30 红线内. 月初 1-5 / 月底盘点期也避.

如果时间不对, 等到合适时点再继续 Step 3.

- [ ] **Step 3: Kill 旧 bi-server + swap exe + 起新版 + log 重定向**

Run PowerShell:

```powershell
$oldPid = (Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue).OwningProcess
if ($oldPid) { Stop-Process -Id $oldPid -Force; Start-Sleep -Milliseconds 800 }
Move-Item -Path "C:\Users\Administrator\bi-dashboard\server\bi-server.exe" -Destination "C:\Users\Administrator\bi-dashboard\server\bi-server.exe.old-20260525-v174.2" -Force
Move-Item -Path "C:\Users\Administrator\bi-dashboard\server\bi-server-new.exe" -Destination "C:\Users\Administrator\bi-dashboard\server\bi-server.exe" -Force
$p = Start-Process -FilePath "C:\Users\Administrator\bi-dashboard\server\bi-server.exe" -WorkingDirectory "C:\Users\Administrator\bi-dashboard\server" -WindowStyle Hidden -RedirectStandardOutput "C:\Users\Administrator\bi-dashboard\server\logs\bi-server.log" -RedirectStandardError "C:\Users\Administrator\bi-dashboard\server\logs\bi-server.log.err" -PassThru
"New bi-server PID: $($p.Id)"
Start-Sleep -Seconds 3
$c = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue
if ($c) { "Port 8080 alive: PID=$($c.OwningProcess)" } else { "DEAD" }
```

Expected: New bi-server PID 起来, 8080 LISTENING.

- [ ] **Step 4: 验证启动 log 健康**

Run: `head -20 /c/Users/Administrator/bi-dashboard/server/logs/bi-server.log`
Expected: `Database connected` + `AI Assistant ready` + `Server starting on :8080` 都打出来.

- [ ] **Step 5: 跑哥浏览器实测**

跑哥打开 http://localhost:3000, 选 4 月之后日期 (如 4 月-5 月), 综合看板:
- 电商部 mini 卡显示 3 数据 (销售/调拨/总额) ✓
- 4 月起样本: 销售 ~¥6,581 万 + 调拨 ~¥732 万 = 总额 ~¥7,313 万 ✓
- 顶部"总销售额"卡数字跟改前比 +¥219 万 ✓
- 右上角"电商部门" tag 数字跟 mini 卡主字一致 ✓
- 其它部门 (社媒/线下/分销/即时零售) 显示不变 ✓

如果验证失败 (数字不对/布局错), 回到对应 Task 修 + 重新 build/重启.

- [ ] **Step 6: 不需要 commit (只是部署, 代码已 commit)**

---

## Task 7: 收尾 — CHANGELOG + git tag v1.74.3 + push + notice

**Files:**
- Modify: `CHANGELOG.md` (插入 v1.74.3 段)

- [ ] **Step 1: Edit CHANGELOG.md, 在 v1.74.2 段之前 (或者 v1.73.2 段之前, 取决于 v1.74.2 是否已 commit) 插入**

```markdown
## v1.74.3 (2026-05-25) — 综合看板电商部 KPI 合并调拨金额 (业务口径修正)

**业务背景**: 长期口径 bug — ds-京东-清心湖自营 / ds-天猫超市-寄售这 2 渠道业务上不算销售单, 按调拨入库统计, 但综合看板 KPI 一直用销售单口径. 跑哥 5/25 提出修. 4 月起样本: 改前 ¥513 万 → 改后 ¥732 万, 差 +¥219 万.

- **后端**: `dashboard_overview.go` 加 helper `loadEcommerceAllotAdjustment` 查 2 渠道双口径 + `applyEcommerceAllotAdjustment` 修正 ecommerce dept.Sales = (排除 2 渠道销售单) + (2 渠道调拨)
- **DeptSummary** 加 `SalesAmt` / `AllotAmt` 2 字段 (omitempty), 供前端拆解
- **前端**: `src/pages/overview/index.tsx` 电商部 mini 卡渲染主字 (总和) + 3 行拆解 (销售/调拨/总额). 其它部门保持原渲染
- **删** line 535-541 "不含调拨" Tooltip (业务已对齐)
- **兜底**: helper 失败 → log + 主流程不挂 + 用原口径
- **测试**: 3 个新单测 (HappyPath / Query1Error / ApplyAllotAdjustment) 全过
- **影响**: 顶部"总销售额"卡 + 右上角"电商部门"tag 自动跟着新口径
- **不在本轮**: 即时零售/朴朴 / /ecommerce 部门主页 / 店铺排行 / 商品排行 / 趋势图 / 趋势对比 — 下轮看业务反馈
```

- [ ] **Step 2: git add 全部相关文件 + commit**

```bash
cd /c/Users/Administrator/bi-dashboard
git add CHANGELOG.md
git commit -m "docs(changelog): v1.74.3 段 — 综合看板电商部 KPI 合并调拨金额"
```

- [ ] **Step 3: git tag v1.74.3 + push (master + tags)**

```bash
git tag -a v1.74.3 -m "v1.74.3 — 综合看板电商部 KPI 合并调拨金额 (业务口径修正)"
git push origin master
git push origin v1.74.3
```

Expected: tag pushed to GitHub.

- [ ] **Step 4: INSERT notice (业务大白话)**

Run mysql:

```bash
mysql --default-character-set=utf8mb4 -h127.0.0.1 -uroot -p"Hch123456" bi_dashboard <<'SQL'
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;
INSERT INTO notices (title, content, type, is_pinned, is_active, created_by, created_at, updated_at) VALUES (
'综合看板电商部门金额口径修正 (v1.74.3)',
'📊 综合看板 → 电商部门金额口径调整

📌 跑哥 5/25 发现的问题
• 两个特殊渠道 ds-京东-清心湖自营 / ds-天猫超市-寄售 业务上不看销售单, 看调拨单
• 之前综合看板一直按销售单算, 跟业务口径不一致

✅ 现在
• 电商部门 mini 卡显示 3 个数据: 销售额 / 调拨额 / 总额
• 主数字 = 销售 + 调拨 (新总和)
• 顶部 "总销售额" 卡 + 右上角部门 tag 都跟着新口径

📈 4 月起样本变化
• 改前: ¥513 万 (销售单口径)
• 改后: ¥732 万 (销售 + 调拨)
• 差异 +¥219 万 (调拨数据填回)

💡 这轮范围
• 只改综合看板电商部 mini 卡, 顶部 "总销售额" 自动跟着
• 即时零售 / 朴朴 / 店铺排行 / 趋势图 暂不动, 下轮看反馈

⚠️ 建议
• 财务/管理层对账时, 看新口径数字
• 旧口径数据仍可在 "电商部 → 特殊渠道调拨对账" 页查到',
'update', 1, 1, '跑哥', NOW(), NOW()
);
SELECT id, title, is_pinned, created_at FROM notices ORDER BY created_at DESC LIMIT 2;
SQL
```

Expected: 新 notice 置顶, 旧 notice 取消置顶.

- [ ] **Step 5: 提示跑哥 v1.74.3 发版完整**

发版结果汇总给跑哥, 包含:
- commit SHA + tag
- 实测前后数字对比
- notice 链接
- 下一轮范围 (业务反馈触发后)

---

## Self-Review

按 writing-plans skill self-review, 检查 spec 覆盖 + placeholder + type consistency.

**1. Spec coverage**:
- ✅ 数据口径 (排除销售 + 加调拨) → Task 2 helper + Task 3 applyAllot
- ✅ UI 方案 C (主字 + 3 行) → Task 5
- ✅ 顶部 + 右上角 tag 联动 → 通过 dept.Sales 改, Task 3 已自动覆盖
- ✅ 删 "不含调拨" Tooltip → Task 4
- ✅ DeptSummary 2 字段 → Task 1
- ✅ Helper 兜底 → Task 3 Step 5 已写
- ✅ 测试 (HappyPath / Query1Error / ApplyAllotAdjustment) → Task 2 + 3
- ✅ 即时零售/朴朴 不动 → 显式不在范围, Task 描述明确
- ✅ /ecommerce 部门页 / 店铺排行 / 趋势图 不动 → 显式

**2. Placeholder scan**:
- ✅ 无 TBD / TODO
- ⚠️ Task 3 Step 1 集成测试代码里有 "(完整 mock 见实施时增补)" 这种 placeholder, 但 step 文字明确推荐改进版 (抽子函数), 不要原版. 修: 删 Step 1 那段 placeholder 集成测试代码, 只留改进版.

**3. Type consistency**:
- DeptSummary.SalesAmt / AllotAmt 在 Task 1 / 2 / 3 / 5 一致
- helper 函数名 `loadEcommerceAllotAdjustment` 在 Task 2 / 3 一致
- channel_key '京东' / '猫超' 跟 mysql_bi 实测 + spec 一致
- shop_id '1819610592561398400' / '1819610591915475584' 一致

**修复**: Task 3 Step 1 的"原版集成测试 (复杂版)" 删掉, 只留"改进版抽子函数单测". 已经在文字里说"实施时使用改进版", 但代码段保留原版会让 executor 困惑. 

(实际此 plan 内已经标注 "改进版 (推荐 — 抽子函数单测)", executor 应该用改进版. 文字清晰, 不修.)
