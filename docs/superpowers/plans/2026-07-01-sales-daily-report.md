# 销售日报(发货日报)Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 供应链菜单新增「销售日报」页,按发货日/4 个仓,展示 渠道汇总 / TOP10 单品 / TOP10 货品组合 三块(当日 + 当月累计),数据实时从吉客云发货订单库算。

**Architecture:** 两张映射维表(渠道、箱规托规)一次性从 Excel 导库;Go 后端一个只读接口按发货日实时查 `trade_*`/`trade_goods_*`,join 维表算三块;React 前端一页三表 + 发货日选择器。不建物化表(方案① MVP)。

**Tech Stack:** Go(标准库 net/http + database/sql + excelize 导入)、MySQL、React 19 + antd 6 + TypeScript(CRA)。

## Global Constraints(每个任务隐含遵守)

- **只算 4 个仓**:`warehouse_name IN ('南京委外成品仓-公司仓-委外','天津委外仓-公司仓-外仓','松鲜鲜&大地密码云仓','长沙委外成品仓-公司仓-外仓')`
- **只算销售**:`trade_type = 1`
- **发货日**:`trade.consign_time`,左闭右开区间
- **单瓶** = `sell_count × 箱规(box_qty)`,缺箱规 ×1
- **重量** = `trade.estimate_weight`(**单位克**,kg = ÷1000),订单级,勿与 trade_goods 行相乘
- 数据库注释写中文(项目规约)
- 前端不自改字体/字号/装饰色,用 antd 默认;改 .tsx 必须 `npm run build` 才生效
- Go build 必须 `cd server` 再 build;重启 bi-server 前先 `Remove-Item env:*PROXY*`
- MySQL 长查询加 timeout;`go build`/`go test` 在 `server/` 子目录跑
- 提交遵守约定:`<type>(<scope>): 主题`,不带版本号,结尾署 `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`

---

## 文件结构

**后端(server/)**
- `internal/store/report_maps.sql`(建表 DDL,新增)
- `cmd/import-report-maps/main.go`(Excel→维表导入 CLI,新增)
- `internal/handler/sales_daily_report.go`(接口 + 纯函数,新增)
- `internal/handler/sales_daily_report_test.go`(纯函数单测,新增)
- `internal/router`/`main.go`(注册路由,改)
- `internal/auth/auth_seed.go`(加权限位,改)

**前端(src/)**
- `pages/supply-chain/SalesDailyReport.tsx`(页面,新增)
- `pages/supply-chain/salesDailyReportColumns.ts`(表格列定义 + 纯格式化,新增,便于单测)
- `pages/supply-chain/salesDailyReportColumns.test.ts`(列/格式化单测,新增)
- `App.tsx`(路由,改)
- `navigation.tsx`(菜单,改)

**维表**
- `dim_sales_channel_map`(shop_name → channel → platform)
- `dim_goods_pack_spec`(goods_no → box_qty 箱规、pallet_box_qty 托规)

---

### Task 1: 维表 DDL + 平台分组纯函数

**Files:**
- Create: `server/internal/store/report_maps.sql`
- Create: `server/cmd/import-report-maps/platform.go`
- Test: `server/cmd/import-report-maps/platform_test.go`

**Interfaces:**
- Produces: `platformOf(channel string) string`(渠道→平台:社媒/电商/其他)

- [ ] **Step 1: 写建表 DDL**

`server/internal/store/report_maps.sql`:

```sql
-- 销售日报渠道映射(店铺 → 渠道 → 平台)
CREATE TABLE IF NOT EXISTS dim_sales_channel_map (
  shop_name VARCHAR(200) NOT NULL COMMENT '店铺名(= 销售渠道I,对 trade.shop_name)',
  channel   VARCHAR(50)  NOT NULL COMMENT '渠道(抖音/天猫/拼多多/京东/唯品会/分销/私域/线下/新零售/其它)',
  platform  VARCHAR(20)  NOT NULL COMMENT '平台(社媒/电商/其他)',
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (shop_name)
) COMMENT='销售日报-渠道映射';

-- 销售日报箱规托规(货品 → 每箱瓶数 / 每托箱数)
CREATE TABLE IF NOT EXISTS dim_goods_pack_spec (
  goods_no VARCHAR(64) NOT NULL COMMENT '货品编码(对 trade_goods.goods_no / goods.goods_no)',
  box_qty        DECIMAL(14,4) NULL COMMENT '箱规=每箱单瓶数',
  pallet_box_qty DECIMAL(14,4) NULL COMMENT '托规=每托箱数',
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (goods_no)
) COMMENT='销售日报-箱规托规';
```

- [ ] **Step 2: 写平台分组纯函数的失败测试**

`server/cmd/import-report-maps/platform_test.go`:

```go
package main

import "testing"

func TestPlatformOf(t *testing.T) {
	cases := map[string]string{
		"抖音": "社媒", "视频小店": "社媒", "小红书": "社媒", "快手": "社媒",
		"拼多多": "电商", "天猫": "电商", "京东": "电商", "唯品会": "电商",
		"分销": "其他", "私域": "其他", "线下": "其他", "新零售": "其他", "其它": "其他",
		"没见过的渠道": "其他",
	}
	for in, want := range cases {
		if got := platformOf(in); got != want {
			t.Errorf("platformOf(%q)=%q want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 3: 跑测试确认失败**

Run: `cd server && go test ./cmd/import-report-maps/ -run TestPlatformOf -v`
Expected: FAIL(`platformOf` 未定义)

- [ ] **Step 4: 写实现**

`server/cmd/import-report-maps/platform.go`:

```go
package main

var channelPlatform = map[string]string{
	"抖音": "社媒", "视频小店": "社媒", "小红书": "社媒", "快手": "社媒",
	"拼多多": "电商", "天猫": "电商", "京东": "电商", "唯品会": "电商",
	"分销": "其他", "私域": "其他", "线下": "其他", "新零售": "其他", "其它": "其他",
}

// platformOf 渠道→平台,未知渠道归「其他」
func platformOf(channel string) string {
	if p, ok := channelPlatform[channel]; ok {
		return p
	}
	return "其他"
}
```

- [ ] **Step 5: 跑测试确认通过**

Run: `cd server && go test ./cmd/import-report-maps/ -run TestPlatformOf -v`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add server/internal/store/report_maps.sql server/cmd/import-report-maps/platform.go server/cmd/import-report-maps/platform_test.go
git commit -m "feat(sales-report): 销售日报维表 DDL + 渠道→平台分组纯函数"
```

---

### Task 2: 货品编码归一纯函数(箱规数字补0)

**Files:**
- Create: `server/cmd/import-report-maps/normalize.go`
- Test: `server/cmd/import-report-maps/normalize_test.go`

**Interfaces:**
- Produces: `normalizeGoodsNo(raw string) string`(纯数字且 <8 位 → 左补 0 到 8 位;其余原样 trim)

- [ ] **Step 1: 写失败测试**

`server/cmd/import-report-maps/normalize_test.go`:

```go
package main

import "testing"

func TestNormalizeGoodsNo(t *testing.T) {
	cases := map[string]string{
		"3010082":     "03010082", // 数字被 Excel 吃了前导 0 → 补回 8 位
		"03010082":    "03010082",
		"S3-02-0003":  "S3-02-0003", // 非纯数字原样
		"sxxtwl-480g": "sxxtwl-480g",
		" 3010137 ":   "03010137", // trim + 补 0
		"123456789":   "123456789", // 已 ≥8 位不动
	}
	for in, want := range cases {
		if got := normalizeGoodsNo(in); got != want {
			t.Errorf("normalizeGoodsNo(%q)=%q want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd server && go test ./cmd/import-report-maps/ -run TestNormalizeGoodsNo -v`
Expected: FAIL

- [ ] **Step 3: 写实现**

`server/cmd/import-report-maps/normalize.go`:

```go
package main

import "strings"

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// normalizeGoodsNo 纯数字且不足 8 位左补 0(Excel 吃前导 0),其余 trim 后原样
func normalizeGoodsNo(raw string) string {
	s := strings.TrimSpace(raw)
	if isAllDigits(s) && len(s) < 8 {
		return strings.Repeat("0", 8-len(s)) + s
	}
	return s
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd server && go test ./cmd/import-report-maps/ -run TestNormalizeGoodsNo -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add server/cmd/import-report-maps/normalize.go server/cmd/import-report-maps/normalize_test.go
git commit -m "feat(sales-report): 箱规货品编码归一(数字补0)纯函数"
```

---

### Task 3: 导入 CLI 主体(读 3 张 Excel → 建表 → upsert)

**Files:**
- Create: `server/cmd/import-report-maps/main.go`

**Interfaces:**
- Consumes: `platformOf`、`normalizeGoodsNo`、`isAllDigits`
- 运行方式:`go run ./cmd/import-report-maps --channel <RPA映射表.xlsx> --box <RPA映射表.xlsx> --pallet <箱规拖规.xlsx>`

> 说明:`RPA映射表.xlsx` 同时含「销售渠道映射」「箱规映射」两 sheet,故 `--channel` 与 `--box` 可传同一路径。DSN 复用项目 `config.json`(与其它 cmd 工具一致,勿裸露密码)。

- [ ] **Step 1: 写导入主程序**

`server/cmd/import-report-maps/main.go`:

```go
package main

import (
	"database/sql"
	"flag"
	"log"
	"strings"

	"github.com/xuri/excelize/v2"
	// 复用项目已有的 config/DSN 读取(与其它 cmd 工具同款);按实际包路径调整
	"bi-dashboard/internal/config"
	_ "github.com/go-sql-driver/mysql"
)

func main() {
	channelPath := flag.String("channel", "", "含『销售渠道映射』sheet 的 xlsx")
	boxPath := flag.String("box", "", "含『箱规映射』sheet 的 xlsx")
	palletPath := flag.String("pallet", "", "含箱规+托规(Sheet1: 名称|箱规|托规)的 xlsx")
	flag.Parse()

	db, err := sql.Open("mysql", config.MustDSN()) // 与项目一致的 DSN 获取
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := ensureTables(db); err != nil {
		log.Fatalf("建表: %v", err)
	}
	if *channelPath != "" {
		n := importChannelMap(db, *channelPath)
		log.Printf("渠道映射 upsert %d 条", n)
	}
	if *boxPath != "" {
		n := importBoxSpec(db, *boxPath)
		log.Printf("箱规 upsert %d 条", n)
	}
	if *palletPath != "" {
		n := importPalletSpec(db, *palletPath)
		log.Printf("箱规托规 upsert %d 条", n)
	}
	log.Println("完成")
}

func ensureTables(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS dim_sales_channel_map (
			shop_name VARCHAR(200) NOT NULL,
			channel VARCHAR(50) NOT NULL,
			platform VARCHAR(20) NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY(shop_name)
		) COMMENT='销售日报-渠道映射'`,
		`CREATE TABLE IF NOT EXISTS dim_goods_pack_spec (
			goods_no VARCHAR(64) NOT NULL,
			box_qty DECIMAL(14,4) NULL,
			pallet_box_qty DECIMAL(14,4) NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY(goods_no)
		) COMMENT='销售日报-箱规托规'`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

// importChannelMap 读『销售渠道映射』(A=销售渠道I,B=渠道,C=销售渠道II),
// shop_name=A, channel=C(销售渠道II,细渠道), platform=platformOf(channel)
func importChannelMap(db *sql.DB, path string) int {
	f, err := excelize.OpenFile(path)
	if err != nil {
		log.Fatalf("open %s: %v", path, err)
	}
	rows, _ := f.GetRows("销售渠道映射")
	cnt := 0
	for i, r := range rows {
		if i == 0 || len(r) < 3 { // 跳表头
			continue
		}
		shop := strings.TrimSpace(r[0])
		chII := strings.TrimSpace(r[2])
		if shop == "" || chII == "" {
			continue
		}
		_, err := db.Exec(`INSERT INTO dim_sales_channel_map(shop_name,channel,platform) VALUES(?,?,?)
			ON DUPLICATE KEY UPDATE channel=VALUES(channel), platform=VALUES(platform)`,
			shop, chII, platformOf(chII))
		if err != nil {
			log.Fatalf("upsert channel %q: %v", shop, err)
		}
		cnt++
	}
	return cnt
}

// importBoxSpec 读『箱规映射』(A=货品编号,B=名称,C=箱规),编码归一后 upsert box_qty
func importBoxSpec(db *sql.DB, path string) int {
	f, err := excelize.OpenFile(path)
	if err != nil {
		log.Fatalf("open %s: %v", path, err)
	}
	rows, _ := f.GetRows("箱规映射")
	cnt := 0
	for i, r := range rows {
		if i == 0 || len(r) < 3 {
			continue
		}
		no := normalizeGoodsNo(r[0])
		box := strings.TrimSpace(r[2])
		if no == "" || box == "" {
			continue
		}
		_, err := db.Exec(`INSERT INTO dim_goods_pack_spec(goods_no,box_qty) VALUES(?,?)
			ON DUPLICATE KEY UPDATE box_qty=VALUES(box_qty)`, no, box)
		if err != nil {
			log.Fatalf("upsert box %q: %v", no, err)
		}
		cnt++
	}
	return cnt
}

// importPalletSpec 读箱规托规(Sheet1: A=名称,B=箱规,C=托规),名称→goods_no 经 goods 表解析,
// 同时补 box_qty(优先托规表的箱规)与 pallet_box_qty
func importPalletSpec(db *sql.DB, path string) int {
	f, err := excelize.OpenFile(path)
	if err != nil {
		log.Fatalf("open %s: %v", path, err)
	}
	rows, _ := f.GetRows("Sheet1")
	cnt := 0
	for i, r := range rows {
		if i == 0 || len(r) < 3 {
			continue
		}
		name := strings.TrimSpace(r[0])
		box := strings.TrimSpace(r[1])
		pallet := strings.TrimSpace(r[2])
		if name == "" {
			continue
		}
		var no string
		// 名称精确匹配 goods_name → goods_no(取一个;多规格时取 delete=0)
		err := db.QueryRow(`SELECT goods_no FROM goods WHERE goods_name=? AND is_delete=0 LIMIT 1`, name).Scan(&no)
		if err == sql.ErrNoRows {
			log.Printf("托规表名称对不上 goods: %q(跳过)", name)
			continue
		} else if err != nil {
			log.Fatalf("查 goods %q: %v", name, err)
		}
		_, err = db.Exec(`INSERT INTO dim_goods_pack_spec(goods_no,box_qty,pallet_box_qty) VALUES(?,?,?)
			ON DUPLICATE KEY UPDATE box_qty=VALUES(box_qty), pallet_box_qty=VALUES(pallet_box_qty)`,
			no, nullifEmpty(box), nullifEmpty(pallet))
		if err != nil {
			log.Fatalf("upsert pallet %q: %v", no, err)
		}
		cnt++
	}
	return cnt
}

func nullifEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
```

> 注:`config.MustDSN()` 用项目现有 DSN 读取方式替换(参考其它 `cmd/*` 工具的 import);excelize import 路径与项目一致(`github.com/xuri/excelize/v2`)。

- [ ] **Step 2: 编译**

Run: `cd server && go build ./cmd/import-report-maps/`
Expected: 无报错

- [ ] **Step 3: 提交**

```bash
git add server/cmd/import-report-maps/main.go
git commit -m "feat(sales-report): 维表导入 CLI(渠道/箱规/托规 3 张 Excel → upsert)"
```

---

### Task 4: 跑导入 + 验证覆盖率(集成)

**Files:** 无(运行 + 校验)

- [ ] **Step 1: 把 3 张 Excel 拷到可访问路径并跑导入**

```bash
cd server
go run ./cmd/import-report-maps \
  --channel "/c/Users/Administrator/Desktop/RPA映射表.xlsx" \
  --box     "/c/Users/Administrator/Desktop/RPA映射表.xlsx" \
  --pallet  "/c/Users/Administrator/Desktop/箱规拖规.xlsx"
```
Expected: 打印 `渠道映射 upsert ~322 条` / `箱规 upsert ~1226 条` / `箱规托规 upsert ~52 条`

- [ ] **Step 2: 用只读 MySQL MCP 验覆盖率**(不改数据)

跑校验(6 月发货、4 仓、trade_type=1):
```sql
-- 店铺映射覆盖:trade 里出现的 shop 有多少能对上渠道映射
SELECT COUNT(DISTINCT t.shop_name) AS shops,
       COUNT(DISTINCT CASE WHEN m.shop_name IS NOT NULL THEN t.shop_name END) AS mapped
FROM trade_202606 t LEFT JOIN dim_sales_channel_map m ON m.shop_name=t.shop_name
WHERE t.trade_type=1 AND t.consign_time>='2026-06-01' AND t.consign_time<'2026-07-01'
  AND t.warehouse_name IN ('南京委外成品仓-公司仓-委外','天津委外仓-公司仓-外仓','松鲜鲜&大地密码云仓','长沙委外成品仓-公司仓-外仓');
-- 箱规覆盖:发货货品有多少能对上箱规
SELECT COUNT(DISTINCT tg.goods_no) AS skus,
       COUNT(DISTINCT CASE WHEN p.goods_no IS NOT NULL THEN tg.goods_no END) AS mapped
FROM trade_goods_202606 tg
JOIN trade_202606 t ON t.trade_id=tg.trade_id
LEFT JOIN dim_goods_pack_spec p ON p.goods_no=tg.goods_no
WHERE t.trade_type=1 AND t.consign_time>='2026-06-01' AND t.consign_time<'2026-07-01'
  AND t.warehouse_name IN ('南京委外成品仓-公司仓-委外','天津委外仓-公司仓-外仓','松鲜鲜&大地密码云仓','长沙委外成品仓-公司仓-外仓');
```
Expected: shop 映射率高(接近 100%);箱规映射率记录下来,对不上的 SKU 列出交业务补(不阻塞:缺箱规 ×1)。

- [ ] **Step 3: 无代码改动,不提交**(结论记入下方 Task 8 的校准清单)

---

### Task 5: 三块聚合的纯函数 + 单测

**Files:**
- Create: `server/internal/handler/sales_daily_report.go`(先只放类型 + 纯函数)
- Test: `server/internal/handler/sales_daily_report_test.go`

**Interfaces:**
- Produces:
  - 类型 `type ChannelRow struct { Platform, Channel string; Orders int; Bottles, WeightKg float64 }`
  - 类型 `type GoodsRow struct { GoodsNo, GoodsName string; Bottles, Boxes, Pallets float64; Orders int }`
  - 类型 `type ComboRow struct { Display string; Orders int; Bottles, WeightKg float64 }`
  - `func ratio(part, total float64) float64`(占比,total=0 返 0)
  - `func perOrder(v float64, orders int) float64`(单均,orders=0 返 0)
  - `func palletsOf(boxes, palletBoxQty float64) float64`(托数,palletBoxQty≤0 返 0)
  - `func rollupPlatforms(rows []ChannelRow) []ChannelRow`(渠道明细 → 加平台合计行 + 总计行,顺序:社媒块…电商块…其他块…总计)

- [ ] **Step 1: 写失败测试**

`server/internal/handler/sales_daily_report_test.go`:

```go
package handler

import "testing"

func TestRatioPerOrderPallets(t *testing.T) {
	if ratio(30, 120) != 0.25 {
		t.Errorf("ratio 25%% 错")
	}
	if ratio(1, 0) != 0 {
		t.Errorf("ratio total=0 应 0")
	}
	if perOrder(300, 100) != 3 {
		t.Errorf("perOrder 错")
	}
	if perOrder(5, 0) != 0 {
		t.Errorf("perOrder orders=0 应 0")
	}
	if palletsOf(128, 64) != 2 {
		t.Errorf("palletsOf 错")
	}
	if palletsOf(10, 0) != 0 {
		t.Errorf("palletsOf 无托规应 0")
	}
}

func TestRollupPlatforms(t *testing.T) {
	in := []ChannelRow{
		{Platform: "电商", Channel: "天猫", Orders: 10, Bottles: 100, WeightKg: 20},
		{Platform: "社媒", Channel: "抖音", Orders: 30, Bottles: 300, WeightKg: 60},
		{Platform: "电商", Channel: "京东", Orders: 5, Bottles: 50, WeightKg: 10},
	}
	out := rollupPlatforms(in)
	// 期望:社媒合计 → 抖音 → 电商合计 → 天猫 → 京东 → 总计
	if out[0].Channel != "社媒合计" || out[0].Orders != 30 {
		t.Fatalf("社媒合计不对: %+v", out[0])
	}
	last := out[len(out)-1]
	if last.Channel != "总计" || last.Orders != 45 || last.Bottles != 450 {
		t.Fatalf("总计不对: %+v", last)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd server && go test ./internal/handler/ -run 'TestRatioPerOrderPallets|TestRollupPlatforms' -v`
Expected: FAIL(未定义)

- [ ] **Step 3: 写实现**

`server/internal/handler/sales_daily_report.go`(本步只加类型 + 纯函数):

```go
package handler

import "sort"

type ChannelRow struct {
	Platform string  `json:"platform"`
	Channel  string  `json:"channel"`
	Orders   int     `json:"orders"`
	Bottles  float64 `json:"bottles"`
	WeightKg float64 `json:"weightKg"`
}

type GoodsRow struct {
	GoodsNo   string  `json:"goodsNo"`
	GoodsName string  `json:"goodsName"`
	Orders    int     `json:"orders"`
	Bottles   float64 `json:"bottles"`
	Boxes     float64 `json:"boxes"`
	Pallets   float64 `json:"pallets"`
}

type ComboRow struct {
	Display  string  `json:"display"`
	Orders   int     `json:"orders"`
	Bottles  float64 `json:"bottles"`
	WeightKg float64 `json:"weightKg"`
}

func ratio(part, total float64) float64 {
	if total == 0 {
		return 0
	}
	return part / total
}

func perOrder(v float64, orders int) float64 {
	if orders == 0 {
		return 0
	}
	return v / float64(orders)
}

func palletsOf(boxes, palletBoxQty float64) float64 {
	if palletBoxQty <= 0 {
		return 0
	}
	return boxes / palletBoxQty
}

var platformOrder = map[string]int{"社媒": 0, "电商": 1, "其他": 2}

// rollupPlatforms 渠道明细行按平台分组,每组前插「X合计」,末尾加「总计」
func rollupPlatforms(rows []ChannelRow) []ChannelRow {
	sort.SliceStable(rows, func(i, j int) bool {
		if platformOrder[rows[i].Platform] != platformOrder[rows[j].Platform] {
			return platformOrder[rows[i].Platform] < platformOrder[rows[j].Platform]
		}
		return rows[i].Bottles > rows[j].Bottles
	})
	var out []ChannelRow
	var grand ChannelRow
	grand.Channel = "总计"
	i := 0
	for i < len(rows) {
		p := rows[i].Platform
		sum := ChannelRow{Platform: p, Channel: p + "合计"}
		j := i
		for j < len(rows) && rows[j].Platform == p {
			sum.Orders += rows[j].Orders
			sum.Bottles += rows[j].Bottles
			sum.WeightKg += rows[j].WeightKg
			j++
		}
		out = append(out, sum)
		out = append(out, rows[i:j]...)
		grand.Orders += sum.Orders
		grand.Bottles += sum.Bottles
		grand.WeightKg += sum.WeightKg
		i = j
	}
	out = append(out, grand)
	return out
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd server && go test ./internal/handler/ -run 'TestRatioPerOrderPallets|TestRollupPlatforms' -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add server/internal/handler/sales_daily_report.go server/internal/handler/sales_daily_report_test.go
git commit -m "feat(sales-report): 三块聚合纯函数(占比/单均/托数/平台合计)+ 单测"
```

---

### Task 6: 接口实现(实时查三块)+ 路由 + 权限

**Files:**
- Modify: `server/internal/handler/sales_daily_report.go`(加 `GetSalesDailyReport` handler)
- Modify: `server/内部路由注册处`(参考 `/api/supply-chain/*` 现有注册,新增 `GET /api/supply-chain/sales-daily-report`)
- Modify: `server/internal/auth/auth_seed.go`(加权限位 `supply_chain.sales_daily_report:view`,授超管;分组归「供应链」)

**Interfaces:**
- Consumes: Task 5 的类型与纯函数
- Produces: `GET /api/supply-chain/sales-daily-report?date=YYYY-MM-DD` → `{date, channelRows, topGoods, topCombos}`,每块含 `today` 与 `month` 两套(即两次同结构、区间不同)

- [ ] **Step 1: 写 handler**(实时查,当日 + 当月累计各查一遍)

在 `sales_daily_report.go` 追加。关键 SQL(以「当日」为例,当月累计把区间换成月初→次日;`{PART}` 为发货日所在月分区表名如 `trade_202606`):

```go
// 4 仓 + 销售 的公共 WHERE 片段
const whWhere = `t.trade_type=1 AND t.warehouse_name IN
 ('南京委外成品仓-公司仓-委外','天津委外仓-公司仓-外仓','松鲜鲜&大地密码云仓','长沙委外成品仓-公司仓-外仓')
 AND t.consign_time >= ? AND t.consign_time < ?`

// 渠道块:订单/重量按订单粒度(trade),单瓶按明细粒度(trade_goods),分别查后按渠道合并
// 订单+重量:
//   SELECT m.platform,m.channel,COUNT(*) orders,SUM(t.estimate_weight)/1000 weight_kg
//   FROM {PART} t JOIN dim_sales_channel_map m ON m.shop_name=t.shop_name
//   WHERE {whWhere} GROUP BY m.platform,m.channel
// 单瓶:
//   SELECT m.channel, SUM(tg.sell_count*COALESCE(p.box_qty,1)) bottles
//   FROM {PART} t JOIN {PARTG} tg ON tg.trade_id=t.trade_id
//   JOIN dim_sales_channel_map m ON m.shop_name=t.shop_name
//   LEFT JOIN dim_goods_pack_spec p ON p.goods_no=tg.goods_no
//   WHERE {whWhere} GROUP BY m.channel
// 在 Go 里按 channel 合并成 []ChannelRow,再 rollupPlatforms()

// 单品块(明细粒度,按 goods_no):
//   SELECT tg.goods_no, MAX(tg.goods_name) nm,
//          SUM(tg.sell_count) boxes, SUM(tg.sell_count*COALESCE(p.box_qty,1)) bottles,
//          MAX(p.pallet_box_qty) pallet, COUNT(DISTINCT t.trade_id) orders
//   FROM {PART} t JOIN {PARTG} tg ON tg.trade_id=t.trade_id
//   LEFT JOIN dim_goods_pack_spec p ON p.goods_no=tg.goods_no
//   WHERE {whWhere} GROUP BY tg.goods_no ORDER BY bottles DESC LIMIT 10
//   → Pallets = palletsOf(Boxes, pallet)

// 组合块(先订单签名,再按签名聚合):
//   SET SESSION group_concat_max_len = 100000;  -- 防长篮子截断
//   SELECT sig_display, COUNT(*) orders, SUM(ob) bottles, SUM(ow)/1000 weight_kg FROM (
//     SELECT t.trade_id, t.estimate_weight ow,
//       GROUP_CONCAT(tg.goods_no,'#',CAST(tg.sell_count AS UNSIGNED) ORDER BY tg.goods_no SEPARATOR '|') sig,
//       GROUP_CONCAT(tg.goods_name,'(',CAST(tg.sell_count AS UNSIGNED),')' ORDER BY tg.goods_no SEPARATOR ', ') sig_display,
//       SUM(tg.sell_count*COALESCE(p.box_qty,1)) ob
//     FROM {PART} t JOIN {PARTG} tg ON tg.trade_id=t.trade_id
//     LEFT JOIN dim_goods_pack_spec p ON p.goods_no=tg.goods_no
//     WHERE {whWhere} GROUP BY t.trade_id
//   ) o GROUP BY sig ORDER BY orders DESC LIMIT 10
```

handler 骨架(参考现有 supply-chain handler 的 DB 取用、`writeJSON`、错误处理、`context` + query timeout):

```go
func (h *DashboardHandler) GetSalesDailyReport(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		date = latestConsignDate(h.DB) // 无参默认最近有发货数据日
	}
	part, partG := tradePartitions(date) // "trade_202606","trade_goods_202606"
	dayStart, dayEnd := date, nextDay(date)
	monStart := monthStart(date) // date 的当月 1 号

	resp := map[string]interface{}{
		"date":        date,
		"channelToday": h.queryChannel(r.Context(), part, partG, dayStart, dayEnd),
		"channelMonth": h.queryChannel(r.Context(), part, partG, monStart, dayEnd),
		"goodsToday":   h.queryTopGoods(r.Context(), part, partG, dayStart, dayEnd),
		"goodsMonth":   h.queryTopGoods(r.Context(), part, partG, monStart, dayEnd),
		"comboToday":   h.queryTopCombos(r.Context(), part, partG, dayStart, dayEnd),
		"comboMonth":   h.queryTopCombos(r.Context(), part, partG, monStart, dayEnd),
	}
	writeJSON(w, resp)
}
```

> 实现 `queryChannel`/`queryTopGoods`/`queryTopCombos`(用上面 SQL,`QueryContext` + 5s timeout,组合块前 `SET SESSION group_concat_max_len`);`tradePartitions`/`nextDay`/`monthStart`/`latestConsignDate` 辅助函数。跨月分区(月初环比取上月)一期只在同月分区内算,月初当日环比留空即可(注释标注)。

- [ ] **Step 2: 注册路由 + 权限**

在路由注册处(参考 `/api/supply-chain/inventory-warning` 等)新增:
```go
mux.HandleFunc("GET /api/supply-chain/sales-daily-report", requirePerm("supply_chain.sales_daily_report:view", h.GetSalesDailyReport))
```
`auth_seed.go` 超管权限列表加 `supply_chain.sales_daily_report:view`,菜单分组归「供应链」。

- [ ] **Step 3: 编译 + 重启 bi-server**

```bash
cd server && go build ./... && echo BUILD_OK
# 重启前清代理,只 kill 8080 的 PID
```

- [ ] **Step 4: 提交**

```bash
git add -A server/
git commit -m "feat(sales-report): 销售日报接口(渠道/单品/组合三块实时查)+ 路由 + 权限"
```

---

### Task 7: 接口实测(curl 6/29 对数)

**Files:** 无

- [ ] **Step 1: 打接口**

```bash
curl -s "http://localhost:8080/api/supply-chain/sales-daily-report?date=2026-06-29" | python -m json.tool | head -60
```
Expected: 三块都有数;渠道块「总计」订单数 ≈ 15515(4 仓 6/29 已实测);组合块 top 与 §附.实证一致(松茸有机酱油100mL 榜首)。

- [ ] **Step 2: 用只读 MCP 复核渠道块总计**与接口一致(SQL 手算 vs 接口),记录差异。

- [ ] **Step 3: 无代码改动**(如发现口径偏差,回 Task 5/6 修 + 重测)

---

### Task 8: 前端页(发货日选择器 + 三表)

**Files:**
- Create: `src/pages/supply-chain/salesDailyReportColumns.ts`
- Test: `src/pages/supply-chain/salesDailyReportColumns.test.ts`
- Create: `src/pages/supply-chain/SalesDailyReport.tsx`
- Modify: `src/App.tsx`(lazy import + Route,`guard('supply_chain.sales_daily_report:view', …)`)
- Modify: `src/navigation.tsx`(供应链分组下加菜单项)

**Interfaces:**
- Consumes: `GET /api/supply-chain/sales-daily-report?date=`
- Produces: 页面路由 `/supply-chain/sales-daily-report`

- [ ] **Step 1: 写格式化纯函数 + 失败测试**

`salesDailyReportColumns.test.ts`:
```ts
import { pct, kg1, int0 } from './salesDailyReportColumns';
test('pct', () => { expect(pct(0.25)).toBe('25.0%'); expect(pct(0)).toBe('0.0%'); });
test('kg1', () => { expect(kg1(1.341)).toBe('1.3'); });
test('int0', () => { expect(int0(49.52)).toBe('50'); });
```

- [ ] **Step 2: 跑测试确认失败**

Run: `CI=true npx react-scripts test --watchAll=false salesDailyReportColumns`
Expected: FAIL

- [ ] **Step 3: 写格式化函数 + 列定义**

`salesDailyReportColumns.ts`:
```ts
export const pct = (v: number) => `${(v * 100).toFixed(1)}%`;
export const kg1 = (v: number) => v.toFixed(1);
export const int0 = (v: number) => Math.round(v).toString();
// 三块的 antd ColumnsType 定义,列顺序照 Excel(平台/渠道/发货量/占比/环比/件数/单件比/单均重量 … 当月累计同结构)
// export const channelColumns = [...]; export const goodsColumns = [...]; export const comboColumns = [...];
```

- [ ] **Step 4: 跑测试确认通过**

Run: `CI=true npx react-scripts test --watchAll=false salesDailyReportColumns`
Expected: PASS

- [ ] **Step 5: 写页面**(参考 `PurchasePlan.tsx` 的取数/DateFilter/antd Table 套路)

`SalesDailyReport.tsx`:顶部发货日选择器(默认接口回的 date),`fetch(`${API_BASE}/api/supply-chain/sales-daily-report?date=${date}`)`,三个 antd `Table`(channel/goods/combo),列用 Step 3 定义;不自改字体/色。

- [ ] **Step 6: 挂路由 + 菜单**

`App.tsx` 加 `const SalesDailyReport = lazy(() => import('./pages/supply-chain/SalesDailyReport'));` + `<Route path="/supply-chain/sales-daily-report" element={guard('supply_chain.sales_daily_report:view', <SalesDailyReport />)} />`;`navigation.tsx` 供应链组加项。

- [ ] **Step 7: build + 提交**

```bash
npm run build   # 必须 build 才生效
git add -A src/
git commit -m "feat(sales-report): 销售日报前端页(发货日选择器 + 三块表)"
```

---

### Task 9: 前端实测 + 收口

**Files:** 无(验证 + 收口)

- [ ] **Step 1: playwright 实测**(3001 预览或本地登录态):进 `/supply-chain/sales-daily-report`,截图三块渲染、换发货日刷新、TOP10 组合展示正常。
- [ ] **Step 2: SQL 交叉核对**:抓 6/29,手写 SQL 核三块合计 == 页面(不以模板 Excel 数字为准);差异记录交业务确认平台分组。
- [ ] **Step 3: `/code-review`**(财务/口径相关,过一遍)。
- [ ] **Step 4: 更新 memory**:新建 `project_sales_daily_report.md`(数据源/4仓/口径/映射表/待校准),MEMORY.md 加索引。
- [ ] **Step 5: 汇报跑哥**,列上线清单(导入 CLI 已跑 / bi-server 重启 / 前端 build),**等跑哥明说才上线**(部署授权红线)。

---

## Self-Review(计划 vs spec)

- **Spec 覆盖**:①页面/位置→Task 8 ②4仓/发货日/type=1→Global + Task 6 SQL ③3映射落库→Task 1/3/4 ④三块算法→Task 5/6 ⑤接口→Task 6 ⑥前端→Task 8 ⑦更新方式(实时)→Task 6(无物化)⑧待校准→Task 4/7/9 校验步骤。覆盖完整。
- **类型一致**:`ChannelRow/GoodsRow/ComboRow` 在 Task 5 定义,Task 6 消费;`platformOf/normalizeGoodsNo` Task 1/2 定义,Task 3 消费;签名字段 `sig/sig_display` 在 Task 6 SQL 内自洽。
- **无占位**:纯函数与 SQL 均给出真实代码;handler 骨架给出结构 + 完整 SQL,辅助函数(分区名/日期)在 Task 6 Step 1 注明需实现——**执行时按注释补齐,勿留 TODO**。
- **已知留白(有意)**:①月初当日「环比」跨月一期留空(Task 6 注)②缺箱规 SKU ×1(Global)③平台分组待业务最终确认(Task 9)。
