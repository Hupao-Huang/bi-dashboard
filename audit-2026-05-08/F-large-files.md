# F 路: 大文件拆分建议 (2026-05-08)

agent: a5303643f398acace
基线: cdb60b1 (v0.83)

## 总结一句话
**真正卡的不是文件长, 是: 重复造轮子 + 1 个函数干 8 件事 + 前端单文件 1485 行**. 拆完预计减 15-20% 代码量 (~2500 行).

---

## 8 个大文件优先级

| 文件 | 行数 | 优先级 | 拆分必要 | 主要痛点 |
|---|---|---|---|---|
| dashboard.go | 3926 | P0 | 必拆 | 5 个业务域 + GetDepartmentDetail/GetMarketingCost 各 660+ 行 |
| import-tmall + import-pdd | 1192+1087 | P0 | 跨文件提取 | 11 个 cmd 文件 100+ 行工具函数重复 |
| StoreDashboard.tsx | 1485 | P0 | 必拆 | 6 个 IIFE platform 内联 + 6 套 KPIRow/Chart 复制 |
| auth.go | 2153 | P1 | 该拆 | 4 个独立模块 + op_douyin 表错位 + 钉钉 SDK 缺失 |
| supply_chain.go | 1629 | P1 | 该拆 | 660 行 12 goroutine 大杂烩 + 双路径补丁 + 3 相似 SQL |
| finance_report.go | 1133 | P1 | 该拆 | GetFinanceReport 275 行 + ExportFinanceReport 240 行 + urlEscape 重造 |
| admin.go | 1514 | P2 | 不强拆 | 内聚 OK, 但 \\x1f hack + loadDataScopes 跟 auth 重复 |

---

## P0 拆分方案 (3 个)

### 1. dashboard.go (3926 → 12 文件)
```
dashboard_cache.go      (~140) overviewCache + WithCache + ClearCache*
dashboard_helpers.go    (~120) buildPlatformCond + offlineRegion + 平台映射
dashboard_overview.go   (~340) GetOverview
dashboard_department.go (~700) GetDepartmentDetail (拆 7 个 query* 子函数)
dashboard_sproducts.go  (~250) GetSProducts
ops_tmall.go            (~400) GetTmallOps + GetTmallcsOps
ops_pdd.go              (~120) GetPddOps
ops_jd.go               (~220) GetJdOps
ops_vip.go              (~70)  GetVipOps
ops_feigua.go           (~240) GetFeiguaData
ops_customer.go         (~430) GetCustomerOverview
marketing_cost.go       (~700) GetMarketingCost (拆 platform-specific)
dashboard_util.go       (~80)  writeJSON/writeError/round2 等
```

### 2. import-tmall + import-pdd (跨 11 个 cmd 文件)
**不是拆这个文件, 而是把工具函数迁到共享包**:
- internal/importutil/excel.go — parseFloat / parseInt / cellStr / formatDate
- internal/importutil/date.go — parseExcelDate (统一多策略, import-pdd 需 MM-DD-YY 美式)
- internal/importutil/header.go — headerIdx / findCol
- internal/importutil/importer.go — importExcelTable 通用 driver

减:
- 11 个 cmd 文件每个 ~100 行工具函数复制 → 1 处
- 13 个 importXxx 函数 ~70 行模板 → 30 行收缩

### 3. StoreDashboard.tsx (1485 → src/components/store/)
```
store/StoreDashboard.tsx          (~250 主调度)
store/hooks/useApiData.ts         (abort + fetch 模板, 替代 7 个 fetchXxx)
store/hooks/useShopList.ts        (shop list + plat tabs)
store/hooks/useStoreOpsData.ts    (7 个 ops fetch 调度)
store/shared/KPIRow.tsx           (替代 6 处 [{title,value,...}].map 模板)
store/shared/TimeSeriesChart.tsx  (bar+line 双轴通用)
store/shared/extractShopName.ts   (5 个 regex → 1 个)
store/shared/summarize.ts         (summarizeInRange<T> helper)
store/platforms/TmallSection.tsx  (240)
store/platforms/VipSection.tsx    (70)
store/platforms/JdSection.tsx     (200, 拆 2 子组件)
store/platforms/PddSection.tsx    (90)
store/platforms/TmallCsSection.tsx (80)
store/platforms/SProductsSection.tsx (80)
```

---

## P1 拆分方案

### 4. auth.go (2153 → 7 文件 + 新建 dingtalksdk 包)
```
captcha.go            (~330) 验证码 + IP 锁
auth_schema.go        (~420) EnsureAuthSchemaAndSeed + seed*  ⚠️ 抖音表迁出
auth_login.go         (~250) Login/Logout/ChangePassword/Me
auth_middleware.go    (~80)  RequireAuth/RequirePermission*
auth_payload.go       (~190) loadAuthPayload/loadDataScopes
auth_session.go       (~135) Session 工具
dingtalk.go           (~440) 钉钉 OAuth (4 路径分支拆)

internal/dingtalksdk/  新建包: client.go (5 个方法替代 4 处复制)
```

### 5. supply_chain.go (1629 → 5 文件)
```
supply_chain_dashboard.go  (~700) GetSupplyChainDashboard (12 goroutine 抽方法) + Trend
purchase_plan.go           (~440) GetPurchasePlan + 3 SQL 抽 buildPlanSQL(scope, classFilter)
supply_chain_sync.go       (~250) SyncYSStock + GetSyncYSProgress + ysOrchestrator struct
supply_chain_intransit.go  (~130) GetInTransitDetail
supply_chain_filters.go    (~80)  excludeAnhuiOrg* / planWarehouses 等
```
⚠️ 干掉 `OR p.product_c_code = ?` 双路径补丁 (是 goods.sku_code 没填全的兜底)

### 6. finance_report.go (1133 → 4 文件)
```
finance_report.go      (~480) GetFinanceReport (拆 4 phase 子函数) + Trend/Compare/Structure
finance_export.go      (~280) ExportFinanceReport (拆 styles+header+dataRows)
finance_import.go      (~240) Preview + Confirm + previewCache
finance_helpers.go     (~80)  placeholders/nullStr 等
```
⚠️ urlEscape 35 行 → url.PathEscape 1 行
⚠️ captureWriter HTTP 内调反模式, 改 data-layer 函数共享

---

## 跑哥批评 lens 命中 (重要)

### "打补丁思维 ≠ 重构思维" 具象例

1. supply_chain.go v0.71-v0.78 **7 重防御** (cooldown/诊断/进度/兜底), 注释自己写"防止前端 bug" — 没追前端根因
2. GetInTransitDetail **双路径 OR `p.product_c_code = ?`** — 是 goods.sku_code 没填全的兜底, 应补数据
3. dashboard.go **strings.ReplaceAll(scopeCond, "shop_name", "s.shop_name")** 5 处 — 函数没接 alias 参数

### "范围思维狭窄不看闭环" 具象例

1. GetDepartmentDetail **666 行干 8 件事** (趋势/店铺/商品/渠道分布/品牌/产品定位/平台列表/平台销售)
2. auth.go 里塞 **op_douyin 抖音表 schema** (270 行) — 跟认证完全无关
3. GetMarketingCost **4 个 case 复制 4 套查询** — 应抽 platform-handler 接口

### "不主动 grep 现有功能就动手" 具象例

11 个 cmd/import-*/main.go 各自重写 parseFloat/parseInt/parseExcelDate/headerIdx/findCol — internal/importutil/ 包已存在但没全用

---

## 重复造轮子清单 (跨文件)

| 模式 | 出现处 | 建议 |
|---|---|---|
| parseExcelDate | 11 个 cmd | internal/importutil/date.go |
| parseFloat/parseInt/cellStr/headerIdx | 11+ cmd | internal/importutil/parser.go |
| `for rows.Next() { Scan; append }` | 200+ 处 | 泛型 scanRows[T] |
| 钉钉 access_token + 调 API | 3-4 处 | internal/dingtalksdk |
| loadDataScopes | auth.go + admin.go | 合并 |
| 平台映射 (4 套) | dashboard.go | platforms.go 单一来源 |
| `\x1f` 字符串拼接 | admin.go | 直接 slice |
| ECharts time series option | StoreDashboard 6 处 | TimeSeriesChart 组件 |
| inSelectedRange().reduce() | StoreDashboard 50+ 处 | summarizeInRange helper |
| schema-as-code | auth.go (op_douyin) + import-pdd 等 | internal/schema/ |
| urlEscape 自实现 UTF-8 | finance_report.go | url.PathEscape |
| Excel REPLACE INTO ?,?,? | 13 个 importXxx | importExcelTable driver |

---

## 最高 ROI 改动 (按工时回报)

1. **internal/importutil 共享 11 个 cmd 工具** (~半天, 减 1100+ 行重复)
2. **StoreDashboard.tsx 拆 6 platform** (~1 天, 1485 → 250 主调度)
3. **dashboard.go 拆 12 文件** (~1 天, 3926 → 最大 700 行)
4. **抽 KPICard/useApiData/TimeSeriesChart** 前端 hook+组件 (~半天, 减 600+ 行)
5. **supply_chain.go GetSupplyChainDashboard 拆方法** (~半天, 660 → 80 主函数 + 12 子方法)

## 可以延后

- admin.go (内聚 OK, ROI 低)
- auth.go SDK 抽取 (4 处复制不多, 逻辑稳定)
- finance_export 样式拆 (代码冷)
