# 松鲜鲜 BI 数据看板 更新日志

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

## 后续规划（通往 v1.0）
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
