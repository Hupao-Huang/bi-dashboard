# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目本质

松鲜鲜内部 BI 看板。前端 React + AntD + ECharts，后端 Go (`net/http` 标准库) + MySQL。聚合多源数据（吉客云 ERP、YS 用友 YonBIP、合思费控、11 平台运营 RPA Excel）做销售/库存/采购/财务/物流分析。**Windows 11 部署**（schtasks 调度，不是 cron），全栈 ~17,000 行 Go + ~30 个 React 业务页。

## 常用命令

### 前端
```powershell
# 开发：CRA dev server，但热更新经常失效 → 用 build+serve 替代更可靠
npm start                  # http://localhost:3000
npm run start:lan          # 0.0.0.0:3000 (LAN 设备可访问)
npm run build              # 必须 build 才能让生产看到改动（前端是 serve -s build 静态部署）
npm test                   # CRA jest

# 改了 .tsx 后**必须** npm run build；dev server 不是生产路径
```

### 后端
```powershell
# 构建主 server（cwd 必须是 server/，不是 repo root，否则会失败）
cd server
go build -o bi-server.exe ./cmd/server

# 重启 server（只 kill 8080 端口的 PID，不要 kill 全部 node/go 进程）
$pid = (Get-NetTCPConnection -LocalPort 8080 -State Listen).OwningProcess
Stop-Process -Id $pid -Force
Start-Process -FilePath "bi-server.exe" -WindowStyle Hidden

# 构建独立 cli 工具（30+ 个 cmd/* 子目录）
cd server && go build -o import-tmall.exe ./cmd/import-tmall
cd server && go build -o build-warehouse-flow-summary.exe ./cmd/build-warehouse-flow-summary

# Go 测试（handler 有少量单测）
cd server && go test ./internal/handler/...
```

### 数据库
```powershell
# DB 配置在 server/config.json（.gitignore 屏蔽，含密码）
mysql -h127.0.0.1 -uroot -p<pwd> bi_dashboard
# 大数据量统计禁止用 information_schema.TABLES.TABLE_ROWS（估算值），必须 SELECT COUNT(*)
```

### Schtasks（定时任务）
所有定时任务用 Windows `schtasks` 调度（不是 cron），账户 `SYSTEM`（除 Z 盘任务用 `Administrator`，否则 RDP 断开会被杀）。重定向 stdout 必须用 `.bat` 包装，不能在 schtasks 命令行直接 `cmd /c` 嵌套引号。任务命名前缀 `BI-`：
- `BI-APIServer` / `BI-Frontend` 开机自启
- `BI-SyncDailySummary` / `BI-SyncStock` / `BI-SyncYS*` / `BI-SyncHesi` 各业务同步
- `BI-Build-WarehouseFlowSummary` 每天 03:30 重建物化表
- `BI-BackupMySQL` / `BI-RotateLogs` 运维

## 架构（big picture）

### 前端 (`src/`)
- **入口**: `index.tsx` → `App.tsx` (路由) → `navigation.tsx` (菜单/权限)
- **业务页 `src/pages/`**: 按部门组织——`overview` (综合) / `ecommerce` / `social` / `offline` / `distribution` / `customer` / `finance` / `supply-chain` / `ai-center` / `system`
- **公共组件 `src/components/`**: `Chart.tsx` (ECharts 包装) / `DateFilter` / `MonthFilter` / `DepartmentPage` / `StoreDashboard` / `ProductDashboard` / `bi-filter-card` / `bi-stat-card`（视觉颗粒度统一）
- **API 调用**: `config.ts` 里 `API_BASE` 用 `window.location.hostname` 自动构造，LAN 不用改前端
- **后端响应包裹**: 所有 `writeJSON` 返回 `{code, data}` 结构，前端 `fetch().then(j => j.data.xxx)`，**不是直接 j.xxx**

### 后端 (`server/`)
- **入口**: `cmd/server/main.go`（276 行，编译产物 = `bi-server.exe`，部署到 `server/` 根目录）
- **Handler 层 `internal/handler/`** (~17k 行 / 25 文件)：
  - `dashboard.go` (4010 行，最大文件) — 综合看板/部门看板核心 API
  - `supply_chain.go` (1297) — 采购计划/库存预警
  - `finance_report.go` (1177) / `business_report.go` (794) — 财务+预决算
  - `warehouse_flow.go` (563) — 快递仓储分析（v0.60 起含物化表双轨路由）
  - `auth.go` / `admin.go` — 用户/角色/钉钉 OAuth/审计
  - `sync.go` / `task_monitor.go` — 同步触发/进度
- **业务子模块**: `internal/jackyun/` (吉客云 SDK) / `internal/yonsuite/` (YS 用友) / `internal/finance/` (财务解析) / `internal/business/` (业务规则) / `internal/importutil/` (Excel 导入工具)
- **数据库结构**:
  - **按月分表**: `trade_YYYYMM` / `trade_goods_YYYYMM` / `trade_package_YYYYMM` / `stock_snapshot_YYYYMM`（约 200 万行/月，单表大避免）
  - **运营 RPA**: `op_*_daily` 系列（11 平台 ~70 张表）
  - **物化预聚合**: `warehouse_flow_summary` / `sales_goods_summary_monthly`（聚合源表，秒级查询）
  - **YS 用友**: `ys_purchase_orders` / `ys_subcontract_orders` / `ys_material_out` / `ys_stock`
  - **财务**: `finance_report` / `business_budget_report` / `hesi_*`
  - **系统**: `notices` / `users` / `roles` / `audit_log` / `sales_channel`

### CLI 工具 (`server/cmd/`)
- **`server/`** = 主 bi-server
- **`import-*`** = 14 个 RPA Excel 导入器（每平台 1 个），由 webhook (`/api/webhook/sync-ops`) 或 schtasks 触发
- **`sync-*`** = 同步工具（不在 cmd 下，独立编译；含 `sync-daily-trades` / `sync-trades-v2` / `sync-stock` 等）
- **`probe-*`** / **`check-*`** / **`debug-*`** = 一次性探针/审计工具（已 .gitignore 部分）
- **`build-warehouse-flow-summary/`** = 物化表重建（v0.60）
- **`diff-finance/`** + **`sheet-peek/`** = 财务报表 audit

### 外部系统
| 系统 | 接入 | 关键文件 |
|------|------|---------|
| 吉客云 ERP（旧 AppKey 56462534 + 新 73983197） | 6 接口（销售/库存/汇总/销售单 scrollId） | `internal/jackyun/` |
| YS 用友 YonBIP | 4 接口（采购/委外/材料出库/现存量），1.1s 限流 | `internal/yonsuite/` |
| 合思费控 | 单据/详情/附件 | `internal/handler/hesi.go` |
| 钉钉 | OAuth 扫码登录 + Webhook 机器人 | `auth.go` 含 dingtalk_*  |
| RPA (Z 盘) | Excel → import-* 导入 | `Z:\信息部\RPA_集团数据看板\` |

## 关键 Gotchas

### Build/Run
- **`go build` 的 CWD**: 必须在 `server/` 子目录，不能 repo root。`cd server && go build ...`
- **改了 main.go 必须重 build exe 拷贝到 `server/` 根目录**，否则手动导入/webhook 走旧版
- **改了 `.tsx` 必须 `npm run build`**，serve 是静态目录，dev server 不算生产
- **进程清理只 kill 端口对应 PID**，不要 `Stop-Process node`（可能杀掉别的服务）

### 数据库
- **大表统计用 `SELECT COUNT(*)`**——`information_schema.TABLES.TABLE_ROWS` 是估算值
- **`ON DUPLICATE KEY UPDATE` / `REPLACE INTO` 必须配 `UNIQUE KEY`**，不然每次跑都是新增
- **数据库表/字段注释必须中文**
- **字段长度不够立即修，不能跑完再改**（`Data too long for column` 立即停）
- **按月分表查询**: `trade_YYYYMM` 按 `consign_time` 分月（已发货时间），不是 `trade_time`
- **仓储/物流分析必排** `trade_type IN (8,12)`（这两类不产生 `trade_package` 记录）

### Go 特定
- **19+ 位 long id（吉客云/YS）必须 `json.UseNumber()`** 解析，否则 float64 精度丢失撞 UK
- **改代码前先 Read 字段/类型定义，禁止猜字段名**
- **YS 用友日期过滤必须用 `simpleVOs`**，文档说的 top-level `vouchdate` 字段静默失效
- **YS 现存量接口 `data` 是直接 array**（不是 `data.recordList`），空 body 即全量

### 部署 + 发版流程（App 版本号模式，2026-05-12 起）

**commit ≠ 发版**——两件事解耦，节奏区分：

**commit 阶段（每个小改动）**
- commit message 格式：`<type>(<scope>): 主题 — 详情`（**不带版本号**）
- 例：`feat(system): 用户管理菜单加待审批徽标`
- 正常 push / rebuild exe / 重启 bi-server / 上生产（跟之前一样）
- **不发 notice**，commit 阶段不打扰用户

**改了 main.go / handler / 路由后**：
1. `cd server && go build -o bi-server.exe ./cmd/server`
2. 拷贝 `server/bi-server.exe` 到 `server/` 根
3. kill 8080 PID 重启
4. 改了 `.tsx` 必须 `npm run build`

**发版阶段（一组改动收尾时，4 步连发）**
1. 升版本号：MINOR 用于"一组功能闭环"，PATCH **只**用于紧急 hotfix（业务正在用的功能炸了）
2. CHANGELOG 段落：`## v1.X.0 (YYYY-MM-DD) — 主题`，合并这组所有 commit 的改动点（4-6 点列）
3. `git tag v1.X.0` + `git push --tags`
4. INSERT 一条 notice（取消所有旧置顶 + is_pinned=1）

**怎么算"一组改动收尾"**：跑哥说"现在发版" / 功能完整闭环 / 一波关联改动告一段落 / 跨天前主动收尾。

**公告内容红线**: 面向用户口吻 / 不写技术变量名/文件路径/函数名 / 不写具体业务数据明细（金额/百分比/SKU 名）。

**反例（避免）**：v1.61.0~v1.61.3 一个功能切 4 刀涨 4 版本。App 模式下应该是 1 个 v1.61.0 一次发完。

### 物化预聚合表
- `warehouse_flow_summary` 等聚合表如果某个月份"漏"，**不能直接 build 补**——可能源数据"还在泡"（业务在补拉），先问，不要自动 fix
- 双轨路由：handler 自动判断 `canUseSummary(ym)`，没物化降级到原 SQL

### CHANGELOG / 版本号
- `CHANGELOG.md` 维护版本演进（v0.1.0 起步，当前 v0.60）
- 每次 commit 走 `<type>(<scope>): 主题 — 详情` 格式（中文 em dash）

## 用户记忆系统

跑哥（用户）使用 Claude Code 自动 memory 系统，路径 `~/.claude/projects/C--Users-Administrator/memory/`，索引在 `MEMORY.md`。**90% 的隐性约定记在 memory 里**——`project_bi_dashboard.md` / `project_todo_next.md` / 各 `feedback_*.md`。开新会话先扫 memory 比看代码更高效。

## 开会话即用清单

跑哥说"继续 BI 看板项目"等开放性指令时：
1. **先 `git status` + `git log -5` + `git diff --stat`**，不能凭 memory 假设 working tree 干净（跑哥可能有 in-progress 改动）
2. 检查 `bi-server` PID（端口 8080）+ frontend PID（端口 3000）是否还在
3. 读 `project_todo_next.md` 看上次结束基线 + 待办
4. 列清单给跑哥确认方向，禁止直接动手 commit/build

## 部署策略（2026-05-12 跑哥拍板）

BI 看板**保持"改完直接生产"模式**（不上 staging）。35-43 人内部用户、只读分析工具、单人维护，staging 投入产出不划算。详见 memory `feedback_deployment_policy.md`。

**配 4 条规矩，每次改动前自查**：

1. **错峰重启** —— 避开 09:00-09:30 / 14:00-14:30 / 月初 1-5 号 / 月底盘点
2. **重大改动发钉钉** —— 新增页面/改业务规则/调字段时，给同事发一句"HH:MM 重启 30 秒新增 XX，刷新即可"
3. **业务红线（必须先本地跑通才推）**：
   - 业务规则/算法（KPI 公式 / 客服平均 / 财务科目 / 库存计费）—— `/codex` 二审
   - 数据库 schema 变更（加表/删表/改字段）
   - 定时任务调度 / 关停同步
   - main.go / 路由 / 权限
4. **回滚步骤**（万一改坏，3 分钟救场）：
   ```powershell
   cd C:\Users\Administrator\bi-dashboard
   git revert HEAD
   # 后端坏: cd server && go build -o bi-server.exe ./cmd/server，然后 kill 8080 PID 重启
   # 前端坏: npm run build （serve 自动 reload，不用重启）
   ```

## Skill routing

When the user's request matches an available skill, invoke it via the Skill tool. The
skill has multi-step workflows, checklists, and quality gates that produce better
results than an ad-hoc answer. When in doubt, invoke the skill. A false positive is
cheaper than a false negative.

Key routing rules:
- 修 bug / 错误 / "为什么坏了" / "wtf" / "这不对" → invoke /investigate
- 测试网站 / 浏览器跑一下 / "还能用吗" / 找 bug → invoke /qa (或 /qa-only 只生成报告)
- 改了代码后 / "看下我的改动" / 代码审查 → invoke /review
- 视觉细节 / "看着不对" / 设计稿审查 → invoke /design-review
- 部署 / "上线" / 创建 PR / "推上去" → invoke /ship
- 第二意见 / "另一个 AI 看看" / 高风险改动复核 → invoke /codex
- 安全模式 / "小心点" / "锁起来" → invoke /careful 或 /guard
- 限制改动范围到某目录 → invoke /freeze 或 /unfreeze
- 浏览网页 / 看 GitHub 仓库 / 看文档 → invoke /browse (禁用 mcp__claude-in-chrome__*)
- 周回顾 / "今天/这周做了啥" → invoke /retro
- 写发版文档 / 发布说明 → invoke /document-release
- 升级 gstack 工具 → invoke /gstack-upgrade

BI 看板特定规则 (按 v0.94 客服越界教训定制):
- **改任何业务规则/算法/口径前必须先 invoke /codex 二审**
  涵盖: 客服 KPI 算法 / 财务科目映射 / 销售部门口径 / 库存计费规则 / 业务预决算公式
  决策点: "这是技术 bug 修复? 还是业务规则改造?" 后者必须 /codex
- 改完代码自检完后 invoke /qa 浏览器实测一遍 (smoke 401 不算实测)
- 多 commit 攒够 4 个以上不要直接 commit, 先 invoke /review 复盘是否分批合理
