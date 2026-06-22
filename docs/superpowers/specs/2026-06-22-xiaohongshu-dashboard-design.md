# 小红书效果看板 设计文档

> 2026-06-22 ｜ 社媒部门新增「小红书看板」

## 背景与目标

社媒部门要在 BI 看板新增「小红书看板」看小红书运营效果。原始诉求是仿照营销中心的 FineBI「KFS报表v2 / 效果数据总览」，但经与跑哥确认收敛：

- FineBI 报表的数据（全站GMV/ROI/成本/利润/外溢/聚光乘风投流费用、话题一二三级分级）是**营销中心从小红书蒲公英/聚光投流后台**拿的，我们 RPA 没采，**不使用**。
- 我们 Z 盘的灵犀（品牌搜索趋势）、乘风（账号记录）、客服数据也**不用**。
- **只用**我们已入库的小红书 **笔记明细 + 商品销售明细** 两张表。
- 布局**自定**（不仿 FineBI），用 BI 看板现有风格组件。

## 数据源

| 表 | 含义 | 字段数 |
|---|---|---|
| `op_xhs_note_daily` | 小红书笔记明细（每日每店全量快照） | 38 业务字段 + stat_date/shop_name |
| `op_xhs_goods_daily` | 小红书商品销售明细（每日每店全量快照） | 29 业务字段 + stat_date/shop_name |

数据由 `import-xhs` 每日 13:00 自动导入（已上线），现有 6/3~6/21（6/4、6/5 RPA 源头未采）。

## 核心数据口径（最关键，全程铁律）

两表均为**每日全量快照**：同一笔记/商品每个 `stat_date` 都有一行，数值是**当天观测值（波动，非累计递增）**——实测同一笔记阅读量跨 16 天为 3483→7243→10847→2864→6920 的波动。

**铁律：禁止跨天 SUM**（同一笔记/商品多天会被重复计）。

- **KPI 卡 / 明细表**：固定到**单个 `stat_date`**（默认取库中最新日）。
- **趋势图**：按 `stat_date` 分组，每天一个点（每点 = 当天全量 SUM）。日期范围**只服务趋势图**。
- **商品表特有维度**：同一 `product_id` 按 `business_type`（全部/自营/带货）×`carrier`（全部/商卡/笔记/直播）拆多行。**默认口径取 `business_type='全部' AND carrier='全部'`**（每商品一行总口径），避免切片重复累加；经营方式/载体作为**可选**筛选维度。

## 放置与权限

- 菜单：社媒部门（`/social`）下新增「小红书看板」，与「飞瓜看板」「营销看板」并列。
- 路由：`/social/xiaohongshu`。
- 权限位：`social.xiaohongshu:view`（`auth_seed.go` 新增；super_admin 自动有；社媒相关角色按需由跑哥在角色管理开）。
- `navigation.tsx` 需接入 4 处：① children 菜单项 ② menuTitles 面包屑 ③ routePermissions ④（如有部门映射处）。

## 页面结构：两个 Tab（AntD Tabs）

### Tab 1 — 笔记效果（`op_xhs_note_daily`）

- **筛选**：数据日期（单选，默认最新）+ 店铺（多选）+ 笔记类型（图文/视频）
- **KPI 卡**（选定日 SUM）：笔记数（COUNT note_id）、总阅读（read_count）、总互动（like+collect+comment+share）、笔记带货GMV（pay_amount）、带货订单（pay_order_count）、平均转化率（ΣpayUV÷Σ商品点击UV 加权，不可对日率简单平均，见 [[feedback_rate_metric_weighted_not_simple_avg]]）
- **趋势图**：按 stat_date → 阅读量（柱）+ 笔记带货GMV（线）双轴
- **明细表**：笔记排行（默认 pay_amount 倒序），列：笔记标题 / 类型 / 作者 / 阅读 / 点赞 / 收藏 / 评论 / 分享 / 带货金额 / 转化率 / 关联商品名（标题可点 note_url 跳转小红书）

### Tab 2 — 商品销售（`op_xhs_goods_daily`，默认 全部×全部）

- **筛选**：数据日期（默认最新）+ 店铺（多选）+ 一级品类 +（可选）经营方式 / 载体
- **KPI 卡**（选定日 SUM）：商品数（COUNT）、总访客（visitor_count）、支付金额（pay_amount）、支付订单（pay_order_count）、支付件数（pay_qty）、退款金额（refund_amount_by_pay）
- **趋势图**：按 stat_date → 支付金额（柱）+ 访客（线）双轴
- **明细表**：商品销售排行（pay_amount 倒序），列：商品名 / 一级品类 / 二级品类 / 访客 / 浏览 / 加购 / 支付金额 / 订单 / 件数 / 转化率 / 客单价 / 退款金额

## 技术方案

### 后端（`server/internal/handler/ops_xiaohongshu.go` 新增）

- `GET /api/social/xiaohongshu/filters` → 返回筛选可选项：店铺列表、最新数据日期、可选日期列表、一级品类列表、笔记类型列表。
- `GET /api/social/xiaohongshu/note` → 参数 `date`（默认最新）、`shops`、`note_type`；返回 `{kpi, trend[], detail[]}`。
- `GET /api/social/xiaohongshu/goods` → 参数 `date`、`shops`、`category_l1`、`business_type`、`carrier`（默认全部×全部）；返回 `{kpi, trend[], detail[]}`。
- KPI/明细 = 单日查询；trend = 按 stat_date GROUP BY（日期范围或全部）。
- 所有 SQL 必须带 timeout（见 [[feedback_no_long_sql_no_timeout]]）；明细表 LIMIT TOP N，前端 reduce 算合计前确认（见 [[feedback_frontend_reduce_with_limit]]），优先用后端全口径合计字段。
- `main.go` 注册三路由，挂 `social.xiaohongshu:view` 权限校验。

### 前端（`src/pages/social/XiaohongshuDashboard.tsx` 新增）

- 参考 `FeiguaDashboard.tsx` 模式：AntD `Tabs`（笔记效果 / 商品销售）。
- 每 tab：`bi-filter-card`（筛选）+ `bi-stat-card`（KPI）+ `Chart`（ECharts 趋势）+ AntD `Table`（明细）。
- 响应包裹 `{code,data}`，前端取 `j.data`。
- 视觉遵循 BI 既有规范：不自改字体/字号/装饰色（[[feedback_kpi_card_no_decoration]]）、文字色用 token。
- `App.tsx` 加路由；`navigation.tsx` 4 处接入。改 .tsx 必须 `npm run build`。

### 权限

- `auth_seed.go` 新增 `social.xiaohongshu:view` 权限位。

## 非目标（明确不做）

- 不用 FineBI「效果数据总览」外部文件。
- 不做客服 / 灵犀（搜索趋势）/ 乘风数据。
- 不做投流 ROI / 成本 / 利润 / 外溢 / 话题分级 指标（无数据）。
- 不做 产品 / 类型 / 策划 / 商务 / 跨域项目 / 人群 / 笔记ID 筛选（数据无）。
- 一期不做前端导出。

## 测试与验收

- 后端 handler 单测（sqlmock）：date 默认最新、shops 过滤、商品默认全部×全部口径、趋势按天分组、空数据兜底。
- 口径验证（真库）：KPI/明细单日 SUM = 真库该日 SUM；趋势每点 = 当天 SUM；商品不重复计切片。
- 前端：`npm run build` 通过 + playwright 实测两 tab 切换 / 筛选联动 / 图表渲染（[[feedback_test_and_verify]]）。
- 部署：错峰重启，纯新增不影响现有功能。

## 风险 / 已知

- **快照不可累加**是最大坑——SQL 与前端全程按单日，趋势按天。
- 商品切片维度默认全部×全部；业务若要看分载体走可选筛选。
- 数据 6/3 起，6/4、6/5 缺（RPA 未采，非看板问题）。
