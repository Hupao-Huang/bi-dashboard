# 松鲜鲜 BI 数据看板 更新日志

---

## 版本号约定 (App 模式, 2026-05-12 起生效)

采用 **SemVer 3 段** 版本号 `vMAJOR.MINOR.PATCH`，**commit ≠ 发版**——按 App 节奏发：

### 三段语义

| 段 | 用户视角 | 触发场景 |
|----|---------|------|
| **MAJOR** (v**X**.0.0) | 整个换样了 / 老用法没了 | 大重构、数据库 schema 大动、部署方式变化 |
| **MINOR** (v1.**X**.0) | 看得到的新东西 / 一组功能闭环 | 新页面 / 新模块 / 新 KPI / 新筛选 / 新业务规则 |
| **PATCH** (v1.X.**Y**) | 紧急 hotfix | 业务正在用的功能炸了 (必须立刻告诉用户) |

### App 模式节奏（commit 与发版解耦）

| 动作 | 频率 |
|------|------|
| commit / push / build / 重启上生产 | 每个小改动 (不变) |
| 升 MINOR + CHANGELOG 段落 + git tag + INSERT notice | **一组改动收尾才发** |
| 升 PATCH | 紧急 hotfix 才单发, 日常 bug 攒到下个 MINOR |

**commit message 不带版本号**：`<type>(<scope>): 主题`，例 `feat(system): 用户管理菜单加待审批徽标`。

**一组改动怎么算收尾**：跑哥说"现在发版" / 功能完整闭环（实测通过 + 跑哥验收）/ 一波关联改动告一段落 / 跨天前主动收尾。

### 反例 (避免)

- **2026-05-09**: 一天 18 个 commit 全升 MINOR (v1.28→v1.45)，14 个其实是字体/列宽 PATCH
- **2026-05-12**: 一个"待办识别 UX"功能切 4 刀涨 4 版本 (v1.61.0/.1/.2/.3)，应该 1 个 v1.61.0 打透

### 历史

- v0.1.0 ~ v0.60：规范 SemVer
- v0.60 → v1.x：曾退化成单段升级
- v1.45：回归 3 段
- v1.62.0 起：切 App 模式（commit 不带版本号 + 一组收尾才发）

---

## v0.1.0 — 项目初始化与基础架构
- 搭建前端 React + Ant Design + ECharts 技术栈
- 搭建后端 Go (net/http) + MySQL 架构
- 对接吉客云 ERP 开放平台 API（AppKey: 56462534）
- 实现吉客云 API 签名算法与通用调用封装
- 接入首个接口：`erp.sales.get`（销售渠道）
- 建立 `sales_channel` 渠道表，完成渠道同步工具

## v0.2.0 — 商品与汇总帐对接
- 接入 `erp.storage.goodslist` 接口，同步商品主数据（goods 表）
- 接入 `birc.report.salesGoodsSummary` 汇总帐接口
- 建立 `sales_goods_summary` 表，存储按日+渠道+仓库+单品维度的汇总数据
- 开发 sync-summary 同步工具（支持环境变量指定日期范围）

## v0.3.0 — 综合看板 v1
- 综合看板首版上线（`/overview`）
- 四大部门（电商/社媒/线下/分销）销售额 KPI 卡片
- 每日销售趋势图（按部门分色堆叠）
- 商品销售排行 TOP15
- 店铺/渠道销售排行 TOP15
- 渠道部门映射：233个渠道分配至4个部门

## v0.4.0 — 库存模块
- 接入 `erp.stockquantity.get` 库存接口（游标翻页）
- 接入 `erp.batchstockquantity.get` 批次库存接口
- 建立 `stock_quantity`、`stock_batch` 表
- 开发 sync-stock、sync-batch-stock 同步工具
- 定时任务：每日 9:00/15:00/21:00 同步库存快照

## v0.5.0 — 销售单明细对接
- 申请吉客云定制接口 AppKey（73983197）
- 接入 `jackyun.tradenotsensitiveinfos.list.get` 销售单接口
- 实现 scrollId 游标翻页（替代传统分页）
- 按月分表：`trade_YYYYMM` + `trade_goods_YYYYMM` + `trade_package_YYYYMM`
- 开发 sync-trades-v2（补拉）、sync-daily-trades（每日增量）、sync-half-day（分段拉取）
- 数据覆盖：2025-06 ~ 2026-04，约500万条销售单
- JSON 解析失败自动重试（最多3次）

## v0.6.0 — 部门看板体系
- 电商部门看板（`/ecommerce`）：店铺数据预览 + 店铺看板 + 货品看板 + 营销费用
- 社媒部门看板（`/social`）：店铺数据预览 + 店铺看板 + 货品看板
- 线下部门看板（`/offline`）：店铺数据预览 + 店铺看板 + 货品看板
- 分销部门看板（`/distribution`）：店铺数据预览 + 店铺看板 + 货品看板
- 通用组件：StoreDashboard、ProductDashboard、StorePreview、DepartmentPage
- GoodsChannelExpand 公共组件（综合看板按部门 / 其他按平台+渠道双层饼图）

## v0.7.0 — 全平台运营数据接入
- **天猫**（13表）：店铺/商品/推广/品牌/会员/人群/CPS/行业月报/复购月报 + 客服4表
- **京东**（11表）：店铺/推广/京准通/京准通SKU/京东客/行业热词/行业排名/客服销售/客服类型
- **拼多多**（8表）：店铺/商品/推广/短视频/服务概览/商品明细/客服销售/客服服务
- **唯品会**（4表）：店铺/TargetMax/唯享客/取消金额
- **天猫超市**（4表）：经营概况/推广/行业热词/市场排名
- **抖音**（7表）：渠道/商品/漏斗/直播/主播/素材/千川直播
- **抖音分销**（4表）：账号/商品/素材/推广时段
- **飞瓜**（2表）：达人/直播
- 开发14个平台数据导入工具（import-tmall/jd/pdd/vip/tmallcs/douyin/douyin-dist/feigua/promo 等）
- RPA 数据自动导入 webhook（`/api/webhook/sync-ops`）

## v0.8.0 — S品渠道销售分析与看板增强
- 电商店铺看板新增 S品渠道销售分析（`/api/s-products`）
- 全部平台→按平台汇总饼图；选平台→按店铺饼图；选店铺→隐藏排行
- 单品排行展开行：GoodsChannelExpand 组件（平台分布+渠道分布双层饼图）
- 综合看板优化：4部门始终显示（含0值）、商品 TOP15 增加产品定位/分类/品牌
- 趋势图选单天时自动扩展日期范围
- 飞瓜看板上线（`/social/feigua`）：抖音直播电商数据
- 社媒营销看板上线（`/social/marketing`）

## v0.9.0 — 供应链管理模块
- 供应链管理看板（`/supply-chain`）
- 计划看板：销售趋势/渠道分布/品类分析/毛利分析
- 库存预警：基于SKU+仓库维度，按日均销量计算可用天数
- 快递仓储分析：物流数据统计
- 每日预警：自动检测异常指标
- 月度账单分析

## v0.10.0 — 线下深度分析
- 高价值客户分析（`/offline/high-value-customers`）
- 周转率及临期管理（`/offline/turnover-expiry`）
- KA 月度统计（`/offline/ka-monthly`）

## v0.11.0 — 登录认证与权限体系
- 登录认证系统：滑块验证码 + 账号密码登录
- Session 管理与 Cookie 认证
- 权限中间件：RequireAuth / RequirePermission / RequireAnyPermission
- 角色管理：创建/编辑/删除角色，分配权限
- 用户管理：创建用户、分配角色
- 32个功能权限点，覆盖所有看板模块
- 数据范围控制：支持部门级、店铺级数据隔离
- 个人中心：头像上传、个人信息修改

## v0.12.0 — 财务模块
- 利润总览（`/finance/overview`）：全局利润 KPI
- 部门利润分析（`/finance/department-profit`）：按部门拆解利润构成
- 月度利润统计（`/finance/monthly-profit`）：利润趋势与环比分析
- 产品利润统计（`/finance/product-profit`）：单品级利润追踪

## v0.13.0 — 合思费控对接
- 对接合思开放 API（报销单/借款单/申请单/通用审批）
- 建立 hesi_flow / hesi_flow_detail / hesi_flow_invoice / hesi_flow_attachment 4张表
- 开发 sync-hesi 同步工具，支持按类型筛选
- 费控管理页面（`/finance/expense-control`）：KPI + 筛选器 + 数据表格 + 详情弹窗
- 附件 URL 实时调合思 API 获取

## v0.14.0 — 客服中心
- 客服总览（`/customer/overview`）
- 支持天猫、抖音、京东、拼多多、快手、小红书六大平台
- 核心指标：首响时间、平均响应、满意率、询单转化率
- 店铺维度排名，各平台指标定制化展示

## v0.15.0 — 系统管理与运维
- 任务监控页面（`/system/tasks`）：同步任务状态查看、手动触发
- 反馈管理（`/system/feedback`）：用户反馈提交与管理员回复
- 公告管理（`/system/notices`）：系统公告发布（更新/通知/维护三种类型）
- 公告铃铛组件：实时提醒新公告
- 定时任务体系：
  - BI-SyncDailySummary（每天 8:00，汇总帐）
  - BI-SyncDailyTrades（每天 8:30，销售单增量）
  - BI-SyncStock（每天 9:00/15:00/21:00，库存快照）
  - BI-SyncBatchStock（每天 9:05，批次库存）
  - BI-SyncHesi（每天 10:30，合思费控）
  - BI-APIServer（开机自启）

## v0.16.0 — 数据口径修正与补全
- 销售额口径统一为 `goods_amt`（销售额），不再使用 `sell_total`（货款金额）
- 综合看板总销售额包含所有渠道（未映射部门归入"其他"）
- 销售单数据补拉（4/8~4/10 API 异常恢复后补全）
- 汇总帐 4/1~4/10 全量重新同步
- CPS 口径修正：成交额用 settle_amount，佣金用 settle_total_cost

> v0.17 ~ v0.23 的变更直接以 git commit 记录，未展开到本文档；简述：
> - v0.17 渠道管理、平台分布、水印、数据修正
> - v0.18 钉钉扫码登录/绑定、用户自动注册审批、批量导入
> - v0.19 产品定位平台分布图、全接口缓存、钉钉注册优化
> - v0.20 安全加固与代码质量优化（合思密钥外移、webhook 认证、请求体限制、内存泄漏清理等）
> - v0.21 剩余安全与质量修复
> - v0.22 采购需求、RPA文件映射、数据库字典、RPA数据监控
> - v0.23 RPA监控优化、归档数据支持（isTableSwitch=2）、渠道管理改进

## v0.24 — 安全修复、前端容错与 RPA 数据对齐

### 安全与稳定性
- **滑块验证码防爆破**：preVerify 失败即销毁 captchaId，成功标记 `verified` 后由 login 消耗（阻止同一 captcha 反复试错）
- webhook 鉴权改 `hmac.Equal` 常量时间比较（避免计时侧信道）
- 合思 API 密钥、MySQL 备份脚本中的硬编码密码移出 git 追踪（通过 gitignore 排除）

### 前端容错
- 新增全局 `ErrorBoundary`，任何渲染异常都兜住不白屏
- 新增 `NotFoundPage` 和 `path="*"` 兜底路由（未匹配 URL 不再渲染空白布局）
- 17 处 `.catch(() => {})` 改为 `catch(err => console.warn(...))`（错误不再静默吞掉）
- 15 个组件加 `AbortController`（RPAMonitor/TaskMonitor/Noticebell 等，卸载后不再 setState）
- antd v6 废弃 API 批量迁移：`destroyOnClose→destroyOnHidden`、`maskClosable→mask.closable`、`Space.direction`、`Drawer.width`、`Card.bodyStyle`（共 10 处）
- 财务总览饼图 label 溢出修复（`avoidLabelOverlap`、`minShowLabelAngle`、legend 滚动）
- `SliderCaptcha` 用 ref 解决 `useEffect` 依赖闭包问题
- 5 处 Table `rowKey` 删除 `index` 参数（避免 React key 抖动）
- 清除孤儿权限 `supply_chain.purchase_plan`

### RPA 数据对齐（大工程）
- **抖音分销** 4 个导入函数 `stat_date` 改用 Excel 日期列（原先用文件名日期，对不上业务日），并重导 2026-02-19~04-18
- **京东联盟双导入冲突修复**：`import-jd.importAffiliate` 删除，统一由 `import-promo.importJDAffiliate` 处理（遍历 Excel 日期列，支持多天）
- 飞鸽客服 170 天补齐（`import-customer` 全量重跑）
- 4 个导入工具全量重跑（jd/douyin/pdd/vip）补齐历史漏跑
- 运营表补 `UNIQUE KEY`（material/industry_keyword/promo_sku），修复 UPSERT 失效
- 新增 RPA 探针工具：`probe-rpa-integrity`、`probe-rpa-deepcheck`、`probe-rpa-vs-db`、`probe-excel-dates`

### 数据库优化
- 45 张月份表注释修正（`trade_*` / `trade_goods_*` / `trade_package_*`）
- 删除 38 个冗余索引（`sales_goods_summary` + `trade_goods_*` + `op_*` 等）
- 15 张 `trade_*` 删除 `idx_trade_status`（cardinality=1 无区分度）
- `hesi_flow.title` 缩到 VARCHAR(1024)，`notices.id` 升到 BIGINT
- `stock_quantity` 删除冗余 `idx_goods_id`
- DROP 12 张 `_unused_*` 备份表（释放约 200MB）
- 新增 SQL 脚本留档：`fix-table-comments.sql`、`fix-redundant-idx.sql`、`fix-drop-useless-idx.sql`

### 运维加固
- 新增定时任务 `BI-BackupMySQL`（每天 02:00，`mysqldump + gzip` 到 `Z:\信息部\bi-backup\`，保留 30 天）
- 新增定时任务 `BI-RotateLogs`（周日 03:00，>10MB 活跃日志轮转，30 天前 `manual-*` 归档，90 天前清理）
- 新增定时任务 `BI-SyncOpsFallback`（每天 13:00 兜底跑 10 个 import 工具，RPA webhook 优先）
- `sync-ops-daily.bat` 扩充到 10 个平台（补 douyin/douyin-dist 等）
- `channel.go` 所有 silent error 加日志
- 归档 29 个旧 server 日志 + 103 个桌面截图/临时 JSON

### 已知待修（吉客云 API）
以下日期 API 返回异常，等吉客云修复后用 `TRADE_ARCHIVE=1 sync-trades-v2.exe` 重跑：
2025-01-18、2026-01-05、02-07、03-03、03-11、03-13

---

## v0.27 — RPA stat_date 全面审计 + PDD MM-DD-YY 日期解析 + 客服总览 T-3 提示

### 核心工作
v0.26 后跑哥指出"RPA 文件名日期 ≠ 文件内日期"这条规则要**全覆盖**，于是对剩余 10 个 import 工具的所有函数做了完整审计 + 修复 + Q1 历史重跑。

### 通用日期解析 helper 严格化
`parseExcelDate` 在 5 个工具（tmall/jd/vip/pdd/customer）统一严格化：
- 必须 4 位年份（避免 YY 误解析导致数据污染 — pdd 早期版本曾将 YY 当 4 位年解析出 2001-01-26 这种荒谬日期）
- 格式不合规返回 ""，调用方 fallback 到文件名日期

### PDD 专用日期解析（MM-DD-YY）
- 拼多多 `销售数据_交易概况/商品概况/服务概况` Excel "统计时间"列是**美式短年份** `MM-DD-YY`（例：`12-29-25` = 2025-12-29）
- `import-pdd/main.go` 的 `parseExcelDate` 专门扩展：识别三段都 ≤ 2 位且第 0 段 ≤ 12（月份合法）时走 MM-DD-YY 分支，YY 补全规则 00-69→20xx / 70-99→19xx
- 标准 ISO YYYY-MM-DD 仍正常解析（按 4 位年分支）

### 新审计工具
- `server/cmd/probe-rpa-headers/`：扫描所有 RPA Excel，对每种文件类型取样本输出 header + 首个业务行（支持扫 15 行跳过汇总标签），一眼看清哪些文件有日期列、列名叫什么、位置在第几列
- `server/cmd/inspect-pdd-date/`：单独探测 PDD Excel 日期列格式
- `server/cmd/check-monthly/`：吉客云月度数据核对工具

### stat_date 修复清单（v0.27 新增）
- **import-pdd.importShopDaily / importGoodsDaily / importServiceOverview** — Excel 第 0 列"统计时间"MM-DD-YY 格式
- **import-jd.importCustomerDaily** — 京东客户数据_洞察 Excel "日期"列（`2025.12.31` 点分隔格式）
- **import-jd.importPromoDaily** — 京东营销数据_百亿补贴/秒杀活动 Excel "日期"列
- **import-customer.importJDWorkload / importJDSalesPerf** — 京东客服工作量/销售绩效 Excel 第 0 列日期
- **import-tmall.importServiceInquiry/Consult/AvgPrice/Evaluation** — 从宽松 parseExcelDate 升级到严格版

### Q1 历史数据重跑（全量）
7 个工具都重跑过 `20260101 20260331`：
- import-tmall（10+ 张表） / import-jd（7 张） / import-vip（4 张） / import-pdd（3 张 + 客服） / import-douyin（7 张） / import-customer（7 张） / import-tmallcs（9 张） / import-douyin-dist / import-promo

### 数据真相暴露（非 bug）
跑完后发现部分表 Q1 数据不全，审计确认是 **RPA 采集本身的特性**，不是代码问题：
- **天猫超市 shop_daily / goods_daily** 从 2026-03-11 起有数据：RPA 在 2026-04-12 才补抓历史目录（Q1 目录是虚拟归档日，Excel 内容是 4-12 时点的 30 天滚动快照）
- **抖音分销** 从 2026-02-19 起有数据：RPA 2-19 才开始采集
- **京东客服 / 快手客服** 只有 4 月起数据：RPA 4 月才开始采集
- **天猫业绩询单**（T-3 滞后）最新到 2026-04-17：RPA 4-20 文件内含 4-17 业务数据，符合预期

### 客服总览 UX 优化
- 天猫 tab "询单人数"列头加 `ℹ️` 悬停提示：「生意参谋业绩询单数据由 RPA 采集，通常存在 T-3 左右延迟（例：4-20 采集的是 4-17 的数据）。近 3 日空值为正常现象。」
- 客服看到 4-18/19/20 询单为 0 不再困惑

### 业界 RPA 采集异常（待 RPA 同事排查）
- 京东推广京东联盟 2026-01-10 文件含 2026-03-14 数据（delta=-63 天）— 文件名跟内容完全不符

### 遗漏但无需改
- **飞瓜 达人数据/归属** Excel 无日期列（每行是达人记录），文件日 = 数据快照日即业务日
- **抖音 其他 6 个函数**（live/goods/channel/funnel/anchor/admaterial）Excel 无日期列
- **拼多多 其他推广表**（明星店铺/直播推广）Excel 第 0 列日期但样本为空，v0.26 已改但不可验证

---

## v0.26 — RPA stat_date 来源修正 + 客服总览咨询/询单拆列

### 核心问题
RPA 文件名日期 ≠ 文件内部业务日期。部分 RPA 产出的文件**滞后 T-N 天**（如天猫生意参谋业绩询单 T-3、抖音推广直播间 T≥4），代码里将文件名日期作为 `stat_date` 入库导致数据错位；多天滚动 Excel（如抖音推广直播间 30 天滚动）如果只取第一行 + 文件名日期，还会覆盖丢失其他 29 天的数据。

### 审计工具
- 新增 `server/cmd/probe-rpa-date-lag/`：遍历所有 RPA Excel，对比"文件名日期 vs 文件内第一个业务日期"的 delta，按文件类型分组统计 delta 分布，一眼看出哪些类型有滞后、哪些是多天滚动。
- 新增 `server/cmd/check-sku-diff/` / `server/cmd/check-api-fields/`：吉客云接口字段对比调研工具。

### 真 bug 修复（probe 证实有滞后）
- **import-tmall.importServiceInquiry** — 天猫业绩询单 T-3 滞后（193/220 样本），`stat_date` 改用 Excel 第 0 列"日期"，新增 `parseExcelDate` helper 兼容 YYYY-MM-DD / YYYY/M/D / 20260417 等格式
- **import-tmall.importServiceConsult / importServiceAvgPrice / importServiceEvaluation** — 天猫其他 3 个客服文件，同模式修复（实测都是 T-0，但按统一规则预防性修复）
- **import-douyin.importAdLiveDaily** — 抖音推广直播间画面（多天滚动 Excel），`stat_date` 改用 `get("日期")` 行级业务日，**16 条→69 条**（还原 30 天滚动数据）
- **import-jd.importShopDaily** — 京东店铺销售 T-1 零星滞后（4/222），读 Excel 第 0 列"时间"
- **import-vip.importShopDaily** — 唯品会店铺销售 T-1 零星滞后（10/183），读 Excel 第 0 列

### 历史数据清洗
- TRUNCATE 4 张天猫客服表（`op_tmall_service_*`）+ DELETE 4 月 `op_jd_shop_daily` / `op_vip_shop_daily` / `op_douyin_ad_live_daily`
- 用新 exe 重跑 2026-04 全月，`stat_date` 全部按业务日对齐

### 客服总览「咨询 / 询单」拆列
- 暴露真相后发现前端"询单人数"字段其实拉的是 `op_tmall_service_consult.consult_users`（咨询人数），跟"询单转化率"（`op_tmall_service_inquiry.daily_conv_rate`）数据源错位，字段名误导
- 后端 `dashboard.go` 新增 `inquiry_users` 字段（天猫=`ti.inquiry_users`，其他平台兼容为同 `consult_users`），贯穿 `customerMetricRecord` / `customerMetricAgg` / `customerPlatformStat` / `customerTrendPoint` / `customerShopStat` 5 个结构体
- 前端 `Overview.tsx` 天猫 tab 拆成 **咨询人数**（consultUsers）+ **询单人数**（inquiryUsers）两列，其他平台（抖音/京东/拼多多/快手/小红书）保持单列"询单人数"向后兼容
- 实测效果：松鲜鲜调味品旗舰店 4-01~4-20 咨询 5,833 → 询单 1,298（~22% 漏斗转化）→ 41.71% 付款转化率

### probe 排除的假警报
以下 probe 曾报"d>0 滞后"但实为多天滚动 Excel + import 已正确遍历处理，**代码无需改动**：
- 天猫超市_销售数据_经营概况 / 淘客诊断 / 销售数据_商品（`importBusinessOverview` / `importTaoke` / `importGoods` 已用 `d[0]` / `get("日期")` / `get("统计日期")`）
- 抖音分销投放推商品 / 推抖音号 / 推素材（v0.24 已修为遍历 rows 用 `get("日期")`）

### 严重 RPA 异常（待联系 RPA 同事）
- 京东推广京东联盟：样本中发现 `20260110` 文件内含 `2026-03-14` 数据（delta=-63 天），文件名与内容完全不符，疑 RPA 抓错文件

### 剩余待改（保留 v0.27 处理）
Probe 显示 `d=0` 无显著滞后但为统一规范，按"所有 RPA 读 Excel 日期列"要求还需批量改：
- import-customer 5 个函数（JDWorkload / JDSalesPerf / DouyinFeige / KuaishouAssessment / XHSAnalysis）
- import-douyin 另外 6 个函数（live/goods/channel/funnel/anchor/admaterial）— 多数 Excel 无日期列，实际可能不适用
- import-feigua 2 个函数（CreatorDaily / CreatorRoster）
- import-pdd 3 个函数（ShopDaily / GoodsDaily / ServiceOverview，Excel 第 0 列"统计时间"）

---

## v0.25 — 天猫超市推广重做、视觉主题重置与凌晨误报消除

### 天猫超市推广模块彻底重做（双店支持）
- 2025-12-31 起天猫超市拆分为**一盘货 + 寄售**两家店，`shop_daily` UK 改 `(stat_date, shop_name)`；`shop_name` 统一存简称（"天猫超市一盘货" / "天猫超市寄售"）
- 原 `op_tmall_cs_campaign_daily` 单表硬编码 "天猫超市" + 字段混装三种推广文件问题严重，**DROP 并改为兼容 VIEW**（UNION ALL 三张新表）
- 新建 6 张推广表：`wujie_scene_daily` / `wujie_detail_daily` / `smart_plan_daily` / `smart_plan_detail_daily` / `taoke_daily` / `goods_daily`
- `import-tmallcs.exe` 完全重写：`rpaShopToName` 原名→简称映射、9 个 import 函数覆盖 11 种文件类型
- 110 天 × 2 店全量重导
- `dashboard.go` `tmall_cs` case 补"店铺CPC分组"query，营销费用页恢复"各店铺推广投入对比"图

### 万象台营销明细接入
- 新表 `op_tmall_campaign_detail_daily`（71 列），`import-tmall.exe` 扩展支持万象台明细 Excel
- 7 天滚动 × 24 商品，161 条入库

### 定时同步优化（凌晨误报消除）
- `sync-daily-trades` / `sync-trades-v2`：吉客云对空时段返回空 body 时，老代码按解析失败 retry 3 次导致凌晨红字
- 改为空 body 直接 `break`，下次定时任务不再刷告警
- 经补拉确认凌晨本就几乎无订单（04-16 凌晨 0~7 时合计仅 1 条），历史凌晨"失败"实为正常空时段

### 后端 Bug 修复
- **费控管理 `/api/hesi/flows` 500**：`SUM(invoice_status='exist')` 在没有匹配 `hesi_flow_detail` 行时返回 NULL，Scan 到 `int` 失败；加 `COALESCE(SUM(...),0)` 修复

### 前端容错修复
- RPA 监控：后端 `ImportProgress` nil 返回完整默认对象 + 前端 `merge state` 兼容 undefined
- 营销费用页：切换平台再切回"全部店铺"数据不刷新 → 合并 `useEffect`、移除 `if selectedShop !== 'all'` guard、`Tabs.onChange` 同时 reset shop
- 店铺看板：`platform` 初始 `''` → `'all'`，消除首次 mount 时 antd Tabs 激活 tab `.focus()` 导致的**光标闪烁**

### 飞瓜数据映射修正
- `docs.go` 飞瓜映射从 1 条拆成 2 条（`fg_creator_daily` 达人数据 + `fg_creator_roster` 达人归属）
- 飞瓜数据本身完整，不需重抓

### 视觉主题重置（BI 专业配色）
- **主色**：`#4f46e5` 靛紫 → **`#1e40af` 深青蓝**（BI 看板经典主色）
- **`chartTheme.ts` 统一**：新增 BI 经典 10 色调色盘（青蓝/金黄/翡翠/辣红/青瓷/紫/橙红/松柏/玫红/石板灰）、`DEPT_COLORS`（电商青蓝/社媒金黄/线下翡翠/分销紫）、`GRADE_COLORS` 产品定位 SABCD 热力渐变（辣红→金黄→青瓷→翡翠→冷灰）
- **消除所有旧色硬编码**：6 处 SABCD `gradeColors` 抽成常量 `GRADE_COLORS` 共享；60+ 处旧主色 `#4f46e5` 批量换成 sage → 最终深青蓝；套装色 `#8b5cf6 / #f97316 / #ec4899 / #1890ff / #d9a45c / #4a8a85 / #c88b3a` 同系替换到新调色盘；毛利率阈值色 `#5f9c68 / #c97a7a` 改 BI 热力 `#059669 / #dc2626`
- **`App.tsx`**：`ConfigProvider` 全局主题 token 改青蓝 + 圆角 10 + Space Grotesk 字体
- **`index.css`**：CSS 变量重置，引入 Space Grotesk，数字字段加 `tabular-nums` 对齐
- **`MainLayout`**：SXX logo 渐变改青蓝→金黄
- 业务语义色（涨跌红绿 `#10b981 / #ef4444`、风险阈值三段、库龄告警）保留原值不动

### 数据库变更留档
- `server/fix-tmallcs-rebuild.sql` — 6 张天猫超市新表建表
- `server/fix-tmallcs-campaign-view.sql` — 兼容 VIEW（UNION ALL）定义

---

## v0.28 ~ v0.58 (2026-04 集中开发月) — YS用友/采购计划/线下大区/财务双流/审计权限

### YS 用友 YonBIP 全套接入 (v0.45 ~ v0.49)
- v0.45 采购订单 (213 字段全量) + access_token 拼接 urlencode 修复
- v0.46 委外订单 (168 字段)
- v0.47 材料出库 (包材实际消耗金标准)
- v0.49 现存量接入 (data 直接 array, 不是 data.recordList)
- 限流 1.1s/请求 + simpleVOs 日期过滤 (top-level vouchdate 静默失效)

### 供应链 — 采购计划仪表盘 (v0.48)
- 5 表 JOIN (吉客云销售 + YS 采购 + YS 委外 + YS 现存量 + sku_code 桥接)
- 在途采购/委外扣减 + lead_time 估算 + 7 仓白名单
- 包材改用 YS 真库存 (替代吉客云估算)

### 线下部门重做 (v0.34 ~ v0.38)
- 大区维度合并展示 (华北/华东/华中/华南/西南/西北/东北/山东/重客)
- 月度销售目标管理 + 大区进度条
- 大区数据预览 KPI 重排 (达标数/日均/店均)
- 货品 TOP15 柱状图

### 综合看板增强
- 产品定位 × 渠道分布饼图 + hover 各部门销售明细
- KPI 卡部门标签优化 (移至右上角)

### 财务模块强化 (v0.50 ~ v0.58)
- 财务报表"预览 → 确认"两步流 (防误覆盖 + 变更预览 + 异常告警)
- v0.57 财务父项 = SUM(子项) 自动补 row (仓储物流费用首例)
- v0.58 业务预决算报表 (4 年 27522 行 + 4 API + 4 子 tab)

### 系统/运维 (v0.55+)
- 审计日志全套 + 数据导出权限
- 移动端适配 + nginx 反向代理
- 系统菜单合并

### 其他
- v0.20 安全加固 (滑块/OTP 一次性、SQL 误报清查)
- 库存手动同步 + 状态轮询 + 缓存失效
- PDD 推广 SKU 级日数据 + 营销看板深化

---

## v0.59 ~ v1.46.0 (2026-05 高频迭代月) — 物化加速 / 分销客户分析 / 组合装BOM / 全站颗粒度对齐

### 性能 — 物化预聚合表 (v0.60)
- `warehouse_flow_summary` 物化预聚合切月查询 7s → <50ms (~140x 提速)
- 双轨路由 `canUseSummary(ym)` 自动判断, 没物化降级原 SQL
- schtasks 03:30 每天自动重建

### 分销·客户分析模块全套 (v1.28 ~ v1.45)
- 高价值客户排名 + 4 KPI (高价值客户数/贡献销售额/占比/月环比)
- 客户名单管理 (48 个 S+A 级业务名单, 29 线上分销 + 19 礼品渠道)
- 销售趋势钻取 Modal (月度时序 + 历年同月对比)
- 销售明细钻取 Modal (按 SKU, 含组合装/单品 Tag)
- 组合装 BOM 全链路 (吉客云 `erp-goods.goods.listgoodspackage`, 8270 父品 / 18481 子件)
- 子件按 `share_ratio` 真实分摊销售额 + 实际卖出件数 (parent.qty × goods_amount)
- antd Table 字段冲突修 (children → packageChildren, 不被 TreeData 自动展开)

### 即时零售部独立 (v1.31)
- 朴朴渠道从电商部搬家到即时零售部
- 特殊渠道调拨对账复制一份给即时零售 (dept=instant_retail)

### 销售单字段补齐 (v1.30)
- 货品自定义字段 customizeGoodsColumn3 (核销费用) + customizeGoodsColumn4 (建议价)
- sync-daily-trades 5min 窗口降低 scrollId 200 截断丢数 (1h→30min→10min→5min, 88.8%)

### 系统/运维 (v1.21 ~ v1.27)
- 钉钉登录 audit_log 用户名修复 + 资源中文翻译 + 历史回填 55 条
- 财务/费控模块软关闭 (保留 DB+backend, 隐藏菜单)
- sync-channels 不覆盖手动改 + 渠道部门改同步月表 + 缓存清理
- 孤儿 PID lock 检测 (importutil.AcquireLock + STILL_ACTIVE 259)
- 手动同步任务状态准确 (不再 0-second 假完成)

### 全站视觉颗粒度对齐 (v1.33 ~ v1.45.1)
- 客户分析/客户名单/审计日志/目标管理 删冗余 Title (面包屑已显示页面名)
- KPI 装饰色清理 (违规 valueStyle, 保留状态语义色)
- 客户编码字体回归 antd 默认 (删 monospace+12px+#94a3b8)
- 灰色 hint 文字统一 #64748b + 13px + tabular-nums

### 数据真实化 + 平台映射修复 (v1.46.0)
- 删除 dashboard_department.go 排行 SQL `LIMIT 20` 截断, 全量返回
- 新增 `shopTotalCount` (COUNT DISTINCT) — 区分排行 vs 真实总数
- 社媒 20→29, 分销 20→54 真实店铺数显示
- GoodsChannelExpand getPlatform() 加视频号识别 (视频小店"其他" → "视频号")
- 分销·货品看板隐藏单值 100% 平台分布 (跟 offline 一致)

### 文档/规范 (v1.45.1)
- CHANGELOG 顶部加"## 版本号约定"段, 启用 SemVer 三段
- 跑哥 2026-05-10 拍板: 保留 v1.x 顺延 (接受历史误升), 三段足够

---

## v1.46.1 ~ v1.55.7 (2026-05-10 ~ 2026-05-11 双日高频迭代) — 销量预测算法体系 / 测试工程化 / 销售单字段补齐 / 仓库卫生

### 销量预测模块全套上线 (v1.48 ~ v1.55, 业务核心)
- 新模块: 线下部门销量预测管理 — 9 大区 × SKU × 月度预测填写, 路由 `/offline/sales-forecast`
- **三套算法可切换**:
  - **内置算法** — 五层智能: 季节系数 + 春节滑动修正 + 客观度判定 + 大区同比 + 环比保护
  - **Prophet** — Facebook 时序预测, 每周日凌晨 3 点自动重训
  - **StatsForecast** — Nixtla 多模型集成 (AutoARIMA / AutoETS / Theta 等)
- **智能模式**: 系统按预测月份自动挑算法 (1-2月春节季 → Prophet, 3-12月常规 → StatsForecast)
- 算法增强:
  - 节假日上下文 Tag (春节滑动 / 国庆 / 端午 / 中秋等)
  - 客观度判定: 季节系数太极端时, 用品类中位数替代 (排除营销污染)
  - 大区同比: 2026-04 回测全国误差 -15.8% → -4.2%
  - 春节季环比保护: 只跳"近1月在春节季"的预测, 避免带飞
- 数据范围:
  - 仅看成品 (10 品类白名单, 排除包材/广宣品)
  - 默认隐藏长期不动的空行 (近6月有销但近3月没卖的 SKU)
- 表格功能:
  - 货品名 / 货品编码 / 季节系数独立列, 不混在一起
  - "线下总计"列 — 9 大区填值实时合计
  - "用新建议覆盖"按钮 + 历史填值/新建议差异提示
  - 季节列加"客观/替代"视觉标识 (Tag)
- **导出 xlsx**: 列宽自适应 (货品名宽 / 编码窄 / 大区数字适中), 文件名 `销量预测_YYYY-MM_YYYY-MM-DD_HHmm.xlsx`
- 顶部加算法说明 Alert, toolbar 简化为 3 按钮 (清空 / 预测 / 保存)

### 销售单字段补齐 + 数据完整性 (v1.47.49 ~ v1.47.52)
- 销售单 21 个 customize 字段全量入库 (含核销费用 / 建议价等)
- scrollId 翻页解析修正: 单次覆盖 87% → 100%
- 即时零售部 API 权限白名单补全 (修加载失败)
- 吉客云销售单异常诊断工具入库 (供运维同学排查翻页问题, 客服证据)

### 测试工程化基建 (v1.47.4 ~ v1.47.40, 整体覆盖 13% → 70.1%)
- 业务老板视角: 系统稳定性提升, 看不到具体变化, 但任何一处改动出错时测试网兜底
- 后端测试覆盖跨过 **工业级退出线 70%** 里程碑 (v1.47.40)
- 八大模块全部 ≥ 67% 覆盖:
  - importutil 94% / dingtalk 89% / finance 87% / business 85% / jackyun 82% / yonsuite 81% / handler 67% / config 100%
- 累计 200 → 600+ Go test case

### 大文件按职责拆分 (v1.47.41 ~ v1.47.45, 60%/70% 测试做安全网)
- auth.go 2171 行 → 6 文件 (captcha / login / session / seed / dingtalk / types)
- supply_chain.go 1769 行 → 5 文件 (dashboard / purchase / ys-sync / intransit)
- admin.go 1517 行 → 4 文件 (meta / users / roles / types-router)
- finance_report.go 1133 行 → 4 文件 (query / import / export / types)
- admin_users.go 764 行 → 2 文件 (CRUD / batch_import)

### 系统工程化升级 (v1.47.46 ~ v1.47.49)
- **CI/CD**: GitHub Actions backend + frontend 自动检查 (push/PR 触发)
- **Secret 管理**: 环境变量覆盖 config.json + .env.example 模板
- **错误监控**: Sentry panic 上报 + 环境变量启用 (默认关)
- **结构化日志**: slog JSON + log 包桥接 + access log middleware

### 仓库卫生 (v1.55.7)
- Go 测试覆盖率报告不再误入库 (.gitignore 加 cov*.out)
- 吉客云销售单异常诊断工具入库
- 清理临时实验脚本

---

## v1.56.0 (2026-05-12) — 系统设置加销售单核对页

- 新页面: **系统设置 → 销售单核对**
- 用法: 选月份, 当月每天一行, 列出销售单数 / 明细数 / 包裹数, 跑哥拿这个数去吉客云后台逐日核对差异
- 顶部三个汇总卡片: 当月销售单合计 / 明细行数合计 / 包裹数合计
- 列支持点表头排序, 表底自带合计行
- 数据口径: 按发货日期统计, 和 BI 看板其他销售口径一致

---

## v1.58.3 (2026-05-12) — 合思筛选输入框改成"点查询才搜"

- 跑哥反馈: 输入"易"就开始搜了, 输错一个字就触发请求, 烦
- 修法: keyword(编码/标题搜索) + approver(当前审批人) 改成 draft 模式
  - 输入框 onChange 只更新本地 draft state (keywordInput / approverInput), 不触发查询
  - 点"查询"按钮或按 Enter 才把 draft 提交到实际 state, 触发请求
- 实测 (playwright):
  - 连输 3 个字符 → 0 次 API 调用 ✅
  - 点查询 → 1 次 API 调用 ✅

---

## v1.61.3 (2026-05-12) — 修反馈管理 KPI 卡筛选后其他状态显示 0 的 bug

跑哥反馈点击"已解决"卡，"已关闭"变成 0 了。根因是 stats 算在被后端筛过的 list 上，筛选后 list 只剩当前状态的数据。

**修法**:
- 后端 `feedback.go ListFeedback` 加 `SELECT status, COUNT(*) FROM feedback GROUP BY status` 一次拿全量 4 状态计数，返回字段 `stats`
- 前端 `Feedback.tsx` stats 从 API 响应取（替换前端 useMemo 在 list 上算的旧逻辑）

**对照**: 用户管理没这 bug 是因为它一次性拉全量 users 到前端，stats 基于 users 算；反馈管理走后端分页+筛选，必须由后端返回全量统计。

**实测**: 点击"已解决" → 待处理 0 / 处理中 0 / 已解决 2 (绿描边) / 已关闭 **1**（之前 = 0）/ 列表 2 行。

---

## v1.61.2 (2026-05-12) — 反馈管理 4 个 KPI 卡可点击切换筛选

对齐 v1.61.1 用户管理改动——反馈管理同模式 4 张卡（待处理/处理中/已解决/已关闭）也都改成可点击。

**改动**:
- `Feedback.tsx` 4 张卡加 `onClick`，点哪张筛到对应状态，再点同一张清除
- 选中卡片加 `outline: 2px solid <对应色>`（橙/蓝/绿/灰）
- `cursor: pointer` + `setPage(1)` 防止筛选后停留在不存在的页码

**实测**: 点击"已解决"（2 条）→ 绿色描边亮起，列表筛到 2 行；再点同张 → 还原 3 条全量。

---

## v1.61.1 (2026-05-12) — 用户管理 4 个 KPI 卡可点击切换筛选

跑哥反馈点击"待审批"KPI 卡不会自动筛选——历史问题（KPI 卡从来没绑点击），既然 v1.61.0 加了徽标筛选 UX，这里顺手对齐。

**改动**:
- `UserAccess.tsx` 4 张卡片（总用户 / 已启用 / 已停用 / 待审批）全部加 `onClick`，点哪张筛到对应状态，再点同一张清除
- 总用户卡 = 点击清除所有筛选（回到全量）
- 选中的卡片加 `outline: 2px solid <对应色>`，视觉上一眼能看出当前过滤状态
- `cursor: pointer` 提示可点击
- 过滤 Tag 扩展支持 active / disabled / pending 三个状态（之前只有 pending），颜色随状态变（绿/默认/橙）

**实测**: 点击"待审批" → 橙色描边亮起 + 列表 1 行 + "仅看待审批 · 1" Tag；再点同一张 → 还原全量。

---

## v1.61.0 (2026-05-12) — 用户管理/反馈管理菜单加待审批徽标 + 一键直达筛选

跑哥反馈: 用户管理有待审批用户/反馈管理有未处理反馈时，菜单上没提醒，管理员要点进去才知道。参考 Slack/Notion/Antd Pro/钉钉后台后台通用做法加上徽标。

**改动**:
- 后端新接口 `GET /api/admin/pending-counts` — 一次返回 users.pending + feedback.pending 两个计数，按权限脱敏（无 user.manage / feedback.manage 权限对应字段返回 0）
- 前端新 hook `usePendingCounts` — 5 分钟轮询（对齐公告铃铛节奏），无权限时不发请求
- `navigation.tsx` 加 `decorateLabel` 注入逻辑：用户管理/反馈管理 label 旁挂数字 Badge（>99 显示 "99+"），系统设置父菜单聚合显示红点 dot（任意子项有就亮）
- `MainLayout.tsx` 菜单点击逻辑：当目标项 pending > 0 时自动追加 `?status=pending`，0 则正常导航
- `UserAccess.tsx` 支持 URL `?status=pending` 默认筛选到待审批用户，过滤区显示橙色"仅看待审批 · N"可关闭 Tag
- `Feedback.tsx` 支持 URL `?status=pending` 默认切到待处理（原有 statusFilter Select 自动同步）

**视觉规则**:
- 0 → 徽标完全隐藏（不占位）
- 1-99 → 红色数字徽标
- 100+ → "99+"
- 父菜单只显示红点（无数字），任意子项有待办就亮

**单点审批**: 待审批是公共池子，多个管理员看到同一个数字；谁先处理谁审，不做已读隔离。

**实测**: 当前 1 个待审批用户 → 用户管理徽标显示 "1"，系统设置父菜单红点亮；点击用户管理 URL 跳到 `?status=pending`，列表自动筛到 1 行，橙色 Tag 提示，× 可清除。反馈管理 0 待办 → 无徽标 → 点击不带 query。

---

## v1.60.5 (2026-05-12) — 个人信息页字段精简 + 头像下方/左下角显示"虎跑-黄承欢"

跑哥进一步反馈: 合思名字 = 钉钉真名 = 同一个真名, 不用重复展示; 真实姓名(BI看板用) 跟钉钉昵称同源也不展示; 顶部头像下方 + 左下角导航卡都用"昵称-真名"格式更直观.

**改动**:
- `auth.go` authUser 加 `DingtalkRealName` 字段, `auth_session.go loadAuthPayload` SELECT 多取 dingtalk_real_name
- 前端 `AuthContext` AuthUser 类型加 dingtalkRealName?
- `MainLayout.tsx` displayName 改成 `${nick}-${realName}` (二者不同时), 左下角卡片自动跟着改
- `Profile.tsx` 左侧账号信息: 删 "合思真名" 行, "钉钉真名" → "真实名字"
- `Profile.tsx` 右侧个人信息 Form: 删 "合思名字" + "真实姓名（BI 看板用）"
- `Profile.tsx` 头像下方 h2: realName-dingtalkRealName

**实测**: 跑哥页面顶/底/左/右 4 处都显示 "虎跑-黄承欢" 一致.

---

## v1.60.4 (2026-05-12) — 个人信息页右侧加 3 个只读字段(钉钉昵称/真实名字/合思名字)

跑哥反馈左侧"账号信息"看了一眼就过去, 右侧"个人信息"编辑区也想要 3 个名字字段并列展示, 方便对比.

**改动**: `Profile.tsx` 右侧个人信息 Form 顶部新增 3 个 disabled Form.Item:
- 钉钉昵称 (real_name)
- 真实名字 (dingtalk_real_name) + suffix "从钉钉同步"按钮
- 合思名字 (hesi_real_name)

下面原"真实姓名"输入框重命名为"真实姓名（BI 看板用）", 显示 real_name 同源, 减少混淆.

---

## v1.60.2 (2026-05-12) — 同步钉钉昵称/真名到 BI 看板

**Why**: v1.60.1 用合思手机号自动绑定 89%, 但合思真名 = 钉钉通讯录真名, 钉钉是登录入口, 是更稳的数据源. 让用户在个人信息页直接看到自己的钉钉昵称(虎跑) vs 真名(黄承欢) 差异, 并支持手动/自动同步.

**这次做的**:
- `users` 表加 `dingtalk_real_name` 字段 (钉钉通讯录真名)
- 后端 3 个接口/钩子:
  - `POST /api/profile/sync-dingtalk` — 当前用户单同步
  - `POST /api/admin/sync-all-dingtalk-names` — 管理员一键全员同步
  - `DingtalkLogin` 钉钉扫码登录后异步触发 `asyncSyncDingtalkName` (不阻塞登录)
- 前端:
  - 个人信息页"账号信息"卡片加 3 行: 钉钉昵称 / 钉钉真名 / 合思真名 + "从钉钉同步" 按钮
  - 用户管理页(/system/access)工具栏加 "同步钉钉真名" 一键全员按钮
- `GET /api/user/profile` 接口返回新字段 `dingtalkRealName` / `hesiRealName`

**实测一键全员**: 36 个绑定钉钉的用户, **36/36 全部同步成功 (100%)** — 比 v1.60.1 合思手机号匹配 89% 还高
- 跑哥 admin (虎跑) → 黄承欢 ✓
- 石榴 → 李磊磊 / 蹄子 → 邢荣荣 / 小雨 → 张思雨 / 贝 → 邢贝贝 (钉钉昵称背后的真名揭晓)

**复用资产**:
- 钉钉 `oauth2/accessToken` + `topapi/user/getbyunionid` + `topapi/v2/user/get` 接口链路 (`getDingtalkDepartment` 函数已用过, 提取通用 `fetchDingtalkRealName`)

**后端**: `server/internal/handler/{profile_dingtalk_sync.go(新), profile.go, auth_dingtalk.go}` + `cmd/server/main.go` 注册路由
**前端**: `src/pages/system/{Profile.tsx, UserAccess.tsx}`
**数据**: `add-dingtalk-real-name.sql`

---

## v1.60.3 (2026-05-12) — 销量预测导出 Excel 带页面样式 + 4 算法回测对比

**Why**: 跑哥要把销量预测管理推广到群里, 导出 Excel 之前是纯白无样式, 跟页面视觉不一致.

**这次做的**:
- `xlsx` → `xlsx-js-style`: 替换库, 支持单元格 s 样式属性
- `SalesForecast.tsx` handleDownload:
  - 表头蓝底白字加粗居中
  - 数值列右对齐
  - 合计列(线下总计)浅黄底加粗
  - 全表细灰边框
- 修复 `prophet_backtest.py` 的 SQL bug: `DATE_FORMAT(stat_date, '%%Y-%%m')=%s` 跟 mysql.connector pyformat 占位符冲突导致 actual=0, 改成 BETWEEN 日期范围
- 新增 `statsforecast_backtest_v2.py`: 月级聚合 + season_length=12 + AutoARIMA+AutoETS+AutoTheta 集成

**实测回测 (2026 年 1-4 月, 大区合计 预测 vs 实际)**:

| 月份 | Prophet | StatsForecast v2 | 智能(按月路由) |
|---|---|---|---|
| 1月 春节备货 | +10.2% | -48.8% | +10.2% (Prophet) |
| 2月 春节 | +8.1% | +59.0% | +8.1% (Prophet) |
| 3月 平稳 | -16.9% | -7.5% | -7.5% (SF) |
| 4月 平稳 | -3.1% | -3.9% | -3.9% (SF) |
| **MAPE** | **9.6%** | **29.8%** | **7.4%** ⭐ |

数据印证代码注释: Prophet 春节月强 + StatsForecast 平稳月强 + 智能按月路由 MAPE 最低 (7.4%, 比单算法都低).

**前端**: `src/pages/offline/SalesForecast.tsx` + `package.json` 加 xlsx-js-style@1.2.0
**回测**: `server/cmd/prophet-backtest/{prophet_backtest.py(fix), statsforecast_backtest_v2.py(新)}`

---

## v1.60.1 (2026-05-12) — 合思机器人按 staffId 精确匹配 (修 91% 用户匹配不上的 bug)

**Why**: v1.59.0 用 BI 看板 `users.real_name` 模糊匹配 `hesi_flow.current_approver_name`. 跑哥发现自己合思真名 "黄承欢", BI 看板昵称 "虎跑", 匹配不上看不到待审批. 数据查证: **44 个用户中 40 个(91%)用昵称, 匹配不上**.

**根因**: BI 看板 real_name 字段实际填的是钉钉昵称(虎跑/石榴/蹄子/小雨/贝/大lin...), 不是身份证真名.

**修法**:
- users 表加 `hesi_staff_id` + `hesi_real_name` 字段 (ALGORITHM=INSTANT)
- 一次性脚本拉合思全员 (867 人) 通过 `GET /api/openapi/v1.1/staffs`
- 按 `users.username == staff.cellphone` 自动匹配, 命中 35 (80%)
- 真名兜底 (BI nick = 合思 name) 命中 3
- admin (跑哥)手工 SQL 绑 → 黄承欢/ID01Fp0kuLamV9
- 合计 **39/44 = 89%** 自动绑定
- 剩 5 个别名账号 (团大/电商/财务/客服/采购计划) 后续手填
- 后端 `profile_hesi_pending.go` 改用 `current_approver_id LIKE '%hesi_staff_id%'` 精确匹配, 不再误命中同名

**实测**:
- 跑哥(admin→虎跑→黄承欢) 登录 "我的待审批" → 出现 S26002382 (采购晋升人员电脑申请单) ✓
- 张俊 (real_name=张俊 自动绑成功) 维持 5 单待审批 ✓

**附带改动**:
- `BI-SyncHesi` schtask 从 Daily 10:30 改成 **Hourly** (每小时 1 次)
- 张俊提交单据 → 本地同步 → 出现在"我的待审批", 最多滞后 1 小时

**后端**: `server/internal/handler/profile_hesi_pending.go`
**数据**: `add-hesi-staff-fields.sql` + `bind-hesi-users.sql`

---

## v1.60.0 (2026-05-12) — 合思机器人加"规则编辑器" (第 2 期 MVP)

**Why**: v1.59.0 只读看清单, 这一期让跑哥能**配自己的自动审批规则** (按字段+操作符+值的 AND 条件组), 但默认干跑模式, 不调合思 API 真审批. 离 v1.62 真自动审批越来越近, 但护栏越上越严.

**这一期做的**:
- 合思机器人页顶部加 **"我的审批规则"** 卡片
- "添加规则" / "编辑" / "删除" 完整 CRUD, 鉴权按 user_id 隔离 (谁的规则谁改)
- 规则字段:
  - 规则名 + 启用开关 + 干跑开关 (默认开)
  - 动作 (同意/驳回) + 金额上限护栏 (默认 1000 元) + 优先级
  - 审批备注 (会作为合思审批意见)
  - 条件 (Form.List 动态加): 单据类型 / 支付金额 / 报销金额 / 借款金额 / 标题 / 当前节点 → 等于/包含/小于/大于...
- 黄色警告条提醒: "当前规则不会真审批 (v1.60 阶段)"
- 数据库新表 `hesi_auto_rule` (user_id + name + enabled + dry_run + action_type + max_amount + conditions_json + priority + 统计字段)

**实测 (playwright)**:
- POST 创建规则 → UI 自动 refetch ✅
- PUT 编辑金额上限 1000 → 2000 ✅
- DELETE 删除规则 ✅
- GET 列表正确反序列化 conditions_json ✅

**路线图**:
- v1.60 ✅ 规则编辑器 (当前)
- v1.61 干跑扫描 (匹配到的单据写日志, 不真审批)
- v1.62 真自动审批 (需财务/合规先批准)

**后端**: 新增 `server/internal/handler/profile_hesi_rules.go` (ListMyHesiRules / CreateMyHesiRule / HesiRuleByPath PUT+DELETE)
**前端**: 新增 `src/pages/system/HesiBotRules.tsx`, HesiBot.tsx 顶部引入

---

## v1.59.0 (2026-05-12) — 个人中心加"合思机器人" Tab (MVP 只读)

**Why**: 跑哥要做合思自动审批机器人. 但全功能涉及自动调合思审批接口, 高风险, 需要财务/合规先批准. 拆 4 期 MVP 推进, 第 1 期纯只读零风险.

**这一期做的**:
- 个人中心改成 Tabs: **个人信息 | 合思机器人**
- "合思机器人" Tab 显示**当前登录用户**的待审批合思单据
- 匹配方式: BI 看板用户 real_name 模糊匹配 hesi_flow.current_approver_name
- 表格列: 单据编码 / 标题 / 类型 / 状态 / 当前节点 / 金额 / 提交日期
- 顶部统计: 等我审批 X 单, 涉及金额合计 ¥X
- 路线图 Alert 写清后续: v1.60 加规则编辑器 → v1.61 干跑模式 → v1.62 真自动审批

**风险评估**:
- 当前阶段零风险 (只读, 没动合思任何数据)
- 已建议跑哥先跟财务/合规沟通 v1.62 真自动审批的合规性
- 同名问题后续 v1.60+ 用合思工号 (operator.code) 兜底

**后端**: 加 GET `/api/profile/hesi-pending` 接口, 按 session.user.real_name LIKE 查
**前端**: 新页面 src/pages/system/HesiBot.tsx + Profile.tsx Tabs 改造

---

## v1.58.2 (2026-05-12) — 合思费控页加"当前审批人"搜索框

- 筛选栏新增**当前审批人**输入框 (在"搜索编码/标题"和日期范围之间), 支持模糊搜索
- 后端 GetHesiFlows 加 approver query param, WHERE current_approver_name LIKE %?%
- 多审批人节点 (例: 张三+李四) 输入任一人名都能命中
- resetFilters 同步清空 approver

用法举例: 跑哥想看"总经理(易子涵)还卡了哪几张单" → 输入"易子涵" → 表格只剩待她审批的单

---

## v1.58.1 (2026-05-12) — 修"看同步日志"显示空的 bug

跑哥反馈点"看日志"只显示 2 行 getAccessToken, 没真实进度.

根因: sync-hesi.exe 主流程用 fmt.Printf 输出 (不是 log.Printf), v1.57.1 加的 MultiWriter 只重定向 log 包, fmt 输出走 stdout 进了 manual-sync-hesi-*.log 而不是 sync-hesi.log.

修复: 后端 admin_task_log_tail.go 加 fallback —
- 优先读固定 log
- 内容 < 5 行时降级 glob 找最新 manual-{key}-*.log 取代
- 跑哥前端"看日志"立刻看到完整进度 (附件 X/X / 同步完成 / 审批状态更新 X 条)

实测: sync-hesi 跑完 6 分 4 秒, 1780 单 / 4366 附件 / 461 单审批回填, 数据库 251 单已写入 current_approver_name (有效审批人姓名), 26 个不同审批人.

---

## v1.58.0 (2026-05-12) — 合思费控页"当前审批"显示真实姓名

接 v1.57.2, 跑哥说不要只显示岗位名要看具体审批人. 这次彻底搞定.

**接口攻关**:
- 翻 GitHub `ekuaibao/open-platform-docs` 仓库找到合思官方文档
- 定位到 `GET /api/openapi/v2/approveStates/[ids]?accessToken=xxx` (path 必须含方括号)
- Response 返回 `stageName` + `operators[{id,name,code}]`, 拿到当前节点+审批人姓名+工号

**数据库**:
- ALTER hesi_flow 加 4 列: current_stage_name / current_approver_id / current_approver_name / current_approver_code (INSTANT 算法, 0 锁表)

**sync-hesi.exe 扩**:
- 加 fetchApproveStates() 函数, 批量 20/批调合思接口, 限流 200ms/批
- 多审批人节点拼接显示 (例: 张三+李四)
- 主流程末尾对 active=1 且 state NOT IN (paid/archived/rejected/draft) 的单据全量回填
- 每天 10:30 自动同步会刷新当前审批人

**后端 hesi.go GetHesiFlows**:
- SELECT 加 current_stage_name + current_approver_name + current_approver_code 3 列
- FlowItem struct 加 3 字段

**前端 ExpenseControl 表格**:
- "当前进度" 列重命名"当前审批", 优先显示真实姓名 (粗体) + 节点 (小字)
- 已结束 (paid/archived/rejected) 显示"已结束"
- 没拉到审批状态时 fallback 显示上一步节点名 + ✓ (兼容 v1.57.2 老逻辑)
- Tooltip 完整显示节点+审批人姓名+工号

---

## v1.57.2 (2026-05-12) — 合思费控页加"当前进度"列

- 表格在"状态"后多了**当前进度**列, 显示单据走到哪一步 (上一步已审批的节点名)
- 例: 显示"直属上级" = 直属上级已通过, 正在等下游审批
- 鼠标悬停看通过时间, 列下方小字附 MM-DD HH:mm 时间戳
- 数据来源: 合思 API 返回 `preApprovedNodeName` 字段, 从 hesi_flow.raw_json 解析, 零数据库变更
- **暂只显示岗位名(节点), 真实审批人姓名待跑哥拿到合思 OpenAPI 审批流文档后扩接口拉**

---

## v1.57.1 (2026-05-12) — 合思费控页加"立即同步"按钮

- **痛点**: 合思每天 10:30 自动同步, 但跑哥临时要看最新数据时只能等
- **新功能**: 费控管理页面顶部多了两个按钮
  - **立即同步合思** — 一键启动后台同步任务 (5-10 分钟), 弹确认框防误触
  - **看同步日志** — 黑底滚动 modal, 每 3 秒自动刷新, 实时看拉取进度
- **底层修复**: sync-hesi.exe 改用 io.MultiWriter 双写 (固定 sync-hesi.log + stdout), 配套改 sync-hesi-silent.vbs 去掉 cmd 重定向避免重复写
  - schtasks 触发(走 vbs) 跟 BI 看板按钮触发(走 bi-server) 都能正确写日志, 不再丢

---

## v1.57.0 (2026-05-12) — 恢复合思费控管理模块

- 跑哥反悔 2026-05-09 的"下架"决定, 恢复**财务部门 → 费控管理**页面
- 实施: `git revert 5bc2996` 反向应用当时的下架 commit, 恢复 3 个文件 (前端页面 + 菜单注册 + 路由)
- 启用 `BI-SyncHesi` 定时任务 (每天 10:30 增量同步合思流水)
- 后端 API / handler / 同步工具 / 4 张数据库表 / 权限定义 一直保留, 这次只是把前端入口加回来
- 历史数据 1.7w 流水 + 3.1w 明细 + 2.8w 发票 + 7.3w 附件 都还在, 直接可看

---

## v1.56.2 (2026-05-12) — 运维监控展示清理 + 销售单定时任务补缺

**严重补缺**:
- **加销售单每日同步定时任务** (BI-SyncDailyTrades, SYSTEM 账户, 每天 04:00 拉昨日)
  原本只有手动按钮触发, 跑哥忘了点就一天没销售单数据流入, 现在自动了

**展示清理**:
- 隐藏 API 服务 / 前端服务的 schtasks 状态条目 (schtasks Last Result 跟服务真实状态脱钩, 已有端口实测代替)
- 加前端服务（端口实测）TCP 探测 3000 端口
- 合思费控同步 (Disabled) 默认隐藏 (跑哥拍板"不做了"后)
- 系统维护类(MySQL备份/日志轮转/刷新上月汇总) + 模型训练类(Prophet/StatsForecast) 默认折叠
- 顶部加两个开关:"显示隐藏任务" + "显示系统维护/模型训练"

**元数据补全**:
- BI-TrainProphet → "Prophet 模型重训" (销量预测季节模型, 每周日 03:00)
- BI-TrainStatsForecast → "StatsForecast 模型重训" (Nixtla 多模型集成, 每周日 03:30)
  不再显示"（未配置中文描述）"

**默认视图收紧**:
- 从 22 项 (混乱) → 默认 18 项可见 (业务 11 + 库存 4 + 服务实测 2 + 模型训练默认折叠 + ops 默认折叠)

---

## v1.56.1 (2026-05-12) — 运维监控加"看实时日志"按钮

- 痛点: 销售单补拉 / 汇总帐补拉 / 库存快照 三个工具的日志一直进不了运维监控页面 (工具内部把 log 接管去固定文件了), 跑哥跑了不知道进度
- 修复: 工具内部改 io.MultiWriter, 既写固定文件又走 stdout, 兼容旧路径
- 新功能: 运维监控页面这 3 个任务卡片旁边多个"看日志"按钮, 点开黑底滚动 modal, 每 3 秒自动刷新末尾 300 行
- 实现细节: 后端加 `/api/admin/sync-tools/log?key=xxx&lines=N` 接口直接读固定 log, 不依赖运行时内存 — bi-server 重启了也照样看

---

## 后续规划（通往 v2.0）
- [ ] 天猫超市两店数据分开存储（一盘货 vs 寄售）
- [ ] 10个未映射渠道归属部门
- [ ] 省份地图可视化
- [ ] 采购计划模块
- [ ] 品牌中心模块
- [ ] 运营数据定时自动导入
- [ ] 抖音素材/主播/随心推数据展示
- [ ] 营销看板 UI 优化（Tab 分区 + 核心 KPI 放大）
- [ ] 综合客单价改 GMV/购买人数
- [ ] 数据安全加固（Cookie Secure 标志、Session 清理）
