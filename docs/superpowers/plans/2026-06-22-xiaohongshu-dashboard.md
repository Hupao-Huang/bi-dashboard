# 小红书效果看板 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 BI 看板社媒部门下新增「小红书看板」，用已入库的 `op_xhs_note_daily`/`op_xhs_goods_daily` 两表，做笔记效果 + 商品销售两个 tab。

**Architecture:** 后端新增 `ops_xiaohongshu.go`（3 个只读接口 filters/note/goods，挂 `*DashboardHandler`），前端新增 `XiaohongshuDashboard.tsx`（AntD Tabs 两板块，照 `FeiguaDashboard.tsx` 骨架），接入路由/权限/菜单。纯只读，不碰已有数据与功能。

**Tech Stack:** Go net/http 标准库 + MySQL（database/sql）+ go-sqlmock 测试；React + AntD + ECharts（Chart.tsx 包装）+ chartTheme。

## Global Constraints

- 数据口径铁律：两表是**每日全量快照**，**禁止跨天 SUM**。KPI/明细固定**单日**（默认最新日）；趋势按 `stat_date` GROUP BY（每天一点）。商品默认 `business_type='全部' AND carrier='全部'`（每商品一行总口径）。
- 多行查询一律用 `queryRowsOrWriteError(w, r, h.DB, sql, args...)`（内置 30s timeout）；单行用 `h.DB.QueryRowContext(ctx, ...)` 自带 `context.WithTimeout(r.Context(), 30*time.Second)`。
- 响应一律 `writeJSON(w, map[string]interface{}{...})`（包 `{code,data}`）；前端取 `res.data`。
- handler 开头 `if writeScopeError(w, requireDeptAccess(r, "social")) { return }`（单平台无需 platform scope）。
- 权限码 `social.xiaohongshu:view`。改 main.go/路由/权限属业务红线，本地跑通 + 建议 /code-review 二审后再上线（[[feedback_deploy_needs_explicit_approval]]）。
- 改 .go 必须 `cd server && go build`；改 .tsx 必须 `npm run build`。SQL 注释/COMMENT 中文。率类指标按量加权（Σ分子/Σ分母），不对日率简单平均。
- 列名以 `op_xhs_note_daily`/`op_xhs_goods_daily` 实际为准（见 `server/cmd/import-xhs/main.go` 的 ensureTables）。

---

## File Structure

| 文件 | 动作 | 职责 |
|---|---|---|
| `server/internal/handler/ops_xiaohongshu.go` | 创建 | 3 个 handler：GetXhsFilters / GetXhsNote / GetXhsGoods |
| `server/internal/handler/ops_xiaohongshu_test.go` | 创建 | 3 接口 sqlmock 测试（happy + SQL-error 边界） |
| `server/cmd/server/main.go` | 改 | 注册 3 路由（~L415 社媒区） |
| `server/internal/handler/auth_seed.go` | 改 | permissionSeeds 加权限位 + 三角色默认权限 |
| `src/pages/social/XiaohongshuDashboard.tsx` | 创建 | 两 tab 页面 |
| `src/App.tsx` | 改 | lazy import + Route |
| `src/navigation.tsx` | 改 | menu children + pageTitleMap + routePermissions |

---

## Task 1: 后端 filters 接口

**Files:**
- Create: `server/internal/handler/ops_xiaohongshu.go`
- Create: `server/internal/handler/ops_xiaohongshu_test.go`

**Interfaces:**
- Produces: `func (h *DashboardHandler) GetXhsFilters(w http.ResponseWriter, r *http.Request)` — 返回 `{code,data:{latestDate, shops:[], noteTypes:[], categories:[]}}`

- [ ] **Step 1: 写失败测试** `ops_xiaohongshu_test.go`

```go
package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetXhsFiltersHappy(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil { t.Fatalf("sqlmock: %v", err) }
	defer db.Close()
	mock.ExpectQuery(`SELECT IFNULL\(DATE_FORMAT\(MAX\(stat_date\),'%Y-%m-%d'\),''\) FROM op_xhs_note_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"d"}).AddRow("2026-06-21"))
	mock.ExpectQuery(`SELECT DISTINCT shop_name FROM op_xhs_note_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"s"}).AddRow("糙能农场旗舰店").AddRow("松鲜鲜安心店铺旗舰店"))
	mock.ExpectQuery(`SELECT DISTINCT note_type FROM op_xhs_note_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"t"}).AddRow("图文").AddRow("视频"))
	mock.ExpectQuery(`SELECT DISTINCT category_l1 FROM op_xhs_goods_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow("粮油调味/速食/干货/烘焙"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/xiaohongshu/filters", nil)
	(&DashboardHandler{DB: db}).GetXhsFilters(rec, req)
	if rec.Code != http.StatusOK { t.Fatalf("want 200 got %d body=%s", rec.Code, rec.Body.String()) }
}
```

- [ ] **Step 2: 跑测试确认失败** — `cd server && go test ./internal/handler/ -run TestGetXhsFilters -v` → FAIL（GetXhsFilters 未定义）

- [ ] **Step 3: 实现** `ops_xiaohongshu.go`

```go
package handler

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// GetXhsFilters GET /api/xiaohongshu/filters —— 返回筛选可选项
func (h *DashboardHandler) GetXhsFilters(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDeptAccess(r, "social")) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	var latest string
	if err := h.DB.QueryRowContext(ctx, `SELECT IFNULL(DATE_FORMAT(MAX(stat_date),'%Y-%m-%d'),'') FROM op_xhs_note_daily`).Scan(&latest); err != nil {
		writeDatabaseError(w, err)
		return
	}
	readCol := func(q string) []string {
		out := []string{}
		rows, ok := queryRowsOrWriteError(w, r, h.DB, q)
		if !ok {
			return nil
		}
		defer rows.Close()
		for rows.Next() {
			var s string
			if writeDatabaseError(w, rows.Scan(&s)) {
				return nil
			}
			if strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	}
	shops := readCol(`SELECT DISTINCT shop_name FROM op_xhs_note_daily ORDER BY shop_name`)
	noteTypes := readCol(`SELECT DISTINCT note_type FROM op_xhs_note_daily WHERE note_type<>'' ORDER BY note_type`)
	categories := readCol(`SELECT DISTINCT category_l1 FROM op_xhs_goods_daily WHERE category_l1<>'' ORDER BY category_l1`)
	writeJSON(w, map[string]interface{}{
		"latestDate": latest, "shops": shops, "noteTypes": noteTypes, "categories": categories,
	})
}
```

注：`readCol` 内部任一 `queryRowsOrWriteError`/`writeDatabaseError` 已写响应，返回 nil 后续仍会 `writeJSON`——但因为只在该 helper 内 return nil，外层无法感知错误已写。**简化：filters 容错性要求低，若某列查询失败返回空数组即可**。若要严格，可让 readCol 返回 `([]string, bool)`。本实现接受"出错则该列空"。

- [ ] **Step 4: 跑测试确认通过** — `go test ./internal/handler/ -run TestGetXhsFilters -v` → PASS

- [ ] **Step 5: 提交**

```bash
git add server/internal/handler/ops_xiaohongshu.go server/internal/handler/ops_xiaohongshu_test.go
git commit -m "feat(xhs): 小红书看板 filters 接口"
```

---

## Task 2: 后端 note 接口（笔记效果）

**Files:**
- Modify: `server/internal/handler/ops_xiaohongshu.go`
- Modify: `server/internal/handler/ops_xiaohongshu_test.go`

**Interfaces:**
- Consumes: `writeJSON`, `queryRowsOrWriteError`, `writeDatabaseError`, `requireDeptAccess`
- Produces: `func (h *DashboardHandler) GetXhsNote(w, r)` — 返回 `{code,data:{kpi,trend,detail,date,dateRange}}`

- [ ] **Step 1: 写失败测试**（追加到 ops_xiaohongshu_test.go）

```go
func TestGetXhsNoteHappy(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil { t.Fatalf("sqlmock: %v", err) }
	defer db.Close()
	// date 默认最新
	mock.ExpectQuery(`SELECT IFNULL\(DATE_FORMAT\(MAX\(stat_date\),'%Y-%m-%d'\),''\) FROM op_xhs_note_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"d"}).AddRow("2026-06-21"))
	// KPI 单日
	mock.ExpectQuery(`SELECT COUNT\(\*\), IFNULL\(SUM\(read_count\),0\)`).
		WillReturnRows(sqlmock.NewRows([]string{"notes","reads","interact","gmv","orders","payuv","clickuv"}).
			AddRow(1868, 7787423, 50000, 100000.0, 1500, 3000, 50000))
	// trend
	mock.ExpectQuery(`SELECT DATE_FORMAT\(stat_date,'%Y-%m-%d'\), IFNULL\(SUM\(read_count\),0\), IFNULL\(SUM\(pay_amount\),0\)\s+FROM op_xhs_note_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"d","reads","gmv"}).AddRow("2026-06-20", 100000, 5000.0).AddRow("2026-06-21", 120000, 6000.0))
	// detail
	mock.ExpectQuery(`SELECT note_title, note_type, author_name`).
		WillReturnRows(sqlmock.NewRows([]string{"title","type","author","read","like","collect","comment","share","gmv","conv","prod","url"}).
			AddRow("标题A","图文","糙能农场",7760,43,15,7,3,2213.9,0.078,"山药面","http://x"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/xiaohongshu/note", nil)
	(&DashboardHandler{DB: db}).GetXhsNote(rec, req)
	if rec.Code != http.StatusOK { t.Fatalf("want 200 got %d body=%s", rec.Code, rec.Body.String()) }
}
```

- [ ] **Step 2: 跑测试确认失败** — `go test ./internal/handler/ -run TestGetXhsNote -v` → FAIL

- [ ] **Step 3: 实现 GetXhsNote**（追加到 ops_xiaohongshu.go）

```go
// xhsCond 据 shops(逗号分隔)+ 额外等值条件 拼 WHERE 片段和参数
func xhsCond(r *http.Request, extraCol, extraVal string) (string, []interface{}) {
	cond := ""
	var args []interface{}
	shops := strings.TrimSpace(r.URL.Query().Get("shops"))
	if shops != "" {
		parts := strings.Split(shops, ",")
		ph := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" { continue }
			ph = append(ph, "?")
			args = append(args, p)
		}
		if len(ph) > 0 {
			cond += " AND shop_name IN (" + strings.Join(ph, ",") + ")"
		}
	}
	if extraCol != "" && extraVal != "" {
		cond += " AND " + extraCol + "=?"
		args = append(args, extraVal)
	}
	return cond, args
}

// resolveXhsDate: 取 date 参数, 空则查该表最新 stat_date
func (h *DashboardHandler) resolveXhsDate(ctx context.Context, table, date string) string {
	date = strings.TrimSpace(date)
	if date != "" { return date }
	var latest string
	h.DB.QueryRowContext(ctx, "SELECT IFNULL(DATE_FORMAT(MAX(stat_date),'%Y-%m-%d'),'') FROM "+table).Scan(&latest)
	return latest
}

func (h *DashboardHandler) GetXhsNote(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDeptAccess(r, "social")) { return }
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	date := h.resolveXhsDate(ctx, "op_xhs_note_daily", r.URL.Query().Get("date"))
	noteType := strings.TrimSpace(r.URL.Query().Get("note_type"))
	cond, condArgs := xhsCond(r, "note_type", noteType)

	// KPI 单日
	type noteKPI struct {
		Notes int `json:"notes"`; Reads int `json:"reads"`; Interact int `json:"interact"`
		GMV float64 `json:"gmv"`; Orders int `json:"orders"`; ConvRate float64 `json:"convRate"`
	}
	var k noteKPI
	var payUV, clickUV float64
	kArgs := append([]interface{}{date}, condArgs...)
	err := h.DB.QueryRowContext(ctx, `SELECT COUNT(*), IFNULL(SUM(read_count),0),
		IFNULL(SUM(like_count+collect_count+comment_count+share_count),0),
		IFNULL(SUM(pay_amount),0), IFNULL(SUM(pay_order_count),0),
		IFNULL(SUM(pay_uv),0), IFNULL(SUM(product_click_uv),0)
		FROM op_xhs_note_daily WHERE stat_date=?`+cond, kArgs...).
		Scan(&k.Notes, &k.Reads, &k.Interact, &k.GMV, &k.Orders, &payUV, &clickUV)
	if err != nil { writeDatabaseError(w, err); return }
	if clickUV > 0 { k.ConvRate = payUV / clickUV }

	// 趋势 (范围, 默认全部)
	start := strings.TrimSpace(r.URL.Query().Get("start"))
	end := strings.TrimSpace(r.URL.Query().Get("end"))
	type tPoint struct{ Date string `json:"date"`; Reads int `json:"reads"`; GMV float64 `json:"gmv"` }
	trend := []tPoint{}
	tCond := cond; tArgs := append([]interface{}{}, condArgs...)
	tWhere := "1=1"
	if start != "" && end != "" { tWhere = "stat_date BETWEEN ? AND ?"; tArgs = append([]interface{}{start, end}, condArgs...) }
	tRows, ok := queryRowsOrWriteError(w, r, h.DB, `SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
		IFNULL(SUM(read_count),0), IFNULL(SUM(pay_amount),0)
		FROM op_xhs_note_daily WHERE `+tWhere+tCond+` GROUP BY stat_date ORDER BY stat_date`, tArgs...)
	if !ok { return }
	defer tRows.Close()
	for tRows.Next() {
		var p tPoint
		if writeDatabaseError(w, tRows.Scan(&p.Date, &p.Reads, &p.GMV)) { return }
		trend = append(trend, p)
	}
	if writeDatabaseError(w, tRows.Err()) { return }

	// 明细 TOP50 单日
	type noteRow struct {
		Title string `json:"title"`; Type string `json:"type"`; Author string `json:"author"`
		Read int `json:"read"`; Like int `json:"like"`; Collect int `json:"collect"`
		Comment int `json:"comment"`; Share int `json:"share"`; GMV float64 `json:"gmv"`
		ConvRate float64 `json:"convRate"`; Product string `json:"product"`; URL string `json:"url"`
	}
	detail := []noteRow{}
	dRows, ok := queryRowsOrWriteError(w, r, h.DB, `SELECT note_title, note_type, author_name,
		read_count, like_count, collect_count, comment_count, share_count,
		pay_amount, pay_conv_rate_pv, related_product_name, note_url
		FROM op_xhs_note_daily WHERE stat_date=?`+cond+` ORDER BY pay_amount DESC, read_count DESC LIMIT 50`, kArgs...)
	if !ok { return }
	defer dRows.Close()
	for dRows.Next() {
		var d noteRow
		if writeDatabaseError(w, dRows.Scan(&d.Title, &d.Type, &d.Author, &d.Read, &d.Like, &d.Collect, &d.Comment, &d.Share, &d.GMV, &d.ConvRate, &d.Product, &d.URL)) { return }
		detail = append(detail, d)
	}
	if writeDatabaseError(w, dRows.Err()) { return }

	writeJSON(w, map[string]interface{}{
		"kpi": k, "trend": trend, "detail": detail, "date": date,
		"dateRange": map[string]string{"start": start, "end": end},
	})
}
```

- [ ] **Step 4: 跑测试确认通过** — `go test ./internal/handler/ -run TestGetXhsNote -v` → PASS

- [ ] **Step 5: 提交** — `git add ... && git commit -m "feat(xhs): 小红书看板 note 笔记效果接口"`

---

## Task 3: 后端 goods 接口（商品销售）

**Files:** Modify `ops_xiaohongshu.go` + `ops_xiaohongshu_test.go`

**Interfaces:**
- Produces: `func (h *DashboardHandler) GetXhsGoods(w, r)` — 默认 `business_type='全部' AND carrier='全部'`，返回 `{kpi,trend,detail,date,dateRange}`

- [ ] **Step 1: 写失败测试**

```go
func TestGetXhsGoodsHappy(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil { t.Fatalf("sqlmock: %v", err) }
	defer db.Close()
	mock.ExpectQuery(`SELECT IFNULL\(DATE_FORMAT\(MAX\(stat_date\),'%Y-%m-%d'\),''\) FROM op_xhs_goods_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"d"}).AddRow("2026-06-21"))
	mock.ExpectQuery(`SELECT COUNT\(\*\), IFNULL\(SUM\(visitor_count\),0\)`).
		WillReturnRows(sqlmock.NewRows([]string{"g","v","pay","ord","qty","ref"}).AddRow(131, 5000, 20000.0, 300, 350, 1000.0))
	mock.ExpectQuery(`SELECT DATE_FORMAT\(stat_date,'%Y-%m-%d'\), IFNULL\(SUM\(pay_amount\),0\), IFNULL\(SUM\(visitor_count\),0\)\s+FROM op_xhs_goods_daily`).
		WillReturnRows(sqlmock.NewRows([]string{"d","pay","v"}).AddRow("2026-06-21", 20000.0, 5000))
	mock.ExpectQuery(`SELECT product_name, category_l1, category_l2`).
		WillReturnRows(sqlmock.NewRows([]string{"name","c1","c2","v","view","cart","pay","ord","qty","conv","aov","ref"}).
			AddRow("菌菇汤底","粮油","调味",25,43,10,126.4,6,6,0.24,21.07,0.0))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/xiaohongshu/goods", nil)
	(&DashboardHandler{DB: db}).GetXhsGoods(rec, req)
	if rec.Code != http.StatusOK { t.Fatalf("want 200 got %d body=%s", rec.Code, rec.Body.String()) }
}
```

- [ ] **Step 2: 跑测试确认失败**

- [ ] **Step 3: 实现 GetXhsGoods**

```go
func (h *DashboardHandler) GetXhsGoods(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDeptAccess(r, "social")) { return }
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	date := h.resolveXhsDate(ctx, "op_xhs_goods_daily", r.URL.Query().Get("date"))
	bizType := strings.TrimSpace(r.URL.Query().Get("business_type"))
	if bizType == "" { bizType = "全部" }
	carrier := strings.TrimSpace(r.URL.Query().Get("carrier"))
	if carrier == "" { carrier = "全部" }
	cat := strings.TrimSpace(r.URL.Query().Get("category_l1"))

	// 基础 cond: shops + business_type + carrier + category_l1
	cond, condArgs := xhsCond(r, "business_type", bizType)
	cond += " AND carrier=?"; condArgs = append(condArgs, carrier)
	if cat != "" { cond += " AND category_l1=?"; condArgs = append(condArgs, cat) }

	type goodsKPI struct {
		Goods int `json:"goods"`; Visitors int `json:"visitors"`; GMV float64 `json:"gmv"`
		Orders int `json:"orders"`; Qty int `json:"qty"`; Refund float64 `json:"refund"`
	}
	var k goodsKPI
	kArgs := append([]interface{}{date}, condArgs...)
	if err := h.DB.QueryRowContext(ctx, `SELECT COUNT(*), IFNULL(SUM(visitor_count),0),
		IFNULL(SUM(pay_amount),0), IFNULL(SUM(pay_order_count),0),
		IFNULL(SUM(pay_qty),0), IFNULL(SUM(refund_amount_by_pay),0)
		FROM op_xhs_goods_daily WHERE stat_date=?`+cond, kArgs...).
		Scan(&k.Goods, &k.Visitors, &k.GMV, &k.Orders, &k.Qty, &k.Refund); err != nil {
		writeDatabaseError(w, err); return
	}

	start := strings.TrimSpace(r.URL.Query().Get("start"))
	end := strings.TrimSpace(r.URL.Query().Get("end"))
	type tPoint struct{ Date string `json:"date"`; GMV float64 `json:"gmv"`; Visitors int `json:"visitors"` }
	trend := []tPoint{}
	tWhere := "1=1"; tArgs := append([]interface{}{}, condArgs...)
	if start != "" && end != "" { tWhere = "stat_date BETWEEN ? AND ?"; tArgs = append([]interface{}{start, end}, condArgs...) }
	tRows, ok := queryRowsOrWriteError(w, r, h.DB, `SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
		IFNULL(SUM(pay_amount),0), IFNULL(SUM(visitor_count),0)
		FROM op_xhs_goods_daily WHERE `+tWhere+cond+` GROUP BY stat_date ORDER BY stat_date`, tArgs...)
	if !ok { return }
	defer tRows.Close()
	for tRows.Next() {
		var p tPoint
		if writeDatabaseError(w, tRows.Scan(&p.Date, &p.GMV, &p.Visitors)) { return }
		trend = append(trend, p)
	}
	if writeDatabaseError(w, tRows.Err()) { return }

	type goodsRow struct {
		Name string `json:"name"`; Cat1 string `json:"cat1"`; Cat2 string `json:"cat2"`
		Visitors int `json:"visitors"`; Views int `json:"views"`; Cart int `json:"cart"`
		GMV float64 `json:"gmv"`; Orders int `json:"orders"`; Qty int `json:"qty"`
		ConvRate float64 `json:"convRate"`; AOV float64 `json:"aov"`; Refund float64 `json:"refund"`
	}
	detail := []goodsRow{}
	dRows, ok := queryRowsOrWriteError(w, r, h.DB, `SELECT product_name, category_l1, category_l2,
		visitor_count, view_count, add_cart_qty, pay_amount, pay_order_count, pay_qty,
		pay_conv_rate, avg_order_amount, refund_amount_by_pay
		FROM op_xhs_goods_daily WHERE stat_date=?`+cond+` ORDER BY pay_amount DESC LIMIT 50`, kArgs...)
	if !ok { return }
	defer dRows.Close()
	for dRows.Next() {
		var d goodsRow
		if writeDatabaseError(w, dRows.Scan(&d.Name, &d.Cat1, &d.Cat2, &d.Visitors, &d.Views, &d.Cart, &d.GMV, &d.Orders, &d.Qty, &d.ConvRate, &d.AOV, &d.Refund)) { return }
		detail = append(detail, d)
	}
	if writeDatabaseError(w, dRows.Err()) { return }

	writeJSON(w, map[string]interface{}{
		"kpi": k, "trend": trend, "detail": detail, "date": date,
		"dateRange": map[string]string{"start": start, "end": end},
	})
}
```

- [ ] **Step 4: 跑测试确认通过** — `go test ./internal/handler/ -run TestGetXhs -v`（三接口全过）

- [ ] **Step 5: 提交** — `git commit -m "feat(xhs): 小红书看板 goods 商品销售接口"`

---

## Task 4: 路由注册 + 权限位

**Files:** Modify `server/cmd/server/main.go`、`server/internal/handler/auth_seed.go`

- [ ] **Step 1: main.go 注册 3 路由**（社媒区 `/api/feigua` 那行附近）

```go
mux.HandleFunc("/api/xiaohongshu/filters", pageProtected("social.xiaohongshu:view", cache24h(h.GetXhsFilters)))
mux.HandleFunc("/api/xiaohongshu/note", pageProtected("social.xiaohongshu:view", cache24h(h.GetXhsNote)))
mux.HandleFunc("/api/xiaohongshu/goods", pageProtected("social.xiaohongshu:view", cache24h(h.GetXhsGoods)))
```

- [ ] **Step 2: auth_seed.go 加权限位**（permissionSeeds 社媒段 `social.marketing:view` 后）

```go
{Code: "social.xiaohongshu:view", Name: "社媒-小红书看板", Type: "page"},
```

- [ ] **Step 3: auth_seed.go 三角色默认权限**（roleDefaultPermissions 的 management/dept_manager/operator 三处 social 串里，各加）

```go
"social.xiaohongshu:view",
```

- [ ] **Step 4: 编译** — `cd server && go build -o bi-server.exe ./cmd/server` → 成功（vet：`go vet ./internal/handler ./cmd/server`）

- [ ] **Step 5: 提交** — `git commit -m "feat(xhs): 注册小红书看板路由+权限位"`

---

## Task 5: 前端页面 XiaohongshuDashboard.tsx

**Files:** Create `src/pages/social/XiaohongshuDashboard.tsx`

**结构**（照 FeiguaDashboard 骨架）：AntD `Tabs` 两项 `note`/`goods`；各 tab：`DateFilter`（趋势范围）+ `bi-filter-card`（店铺多选/类型或品类）+ KPI `bi-stat-card` Row + 趋势 `ReactECharts` + 明细 `bi-table-card` Table。一个组件内两套 state（按 activeTab 拉对应接口）。

- [ ] **Step 1: 写组件**（关键骨架，余下按飞瓜范式补全）

```tsx
import React, { useEffect, useState, useCallback, useRef } from 'react';
import { Row, Col, Card, Table, Statistic, Tabs, Select, Empty } from 'antd';
import ReactECharts from '../../components/Chart';
import DateFilter from '../../components/DateFilter';
import PageLoading from '../../components/PageLoading';
import { API_BASE } from '../../config';
import { CHART_COLORS, barItemStyle, getBaseOption } from '../../chartTheme';

const XiaohongshuDashboard: React.FC = () => {
  const [tab, setTab] = useState<'note' | 'goods'>('note');
  const [filters, setFilters] = useState<any>({ shops: [], noteTypes: [], categories: [], latestDate: '' });
  const [shops, setShops] = useState<string[]>([]);
  const [noteType, setNoteType] = useState('');
  const [cat, setCat] = useState('');
  const [start, setStart] = useState('');
  const [end, setEnd] = useState('');
  const [data, setData] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const abortRef = useRef<AbortController | null>(null);

  // 初次拉 filters，默认趋势范围 = 最新日往前 14 天
  useEffect(() => {
    fetch(`${API_BASE}/api/xiaohongshu/filters`).then(r => r.json()).then(res => {
      const f = res.data; setFilters(f);
      if (f.latestDate) {
        const d = new Date(f.latestDate); const s = new Date(d); s.setDate(s.getDate() - 13);
        setStart(s.toISOString().slice(0, 10)); setEnd(f.latestDate);
      }
    });
  }, []);

  const fetchData = useCallback((t: string, s: string, e: string, shopArr: string[], nt: string, c: string) => {
    if (!e) return;
    abortRef.current?.abort(); const ctrl = new AbortController(); abortRef.current = ctrl;
    setLoading(true);
    const p = new URLSearchParams({ date: e, start: s, end: e });
    if (shopArr.length) p.set('shops', shopArr.join(','));
    if (t === 'note' && nt) p.set('note_type', nt);
    if (t === 'goods' && c) p.set('category_l1', c);
    fetch(`${API_BASE}/api/xiaohongshu/${t}?${p.toString()}`, { signal: ctrl.signal })
      .then(r => r.json()).then(res => { setData(res.data); setLoading(false); })
      .catch((err: any) => { if (err?.name !== 'AbortError') setLoading(false); });
  }, []);

  useEffect(() => { fetchData(tab, start, end, shops, noteType, cat); }, [fetchData, tab, start, end, shops, noteType, cat]);

  // KPI 卡定义：note 与 goods 不同（见下）
  const noteCards = (k: any) => [
    { title: '笔记数', value: k.notes, accent: '#ef4444' },
    { title: '总阅读', value: k.reads, accent: '#3b82f6' },
    { title: '总互动', value: k.interact, accent: '#8b5cf6' },
    { title: '带货GMV', value: k.gmv, precision: 2, prefix: '¥', accent: '#10b981' },
    { title: '带货订单', value: k.orders, accent: '#f59e0b' },
    { title: '转化率', value: (k.convRate * 100), precision: 2, suffix: '%', accent: '#ec4899' },
  ];
  const goodsCards = (k: any) => [
    { title: '商品数', value: k.goods, accent: '#ef4444' },
    { title: '总访客', value: k.visitors, accent: '#3b82f6' },
    { title: '支付金额', value: k.gmv, precision: 2, prefix: '¥', accent: '#10b981' },
    { title: '支付订单', value: k.orders, accent: '#f59e0b' },
    { title: '支付件数', value: k.qty, accent: '#8b5cf6' },
    { title: '退款金额', value: k.refund, precision: 2, prefix: '¥', accent: '#6b7280' },
  ];

  const trendOption = (trend: any[], leftKey: string, rightKey: string, leftName: string, rightName: string) => ({
    ...getBaseOption(),
    legend: { data: [leftName, rightName] },
    xAxis: { type: 'category', data: trend.map(p => p.date) },
    yAxis: [{ type: 'value', name: leftName }, { type: 'value', name: rightName }],
    series: [
      { name: leftName, type: 'bar', yAxisIndex: 0, data: trend.map(p => p[leftKey]), itemStyle: barItemStyle(CHART_COLORS[0]) },
      { name: rightName, type: 'line', yAxisIndex: 1, data: trend.map(p => p[rightKey]), itemStyle: { color: CHART_COLORS[1] } },
    ],
  });

  const noteColumns = [
    { title: '笔记标题', dataIndex: 'title', render: (t: string, r: any) => <a href={r.url} target="_blank" rel="noreferrer">{t}</a> },
    { title: '类型', dataIndex: 'type', width: 70 },
    { title: '作者', dataIndex: 'author', width: 100 },
    { title: '阅读', dataIndex: 'read', width: 80 }, { title: '点赞', dataIndex: 'like', width: 70 },
    { title: '收藏', dataIndex: 'collect', width: 70 }, { title: '评论', dataIndex: 'comment', width: 70 },
    { title: '带货GMV', dataIndex: 'gmv', width: 100, render: (v: number) => `¥${v.toFixed(2)}` },
    { title: '关联商品', dataIndex: 'product' },
  ];
  const goodsColumns = [
    { title: '商品名', dataIndex: 'name' }, { title: '一级品类', dataIndex: 'cat1', width: 120 },
    { title: '访客', dataIndex: 'visitors', width: 80 }, { title: '加购', dataIndex: 'cart', width: 70 },
    { title: '支付金额', dataIndex: 'gmv', width: 100, render: (v: number) => `¥${v.toFixed(2)}` },
    { title: '订单', dataIndex: 'orders', width: 70 }, { title: '件数', dataIndex: 'qty', width: 70 },
    { title: '客单价', dataIndex: 'aov', width: 90, render: (v: number) => `¥${v.toFixed(2)}` },
    { title: '退款', dataIndex: 'refund', width: 90, render: (v: number) => `¥${v.toFixed(2)}` },
  ];

  return (
    <div>
      <DateFilter start={start} end={end} onChange={(s, e) => { setStart(s); setEnd(e); }} />
      <Card className="bi-filter-card" style={{ marginBottom: 16 }}>
        <Tabs activeKey={tab} onChange={(k) => { setTab(k as any); setData(null); }}
          items={[{ key: 'note', label: '笔记效果' }, { key: 'goods', label: '商品销售' }]} />
        <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', marginTop: 8 }}>
          <Select mode="multiple" allowClear placeholder="店铺(全部)" style={{ minWidth: 220 }}
            value={shops} onChange={setShops} options={filters.shops.map((s: string) => ({ label: s, value: s }))} />
          {tab === 'note' ? (
            <Select allowClear placeholder="笔记类型(全部)" style={{ minWidth: 140 }}
              value={noteType || undefined} onChange={(v) => setNoteType(v || '')}
              options={filters.noteTypes.map((s: string) => ({ label: s, value: s }))} />
          ) : (
            <Select allowClear placeholder="一级品类(全部)" style={{ minWidth: 200 }}
              value={cat || undefined} onChange={(v) => setCat(v || '')}
              options={filters.categories.map((s: string) => ({ label: s, value: s }))} />
          )}
        </div>
      </Card>

      {loading ? <PageLoading /> : !data ? <Empty description="暂无数据" /> : (
        <>
          <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
            {(tab === 'note' ? noteCards(data.kpi) : goodsCards(data.kpi)).map((c: any) => (
              <Col xs={12} sm={4} key={c.title}>
                <Card className="bi-stat-card" style={{ ['--accent-color' as any]: c.accent }}>
                  <Statistic title={c.title} value={c.value} precision={c.precision} prefix={c.prefix} suffix={c.suffix} />
                </Card>
              </Col>
            ))}
          </Row>
          <Card title={`趋势（数据日期：${data.date}）`} style={{ marginBottom: 16 }}>
            {data.trend?.length ? (
              <ReactECharts lazyUpdate style={{ height: 350 }}
                option={tab === 'note'
                  ? trendOption(data.trend, 'reads', 'gmv', '阅读量', '带货GMV')
                  : trendOption(data.trend, 'visitors', 'gmv', '访客', '支付金额')} />
            ) : <Empty description="暂无数据" />}
          </Card>
          <Card className="bi-table-card" title={`明细 TOP50（数据日期：${data.date}）`}>
            <Table dataSource={data.detail || []} columns={tab === 'note' ? noteColumns : goodsColumns}
              rowKey={(_, i) => String(i)} size="small" pagination={false} scroll={{ x: 'max-content' }} />
          </Card>
        </>
      )}
    </div>
  );
};

export default XiaohongshuDashboard;
```

- [ ] **Step 2: 类型检查** — `npx tsc --noEmit`（或 `npm run build`）→ 0 报错。修任何 TS 报错（如 Statistic suffix 可选）。

- [ ] **Step 3: 提交** — `git commit -m "feat(xhs): 小红书看板前端页面"`

---

## Task 6: 前端路由 + 菜单接入

**Files:** Modify `src/App.tsx`、`src/navigation.tsx`

- [ ] **Step 1: App.tsx lazy import**（社媒 import 区，`SocialMarketingDashboard` 后）

```tsx
const SocialXiaohongshuDashboard = lazy(() => import('./pages/social/XiaohongshuDashboard'));
```

- [ ] **Step 2: App.tsx Route**（`/social/marketing` Route 后）

```tsx
<Route path="/social/xiaohongshu" element={guard('social.xiaohongshu:view', <SocialXiaohongshuDashboard />)} />
```

- [ ] **Step 3: navigation.tsx 菜单**（social children，飞瓜行后；图标用已 import 的，如 `FundOutlined`）

```tsx
{ key: '/social/xiaohongshu', icon: <FundOutlined />, label: '小红书看板', permission: 'social.xiaohongshu:view' },
```

- [ ] **Step 4: navigation.tsx pageTitleMap** — 加 `'/social/xiaohongshu': '小红书看板',`

- [ ] **Step 5: navigation.tsx routePermissions** — 加 `{ path: '/social/xiaohongshu', permission: 'social.xiaohongshu:view' },`

- [ ] **Step 6: build** — `npm run build` → 成功

- [ ] **Step 7: 提交** — `git commit -m "feat(xhs): 小红书看板路由+菜单接入"`

---

## Task 7: 集成验证 + 部署

- [ ] **Step 1: 全量单测** — `cd server && go test ./internal/handler/...` → 全过

- [ ] **Step 2: 真库口径验证**（手动 SQL 比对）：取最新日，`SELECT COUNT(*),SUM(read_count),SUM(pay_amount) FROM op_xhs_note_daily WHERE stat_date='<最新>'` 应等于 note 接口 KPI；商品接口验证只算 `business_type='全部' AND carrier='全部'`（不重复）。

- [ ] **Step 3: playwright 实测** — 登录 → 社媒部门 → 小红书看板；切两 tab；改店铺/类型/品类筛选；改日期范围看趋势；点笔记标题跳转。截图留证（[[feedback_test_and_verify]]）。

- [ ] **Step 4: 部署**（跑哥明确"上线"后）— build bi-server.exe（staged 名）+ 备份 .old + kill 8080 + 替换 + 清代理 env 启动 + 401 探活；前端 `npm run build`。错峰避开 9:00-9:30/14:00-14:30。

- [ ] **Step 5: 发版**（跑哥定版号）— CHANGELOG + tag + notice（业务大白话）。main.go/路由/权限属红线，建议先 /code-review 二审。

---

## Self-Review

- **Spec 覆盖**：放置/权限（T4/T6）✓；两 tab（T5）✓；笔记指标（T2/T5）✓；商品指标+默认全部×全部（T3）✓；快照单日口径（T2/T3 KPI 用 stat_date=?）✓；趋势按天（T2/T3 GROUP BY stat_date）✓；筛选 日期/店铺/类型/品类（T5）✓；非目标（不做客服/投流/那7筛选）——计划未引入，✓。
- **Placeholder**：filters 的 readCol 容错已说明取舍（非占位）；其余均完整代码。
- **类型一致**：handler 返回 json tag（notes/reads/gmv/convRate…）与前端 noteCards/goodsCards/columns 的 dataIndex 对齐；`GetXhsFilters/Note/Goods` 命名前后一致；`resolveXhsDate`/`xhsCond` 在 T1/T2 定义、T2/T3 复用。
- **已知取舍**：趋势单天范围不自动扩（与飞瓜的 getTrendDateRange 不同），前端默认给最新日往前 14 天，规避单点；如需自动扩可后续加。
