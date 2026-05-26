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

## v1.75.3 (2026-05-26) — 主体校验扩展到所有报销类单据 (从 1 个模板 → 4 个)

**业务背景**: 跑哥反馈出差申请单看不到主体校验 Tag, 确认设计如此 (v1.75.0 只做日常报销单). 5/26 拍板 B 方案: 扩展到所有真实花钱的报销类单据.

**变更**: 后端校验条件 `specificationId 前缀 ID01Fk3qJYYFvp` → `formType=='expense'`

**新覆盖模板** (3 个, 之前不校验):
- 付款单/票到付款 (3762 单)
- 费用核销申请单 (3062 单)
- 银行支付申请单 (1188 单)

合计校验范围: **2744 → 10756 单 (×3.9 倍)**

实测 3 张付款单 mismatch 全识别:
- B26003170 张勇: 填"华鲜高新" 应"世用食品"
- B26003167 周翻翻: 填"浩鲜自然" 应"华鲜高新"
- B26003166 周翻翻: 填"苏州松鲜鲜" 应"华鲜高新"

申请类/借款类不校验 (没真金白银花钱, 错填主体业务影响小).

---

## v1.75.2 (2026-05-26) — 单据详情 Tab 自适应: 无数据的 Tab 自动隐藏

**业务背景**: 跑哥反馈"出差申请单这种没金额, 但还显示发票 Tab 和金额, 没必要".  
按 17 个模板真实数据分析:
- 报销类 (10756 单): 平均 30.9 个发票/单, 明细 31.5/单 — 该显
- 申请类 (11108 单): 平均 0.3 个明细, 0 发票 — 该藏
- 借款类 (1348 单): 平均 0 明细 0 发票 — 该藏
- 商城类 (315 单): 全 0 — 该藏

**自适应规则** (单文件改动, 不写死模板字典):
- 基本信息: 永远显
- 费用明细: count > 0 才显
- 附件: count > 0 才显
- 发票: count > 0 才显; **例外**: form_type=expense 即使 0 也显作业务异常警告 (报销单没发票=数据缺失, 不能藏)

**实测**:
- 出差申请单 S26002764: 4 Tab → **1 Tab** (只基本信息), 信息密度大幅提升
- 报销单 B26003170: 3 Tab (基本/明细 1/发票 0), 发票 0 作为业务警告保留
- 商城类: 1 Tab (只基本信息)
- 新模板自动适配, 不依赖代码维护模板字典

同步改 HesiBot.tsx 审批界面, 两个页面规则一致.

---

## v1.75.1 (2026-05-26) — hotfix: 钉钉 queryonjob 翻页 bug, 在职员工 147→394, 覆盖率 38.8%→93.5%

**故障复盘**: 跑哥实测 B26003133 朱素花的报销单显示"无法核对", 但跑哥确认她在钉钉组织里且合同公司已填. 排查发现 sync-dingtalk-contract CLI 的翻页终止条件错: `len(data_list) < size` 提前 break, 但钉钉接口每页可能返不满 size (如第三页 47<50) 同时 next_cursor 非 0 (538491) 还有更多人.

**修复**: 翻页只看 `next_cursor==0` (钉钉文档: 0=无更多), 不看 page size.

**数据变化**:
- 钉钉在职员工: 147 → **394** (多了 247 个)
- 合思 880 员工桥接: 148 → **397** (翻 1.7 倍)
- 近 3 月日常报销单 245 unique 发起人覆盖率: 38.8% → **93.5%** (229/245)

剩 16 个 (6.5%) 未匹配可能是: 钉钉已离职 (status=4 不拉) / 钉钉花名册"合同公司"字段空 / 外部账号.

---

## v1.75.0 (2026-05-26) — 费控"日常报销单"主体校验: 跟钉钉花名册合同公司对比

**业务背景**: 跑哥反馈"想识别申请人填的法人实体跟钉钉花名册里真实劳动主体是不是一致"  
原始用例: 申请人选错主体导致财务对账出错 / 法人实体跨签合同口径不清.  
仅对**日常报销单**模板做校验 (specificationId 前缀 ID01Fk3qJYYFvp), 其他模板不影响.

**实现链路**:
- 数据源: 钉钉智能人事 `POST /topapi/smartwork/hrm/employee/v2/list` 返回 `sys05-contractCompanyName` (合同信息组→合同公司)
- 桥接 key: 手机号 (合思 staffs.cellphone ↔ 钉钉 sys00-mobile, 钉钉返回带"+86-"前缀需剥离)
- 字典体量: 钉钉 41 个合同公司 vs 合思 48 个法人实体, 字面基本一致 (抽样 4 家全对), 无需别名表

**变更点**:
- 新增本地表 `hesi_employee_contract_company` (合思员工 ID → 钉钉合同公司映射, 5 字段含合同类型/到期日, UPSERT 模式)
- 新增 CLI `sync-dingtalk-contract` (合思 880 员工 → 钉钉 queryonjob 在职 147 → v2/list 100 一批拿 sys05 → 手机号桥接)
- 新增 schtasks `BI-SyncDingtalkContract` 每日 03:00 SYSTEM /RL HIGHEST (跟 BI-SyncHesi 错峰)
- 新增 config.dingtalk.notify_agent_id (智能人事接口需要 AgentId, 4555938649)
- 后端 `GetHesiFlowDetail` 在日常报销单 specificationId 命中时, 查 hesi_employee_contract_company.contract_company_name 跟 flow.LegalEntityName 字面比对, 返 `entityCheck`/`entityCheckExpected`/`entityCheckReason` 三字段
- 前端 ExpenseControl.tsx 详情弹窗 "公司（法人实体）" 行后加 Tag:
  - ✅ 已核对 (绿色, 一致)
  - ⚠️ 主体可能选错 · 应为 XX (红色, 不一致显示应为的公司)
  - 无法核对 (灰色, 钉钉花名册无数据或未匹配)
  - 鼠标悬停 Tooltip 显示具体 reason 文案

**初版数据**:
- 钉钉在职 147 人, 全部桥接成功 (mobile=147, name 兜底=1)
- 近 3 个月日常报销单 245 个 unique 发起人, 95 个能查到钉钉合同公司 (覆盖 38.8%)
- 抽样 4 单 mismatch 全识别: 高诚(应南京分公司)/罗加芸(应华鲜高新)/徐园(应华鲜高新)/聂煜轩(应华鲜高新)
- 剩 150 个发起人钉钉花名册"合同信息"字段为空, 需 HR 在钉钉智能人事补全后 T+1 自动生效

**踩坑**:
- 钉钉 v2/list 必须传 agentid (跟 staffs 列表 / queryonjob 不一样)
- 钉钉 field_code 是 sys00-name/sys00-mobile, 不是 meta/get 返回的中文 "姓名"/"手机号"
- 钉钉手机号格式 "+86-13357134296", 桥接前必须剥离 "+86-" 前缀

---

## v1.74.9 (2026-05-26) — 费控管理单据详情显示发起人/公司/部门名字

**业务背景**: 跑哥反馈费控管理点开单据详情, 弹窗"基本信息"只看到金额/状态/时间, 看不出来"是谁发的、哪家公司、什么部门" — 之前数据库只存合思的 ID, 名字得调字典. 5/26 拍板 A 方案 (法人实体作为"公司"全显).

**变更点**:
- 后端 `hesi_specifications.go` 新增 3 个字典缓存 (5 分钟 TTL, 仿原模板字典模式):
  - `LookupStaffName` — 合思员工 880 条 (`/openapi/v2/staffs`)
  - `LookupDeptName` — 合思部门 511 条 (`/openapi/v2/departments`)
  - `LookupLegalEntityName` — 合思自定义维度"法人实体" 48 条 (`/openapi/v2/dimensions/items`)
- 后端 `hesi.go` `GetHesiFlowDetail` 在返回单据时同步查 5 个名字:
  - `ownerName` / `submitterName` (员工字典)
  - `departmentName` (报销/借款部门) / `ownerDepartmentName` (发起人部门, 同时补 SELECT `owner_department` 字段, 之前没读)
  - `legalEntityName` (从 `raw_json["法人实体"]` 解析后查公司字典)
- 前端 `ExpenseControl.tsx` 详情弹窗"基本信息" tab 5 行新字段, 放在"单据类型"之后、"状态"之前 (用户最先关注 = 谁/什么公司). 公司用整行 (span=2).

**性能**: 首次拉 3 个字典共 ~1s (3 次合思 API 调用), 之后 5 min 内点详情 0 额外延迟. 已实测点单据 B26003150 弹窗:
- 公司: 杭州华鲜高新技术有限公司
- 发起人/提交人: 虞希
- 发起人部门: 拼多多运营事业部
- 报销/借款部门: 京东粮油调味组

**踩坑**: 合思 `count` 参数上限 1000, 不是无限. 第一版 count=10000 全部 HTTP 400 静默 fallback 空字符串 ("姓名空"问题). 通过 stderr log 定位修. 当前 3 个字典最大 880 (员工), 离 1000 上限有缓冲. 将来超 1000 需加分页.

---

## v1.74.8 (2026-05-26) — SQL 30s 超时全局覆盖 (Phase 1: 103 处 hot path 自动防锁)

**业务背景**: PUA 字节 4 路 audit 性能稳定 Critical #1. memory `feedback_no_long_sql_no_timeout` 翻车原型: 一条慢 SQL 撑满 `MaxOpenConns=100` 后, 所有新请求阻塞 + 定时任务排队. 当前 175 处 `db.Query/Exec/QueryRow` 全部走原始 (非 Context), 没任何 SQL 设过 timeout. 单点最毒.

### Phase 1 策略 (高 leverage, 0 业务公式改动)
不手撸 175 处 caller, 改 `queryRowsOrWriteError` helper 1 处, 自动覆盖**103 处 hot path** (handler 层 SQL).

### 改动
- `server/internal/handler/db_helpers.go`:
  - 加 `const defaultQueryTimeout = 30 * time.Second`
  - 加 `type rowsWithCancel struct { *sql.Rows; cancel context.CancelFunc }` wrapper
  - 覆盖 `Close()` 方法: 先 `cancel()` 再 `Rows.Close()`, 确定性 cleanup ctx tree
  - `queryRowsOrWriteError` signature 加 `r *http.Request` 参数 + `WithTimeout(r.Context(), 30s)` + `db.QueryContext`
- **17 个 handler file** sed 替换 caller `queryRowsOrWriteError(w, h.DB,` → `queryRowsOrWriteError(w, r, h.DB,`:
  - admin.go / admin_meta.go / admin_users.go
  - dashboard_department.go / dashboard_overview.go / dashboard_sproducts.go
  - douyin.go / finance_report_query.go / marketing_cost.go
  - offline_sales_forecast.go / offline_target.go
  - ops_customer.go / ops_feigua.go / ops_jd.go / ops_pdd.go / ops_tmall.go / ops_vip.go
  - stock.go (含 writeStockResponse 内部 helper 一并加 r 参数 + 2 个 caller 同步)
- caller 函数体 **0 行改动** (Go 字段提升: `rows.Next/Scan/Err/Close` 仍走 embedded `*sql.Rows`)

### codex 二审 2 轮闭环
- **Round 1 P1**: `runtime.SetFinalizer` 不可靠, GC 时机不定累积 timer/资源, 高负载场景会爆
  - Fix: 改用 `rowsWithCancel` wrapper, `Close()` 时确定性 cancel ctx
- **Round 2**: GATE PASS ✅ "did not find any concrete regressions"

### ctx 生命周期 (Go idiomatic)
```
HTTP Request 进 → handler 拿到 r → queryRowsOrWriteError(w, r, ...) 走 helper
  → WithTimeout(r.Context(), 30s) → QueryContext → 包 rowsWithCancel 返
caller defer rows.Close()                ↓
  → rowsWithCancel.Close() → cancel() + Rows.Close() → ctx 释放
```

3 种保护:
- **HTTP client 主动断开** → r.Context() cancel → SQL 立即停 (浪费 connection 短)
- **30s 超时强制中断** → 慢 SQL 不会无限拖, 释放 connection 回 pool
- **正常完成** → defer Close 链触发 deterministic cleanup

### 不动 (Phase 2 留)
- **72 处直接 `h.DB.Query/QueryRow/Exec`** 没走 helper, 不在 Phase 1 范围:
  - hesi worker (7 处, 后台异步)
  - supply_chain_dashboard.go (24 goroutine 并发)
  - distribution_customer.go (4 跨月 N+1)
  - 其他若干
- Phase 2 设计: 加 `dbQueryCtx/dbExecCtx` 类 helper + caller 改, 或者把直接调用迁到 helper
- 留下波集中改 (跟 175 SQL 全清扫一起)

### 影响
- 后端: db_helpers.go 重写 (15→57 行) + 17 file 102 caller sed + stock.go 加 r (含 internal helper) - bi-server.exe 重 build 重启 (PID 5224 → 31268)
- 前端: 0 行
- 用户: **0 视觉变化**, 但慢 SQL 不再锁库 (理论 30s 上限). 极端情况某用户复杂报表查询 > 30s, 会失败提示 "database query failed" 而不是拖死全站

### Pre-existing 直接 db.Query 漏 (列下波修)
跟 [[project_dingtalk_bind]] 二期前置依赖 + 财务板块大调整一起做.

---

## v1.74.7 (2026-05-26) — 综合看板 TOP15 商品补回 2 调拨渠道 SKU + codex 3 轮二审

**业务背景**: PUA 字节多 agent 4 路 audit Critical #2. 综合看板 TOP15 商品销售排行直接 `FROM sales_goods_summary`, 但 2 调拨渠道 (清心湖自营 / 猫超寄售) 的 SKU 数据**不在 sales_goods_summary** (在 allocate_orders/details), 综合看板 TOP15 完全缺少这些 SKU. 跟 v1.74.3 KPI/趋势/店铺榜对齐口径.

### 改动
- `server/internal/handler/dashboard_overview.go` GetOverview() topGoods 段:
  - SQL `ORDER BY sales DESC LIMIT 15` → `LIMIT 50` (给 merge 留 buffer)
  - 调 `loadEcommerceGoodsAllotDetail` 拿 2 渠道调拨 SKU
  - 已在 topGoods 的 SKU: 加和 sales/qty (profit 不加, 调拨无毛利)
  - 不在 topGoods 的 SKU: append 新 entry
  - 重排 + unconditional final trim 回 LIMIT 15

### codex 二审 3 轮闭环 (业务红线 KPI ranking)
- **Round 1 P1**: `loadEcommerceGoodsAllotDetail` 没传 scopeCond → 受限用户跨权限注入 SKU
  - Fix: 加 scope guard, 受限用户 Depts 不含 ecommerce → 跳过 merge
- **Round 2 P1 #1**: scope guard 只看 Depts, 漏 Platforms/Shops
  - Fix: 保守策略, 任何 scope 限制都跳过 (调拨数据本身无 platform/shop 维度精准过滤)
- **Round 2 P1 #2**: SQL `LIMIT 15` 之外的 SKU 加调拨 sales 后应进 TOP15 但被截
  - Fix: SQL 改 `LIMIT 50` buffer
- **Round 3 P1**: SQL `LIMIT 50` fallback 路径 (scope skip / helper err / 空 allot) 未 trim → 返 50 行 regression
  - Fix: unconditional final trim 移到 if-else 外, 任何路径都 cap 回 15
- **Round 4**: GATE PASS ✅ "logic appears internally consistent... I did not find a clear, actionable regression"

### 不动
- helper `loadEcommerceGoodsAllotDetail` signature 0 改动 (沿用 v1.74.3 拓范)
- 前端 `OverviewDashboard.tsx` 0 改动 (后端 topGoods 字段值变了, 前端展示自动跟上)
- DB schema 不变

### ⚠️ Pre-existing 同款 scope 漏 (列下波修)
- `loadEcommerceAllotAdjustment` (KPI 卡)
- `loadEcommerceDailyAllot` (趋势, v1.74.6 也复用)
- `loadEcommerceShopAllot` (店铺榜)
- `loadInstantRetailDailyAllot` (即时零售趋势)

这 4 个 helper 的 allocate_orders SQL 都没传 scopeCond. 跟 v1.74.7 同款问题, 但属 v1.74.3 已 ship 功能, 改它影响范围大 (4 部门页 KPI + 趋势), 留下波集中修.

### 影响
- 后端: `dashboard_overview.go` +44 行 (helper 调用 + merge + scope guard + final trim) - 2 改 (SQL LIMIT 15→50, 内部 trim 移外), bi-server.exe 重 build 重启 (PID 30368 → 5224)
- 前端: 0 行
- 用户: 浏览器硬刷 (Ctrl+F5) 看到综合看板 TOP15 商品排行可能新增 2 调拨渠道独占 SKU (尤其有大笔调拨日 4/7 / 4/22 等)

---

## v1.74.6 (2026-05-26) — 电商部门页"每日趋势"图补回 2 调拨渠道 (月级 ¥667 万缺口)

**业务背景**: PUA 字节多 agent 安全/业务/性能 4 路 audit 暴露的业务 Critical 之一. v1.74.3 拓范时电商部页所有 KPI 卡/店铺榜/TOP15 商品/品牌/产品定位/平台销售全部加了 helper 合并 2 调拨渠道 (清心湖自营 + 猫超寄售), **唯独"每日销售趋势"图漏加**, 月级缺口 ~¥667 万 (2026-04 实测: 4/7 ¥460 万 + 4/22 ¥207 万), 跨同一页面的 KPI 卡数对不上.

### 改动
- `server/internal/handler/dashboard_department.go`: GetDepartmentDetail trend loop 之后, `dept=ecommerce` 时调用 `loadEcommerceDailyAllot` (helper 已就绪, 综合看板同款), 把日级 `allotAmt/allotQty` 加回 daily slice
- 兜底: helper 失败 → log + 不阻塞, 趋势图用原口径 (跟 KPI 对不上但页面不挂)

### 不动
- 业务算法/口径无新发明, 沿用 v1.74.3 helper (`allocate_orders.channel_key IN ('京东', '猫超')` 锁定 2 渠道)
- 前端 `DepartmentPage.tsx` 0 行改动 (后端 daily 字段值变了, 前端 reduce 自动跟上)
- KPI 卡 / 店铺榜 / TOP15 已在 v1.74.3 修过, 这次只补漏趋势

### 影响
- 后端: `dashboard_department.go` +16 行 (1 个 `if dept=ecommerce` 块 + 调 helper + 累加 loop), bi-server.exe 重 build 重启 (PID 29308 → 30368)
- 前端: 0 行改动, 不需要 npm build
- 用户: 浏览器硬刷 (Ctrl+F5 防 cache24h) 即可看到电商部门趋势图 4/7 和 4/22 调拨金额回归

---

## v1.74.5 (2026-05-26) — 费控管理"费用明细"展示扩容: 看合思 API 完整原始字段

**业务背景**: 跑哥反馈费控管理 → 单据详情 → 费用明细 Tab 内容太少 (原 5 列: 序号/金额/消费时间/发票/消费原因). 合思 API 实际返回字段远多于此 (币种/单位/差旅出发地目的地/付款截图/出差补贴金额分类等), `raw_json` 字段已全存在 `hesi_flow_detail` 表 (5/9 起一直存), 但前端没用.

### 改动
- 后端 `hesi.go` GetHesiFlowDetail: detail 响应加 `rawJson` (json.RawMessage passthrough, IFNULL 兜底) + `specificationId` 字段
- 前端 `ExpenseControl.tsx` 费用明细 Tab:
  - 主表加 2 列: **费用类型** (合思 ID Tag + Tooltip) + **本明细附件数**
  - 发票列升级: 显示张数 (例 "3 张" 不再只 "有/无")
  - 消费时间列增强: 差旅类支持 `feeDatePeriod` (起 ~ 止) fallback
  - **加展开行** (左侧 ▶): Descriptions 平铺 `raw_json.feeTypeForm` 所有字段
    - 自动 transform: 金额对象 → "¥X 元", 城市 JSON → "省/市/区" 解析 label, 时间戳 → YYYY-MM-DD, 附件数组 → 文件名列表, 自定义 `u_xxx` 字段 → 提取中文 label, 嵌套对象 → 短 JSON 可复制, 纯合思 ID → Tag + Tooltip "未匹配字典"
  - 跳过重复字段 (主表已有 amount/feeDate/invoice/consumptionReasons + 发票 Tab 已专题 invoiceForm)

### 字段全集 (5 个 fee_type sample)
| 费用类型 | 新展示字段 (展开行) |
|---|---|
| 差旅交通 | 出发地 / 目的地 / 出发车站 / 到达车站 / 行车记录附件 |
| 出差补贴 | 天数 / 出差补贴金额 / 市内补贴金额 / 餐费补贴金额 |
| 私车公用 | 申请报销金额 / 私车公用金额 |
| 通用报销 | 付款截图 (附件) / 平台类型 / 可抵扣发票张数 / 预算费用 |

### 不动
- 业务算法/口径无改动 (纯展示扩容, 不算 CLAUDE.md 业务红线, 跳过 /codex 二审)
- DB schema 不变
- 老数据 (5/9 之前 sync 入库) `raw_json` 可能为空, 展开行提示"老数据未存原始字段"

### 影响
- 后端: `hesi.go` +6 行 (SQL + Scan + struct), 0 业务逻辑改动, bi-server.exe 重 build 重启 (PID 22796 → 29308)
- 前端: `ExpenseControl.tsx` +100 行 (helpers + Table expandable + 加列)
- 用户: 硬刷 (Ctrl+F5) 即可看到新展开行

## v1.74.4 (2026-05-26) — 财务/部门"产品利润"页 KPI 总数修复 + 电商调拨口径对齐

**业务背景**: 跑哥 5/25 晚发现财务·产品利润页选电商部门显示 ¥513 万, 跟综合看板 mini 卡对不上 (差 ¥670+ 万). RCA 双根因:
1. **5/20 ProductDashboard 同款 reduce TOP15 bug 在产品利润页一直没扫到** (memory `feedback_frontend_reduce_with_limit`): 前端 `goods.reduce` 把 backend `LIMIT 15` 的 TOP15 SKU 合计当成"全部 SKU 总销售". 各部门数都偏小 (电商 39% / 线下 19% / 社媒 12% / 分销 43%)
2. **电商部门没合 v1.74.3 调拨口径**: 财务页是 v1.74.3 "未在本轮"列表第 3 项, 销售单口径剔除 2 调拨店, 没把调拨业务加回

**修复方案 (极简 - backend 0 改动)**:
- 前端 `FinanceProductProfit.tsx`: KPI 改用 `data.totalSales ?? goods.reduce(...)` (复用 backend v1.74.3 已合并好的字段), SKU 种类数同理. 毛利保留 reduce (调拨业务无毛利字段, 维持销售单口径). 顶部加 antd Alert 告知用户口径.
- 前端 `ProductProfit.tsx`: 4 个部门页 (电商/社媒/线下/分销) 的 `/dept/product-profit` 路由同款修复 (使用同一组件)
- Owner mindset scan: `goods.reduce` 全 grep, 确认 `StoreSProductsSection.tsx` (主推 S 产品段) 因 backend SQL 没 LIMIT 是合理 reduce 不修, 其他 `daily/depts/monthly/pageData/channels` reduce 都是合理范围内不动

### 业务感知 4 月样本数 (修复前 → 修复后, 单位万)
| 部门 | 修前 TOP15 reduce | 修后 全部 SKU 含调拨 | 偏差 | 原因 |
|---|---|---|---|---|
| 电商 | ¥316 万 | **¥1184 万** | **+275%** | TOP15 漏 (¥316→¥518) + v1.74.3 调拨合并 (+¥667) |
| 线下 | ¥1753 万 | ¥2156 万 | +23% | TOP15 漏 (印证 5/20 跑哥发现的 18.7% 同款) |
| 社媒 | ¥754 万 | ¥860 万 | +14% | TOP15 漏 |
| 分销 | ¥274 万 | ¥479 万 | +74% | TOP15 漏 (TOP15 集中度低 → 漏 43%) |

### 影响
- 前端: `FinanceProductProfit.tsx` +12 行 (KPI 字段 + Alert), `ProductProfit.tsx` +6 行 (KPI 字段)
- 后端: 0 改动 (v1.74.3 已 ready totalSales/totalQty/totalSku 含调拨合并)
- 毛利暂未含调拨业务: allocate_details 缺毛利字段 (`excel_amount` 售/进价未定), 跑哥决策"财务板块后续大调整一起重算"
- 综合毛利率会因分母变大相应降低, 属正常 (Alert 已告知)

## v1.74.3 (2026-05-25) — 综合看板/电商部页/货品看板全模块合并 2 调拨渠道 (业务口径修正)

**业务背景**: ds-京东-清心湖自营 / ds-天猫超市-寄售 业务上不算销售单, 按调拨入库统计 (v0.62 已上线对账页). 但综合看板/部门页/货品看板长期用销售单口径, 跟业务对账不一致. 跑哥 5/25 拓范, 一日打完全模块合并 (18 commit).

**业务感知**: 4-5 月样本数据从 ¥513 万 (销售单口径) 涨到 ¥730+ 万 (调拨口径), +42%

### 综合看板 (/overview)
- KPI 头部主卡 (总销售/总货品数/客单价): 自动跟 dept.Sales/Qty 新口径
- 电商部 mini 卡: 主字 = 销售+调拨总和, 1 行小字"销售 ¥X · 调拨 ¥Y", 跟其它部门等高布局
- 每日趋势图: ecommerce 折线按日合并 2 渠道调拨
- 店铺排行 TOP15: 调拨 shop 替换销售口径 + 重新排序
- 删 "不含调拨" Tooltip (业务对账已对齐)

### 电商部页 (/ecommerce)
- 店铺数据预览 (StorePreview): shopList + KPI 总销售 + 总店铺数 含调拨
- 店铺看板 (StoreDashboard) 加 "调拨专区" 独立 tab:
  - 京东 / 天猫超市 tab: 干净不含调拨店
  - 调拨专区 tab: 始终显示 2 调拨店 (即使 ¥0, 告知用户调拨业务状态)
  - 全部 tab: 含 2 调拨店 (跟综合看板一致)
- 货品看板 (ProductDashboard) 5 section 合并:
  - 商品 TOP15: by goods_no merge + 加和
  - KPI 总销售/总货品/总 SKU
  - 品牌分布: by brand_name 合并 + LIMIT 10
  - Grade 分布: by goods_field7 (S/A/B/C/D 顺序)
  - 平台销售: 京东 + 天猫超市 各加调拨金额

### sync-allocate ETL 治本 (跑哥 DB202604290001 发现)
- 长期 bug: 默认 7 天滚动 + stat_date 仅 in_status=3 时设 → 老单状态老化 + NULL stat_date 漏算
- 一次性手动 -start=2026-04-01 -end=2026-05-25 拉新, 2145 单 / 18095 行明细更新
- NULL stat_date 101→71 (剩下是真未入库)
- 新增 sync-allocate `-refresh-pending` flag: 从 DB 最早未完成单 audit_date 起扩范围拉新
- 新 schtask **BI-SyncAllocateRefresh** 每天 03:00 自动跑

### 技术 helper (复用)
- loadEcommerceAllotAdjustment: 2 渠道总 sales/qty 双口径
- loadEcommerceDailyAllot: 按日 day-level 双口径
- loadEcommerceShopAllot: 按 shop_name 双口径
- loadEcommerceGoodsAllotDetail: 按 goods_no SKU 详情 (LEFT JOIN goods 拿 brand/cate/grade)
- applyAllot* 子函数: 内存合并逻辑, 便于单测
- 14 个 sqlmock + 子函数单测全过

### 顺手
- DeptSummary / TrendPoint / ShopRank 提到包级别 (helper 引用)
- dashboard_cache_test.go itoa 跟 hesi_audit_rules.go itoa 重复 (pre-existing fix)
- bi-server 启动加 stdout/stderr 重定向到 logs/bi-server.log
- v1.74.2 AI 助手 3 bug 修复合并进本 tag (本周时间口径 / persist ctx / 兜底告警)

### 不在本轮 (下轮 v1.74.4)
- 商品×渠道分布 goodsChannels (hover tooltip, 业务用得少, 已有 loadEcommerceShopAllot helper 可复用)
- 财务/产品利润页深度合并 — **业务红线**: 调拨端 `allocate_details` 缺毛利字段 (`excel_amount` 是售价还是进价未知), 需先对齐口径 + /codex 二审

### 影响
- 后端: dashboard_overview.go + dashboard_department.go + dashboard_helpers.go + sync-allocate/main.go +600 行
- 前端: src/pages/overview/index.tsx +30 行
- 单测: cache_test.go + dashboard_overview_test.go +250 行
- 数据库: 无 schema 变更, 数据 NULL stat_date 修复
- 性能: helper 加 5-10ms (cache 命中后忽略)

### 5/25 收尾 hotfix (3 波: v1.74.3-1 / v1.74.3-2 / v1.74.3-3)

**起因**: v1.74.3 发完后, 跑哥实测发现趋势图 + 即时零售部 tab 还有遗留 bug, 同时业务上即时零售部 (朴朴) 也应当合并调拨. 一气追加 3 波收尾.

#### v1.74.3-1 (b745028) — 趋势图调拨黄柱 UX 收尾 (4 commit)
**起因**: 趋势图按销售额 + 调拨额堆叠柱后, 5/14/18 号黄柱特别长 (¥132 万), 跑哥要求拆开 + 防御性 guard. 实测换浏览器+清缓存切到非 ecommerce dept tab 仍见黄柱 ¥132.79 万 (= 5/14 ecommerce 调拨额完全吻合).

- ECharts 切 dept tab 残留 series **真凶**: `Chart.tsx` 加默认 `notMerge=true`, 切 tab 强制全替换 series (d0a68b1)
- `trendOption` useMemo deps 漏 `trendAllot/hasAllot` → 切 tab activeDept 变但 chart option 不重算 (3121154)
- `trendAllot` 前端防御 guard: 强制只在 `activeDept === 'ecommerce'` 才算调拨, 非 ecommerce 强制全 0 数组 (c61f88a)
- main bundle hash 变 (833828ba → b94c540c), 跑哥重开浏览器 tab 拿新 JS

#### v1.74.3-2 (906ff97) — 即时零售部合并朴朴调拨 (KPI 层)
**起因**: 跑哥 5/25 早上 brainstorm 时定"本轮只改电商部, 即时零售不动", 收工前确认需求改成"全部一致".

- `GetOverview` dept=instant_retail mini 卡 Sales/Qty 加朴朴调拨 (SalesAmt=原销售单, AllotAmt=朴朴调拨)
- `GetDepartmentDetail` dept=instant_retail:
  - shopList 加 `js-即时零售事业一部（世创）-朴朴` entry
  - KPI totalSales/totalQty/totalSku 加调拨
- 朴朴无销售单 (shop_name 不在 sales_goods_summary), 简化处理: 没抽 helper, inline SQL (只 1 个 channel_key)
- **业务感知 5/1-5/24**: 即时零售部 ¥127 万 → ¥167 万 (+31%, 调拨口径修正)

#### v1.74.3-3 (4a40c73) — 即时零售部趋势加日级朴朴调拨黄柱 (趋势层)
**起因**: v1.74.3-2 只补了 KPI 卡 + 总数, 趋势图日级朴朴黄柱还缺. 完成"即时零售部"全模块合并闭环.

- 后端 `dashboard_overview.go` +59 行:
  - 加 `loadInstantRetailDailyAllot` 查朴朴日级调拨 (`allocate_orders.channel_key='朴朴'`)
  - 加 `applyInstantRetailDailyAllot` 设置 `trend.AllotSales/AllotQty` (朴朴无销售单, 不排除)
  - `GetOverview` 调用 + 错误兜底 (失败 → log, 趋势缺朴朴柱不阻塞)
- 前端 `overview/index.tsx`: `trendAllot` guard 加 `instant_retail` (跟 ecommerce 同模式)

#### App 模式反例自省
**这 3 波 hotfix 严格按 CHANGELOG 反例规矩 (一功能切 N 刀涨 N 版本 = 错), 不该单独打 tag, 应该 1 个 v1.74.3 一次打透**. 之所以拆 3 个 tag 是跑哥 5/25 一日打完 25+ commit 节奏紧, 收尾后这里追写补全, 后续避免.

## v1.74.1 (2026-05-25) — hotfix: 财务报表导入预览端点炸了 (MySQL 9.x 兼容性)

**业务影响**: 跑哥点 "下一步：预览变更" 报 "计算变更预览失败", 财务无法导入. 紧急修复.

- **根因**: `parser.go:806` SQL `COUNT(*) AS rows` 用了保留字. MySQL 5.x/8.0 早期能跑, 升 MySQL 9.6 后 parser 直接拒.
- **修复**: `AS rows` → `AS rec_count` (摆脱保留字, 未来升级不怕)
- **顺手**: bi-server 启动加 stdout/stderr 重定向到 `logs/bi-server.log` (我之前手动 Start-Process 没做的, 排查 bug 时 log 全丢)
- 实测: mysql_bi 跑新 SQL OK + bi-server PID=33468 v1.74.1 健康 + 跑哥前端 Widget 仍在轮询
- 同类扫: `AS rows` / 其它 MySQL 8.0+ 保留字 (groups/rank/over 等) 全 0 — 没别的炸点
- **顺手 audit**: 跑 probe 实测 4 月 Excel 的 1-3 月单元格行为, 确认 incremental 模式不会丢业务数据 (8 部门真空 / 国际零售 2/3 月仅 1 行占位 0)

## v1.74.0 (2026-05-25) — AI 智能助手提速: 答案 cache 让重复问题 < 50ms

**背景**: 5/22 GA 后 3 天 0 个真实用户使用. 怀疑 40 秒等待是劝退头号嫌疑.
**核心**: 把重复问题压到 < 50ms (实测 2500-4700 倍加速), 半夜预计算 16 道标准题让早起用户首次问也秒回.

- **P0 答案 cache** — 完整 AskResult 缓存, 跳过 2 次 LLM 调用
  - Key: `SHA256(question normalize + today)` (跨日自动失效, 不命中昨天答案)
  - TTL: 1 小时 (远小于按天 sync 频率, 符合 cache-sync 同频规矩)
  - 反污染: unknown / warning 兜底答案不进 cache
  - 浅拷贝返回, 每次请求 DurationMs / FromCache / SessionID 都正确
  - 实测重复问题: 第 1 次 42 秒 (LLM full) → 第 2 次 17ms ⚡ → 第 3 次 9ms ⚡
- **P2 半夜预计算 warm cache** — bi-server 内部 goroutine, 不走 schtasks 独立进程
  - 16 道标准题 (8 模块 × 2 题, 涵盖管理层高频问法)
  - 每天 00:30 自动触发, 题间 5s 错峰避 LLM 限流, 约 12 分钟灌满
  - 用户 08:00 第一次问 "公司本月销售多少" 直接命中 → 50ms 内
- 新增配置 (`config.json` `ai_assistant` 段): `cache_enabled` / `cache_ttl_seconds` / `warm_cache_enabled` / `warm_cache_hour` / `warm_cache_minute`
- 新增方法: `Service.WarmAsk` / `Service.WarmCache` / `Service.RunWarmCacheLoop` / `Service.CacheStats` / `Service.CacheClear`
- 测试: `cache_test.go` 14 个单元测试 (Normalize / Key / Hit / Expire / Stats / Clear / Clone / WarmTime) 全过
- **顺手治本**: `start-silent.vbs` 检测 Z 盘已挂则跳过 `net use` — 修复 5/25 开机自启卡死 24 分钟的根因

## v1.73.2 (2026-05-22) — hotfix: hesi cache TTL 跟 sync 同频

### 🐛 跑哥发现真问题
v1.73.1 给 hesi/stats 加了 cache24h, 但实测 **BI-SyncHesi schtasks 每 15 分钟跑一次**.
结果用户 95% 时间看到 24h 旧数据, 严重影响财务实时性. 跑哥眼尖问"15分钟更新一次, 24h cache 还更新吗?" 救场.

### 🔧 双管齐下
1. **TTL 调整**: hesi/stats `cache24h → cache15m` (跟 sync-hesi schtasks 同频, 最多延迟 15min)
2. **主动失效**: sync-hesi 完成后调 `POST /api/webhook/clear-cache?prefix=api|/api/hesi/`, 数据立刻新鲜 (不用等 15min cache 过期)

### 🛠️ 改造
- `internal/handler/sync.go` `ClearCache` endpoint 加 `?prefix=` 参数支持, 不传默认清全部 (向后兼容)
- `cmd/sync-hesi/main.go` 同步完成调 webhook, 失败只 log 不阻塞 sync
- `cmd/server/main.go` 加 `cache15m` 辅助函数, hesi/stats 改用

### 📋 数据新鲜度对照表
| 接口 | sync 频率 | cache TTL | 主动失效 | 用户感知延迟 |
|------|---------|----------|---------|-------------|
| hesi/stats | 15min | 15min | ✅ sync 完即清 | < 1s |
| hesi/specifications | 不变 | 24h | - | 0 (规格定义几乎不变) |
| hesi/flows | 15min | 无 cache | - | 0 (实时查 DB, 600ms) |

### 🚀 部署
- bi-server PID 44788 (21:30 启动 v1.73.2)
- sync-hesi.exe 已重 build 部署到 server/ 根, 下次 schtasks 21:45 自动用新版

---

## v1.73.1 (2026-05-22) — 性能体检 + hesi cache 修复

### 🎯 起因
跑哥要求 playwright 实测全 56 页打开速度. 测出 5 个慢页 (>3s), 其中 4 个已 cache24h 只是 cache miss 首次慢, 1 个 (合思费控) **完全没缓存** 是真 bug.

### 🐛 P0-3 修复: 合思费控 stats 接口加 cache24h
- `/api/hesi/stats` 修前: 每次 2.5s
- `/api/hesi/stats` 修后: 首次 2.1s (填 cache), 后续 **12ms** (175x 提速)
- `/api/hesi/specifications` 同步加 cache24h (无参数, 全员共享)
- `/api/hesi/flows` 不加 (带分页/过滤参数, cache 命中低, 浪费内存)

### 📊 全 56 页性能体检结果
- ✅ 33 页秒开 (< 200ms, 含 cache 命中)
- ✅ 12 页正常 (200ms-1s)
- ⚠️ 3 页偏慢 (1-2s): customer-analysis / system/ops / supply-chain/purchase-plan
- 🚨 5 页慢 (>3s, 但都已 cache24h, 仅首次访问慢):
  - trade-audit 3.9s / sales-forecast 3.8s / plan-dashboard 3.7s / product-profit 3.4s
- ⚪ 6 页空数据 (待跑哥确认设计如此还是真缺)

### 🔬 整体健康度: B+ (75/100)
80% 页秒开, 但 5 个核心页 >3s 首次. 跑哥决策不做 warmup (5-10 人/天 × 1 次/3s = 微痛), 后续物化表方案待 P1.

### 🚀 部署
- bi-server PID 31728 (13:21 启动 v1.73.1)

---

## v1.73.0 (2026-05-22) — AI 智能助手 GA: 8 模块路由 + 端到端跑通 + 三级 fallback

### 🎯 起因
v1.72.1 把 W1+W2 收尾, Z.AI 充值后第一时间跑 W3a (路由扩展 + 业务术语字典) + 端到端实测 + 修暴露的 3 个真实 bug, 8/8 路由准 + 7/8 端到端成功, AI 智能助手正式 GA 给管理层试用.

### 🏗️ W3a 路由扩展 (commit `63f0076`)

**8 个 module 全覆盖管理层高频问题:**

| Module | 答能 | 示例问题 |
|--------|------|---------|
| `department` | 单部门销售 | "上月电商部销售多少" |
| `overview` | 全公司总览 + 部门拆分 | "公司本月销售多少" |
| `shop_rank` | 店铺排行 (dept 过滤 / 升降序 / TOP N) | "本月哪个店卖最差" |
| `product_rank` | 商品/品类/SKU/品牌 4 维排行 | "本月卖最好的商品 TOP 5" |
| `trend` | 两段日期对比 (含 deltaPct) | "本周对比上周" |
| `stock_warning` | 缺货 SKU (复用 GetStockWarning 公式) | "哪些 SKU 缺货" |
| `warehouse_flow` | 仓储发货量 (orders/packages) | "本月发了多少单" |
| `rpa_status` | 各 RPA 平台数据最新日期 | "天猫数据到 5/22 了吗" |

**业务术语字典 (写在 system prompt):**
- 部门映射: 电商部/社媒部/线下/分销/即时零售 → 5 个严格 dept 枚举
- 店铺命名规律: ds-/社媒-/sy-/分销X组/线下渠道-
- 平台关键字: tmall/jd/pdd/douyin/xhs/vip/kuaishou
- 业务术语: 销售=goods_amt / 销量=goods_qty / 毛利=gross_profit / 毛利率按字面值
- 时间口径: 今天/昨天/本周/本月/上月/近7天 (动态注入今天日期)

**修正 W1 错误**: 之前 system prompt 写 zhongtai 部门, 实际 DB 只有 ecommerce/social/offline/distribution/instant_retail 5 个, 已删 zhongtai 加 instant_retail.

**安全**: dept/dim/platform 全部白名单 + ym 格式校验 + limit clamp [1,50] + 表名硬编码

### 🔧 W3a fix (commit `d75d60a`)

端到端实测后修 3 个真实 bug:

1. **Intent.Params 自定义 UnmarshalJSON** — LLM 偶尔返 `"limit":10` (number), 自动强转 string
2. **三级 fallback 策略** — fast (glm-4.7-flash) → primary (glm-4.7) → 代码模板, 用户绝不见 "AI 失败"
3. **truncateForLLM 反射版** — 兼容任意 slice 类型, 大列表自动截 TOP 5

### 📊 端到端实测 (8 个典型问题)
- 路由准确率: **8/8 = 100%** (intent → module 全对)
- 端到端成功: 7/8 = 88% (Z.AI 限流偶发, 实际生产 1 人/分钟不会触发)
- fast→primary fallback: 8/8 救活
- 答案质量: 数字准 / 单位对 (万元) / 时间清 / 中文人话

**示例答案:**
- Q: "上月电商部销售多少" → A: "上月电商部销售总额为 867.39 万 (2026年4月)。"
- Q: "本周对比上周销售情况" → A: "本周销售 670.7 万, 较上周 771.8 万下降 13.1%。"
- Q: "本月卖最好的商品 TOP 5" → A: "松茸调味料108g×15瓶 203.34万元、松茸鲜调味料180gX18袋 167.54万元..."

### 🧪 测试
- 18 个 unit test (route + UnmarshalJSON + clampLimit) 全通过
- 真 DB integration test (config.json 读不到自动 t.Skip, CI 友好)

### 🚀 部署
- bi-server PID 16876 (11:14 启动 v1.73.0)
- config.json: llm_timeout_seconds 30 → 90 (跑哥已手动改)

### ⚠️ 已知限制
- glm-4.7-flash 免费 tier 限流极严 (持续 1302), 实际全靠 primary fallback 救
- glm-4.7 单次推理 30-50s, 用户感受偏慢, 后续考虑缓存常见问题 + 预计算
- 仍未做 Text-to-SQL fallback (跨模块/复合问题落 unknown), W3b 待办
- 业务术语字典是 system prompt 写死, 没做动态 RAG (规模小够用)

### 🔬 下一步
1. 跑哥跟领导对齐 5 个种子用户 (管理层) Beta 1 周
2. 收集反馈, 准确率达 90%+ 后开放全员
3. W3b: Text-to-SQL fallback (LLM 生成 SQL + 白名单校验) — 真要做再上
4. 性能优化: 高频问题缓存 + 数据预计算

---

## v1.72.1 (2026-05-22) — AI 智能助手 W1+W2 (内部 dev, 待 Beta)

### 🎯 起因
领导提需求: 在 BI 看板加 AI 智能助手, 用人话问数 (看数 / 排行 / 趋势 3 类场景), 准确率 90%.

设计文档 `docs/ai-assistant-design.md` 草案 v0.1 (508 行) 已 commit.
跑哥拍板: Hybrid 路线 (90% 走预定义路由调现有 133 接口 + 10% Text-to-SQL fallback), 用 Z.AI GLM-4.7, 管理层视角无权限隔离, 不设预算上限.

### 🏗️ W1 后端骨架 (commit `b2331a0`)
- `internal/ai_assistant/client.go`: Z.AI OpenAI 兼容 chat client (timeout / JSON mode / 错误降级)
- `internal/ai_assistant/service.go`: 主流程 classifyIntent → route → formatAnswer (LLM 2 次调用)
- `internal/handler/ai_assistant.go`: POST `/api/ai-assistant/ask` (走 RequireAuth)
- `internal/config/config.go`: 加 AIAssistantConfig 结构
- `config.json`: 加 ai_assistant 块 (复用 hermes Z.AI key)
- `cmd/server/main.go`: 初始化 Service + 注册路由

### 🏗️ W2 持久化 + 前端浮窗 (commit `c8b41ed`)
- 数据库 3 张表: `ai_chat_session` / `ai_chat_message` / `ai_chat_feedback` (含 UK + 索引)
- 后端 3 个新接口: GET `/sessions` GET `/messages` POST `/feedback` (严格权限隔离, 防越权)
- 前端 `src/components/AIChatWidget.tsx` 浮窗组件:
  - 右下角 56px 圆形按钮 + 380x560 对话框
  - 历史会话 localStorage 持久化 sessionId, 打开拉服务端历史
  - 答案附 sourceAPI / 置信度 / 耗时 / token (业务能核对, 90% 准确率底线)
  - 👍/👎 反馈 (UPSERT 防重复)
  - 新会话按钮, Enter 发, Shift+Enter 换行
- `src/layouts/MainLayout.tsx` 全局挂载

### 🐛 已知限制 (待 W3+Beta 解决)
- ⛔ **Z.AI 账户欠费 (1113)**, 实际未跑通 LLM 调用. 跑哥充值中
- 仅支持 "看数" 类问题 (W2 后续扩 30 接口 / W3 加 Text-to-SQL fallback)
- 仅路由 `department` 1 个内部接口 (走 SQL 直查 sales_goods_summary)
- 无 Text-to-SQL fallback
- 无业务术语字典/Few-shot prompt (W3)
- 种子用户名单跑哥跟领导对齐中

### 📚 文档
- `docs/ai-assistant-design.md` (508 行 MVP 设计) — Hybrid 架构/3 类场景/安全/Sprint Plan/14 个决策
- `CLAUDE.md` 修 handler 大文件清单 (dashboard.go 4010 已过时 → top 5 实际数字)
- memory 全量 audit (112 文件): project 6 个更新基线 + 3 归档 + 1 状态注释; CLAUDE.md 修

### 🚀 部署
- bi-server PID 57504 (10:12:46 启动 v1.72.1)
- 前端 npm run build 完成
- 数据库 migration `v1.73.0-ai-chat-tables.sql` 已应用

### 🔬 下一步 (跑哥)
1. 充值 Z.AI 让 LLM 跑通
2. 跟领导对齐种子用户名单 + 3 类典型问题
3. W3 Sprint: Text-to-SQL fallback + 业务术语字典 + 30 接口路由
4. Beta 5 人测 1 周 → 准确率 90% 达标 → GA v1.73.0

---

## v1.72.0 (2026-05-22) — P1 全清: 前端容错 + 大区告警 + YS 重试 + 登录内存 GC

### 🎯 起因
v1.71.1 PATCH 修了 2 处数据完整性, P1 剩 6 类未清. 跑哥拍板"全清", v1.72.0 一波带走.

### 🔬 修复 6 处 (其中 verify 出 3 处 agent 误报跳过)

#### 前端用户体验 (3 处)
- `ecommerce/SpecialChannelAllot.tsx` 2 处 fetch 加 `.catch + message.error`: 网络错误用户不再"永远转圈"
- `futures/index.tsx` 1 处 try/catch 加 `message.error`: 原料行情加载失败有提示
- `finance/Report.tsx` 2 处列表 key 用内容+index 而非纯 index: 防 warnings/unmapped 数组变化时 React diff 错位

#### 后端可靠性 (3 处)
- `business/parser.go` 未匹配 sheet 加 ⚠️ log 告警: 加新大区时静默跳过不再无声 (memory feedback_dept_enum_grep)
- `yonsuite/webhook.go` ClearBIServerCache 加 3 次重试 + 2s 退避: bi-server 短暂繁忙不再让 UI 看 30min 旧数据
- `auth_login.go` + `auth_dingtalk.go` loginAttempts 加 lastTouched 字段 + cleanup 扫超 1h 未活动: 防 DDoS map 撑爆

### ❌ verify 出的 agent 误报 (跳过, 不修)
- `PurchasePlan.tsx` L96-111 — **已有 `.catch`**, 不是 bug
- `MarketingDashboard.tsx` L26-43 — **已有 2 处 `.catch`**, 不是 bug
- `sync-daily-summary` 清零无事务 — 单 UPDATE MySQL 自动原子, 不需事务

### 🚀 部署
- bi-server.exe + 5 cmd exe rebuild (1 business + 4 yonsuite caller)
- 前端 npm run build (3 个 .tsx 改动)
- 错峰 07:23 重启 bi-server

### 📋 v1.72.0 后 P1 剩 0 处, 进入 P2 阶段
- dashboard.go 4010 行 / RPAMonitor.tsx 1001 行拆分 (P2 长期)
- CORS 白名单移 config.json (P2)
- audit_logs 归档策略 (P2)

---

## v1.71.1 (2026-05-22) — P1 数据完整性 hotfix (财务核对漏行 + 吉客云解析返 0 告警)

### 🎯 起因
v1.71.0 errcheck 后 P1 剩 6 类 23 处. 数据完整性 2 处 (财务核对漏行 + 销售明细金额变 0) 紧急修.

### 🔬 修复 2 处
- `admin_trade_audit.go` 3 处 rows.Err() 检查: 销售单核对中途断连不再静默漏行, 直接 500 告警
- `jackyun/trade.go` FlexFloat/FlexInt 解析失败加 log: 吉客云返回格式异常时不再"假 0", 日志可定位

### 🚀 部署
- bi-server.exe + 16 个 jackyun caller cmd 全 rebuild (按 feedback_deploy_full_module)
- 错峰重启 bi-server

### 📋 剩余 P1 (排 v1.72.0)
- 前端 7 处 fetch 不 catch (用户看到永远转圈)
- sync-daily-summary 清零无事务 / YS webhook 清缓存不重试
- business/parser.go 大区名硬编码
- auth.go loginAttempts 内存泄漏 (内网 35 人风险≈0, 倾向跳过)

---

## v1.71.0 (2026-05-22) — errcheck 大扫除: 45 处错误处理补全 (handler / sync / import 全覆盖)

### 🎯 起因
v1.70.6 review 时只修了 11 处明显的, 跑哥拍板"errcheck 一波带走"彻底清完隐性漏洞.

### 🔬 扫描结果
跑 `errcheck -ignoretests ./...` 全扫:
- 总警告 597 处 → 修后剩 552 (主要降的是**真业务影响**, 剩下都是 deferred Close 等 Go 标准模式, 非误报但不影响安全)
- **真业务高风险 49 处 → 修后剩 5** (只剩 probe-* 一次性探针工具不修)
- 修复率 **95%** (45/49 + 同类扩搜)

### 🚀 3 commit 实施

#### Commit 1 — handler 层 20 处 (8f36120)
- `auth_dingtalk.go` 10 处: 8 处钉钉 OAuth 链路 json.Unmarshal + DingtalkBind UPDATE 失败必返错误 + session cleanup log
- `hesi_approval_worker.go` 6 处:
  - ⚠️ **L197 json.Unmarshal 修了一个真 P0**: 之前响应损坏会被默认 Error=0 误判"全部成功", 现在改 fail 整批
  - L209/230 status='success'/'failed' UPDATE 失败 log 警告 (防 worker 重启时 running→queued 重复处理)
  - L275/305 refresh/审计降级
- `profile_dingtalk_sync.go` 3 处: 钉钉同步 token/userid/detail 解析 log
- `auth_session.go` 1 处: idle timeout 删 session log

#### Commit 2 — sync 工具 14 处 (f911a5d)
- `sync-hesi/main.go` 9 处: 单据明细/发票/附件 UPSERT 失败 log + cleanup 3 处
- `sync-stock/main.go` 1 处: CREATE TABLE 失败必须 Fatal 阻断 (否则后续 INSERT 全挂)
- `sync-daily-trades/main.go` 1 处: trade_package INSERT IGNORE 失败 log
- `sync-detail/sync-goods/sync-goods-blend` 3 处: wrapper json 失败处理

#### Commit 3 — import 工具 11 处 (a991738)
- `import-customer/main.go` 2 处: xhs 客服 trend + excellent_trend
- `import-douyin/main.go` 3 处: item json + anchor REPLACE + ad_material UPSERT
- `import-douyin-dist/main.go` 2 处: material + promote_hourly
- `import-vip/main.go` 3 处: vip_cancel / targetmax / weixiangke
- `import-pdd/main.go` 1 处: resp json

### 📊 修复模式
- **关键路径** (钉钉 OAuth / 合思 worker): 失败 return 错误给客户端
- **批量 sync/import**: 失败 log 记录 (定位"哪行/哪个 shop/哪个 material") + continue 不阻塞整批
- **cleanup / session / 审计**: 失败只 log

### 🚀 部署
- bi-server.exe + 36 cmd exe 全 rebuild
- bi-server 错峰重启

### 🔬 没修的 (低优, 后续)
- probe-* 5 处 (一次性探针工具, 跑哥手动跑)
- rows.Close / db.Close / tx.Rollback 174 处 (Go 标准 defer 模式, errcheck 误报)
- 其他 os.Remove / os.MkdirAll 12 处 (cleanup, 失败无影响)

### 🎁 长期受益
未来再 review **再也不会出现 49 个"db.Exec 不查 error"清单**, 钉钉绑定/合思审批/RPA 导入的疑难 bug 排查从"瞎猜"变"查日志".

---

## v1.70.6 (2026-05-21) — 全项目 review 修复 6 个 P0 (数据完整性 / 限流 / 错误处理)

### 🩺 6 个 agent 全项目体检 (跑哥发起 /gstack 全扫)

| 模块 | 扫到 P0 | 实修 | 误报 |
|------|--------|------|------|
| handler 层 | 6 | 5 | 0 |
| sync/import 工具 | 4 | 2 + 同类扩 3 | 1 (sync-allocate 已有 tx) |
| 前端 pages | 0 | 0 | - |
| schema/安全 | 4 | 1 | - |
| internal/ 子包 | 3 | 2 | - |
| cmd 杂项 + 前端非 pages | 4 | 0 | 1 (Chart.tsx 用 echarts-for-react 库自带 dispose) |

实际修了 **6 个真 P0 + 同类扩搜出 3 个**, 误报 2 个 (跳过).

### 🔒 数据完整性 (3 处)
- `importutil/parser.go` ParseFloat/ParseInt 失败加 log.Printf 警告, 返 0 行为不变 (51 个 RPA 导入点全兼容)
  - 区分"业务真 0" vs "数据污染", 之前两者完全无法分辨
- `finance/parser.go:632` 跳行前加 log (sheet/部门/科目/月份/行列/原值/清洗值) — 财务对账"找不到原因"从此可定位
- `cmd/sync-daily-trades/main.go:204` 吉客云销售单 wrapper json.Unmarshal 加 err 检查, 上游格式抖动不再静默丢页

### 🛡️ 系统可靠性 (2 处)
- `importutil/lock.go` 锁文件写入失败从 silent ignore 改 log.Fatalf 中止, 防双开同步把表搞乱
- `auth_seed.go` addCols 补 3 列定义 (dingtalk_real_name / hesi_staff_id / hesi_real_name) — 新部署兜底, 生产无影响 (列已存在 1060 自动跳)

### ⏱️ YS 用友限流修复 (4 处 — 同类扩搜)
扫 `time\.Sleep\(time\.Duration\(attempt` pattern 抓出 4 个 YS sync 全有同样 bug:
- sync-yonsuite-purchase / subcontract / stock / materialout 重试退避加 1.1s baseline
- 旧: 2/4/6s 退避 (快速重试触发 YS 反爬, IP 封 1 小时整链停)
- 新: 2/4/6s + 1.1s 兜底, 兼容 YS 1.1s 限流规则

### 📢 用户身份链路加错误提示 (5 处)
之前 `db.Exec(...)` 不查 error → 用户以为操作成功实际写库失败:
- `auth_dingtalk.go` 4 处: 钉钉登录写 remark / 自动绑定 / 解绑 / 最后登录时间
- `hesi_approval_worker.go:127` 标 status=running — 失败前停, 防止下游合思 API 调用 + DB 未标记 → 重启后重复提交

### 🚀 部署
- bi-server.exe + 36 个 cmd exe 全 rebuild (importutil/lock.go 变更, 按 `feedback_deploy_full_module` 全模块同步)
- bi-server 22:51 重启一次, 23:01 二次重启
- 智能错峰 (22:53-23:01 区间, 无活跃用户)

### 🔬 没修的 (按报告归类, 排 v1.71.0+)
- 23 个 P1: 前端 7 处 fetch 不 catch / handler 3 处 rows.Err() / 吉客云 FlexFloat 解析 / 业务硬编码大区 / 登录失败计数器内存泄漏 等
- 10 个 P2: dashboard.go 4010 行拆分 / CORS 白名单移 config / audit_logs 归档策略
- **下个 v1.71.0 计划: errcheck 一波带走** (用 golangci-lint 扫所有 db.Exec/json.Unmarshal 不查 error, 估计 30+ 处)

---

## v1.70.5 (2026-05-21) — 合思 "款付票未到" KPI 口径修正 (1742 → 219, 跟合思后台对齐)

### 🐛 跑哥报 (业务反馈)

费控管理页 KPI 卡 "款付票未到" 显示 1742 笔, 财务后台只有 219 笔, 业务对账对不上。

### 🔍 调查 (3 段实证)

**第 1 段 — 旧 SQL 把无关的都算进去了**:
按 `hesi_flow_detail.invoice_status=noExist` 筛, 把 "内部往来 / 公积金汇缴 / 差旅核销 / 团建招待 / 维修费 / 备用金" 等 **本来就不需要发票** 的流程全揉了进来 (TOP spec 调出 "无票核销" / "公积金汇缴" 等明确不要发票的 451+60+...)。

**第 2 段 — 合思 "未核销" 不在单据本身**:
查合思官方文档 (https://docs.ekuaibao.com/docs/open-api/flows/get-loanInfo-ByFlowId) 知道, 预付款核销机制是: 出纳付款 → 系统生成"借款包(loanInfo)" → 借款人后续报销单关联核销 → 借款包余额(remain)归零 → 单据归档(archived). 跟单据本身的 "发票" 字段没关系, 发票其实挂在后续核销报销单上。

**第 3 段 — 合思 OpenAPI URL 模板坑**:
找到借款包接口后, 调用一律 HTTP 404. 找合思客服反馈, 客服回复指出: 文档里 URL 写的 `$flowId` 中 `$` 是 **URL 路径字面字符**, 不是 markdown 模板变量。带上 `$` 后立刻 200 OK。

### 🛠️ 修法

**1. 扩同步**: sync-hesi 同步 loan 类单据时多调一次借款包接口, 入新表存余额/状态。一次性回填了 1276 条历史借款包。

**2. KPI 口径**: 从 "按发票筛选" 改为 "按借款包还款状态筛选":
   - 旧 SQL: JOIN 单据明细按 `invoice_status=noExist` → 1742 (混入大量无票流程)
   - 新 SQL: 查借款包表 `state=REPAID AND active=1` → 219 (跟合思后台 1:1)

**3. KPI 卡名改**: "款付票未到" → "借款待还款" (语义更准, 含预付款 + 员工借款, 与合思后台默认口径一致)

**4. KPI 卡禁点**: 借款包筛选条件跟单据列表的筛选维度不同, 之前点卡跳列表会显示 ≠ 219 的数, 容易误导。卡片改纯展示, 跟"发票文件"卡一致。

### ✅ /codex 二审 GATE FAIL → 2 P1 已修

按 CLAUDE.md 业务口径红线走 codex 二审, 拿到 2 个 P1:
- **P1#1**: SQL 漏 `active=1` 过滤, 未来同步标记 inactive 会含脏数据 → 加上
- **P1#2**: KPI 数(借款包 REPAID 跨所有状态) ≠ onClick 列表数(单据 loan+paid) → 移除 onClick

### 🎓 教训记 (memory)

- `feedback_hesi_no_rate_limit`: 合思 API 不限流, 1.1s 限流是 YS 用友的
- `feedback_hesi_dollar_prefix`: 合思文档 `$flowId` 是字面 `$`, 别去掉

### 🔁 跑哥追加 (同版本, 16:46 重启)

跑哥反馈: 其他 KPI 卡都能点跳列表, 唯独"借款待还款"不能点显得突兀。
- 后端 `GetHesiFlows` 加新筛选维度 `loanRepaid=1` → JOIN `hesi_loan_info` 精准筛 219 条单据
- 前端 `quickFilter` 加第 4 参数, 卡片 onClick 恢复, 清空筛选时一并重置
- 列表数 = KPI 数 = **219** 完全一致, 解决 codex P1#2 同时保住一致体验

### 🔁 跑哥追加 2 (同版本)

跑哥反馈: 219 笔列表里, 单据状态都是"已支付", "当前审批"列还显示"姚晓倩 出纳支付", 应该是"已结束"才对。
- 排查: 219 笔里 166 条已经显示"已结束", 35 条还挂着出纳"姚晓倩 出纳支付", 14 条挂"完成"
- 根因: sync-hesi 的 `fetchApproveStates` 只对 active 状态(非 paid/archived/rejected)拉, 单据从 approving → paid 那刻的"出纳支付"信息被冻结
- 修法: 前端表格"当前审批"列, paid/archived/rejected 终态一律显示"已结束", 不再渲染冻结的过期审批人(更高优先级)

---

## v1.70.4 (2026-05-20) — 计划看板 7 个 KPI 卡布局调整 (上 3 大 + 下 4 小, 0 留白)

### 🐛 跑哥报 (2026-05-20)

v1.70.0 给计划看板加了第 7 个 KPI 卡"单仓缺货率", 老布局 `lg={4}` 一行 6 个 → 7 个变成 **6+1 留 5 格空白**, 第二行只有一个孤零零的卡片, 视觉很难看.

### 🛠️ 修法 (lg span 改, 字号不动)

按 memory `feedback_kpi_card_no_decoration` 不改字号/字体/颜色, 只调 Col span:

- **上行 3 个金额/天指标** (销售GMV / 库存成本 / 库存周转): `lg={8}` (24/3=8 列, 占满首行)
- **下行 4 个比例/指标** (高库存占比 / 缺货率 / 单仓缺货率 / 库龄>90天): `lg={6}` (24/4=6 列, 占满次行)
- `kpiCards` 数组每项加 `colSpan` 字段, 单 map 渲染自动 wrap

响应式适配:
- xs (手机): 上 3 大卡每个 xs={24} 整行, 下 4 小卡 xs={12} 两列
- sm (平板): 上 3 大卡前两个 sm={12}, 周转 sm={24} 整行; 下 4 小卡 sm={12}
- lg (桌面): 上 8/8/8 + 下 6/6/6/6 = 0 留白

### 🎯 效果

```
[   销售GMV   ] [   库存成本   ] [   库存周转   ]
[高库存占比][缺货率][单仓缺货率][库龄>90天]
```

业务意图也清楚了: 上排是"金额/天数"类绝对数, 下排是"%/状态"类相对指标. 跟综合看板 / 部门看板的"大数字优先"风格一致.

---

## v1.70.3 (2026-05-20) — sync-hesi --full 提速 ~4 倍 (附件批量 50→200 + 超时 30→45min)

### 🔬 v1.70.2 修完日志后, 实测发现 full 30 分钟超时不是卡死, 是真慢

12:53 手动跑 --full (v1.70.2 完整进度日志):
- 12:53-12:59 (6 分钟): 拉 22,698 个单据 ✅ (4 formType × 5 state)
- 12:59-13:23 (24 分钟): 附件元信息 50/22698 → 18800/22698, 还差 3,898 个
- 13:23:45 30 分钟超时强制退出 ❌

**算法瓶颈**: `attachBatch=50` × 22,698 单据 = 454 batch × ~4s/batch (含 300ms sleep) = **30 分钟必超时**.

### 🛠️ 修法 (三个常量改动)

`cmd/sync-hesi/main.go`:
- **attachBatch 50 → 200** (合思 API 请求体里传 flowIds, 没硬性 batch 上限, 实测 200 OK)
  - 454 batch → 114 batch, 4 倍提速
- **time.Sleep 300ms → 100ms** (节流够用, 省 ~2 分钟)
- **超时 30 → 45 分钟** (兜底 50% 余地, 防偶尔合思 API 慢)

### 📊 预期效果

| 阶段 | v1.70.2 | v1.70.3 (预期) |
|------|---------|----------------|
| 拉单据 22,698 个 | 6 分钟 | 6 分钟 (不变) |
| 拉附件元信息 22,698 个 | 30 分钟超时退 (拉到 18,800) | ~4 分钟 (114 batch × 2s) |
| **full 总耗时** | **30 分钟超时 ❌** | **~10 分钟 ✅** |

副产品: 合思 API 总请求数从 454 → 114 (减 75%), 减轻服务端压力.

### ✅ 验证 (5/24 周六凌晨 02:30 BI-SyncHesiFull 自动跑)

或跑哥手动 `schtasks /Run /TN BI-SyncHesiFull` → 看 sync-hesi.log 是否 ~10 分钟内出现:
```
========== 同步完成 (full 模式) ==========
单据: 22698 条
附件: NNNNN 个
```

---

## v1.70.2 (2026-05-20) — sync-hesi 进度日志改 log.Printf 进文件 (告别"日志只有一行")

### 🐛 跑哥报 (2026-05-20)

手动触发 sync-hesi --full 跑了 5+ 分钟, 但 `server/sync-hesi.log` **只有一条**
`12:20:12 获取授权成功`. 跑哥以为同步卡了/挂了, 但实际进程健康
(PID 53092 同时连着 MySQL 3306 + 合思 API 443, 在拉数据).

### 🔬 根因

`cmd/sync-hesi/main.go` 把 logfile 接到 `log.SetOutput(MultiWriter(file, stdout))`,
意思是 `log.Printf` 会写文件. 但**进度信息全用了 `fmt.Println` / `fmt.Printf`**,
这些只走 stdout. schtasks 触发的 `.bat` 没 redirect stdout → 进度全部被吞.

剩下进 sync-hesi.log 的只有 `log.Printf("获取授权成功")` 一行 + error log,
所以 sync-hesi 正常跑完, 文件就显示 "成功" 一行, 看着像挂了.

### 🛠️ 修法

`main.go` 全部 18 处 `fmt.Println` / `fmt.Printf` → `log.Printf` / `log.Println`:
- "========== 全量同步 ==========" / "========== 增量同步 =========="
- "=== 拉 expense 单据 ===" / "=== 拉 loan 单据 ===" 等
- "  附件进度 N/M" / "需要补充发票信息: N 条" / "审批状态更新: N 条"
- "========== 同步完成 (full 模式) ==========" / "单据: N 条" / "附件: N 个"

顺手把 log 调用里多余的 `\n` 去掉 (log 包自动加换行, 避免双行).

### ✅ 效果

下次 sync-hesi 跑, sync-hesi.log 里能看到完整时间线:
```
12:20:12 获取授权成功
12:20:13 ========== 全量同步 ==========
12:20:14 === 拉 expense 单据 ===
12:20:18   expense(approving): 47 条(拉取100)
12:21:33   expense(paying): 312 条(拉取400)
...
12:38:01 ========== 同步完成 (full 模式) ==========
12:38:01 单据: 1247 条
12:38:01 附件: 3892 个
```

跑哥从此能看到同步真实进度, 不再以为卡死.

---

## v1.70.1 (2026-05-20) — 修钉钉 "同步任务失败" 误报 (撞锁跳过被错判 failed)

### 🐛 跑哥报 (2026-05-20)

钉钉收到告警:
- 合思费控同步 [失败] 5/20 11:30:01
- BI-SyncHesiFull [失败] 5/17 02:30:00 (exit 1)

### 🔬 RCA 铁证 (sync-hesi.log 时序)

**5/20 11:30 失败** (其实是误报):
- 11:27:32 sync-hesi (hourly) 启动 PID 50328
- **11:30:01** schtasks 再次触发 → 撞 PID 50328 的锁 → 跳过 → exit 1 → task_health 推钉钉
- 11:45:01 又触发 → 成功 (Last Result=0)

**5/17 02:30 失败** (同样误报):
- 02:15:02 sync-hesi (hourly) 启动 PID 41204 (异常慢, 跑了 45 分钟)
- **02:30:00** sync-hesi-full (周六全量) 触发 → 撞 PID 41204 的锁 → 跳过 → exit 1
- 03:00:00 sync-hesi 触发 30 分钟超时强制退出, 释放锁

### 🎯 根因

`internal/importutil/lock.go:41` 锁存在时用 `log.Fatalf` (内部 os.Exit(1))
→ schtasks 退出码 = 1
→ `task_health.go` 判 LastTaskResult != 0/267011/267014 都算 failed
→ 推钉钉给 admin

但**撞锁跳过是正常的并发防撞策略**, 不是业务失败. sync-hesi 数据流一直健康
(hesi_flow 近 24h 更新 265 行, 11:45/12:00 都跑成功).

### 🛠️ 修法

`lock.go` AcquireLock 撞锁逻辑改:
- `log.Fatalf("任务正在运行中...")` (exit 1) → `log.Printf("[xxx] 撞锁跳过本次执行...") + os.Exit(0)` (exit 0)
- 日志内容仍然写明白哪个 PID 占着, 跑哥在运维监控页面看 log tail 仍然能看到跳过历史
- schtasks 视为 success, task_health 不再误报

影响范围: 重 build **全部 25 个**用 AcquireLock 的 import-*/sync-* exe (按 memory `feedback_deploy_full_module` 同步整个模块, 不能只换 sync-hesi).

### ✅ 实测

```
1. 写假 PID 锁 (PID=300)
2. 跑 sync-hesi.exe → 输出 "撞锁跳过本次执行", exit code = 0 ✅
3. 清锁
```

---

## v1.70.0 (2026-05-20) — 计划看板新增"单仓缺货率"KPI + 全仓口径说明

### 🐛 跑哥报 (2026-05-20)

计划看板 → 本月 → 缺货率 显示 **0%**, 跑哥追问"单仓缺货也算上吗".

### 🔬 SQL 复现 + 业务深挖

| 口径 | 缺货 | 在售 | 缺货率 |
|------|------|------|--------|
| 全仓汇总 (当前 KPI) | 0 SKU | 51 SKU | **0%** |
| 按 (SKU × 仓) 单元 | 63 个 | 361 个 | **17.45%** |

**数据上 0% 没错** — 10 品类·7 仓·剔除非卖品标签后, 全仓汇总确实没有 SKU 缺货.

**但被算法掩盖的真相**:
- 西安仓 **30 个 SKU** 单仓缺货 (调拨成本/物流时效)
- 松鲜鲜云仓 **16 个 SKU** 单仓缺货, 月销量 **25,269 件** (用户体验真实受影响)
- 天津/长沙/南京分销仓也都有单仓缺货
- 63 个 (SKU × 仓) 单仓缺货被"全仓汇总"算法藏起来了

### 🛠️ 修法

不改"缺货率"现有口径 (业务定义不主动改), 新增第 7 个 KPI 卡 "单仓缺货率" 跟全仓口径并列:

- **后端** `supply_chain_dashboard.go`: 加新 goroutine 跑单仓缺货 SQL (按 SKU×仓 单元 SUM, 不 GROUP BY goods_no), kpi 返回 3 新字段 `perWhStockoutRate` / `perWhStockoutUnits` / `perWhSalesUnits`
- **前端** `PlanDashboard.tsx`: KPI 卡 6 → 7 个
  - 缺货率 tip 加 "全仓视角" 说明 + "看单仓缺货请看右边" 引导
  - 单仓缺货率 tip 完整解释 (SKU×仓 单元 + 调拨成本 + 采购预警信号)
  - 高库存占比 tip 也补 "10 个核心调味品类、7 个成品仓" + "广宣品/礼盒/子公司自营产品不计入"
  - desc "0/51 核心SKU" + "63/361 SKU×仓" 单位标清楚

### 🎯 效果

跑哥能同时看 0% (全仓) 和 17.45% (单仓), 两个口径并列, 真实采购预警不再被掩盖.

---

## v1.69.1 (2026-05-20) — 货品看板 KPI 卡用 TOP 15 算"全部" 修正 (6 部门同步)

### 🐛 跑哥报 (2026-05-20)

线下部门 → 货品看板 → 4 月一整月: 总销售额 17,525,587.25, **跟综合看板对不上**.

### 🔬 RCA 铁证

| 口径 | 4 月线下总销售额 |
|------|-----------------|
| 货品看板"总销售额" (TOP 15 sum) | 17,525,587.25 ← 跑哥看到的 |
| 全部 4 月线下 (综合看板真值) | 21,556,219.59 |
| 差额 | 4,030,632.34 (**18.7% 缺失**) |

根因: `dashboard_department.go:205` 商品 SQL 末尾 `ORDER BY sales DESC LIMIT 15`, 前端 `ProductDashboard.tsx:144` `totalSales = goods.reduce(...)` 把"TOP 15 合计"当"全部"sum.

### 🎯 影响范围

4 个 KPI 卡都用 `goods` 数组算 (全错):
- 总销售额 (sum) — 偏小约 18%
- 总货品数 (sum qty) — 偏小
- 综合客单价 (sum/sum) — 接近真值但分母也错
- **商品种类(SKU)** — `goods.length` = **永远 15 种** ❗ 实际线下 4 月数百种

6 个部门货品看板都受影响 (`/electronic-commerce/product-dashboard` / `/social/...` / `/offline/...` / `/distribution/...` / `/instant-retail/...` 共享同一个 `ProductDashboard` 组件 + `/api/department` 接口).

### 🛠️ 修法

不动 `LIMIT 15` (保留 TOP 15 表格/图表渲染性能), 后端独立 SUM 全部商品给 KPI 准确口径:

- **后端** `dashboard_department.go`: 在 goods SQL 后加一行独立 SUM 全部商品 SQL, 返回 `totalSales` / `totalQty` / `totalSku` 三个字段
- **前端** `components/ProductDashboard.tsx`: 4 个 KPI 卡用 `data.totalXxx` 字段替代 `goods.reduce` / `goods.length`

不影响:
- ❌ `brands LIMIT 10` (前端没 reduce brands 当 KPI 用, 只用于图表)
- ❌ `StoreDashboard` (店铺 SQL 没 LIMIT, reduce 全部对的)
- ❌ 综合看板 (用 GROUP BY department 直接 SUM, 没 LIMIT, 一直对的)

---

## v1.69.0 (2026-05-16) — RPA 监控彻底修 T+1 业务日 lag 误报"未导入" (京东/拼多多等)

### 🐛 问题现场 (跑哥 2026-05-16 12:59 报)

京东 5/15 显示 "未导入" + "导入" 按钮; 跑哥点了"导入"再刷新, 还是 "未导入".

### 🔬 RCA 锁定 (SQL + 复现工具铁证)

1. 手动跑 `import-jd.exe 20260515 20260515` → 报告 `shop_daily: 2 条` 入库成功
2. 但查 `op_jd_shop_daily` 最新仍是 5/14, **没有 5/15 行**
3. 根因: **京东 RPA 是 T+1 节奏** — 5/16 早上采的 20260515 文件夹, 里面 Excel 业务日期 = 5/14, import 后入 stat_date=5/14 那一行 (`ON DUPLICATE KEY UPDATE` 覆盖, 不新增 5/15 行)
4. **enrichDBStatus 拿"文件夹日期 5/15"严格匹配 op_*_daily.stat_date** → 永远查不到 → 显示"未导入"
5. 这是**预存在的语义错位 bug**, 跑哥之前一直没察觉是因为 T+1 lag 正好"看起来对" (5/14 行实际是 5/15 文件夹的功劳)

### 🛠️ 修法: 新表 `rpa_import_history` 二元判定

不动 14 个 `import-*.exe` 任何代码, 只在 `sync.go` 3 个 spawn 入口处包 wrapper:

- **新表** `rpa_import_history` (id/platform/folder_date/tool/status/rows_digest/triggered_by/started_at/finished_at + UK), 一行一个工具一次跑
- **wrapper** `runSyncToolWithHistory(db, exeDir, tool, dateStr, platform, triggeredBy)` 内部调原 `runSyncTool`, 完成后 `INSERT ON DUPLICATE KEY UPDATE` 写 history
- 3 个调用点改成 wrapper:
  1. `runSync` webhook 通道 — `lookupPlatformByTool` 兜底 (tool:N platforms 取首个)
  2. `ManualImport` 手动导入 — req.Platform 或兜底
  3. `runAutoImportAfterSync` 影刀同步后自动入库 — 有 platform 参数
- **enrichDBStatus** 加 OR 查询: `op_*_daily.stat_date` 集合 **OR** `rpa_import_history.folder_date` (status='success') → 任一来源有 = "已导入"

### 🎯 效果

- T+1 平台 (京东/拼多多/天猫等) 监控页"未导入"误报消除
- 顺带得到一个 **"RPA 处理历史"审计能力** (任一文件夹什么时候被哪个工具处理过, 写了多少行, 谁触发的)
- 旧数据 (没历史记录) 仍走 stat_date 旧逻辑作为 fallback, 平滑过渡

### 📝 顺带

- memory `feedback_rpa_monitor_t1_lag` 记录 RCA 全过程, 下次再碰不用重新查

---

## v1.68.0 (2026-05-16) — 在线用户页面 + 影刀机器人下拉 + RPA"已导入"判定多表

### 🆕 在线用户页面 (新菜单 `/system/online`)

- 系统设置 → 在线用户: 列出最近 N 分钟 (默认 5, 可调 1-60) 还在用 BI 看板的人
- 显示真实姓名 / 部门 / IP / 设备数 / 上次活跃秒数; 一个人多设备多浏览器算 1 行 + 设备数累计
- 自动 30s 刷新; KPI 卡显示总在线人数
- 跑哥日常想知道"现在谁在看", 不用再凭感觉

### 🤖 RPA 平台映射"机器人账号"改下拉

- 系统设置 → RPA 管理 → RPA 文件映射: "机器人账号" 列从手输 Input 改成 Select 下拉
- 列出影刀全量机器人, 显示状态 Tag: 空闲(绿) / 运行中(金) / 离线(灰), 跟影刀控制台一致
- 排序: 空闲优先 → 运行中 → 离线 (选机器人时空闲的优先看到)
- 选项里带主机名 (lhx@sxx · DESKTOP-XXX), 多机器人不会混
- 卡片顶部汇总 3 个 Tag (空闲 X / 运行 Y / 离线 Z) 一眼看健康度
- 不用再凭记忆输入机器人账号, 也能一眼看到哪台机器人离线了
- 接口出处: `ying-dao/yingdao_mcp_server` 开源仓库 (影刀官方 SPA 文档没列), 5 min 缓存

### 🛠️ RPA 监控"已导入"判定多表 OR (修天猫超市误报)

- 旧逻辑只查"店铺"一张表判 "已导入", **天猫超市早期 RPA 没"店铺"sheet 但有广告/无界/淘客 sheet**, 误判 "未导入"
- 改成多表 OR: 天猫超市 7 张表 (shop / campaign / goods / wujie_scene / wujie_detail / smart_plan / taoke) 任一有数据 = 已导入
- 主表 MIN(stat_date) 算业务起点, 避免广告类表 fallback 入库的假早期数据污染时间轴
- 顺手: 批量同步钉钉通知"应用"字段不再硬编码空 (查 rpa_platform_mapping.robot_name 填上)

### 🐛 合思机器人附件链接修复

- "查看详情 → 附件下载" 弹 "获取附件链接失败" 修复
- 后端响应漏 BI 全站 {code, data} 包裹规范, 前端校验 code===200 失败
- 影响 `/api/profile/hesi-attachment-urls` 和 `/api/hesi/attachment-urls` 两个路由

---

## v1.67.1 (2026-05-15) — 批量同步转后台串行队列 + 缺失日期可见 + 自动入库

### 🤖 RPA 监控

1. **缺失日期可见** — 之前 Z 盘没目录的日期直接消失看不见, 现在 2026-01-01 ~ 昨天 (T-1) 全部展示, Z 盘没文件的标"无目录" + 灰色, 每行可点"同步"触发影刀重跑或"导入"补入库
2. **批量同步转后台串行队列** — 勾多个日期点"批量同步" → 后端 goroutine 入队, 完成一个再发下一个 (只有一个机器人, 避免并行抢资源); 关浏览器影刀任务后台继续跑
3. **同步成功后自动入库** — 影刀同步成功后自动调对应平台的 `import-*.exe` 入库, 不用再手动点"导入"按钮
4. **右下角浮窗看进度** — 跑哥在任意页面 (综合看板/客服/财务...) 都能看到右下角云朵图标 + 角标 = 当前等待中+跑中数量; 点开 Drawer 看每个 batch 的逐个日期进度; 全部跑完自动隐藏

### 🚀 性能

- 切 RPA 监控页 / 切平台 tab / 点击"问题汇总" 都明显变快:
  - **后端**: Z 盘 N+1 IO 修复 (`os.ReadDir(dateDir)` 在每个 store 循环重复读, 提到外面只读 1 次)
  - **前端**: Tabs 加 `destroyOnHidden` (11 个平台 tab 不再常驻 DOM); "问题汇总" 表格加 `virtual` 虚拟滚动 (2000+ 行不卡)

### 🎨 视觉

- "正在同步" 按钮的 Badge 角标位置修正 (之前 marginRight 在 Button 上把 badge 截掉, 现在挪到 Badge 上 + size=small)

### 📋 合思机器人 (顺带)

- 合思机器人页加 "报销单审批标准" 卡片 (来源: 张俊 Excel v2026.5.13), 26 条规则按 4 板块结构化展示 (基本信息 / 费用明细集团版 / 线下版差异 / 其他人工), 每条标记 "已自动 / 可自动·待开发 / 需关联查询 / 需 OCR / 人工复核"
- 仅审批人是张俊时才显示, 与后端 `hesi_audit_rules.go` 的"含张俊"判断一致

---

## v1.67.0 (2026-05-14) — 影刀 RPA 一键同步 + 批量补数 + 后台任务可见

### 🤖 RPA 监控页 (主线功能)

跑哥从此不用再手动跑去影刀控制台触发数据采集——直接在 BI 监控页一键同步:

1. **每行"同步影刀"按钮** — RPA 监控页每个日期×平台都能点同步, 触发影刀对应子应用按当天日期采集
2. **批量选中** — 表格每行 checkbox, 选多天 → 顶部"批量同步 (N)" → 一键排队跑 (适合补一周/一月历史)
3. **状态筛选** — "全部 / 只看异常" Radio 一键过滤需要补的日期, 不用一行一行翻 (1000+ 历史日期场景)
4. **同步进度 Modal** — 实时秒表 (1s 跳) + 影刀执行日志 (5s 轮询拿最新), 可最小化后台跑
5. **后台任务面板** — Card 标题 "正在同步 N" Badge + Drawer 列所有跑中任务, 点"查看进度"可重新打开 Modal
6. **完成钉钉通知** — 跑完自动发到 BI 群, 含平台/耗时/失败原因
7. **后端自动巡检** — 30s 扫一次 running 任务, 主动跟影刀对账更新状态; 6 小时未拿到终态自动 timeout

### ⚙️ RPA 文件映射 (新管理 UI)

跑哥可视化维护"BI 平台 → 影刀子应用"映射:
- "影刀任务映射"卡片在 RPA 文件映射 Tab 顶部
- 影刀子应用下拉选 (从影刀 OpenAPI 实时拉, 5min 缓存)
- 11 平台预置初始映射 (京东/京东自营/唯品会/天猫/天猫超市/小红书/快手/抖音/抖音分销/拼多多/飞瓜)

### 🛡️ 防冲突
- 同 (平台, 日期) 已 running 时点同步按钮自动复用现有 trigger_id, 不让影刀机器人重复跑
- 已在跑的日期表格"同步"按钮显示"同步中"灰色 disabled, checkbox 也不让选

### 🚀 性能
- 表格 antd 6 virtual prop 虚拟滚动, 全量历史日期 1000+ 行不卡
- 同步完成不全量重拉数据 (避免批量 N 个 done 叠加渲染), 只刷活跃任务列表

### 🔧 技术细节
- 新增 `internal/yingdao` 客户端模块 (client.go + jobs.go + schedules.go): token 缓存 (2h) + 启动应用 + 查状态 + 通知/轮询日志 + 任务列表/详情
- 新增 8 个后端接口: `/api/admin/rpa/{trigger,job-status,active-tasks,platform-mapping,platform-mapping/update}` + `/api/admin/yingdao/{tasks,sub-apps}`
- 新增 2 张表: `rpa_platform_mapping` (映射) + `rpa_trigger_log` (触发日志, 含 run_date)
- 后台 goroutine `StartYingDaoStatusReaper` 30s 巡检
- 影刀子应用入参 `run_data` (YYYY-MM-DD) 由跑哥在影刀控制台预先添加 11 平台
- 修 2 个影刀 API 解析 bug: notify log 响应 data 是字符串 (不是对象) / log requestId 必须每次重新 notify (不能缓存, 旧的拿空)
- 删假进度条 (影刀 OpenAPI 没暴露真实进度), 改成 "影刀正在执行..." 蓝色提示 + Spin 动画

### 📂 配置
- `server/config.json` 新增 `yingdao` 段 (access_key_id / access_key_secret / auth_url / biz_url / default_account)
- `internal/config/YingDaoConfig` + 环境变量覆盖 `BI_YINGDAO_ACCESS_KEY_ID/SECRET`

---

## v1.66.0 (2026-05-14) — 销量预测精简到 1 个智能算法 + 节假日因子 + 大区12月趋势

### 📊 销量预测 (主线, 重大重构)

跑哥决定: 删除所有算法回测对比, 只保留一个智能算法, 综合考虑 4 因素 (近12月趋势 + 近3月平均 + 同比 + 环比 + 中国节假日).

1. **新智能算法** — 4 因素加权融合 + 节假日因子 + 大区趋势调整
   - 公式: 预测 = (α × 近3月均 + β × 同比 + γ × 环比) × 季节系数 × 节假日因子 × 大区12月趋势
   - 12 月权重动态: 1月β=70%(春节)/2月β=80%/3-5月α=60%/6月节假日×1.05/7-8月α=70%/9-10月节假日×1.05/11月节假日×1.10/12月α=50%
   - 大区12月趋势线性回归: 上升趋势(>+5%/月)×1.05, 下降(<-5%/月)×0.95, 平稳×1.00
2. **删除"历史回测"Tab** — 整个回测页 SalesForecastBacktest.tsx 删除, 不再展示算法对比
3. **删除算法切换器** — 业务页面只显示 1 个智能算法, 不再有 auto/builtin/statsforecast/yoy_v2 切换
4. **顶部"本月公式"Tag** — 实时显示当前月用的权重 (例: "近3月×60% + 同比×30% + 环比×10%"), hover 看完整公式 + 节假日因子 + 趋势调整
5. **Cell tooltip 升级** — 鼠标移到表格单元格能看到完整计算链路: 近3月均 ÷ 季节系数 × 节假日因子 × 大区趋势 = 建议值

### 🗑️ 清理 (3 张表 + 2 schtasks)
- DROP TABLE offline_sales_forecast_backtest
- DROP TABLE offline_sales_forecast_statsforecast
- 删 BI-RunForecastBacktest schtasks
- 删 BI-TrainStatsForecast schtasks

### 🔧 技术细节
- 后端新增 offline_sales_forecast_smart.go (250+ 行, 算法核心)
- 后端 GetOfflineSalesForecast 返回 forecast_summary + region_trend 字段
- Python ML 训练脚本 (Prophet/StatsForecast/LightGBM) 代码保留, 但定时任务已停, 不再实际运行
- v1.65 之前所有算法对比的代码 (chooseAutoAlgo/algoLabelCN/GetOfflineSalesForecastBacktest) 全部删除

---

## v1.65.0 (2026-05-14) — 销量预测算法精简 + 手算同比上线

### 📊 销量预测 (主线, 5 件)

跑哥手算同比验证, 4 个算法回测平均误差 > 50% 全部下线, 加新版同比 yoy_v2 接管 1-2 月春节先验.

1. **删除 4 个高误差算法** — Prophet (99.03%), yoy (56.44%), last_month (52.84%), lightgbm (51.18%) 数据库物理清理 + 前端选择器移除
2. **新增 yoy_v2 (去年同期)** — 取去年同月销量当本月预测, 实时从汇总账拉数据, 春节月最稳 (跑哥手算验证 1 月误差 28% / 2 月误差 11% 比所有 ML 算法都低)
3. **智能路由收窄** — 候选只剩 statsforecast / yoy_v2; 1/2 月春节兜底从 Prophet 切换到 yoy_v2
4. **删除 BI-TrainProphet schtasks** — 不再训 Prophet 模型, BI-TrainStatsForecast 保留
5. **数据库整表清理** — DROP TABLE offline_sales_forecast_prophet, 节省存储

### 🔧 技术细节
- 重跑 baseline-backtest 灌 4 月 (1-4月) yoy_v2 回测样本
- offline_sales_forecast_backtest 现存 5 个算法 (lightgbm_sku/statsforecast/avg3m/wma3/yoy_v2)
- 智能路由 SQL 候选 algo IN ('statsforecast','yoy_v2'), 大幅减少噪声

---

## v1.64.0 (2026-05-14) — 原料行情新模块 (期货行情)

### 📈 原料行情 (新菜单)

为采购/管理层提供原材料和包装材料的期货行情参考, 跟业务联动判断采购时机.

**1. 行情总览** (`/futures`)
- 16 个品种网格首页, 分三类: 主要原料(豆粕/豆油/棕榈油/菜籽油/菜籽粕/白糖/玉米) / 包材(聚乙烯/聚丙烯/玻璃/纸浆) / 大宗(原油/黄金/螺纹钢/铜/沪深300)
- 每张卡片: 当前价 + 涨跌 + 最近 30 天迷你折线 (红涨绿跌)
- 底部涨跌幅 TOP5 / 跌幅 TOP5 排行

**2. 走势图** (`/futures/trend`)
- 多品种叠加对比: 自动按涨跌百分比归一化 (避免不同单位混淆)
- 单品种 K 线模式
- 时间范围: 1月/3月/半年/1年/全部 + 自定义

**3. 品种详情** (`/futures/detail?code=XX`) — **同花顺/通达信级专业 K 线**
- 顶部价格信息栏: 当前价大字 + 8 个指标 (今开/昨收/最高/最低/振幅/成交量/持仓量/年初至今)
- 主图: 价格蜡烛 + MA5/MA10/MA20/MA60 四条均线
- 中图: 成交量柱 (按涨跌染色)
- 副图: 4 种技术指标切换 — MACD / KDJ / RSI / BOLL
- 3 种周期: 日K / 周K / 月K (前端按 ISO 周/月聚合, 切换瞬时)
- 鼠标十字光标三 grid 联动, tooltip 显示完整 OHLC + 涨跌 + MA 数值
- 滚轮缩放 + 底部 slider 选区间

### 🔧 数据底层
- 数据源: 新浪财经期货免费 JSONP 接口 (业内量化平台广泛使用, 稳定 20+ 年)
- 历史数据: 16 品种从 2018-01-01 起共 32,101 条日线全量灌库
- 定时任务: `BI-SyncFutures` 每天 17:30 SYSTEM 账户自动同步前一日收盘数据
- 数据库: 新增 `futures_symbol` (品种字典) + `futures_price_daily` (日线 OHLCV) 两张表

### 📊 关联业务
- 调味品行业核心原料 (豆粕/豆油/白糖) 行情可参考采购时点
- 包材类 (聚乙烯/聚丙烯/玻璃/纸浆) 用于评估包装成本走势
- 大宗类 (原油/铜/螺纹钢) 反映宏观经济与运输/能源成本

---

## v1.63.2 (2026-05-13) — 费控管理加单据模板筛选

### 💰 费控管理 (1 件)
1. **单据模板筛选** — 筛选栏 "发票状态" 旁边新增 "单据模板" 下拉, 支持搜索 (按拼音/中文模糊), 选了就只看该模板的单据 (出差申请/报销单/借款单 等 19 个). 后端按 `specification_id` 前缀匹配 (字典 id 是合思 specId 的前缀)

---

## v1.63.1 (2026-05-13) — 销量预测易用性增强 + 用户审批自动钉钉通知

v1.63.0 发完之后业务用着挖出来的 3 个小迭代, App 模式攒一起发 PATCH:

### 📊 销量预测易用性 (2 件)
1. **货品编码 hover 弹近 13 月趋势** — 鼠标放在货品编码上, 右侧弹出 720×360 的折线图, 每个月数据点上标数字, 业务一眼看清"涨/跌/季节性/异常"四种走势, 同 SKU 二次 hover 不重复请求
2. **"线下总计" 偏离 ±20% 自动标记** — 预测本月合计 vs 前 3 月销量均值, 偏离 ≥20% 显示 ↑/↓ Tag, ≥50% 红色, hover Tag 看具体数字 ("预测 XXX / 近3月均 YYY / 偏离 ±XX%"). 例: 预测 6 月时基准 = 3/4/5 月均, 预测 4 月时基准 = 1/2/3 月均, 自动滑动

### 👤 用户管理 (1 件)
1. **账号开通自动钉钉通知** — 管理员把用户状态从"待审批/已禁用"改成"启用"时, 自动钉钉私聊推送给本人: "你的 BI 看板账号已开通, 可以登录使用了 + 登录地址 + 用户名 + 审批人". 解决"账号开通了同事不知道"问题. 用户未绑钉钉/凭证未配置静默跳过

---

## v1.63.0 (2026-05-13) — 销量预测算法回测体系 + 合思机器人增强 + 权限治理

本期主线: **算法决策从拍脑袋升级到数据驱动** —— 销量预测页加历史回测 Tab, 跑了 8 个算法的 MAPE 对比, "智能模式" 从固定路由改成基于历史回测自动选最准的算法 + 每月自动回测。配套合思机器人增强 + 个人中心权限治理。

### 📊 销量预测算法回测 (主线, 8 件)
1. **历史回测 Tab** — 销量预测页新增"历史回测", 展示月份 × 算法 × 大区 MAPE 矩阵 + 趋势 + 明细
2. **8 算法摆开比较** — 当前 SKU 级梯度提升 36.9% (新冠军) → 统计集成 37.7% → 近3月均 37.9% → 上月直推 52.8% → 贝叶斯时序 99%
3. **智能路由数据驱动** — 优先看同月份历史 MAPE 选, 退而看全部历史平均, 都没数据再兜底硬编码
4. **路由理由透明展示** — "本月走 XX" Tag hover 显示选择理由 ("MAPE 37.7%, 30 条样本")
5. **SKU 级训练加入** — 数据量从 200 行→2.9 万行, MAPE 从 51% 降到 37%, 验证 M5 冠军方案
6. **节日特征实验 (诚实回滚)** — 加 10 个传统节日 + 电商节特征, 实测反退 4.2pp, 已回滚, 工具函数留存待后续旬级实验
7. **算法名全中文化** — 业务能一眼读懂 (统计集成/贝叶斯时序/梯度提升·SKU级/近3月均), hover 还能看英文学名
8. **每月 2 号自动回测** — schtasks BI-RunForecastBacktest 每月跑上月数据, 算法 MAPE 持续累积

### 🤖 合思机器人 (4 件)
1. **字段对齐费控管理** — 待审批列表加 单据模板/发票/附件/创建时间 列, 单据编码/标题点击弹详情 Modal (4 Tab: 基本/明细/发票/附件), 跟费控管理体验一致
2. **AI 审批建议范围限定** — 自动审批建议只对张俊老师生效 (规则来自张俊 Excel), 其他审批人不展示, 避免误用
3. **刷新按钮加说明** — 鼠标 hover 解释"从本地数据库重读, 想看合思最新→费控管理立即同步合思", 避免误会
4. **个人中心权限治理** — 加"个人中心-合思机器人"权限开关, 默认 财务/部门负责人/管理层 可见, 运营/供应链 不可见

### 💰 费控管理 (1 件)
1. **顶部加"上次同步时间"** — 业务一眼看到合思同步是否新鲜, 每分钟自动刷新

### 🔧 技术细节 (内部)
- 8 算法回测落地表 `offline_sales_forecast_backtest`, schtasks 月度自动跑
- 4 Python 脚本 (Prophet/StatsForecast/LightGBM/LightGBM-SKU) + 1 Go cmd (4 baseline) 串行入库
- 个人中心权限种子 `profile.hesi_bot:view`, 自动随启动同步给默认角色
- 合思详情/附件接口拆出 profile 版, 接审批人/提交人/管理员业务级鉴权

---

## v1.62.1 (2026-05-13) — 合思机器人手动审批 + 异步队列 + 审批建议 + 费控模板列

App 模式累积发版第一弹——本来散在 v1.62.0 后 13 个 commit, 收尾时整组打包成一个 PATCH。

跑哥反馈"反复发版颗粒度太碎", 改成 commit 期间不发 notice, 整组功能闭环才升 MINOR/PATCH。

### 🤖 合思机器人 (3 件)
1. **手动审批按钮 + Modal** — 列表每行加"审批"按钮; 点击弹 Modal 选同意/驳回 + 备注; 调合思 OpenAPI 真审批 (修 path [方括号]/approveId 完整 corp:staff 格式两个坑)
2. **异步审批队列 + 批量合并** — 防多人同时审批撞合思限流 (60s 间隔/10单一批); 单据进队列后 modal 立刻关闭, 列表显示"排队中"chip, worker 串行处理 + 60s 间隔 + 同 approve_id 合并; 失败钉钉通知
3. **AI 审批建议列** — 报销单按 4 条字段规则跑 (消费事由空/含合计/超50字/法人实体不在白名单), 给出 ✅同意 / 🟡转人工 / ❌驳回 三色 Tag + 鼠标 hover 看原因; 实测张俊 74 单: 77% 同意 / 15% 转人工 / 8% 驳回

### 💰 费控管理 (4 件)
1. **默认隐藏已结束** — 状态筛选默认"未结束"组 (approving/paying/pending/PROCESSING), 减少历史已 paid/archived/rejected 干扰; Select 加 5 个分组选项
2. **单据模板列** — 列表新增"单据模板"列, 显示真实模板名 (日常报销单/出差申请单/付款单等); 实时调合思 specifications API 拉字典 + 60s 内存缓存; 详情 modal 同步显示
3. **搜索筛选** (合思机器人页) — 单据编码/标题模糊 + 类型多选 + 提交日期范围
4. **审批人实时刷新** — 跑哥审批后, 单据当前审批人/节点立即同步合思 approveStates 接口, 不再等下次定时同步

### ⚙️ 运维优化 (3 件)
1. **sync-hesi 调度修复** — schtask 用 wscript+vbs 在 SYSTEM 账户失败 (0x800710E0), 改 .bat 直调; sync 数据连续滞后 24 小时问题解决
2. **sync 增量优化** — archived 不再每次拉取 (16587 单永久归档); paid/rejected 改"近 7 天"过滤 (取代固定 200 条); 单次 sync 5-10min → 4min; 周日 02:30 加 BI-SyncHesiFull 全量兜底
3. **sync 频率提升** — 每小时 1 次 → 每 15 分钟 1 次; 本地数据滞后窗口压缩 4 倍

---

## v1.62.0 (2026-05-13) — 需求池上线 + 反馈管理改造为「需求与反馈」

第一个按 App 模式发的版本——commit 不带版本号，整组功能闭环才发一次。

跑哥之前所有业务方提的需求散在 7 处 memory 里没结构化，从这一版起搬进 BI 看板做统一管理。

**改动**:
- 系统菜单 "反馈管理" 改名 "需求与反馈"
- 顶部 Tabs 切换 [反馈] [需求]，URL `?tab=requirements`
- 右上角 "反馈" 按钮改 Dropdown：反馈问题 / 提交需求
- 任何登录用户都能提需求（沿用反馈管理 modal，加 Radio 切换类型）
- 需求模块三视图：
  - 列表：4 活跃 KPI + 3 次要 KPI + 优先级×版本排序 + 详情 modal（管理员可改状态/排期/备注/标签）
  - 看板：5 列拖拽改状态（待评估→已接受→排期中→开发中→已完成），HTML5 原生拖拽零 dep
  - 甘特图：按目标版本号分组，未排期组在最前，暂缓清单（已搁置/已拒绝）单独列底部
- 状态机：待评估 → 已接受 → 排期中 → 开发中 → 已完成（+ 已搁置/已拒绝）
- 优先级 P0/P1/P2/P3 + 排期 BI 版本号关联
- 钉钉通知：新需求通知所有管理员；状态变更（接受/完成/拒绝/搁置）通知提需求人
- 菜单徽标对齐 v1.61.0 模式：「需求与反馈」红色数字 = 反馈 pending + 需求 pending
- 新表 `requirements`：15 字段 + 5 索引 + 中文注释
- 新权限 `requirement.manage`（super_admin 自动获得）
- 6 个新 API：`/api/requirements`（POST 提交）+ `/list`/`/stats`/`/gantt` + `PUT/DELETE`
- 初始数据：录入 9 条 memory 项目（车图自动化/合思 v1.61/钉钉绑定二期/销量预测打磨/客服 KPI/财务科目/上云 C/OA 自动审批/产品机器人文档）

**反馈 tab 兼容**: URL `/system/feedback` 不变；默认 tab=feedback 老用法不受影响；feedback 4 KPI/筛选/详情 modal 完整保留。

**实测**: 9 条初始数据按优先级×状态正确分布到 7 个 KPI 卡 + 5 列看板 + 3 个甘特图分组；菜单徽标显示「需求与反馈 2」（2 个 pending 需求）；反馈 tab 回归无破坏。

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
