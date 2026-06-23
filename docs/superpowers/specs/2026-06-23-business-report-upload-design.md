# 业务报表 网页上传 Excel — 设计文档

- 日期: 2026-06-23
- 模块: 财务 → 业务报表 (业务预决算报表)
- 状态: 待跑哥审阅
- 关联代码: `src/pages/finance/BusinessReport.tsx` / `server/internal/handler/business_report.go` / `server/internal/business/parser.go` / `server/cmd/import-business-report/`

## 1. 背景与目标

业务报表(财务页的一个 tab)当前数据靠数据组手动跑 CLI(`import-business-report --snapshot YYYY-MM --year YYYY --xlsx <path>`)导入。页面上那个「上传 Excel」按钮是 **disabled 占位**,提示"下个版本支持自助上传,目前请联系数据组导入"。

**目标**:把这个按钮做成真功能,让有权限的人(财务/数据组)在网页上**自助上传 xlsx 入库**,不用再找数据组跑命令。

**做法**:1:1 套用财务报表 tab 已经验证稳定的两步上传流程(`finance_report_import.go` + `Report.tsx`),最大化复用。

## 2. 非目标 (YAGNI)

- 不改业务报表的展示逻辑/列结构/数据组织规则(只加上传入口)。
- 不动 CLI 导入工具(`import-business-report` 保留,作为兜底/批量手段)。
- 不做导入历史管理 UI(已有 `business_budget_import_log` 表记日志,本期不做查看页)。
- 不做 xls(老格式)解析,只收 .xlsx。

## 3. 现状 / 可复用资产(已查证)

| 资产 | 现状 | 复用方式 |
|------|------|---------|
| 解析器 `business.ParseFile(fpath, snapshotYear, snapshotMonth, year)` | 现成,3 layout 自适应解析,返回 `*ParseResult` | 直接复用,**不改** |
| 写库 `business.WriteResult(db, *ParseResult)` | 现状=整版删重写 `DELETE WHERE snapshot_year=? AND snapshot_month=?` + 批量 INSERT(500/批)+ 写 log | 加 mode 参数(见 4.3);`imported_by` 现硬编码 "admin",改成传真实用户 |
| 两步流框架(财务报表) | `ImportFinancePreview` + `ImportFinanceConfirm`:临时存盘 + 32hex token + 磁盘缓存 + 30min TTL + 互斥锁 + 过期清理 | 照搬结构到 `business_report_import.go` |
| 前端上传 Modal(财务报表 `Report.tsx`) | antd Upload + beforeUpload 抠文件名 + 两步(preview→confirm)+ 权限门控 | 抄到 `BusinessReport.tsx` |
| 数据表 `business_budget_report` / `business_budget_import_log` | 现成 | 不改结构 |

`ParseResult` 行字段(diff 要用):SnapshotYear / SnapshotMonth / Year / Channel / SubChannel / SheetOrder / Subject / SubjectLevel / SubjectCategory / ParentSubject / SortOrder / PeriodMonth / **BudgetYearStart / Budget / Actual**(金额三项)/ RatioYearStart / RatioBudget / RatioActual / AchievementRate(率四项)。

## 4. 设计

### 4.1 整体流程(两步,跟财务报表一致)

```
[选 xlsx] → [选 全量/增量] → 点"上传预览"
   → POST /api/finance/business-report/import/preview  (multipart: file, mode, 可选 snapshotMonth)
   → 后端: 存临时文件 → ParseFile → ComputeDiff → 存 token(磁盘缓存) → 返回预览
   → 前端弹框展示: 快照(2026年04月) / 新增 or 覆盖 / 渠道列表 / 行数 / 逐格金额变更明细 / 认不出的科目
   → 点"确认导入"
   → POST /api/finance/business-report/import/confirm  (json: {token})
   → 后端: 凭 token 取缓存 → WriteResult(mode) → 清缓存 → 返回结果
```

防护(全部照搬财务):token 32位hex(防路径穿越)、磁盘缓存 30min TTL、`businessImportMu` 互斥锁(防两人同时导)、best-effort 清理过期缓存。

### 4.2 文件名 → 快照年月解析(跑哥定:文件名带月份)

- 新命名约定:`2026年04月业务预决算报表.xlsx`
- 后端正则:`(\d{4})\s*年\s*(\d{1,2})\s*月` → 抠出 snapshotYear + snapshotMonth
- `year`(业务年份)默认 = snapshotYear(与 CLI 行为一致;如需可由前端额外传)
- 抠不出月份 → 返回 400 提示改名
- **兜底**:preview 接口接受可选 `snapshotMonth` 表单字段;前端弹框放一个月份下拉,文件名不规范时可手选,手选值覆盖文件名解析值。保证财务命名不统一时不被卡死。

### 4.3 全量 / 增量语义(跑哥要,与财务报表平行)

| 模式 | DB 行为 | 用途 |
|------|---------|------|
| **full(全量)** = 现有行为 | `DELETE FROM business_budget_report WHERE snapshot_year=? AND snapshot_month=?` 整版删 → 全量 INSERT。文件里没有的渠道/子渠道也被清掉 | 财务出完整新版,整版替换 |
| **incremental(增量)** = 新增 | 只删本次文件里**出现的 (渠道, 子渠道)**:`DELETE ... WHERE snapshot_year=? AND snapshot_month=? AND channel=? AND sub_channel=?`(逐组合),其他 (渠道,子渠道) 保留 → INSERT 本次行 | 只补/改某几个子渠道 |

实现:给 `WriteResult` 加 `result.Mode`(沿用财务的 `ImportModeFull`/`ImportModeIncremental` 常量风格,在 business 包新建)。增量分支用 `CollectChannelSubs(result)` 收集文件出现的 **(channel, sub_channel) 组合集合**,逐组合精确删。

> ⚠️ **删除粒度必须是 (channel, sub_channel) 不是 channel**(蓝军审查发现):查代码 `parseSheetName` 决定——**一个 sheet = 一个 (channel, sub_channel)**;查库 2026-04 快照 11 个 channel 下有 31 个 (channel, sub_channel) 组合(电商下有 TOC/TOB 等)。若按 channel 级删,增量补"电商TOC"会把"电商TOB"(在另一 sheet、本次文件没有)一起删掉 → 数据丢失。
>
> **特殊 sheet 处理**:业务报表还含"经营指标"(KPI)和"中后台合计"两类特殊 sheet,其 `sub_channel` 为空字符串(parser.go:731 `br.SubChannel = ""`)。增量删除的 (channel, sub_channel) 组合集合要把这些 sub_channel="" 的组合一并纳入,按同样规则精确删,不能漏删或错删。

### 4.4 diff:摘要为主 + 按渠道可展开明细(跑哥定,基于数据修订)

> **为什么不用财务那种"全量逐格列出":** 蓝军审查 + 查库证据——业务报表一个快照 **11494 行**(2026-04;2025-12 是 9362 行)。财务每月出新版时实际数滚动更新,全量替换近乎每行金额都变,逐格列出≈1万条,弹框/接口payload/缓存文件全爆炸。财务报表能逐格是因为它数据量小,业务报表不能照搬。

新写 `business.ComputeDiff(db, *ParseResult) (*DiffSummary, error)`,算法参照 `finance.ComputeDiff` 但**输出分层**:

- **比对粒度(行 key)**:`channel | sub_channel | parent_subject | subject | period_month`(parent_subject 是否进 key 取决于真实 UK,见待核实点1)
- **比对值**:Budget(预算)、Actual(实际)、BudgetYearStart(年初预算)三项金额(率类是派生值,不进 diff)
- **算法**:
  1. 查现库该快照现有行建 `oldByKey`;解析结果建 `newByKey`
  2. 逐 key 比对:新增 / 删除 / 修改(金额变化超阈值,如绝对值>0.01)/ 不变
  3. full 模式:old 全集参与删除判定;incremental 模式:只有本次出现的 (channel, sub_channel) 组合的 old 参与删除判定(其他组合保留,不算删)
- **输出 `DiffSummary` 分两层(关键)**:
  - **顶层摘要**:总 新增/删除/修改 计数 + 是"新增快照"还是"覆盖已有快照" + 涉及渠道列表 + 总行数 + 未识别科目(unmapped)
  - **按 (channel, sub_channel) 分组汇总**:每组 新增/删除/修改 各几格(这是默认展示)
  - **逐格明细按组装载**:每组的明细(key + 旧值→新值)。为控 payload,preview 接口每组明细**截断到前 N 条**(如 50)并带 `truncated`/`totalChanges` 标记;前端默认只渲染分组汇总,点开某组才展示该组明细。
- **前端**:弹框默认显示顶层摘要 + 各 (渠道,子渠道) 一行汇总;点某行展开看该组逐格变更。增量场景行数少,自然全展示。

> 工作量大头在这。业务报表矩阵(渠道×子渠道×科目×月份)比财务报表(部门×月×科目)维度多,算法同构可参照 `finance/parser.go:773 ComputeDiff`,但**分组+截断的输出结构是新的**,前端 diff 渲染组件需按业务字段(渠道/子渠道/科目/月份)重写,不是直接抄财务的(财务是 部门/月/科目)。

### 4.5 权限(跑哥定:新建独立权限)

- 新增权限点 `finance.business_report:import`(action 类),在 `auth_seed.go` 注册,中文名"财务-业务报表导入"。
- 两个新接口用 `pageProtected("finance.business_report:import", ...)` 保护。
- 前端按钮:`session.isSuperAdmin || permissions.includes('finance.business_report:import')` 才可点,否则保持 disabled + tooltip。
- 查看权限不变(业务报表查看现用 `finance.report:view`)。
- 跑哥后续在角色管理给对应角色勾选此权限。

### 4.6 错误处理 / 边界

- 非 .xlsx → 400。文件名抠不出快照且未手选月份 → 400。
- 解析失败(layout 不认/sheet 缺) → 500 带原因。
- 空结果(0 行) → 400"无数据写入"(WriteResult 现有保护)。
- token 过期/非法/不存在 → 410/400/404,提示重新上传。
- 导入并发 → 409"有其他导入进行中"。
- 认不出的科目(UnmappedSubjects):不阻断,预览里列出,确认后照常入库(status=partial),与财务一致。

## 5. 改动清单

### 后端
1. 新建 `server/internal/handler/business_report_import.go`:`ImportBusinessReportPreview` + `ImportBusinessReportConfirm`(仿 finance_report_import.go),含 business 专属的 previewPayload / token 目录 / TTL / 互斥锁。
2. `server/internal/business/parser.go`:
   - 加 `ImportModeFull`/`ImportModeIncremental` 常量 + `result.Mode` 字段
   - `WriteResult` 加 mode 分支(增量逐 (channel, sub_channel) 删)+ `imported_by` 改为接收真实用户(签名加参数或 result 带 ImportedBy)
   - 新增 `CollectChannelSubs(result)` 收集 (channel, sub_channel) 组合 + `ComputeDiff(db, result)` + `DiffSummary` 类型(分层:摘要 / 分组汇总 / 截断明细)
   - 新增 `ParseSnapshotFromFilename(filename) (year, month int)`
3. **`server/cmd/import-business-report/main.go`(易漏!)**:它在 :121 调 `business.WriteResult`、:85 调 `ParseFile`。WriteResult 改签名后 CLI 调用方**必须同步改**(默认传 `ImportModeFull` + imported_by="cli"),否则编译不过。改完 `cd server && go build` 重出 exe 拷到 server 根(见 feedback_deploy_exe)。
4. `server/cmd/server/main.go`:注册 2 条路由,`pageProtected("finance.business_report:import", ...)`。
5. `server/internal/handler/auth_seed.go`:加权限点 `finance.business_report:import`。

### 前端
5. `src/pages/finance/BusinessReport.tsx`:上传按钮解绑 disabled;加两步 Modal(选文件+全量/增量+月份兜底下拉 → 预览 diff → 确认),抄 `Report.tsx` 的 Upload/doPreview/doConfirm 模式;权限门控。

### 测试
6. `business` 包:WriteResult 全量/增量两分支 + ComputeDiff(新增/删除/修改/不变 + full vs incremental)单测(sqlmock,仿 finance 的 parsefile_writeresult_test.go / computediff_logimport_test.go)。
7. handler:preview/confirm 的 happy path + 错误分支(非xlsx/无月份/token过期/并发锁)。

## 6. 测试计划(验收标准)

- `cd server && go test ./internal/business/... ./internal/handler/...` 全绿。
- 真实 xlsx 手测:① 全量导入一版新快照 → 库行数对、展示页正常 ② 同快照增量只改一个 (渠道,子渠道) → **同渠道其他子渠道 + 其他渠道数据均不变**(SQL 核对,这是问题2的回归点)③ 预览 diff 的分组汇总数字与实际增删改一致、明细截断标记正确 ④ 无权限用户按钮 disabled ⑤ 特殊 sheet(经营指标/中后台)增量不被误删。
- 前端 `npm run build` 通过 + playwright 实测上传两步流(按 feedback_test_and_verify,build 通过≠完成)。
- 财务红线:业务报表属财务数据,上线前走 `/code-review` 二审(改了写库逻辑)。

## 7. 风险与待核实点(实现阶段务必查证)

1. **表 UK 是否含 parent_subject**:代码注释(parser.go:560)写 UK = (snapshot_year, snapshot_month, channel, sub_channel, subject, period_month) **不含 parent_subject**,但 memory 记的含 parent_subject。diff 的 row key 必须与真实 UK 对齐,否则跨父级同名科目(如"人工成本"在销售费用+管理费用各一行)会被 diff 误判为同一行。**实现前 `SHOW CREATE TABLE business_budget_report` 核实**。
2. ~~**ParseFile 真实签名**~~ **✅ 已核实**:`ParseFile(fpath, snapshotYear, snapshotMonth, year)`(parser.go:78),**不依赖文件名**(年月走参数),网页存临时文件再传路径可用。channel/sub_channel 来自 `parseSheetName(sheetName)`(parser.go:131),一个 sheet=一个 (channel, sub_channel)。
3. **增量删除粒度**:本设计已修订为 **(channel, sub_channel) 级**。若业务上还存在"同子渠道只补某几个月"的诉求,本方案会清掉该子渠道其他月——上线前跟财务确认增量到底是"按子渠道补整列"还是要细到"按子渠道+月补"。
6. **特殊 sheet(经营指标 / 中后台合计)**:它们 sub_channel="";确认这两类 sheet 的 channel 值是什么、是否进 `business_budget_report` 同表,使 diff 与增量删除的 (channel, sub_channel) 集合正确覆盖它们(不漏不错删)。memory 记"经营指标入库但 /channels API 查询时过滤"——入库是入的,故 diff/删除必须算上。
4. **imported_by 用户来源**:从 auth context 取真实用户(财务 LogImport 用 userID,业务报表当前硬编码 admin,要打通)。
5. **临时文件/缓存目录**:business 用独立目录(如 `bi-business-import` / business preview dir),不与财务共用,避免 token 串。

## 8. 复用 vs 新增 一览

- **纯复用(不改)**:`business.ParseFile`、两步流的设计模式、token/TTL/锁机制、前端 Upload 交互骨架、数据表。
- **改**:`business.WriteResult`(加 mode + 真实用户)。
- **新增**:business 包 ComputeDiff(分层输出)/CollectChannelSubs/ParseSnapshotFromFilename/Mode 常量;handler 两个接口;1 个权限点;前端 Modal + 业务字段版 diff 渲染;两批测试。
