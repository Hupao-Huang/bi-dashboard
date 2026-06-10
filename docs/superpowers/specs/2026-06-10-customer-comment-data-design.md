# 客服 - 评论数据模块 设计文档

- 日期：2026-06-10
- 模块：客服部门 → 评论数据
- 状态：设计已与跑哥对齐，待评审 → 转实现计划

## 1. 背景与目标

客服部门有一套 RPA，每天抓取各电商平台店铺后台的「评价数据」，汇总成 Excel 落在 Z 盘。
现在要把这批数据接入 BI 看板，让客服在网页上**选平台 + 日期 + 店铺，查看评价明细表**（像 Excel 一样平铺展示，不做统计图表 / 不做差评预警 / 不做评分美化）。

明确不做（YAGNI，客服明确说"和 Excel 一样就行"）：KPI 卡片、评分分布图、趋势图、差评告警、评分筛选。

## 2. 数据源

- 路径：`Z:\信息部\RPA_客服_店铺后台评价抓取\结果汇总`
- 文件：`YYYY-MM-DD评论数据.xlsx`，每天一份；**文件名日期 = RPA 抓取日，不是业务日期**
- 范围：`2025-12-29` 起至今，共 162 份（截至 2026-06-09），约 86 条/份，总量约 1.4 万条
- 单 sheet「抓取数据汇总」，7 列，一份文件内多平台混排：

| 列 | 列名 | 说明 |
|----|------|------|
| 0 | 平台 | 抖音 / 拼多多 / … |
| 1 | 店铺 | 店铺名 |
| 2 | 时间 | **格式不统一**：抖音是文本 `2026年06月07日 12:39:34`；拼多多是 Excel 日期(datetime) |
| 3 | 订单编号 | **格式不统一**：抖音是纯数字 19 位 `6953400731356894751`；拼多多带前缀 `订单编号：260527-277872699724055` |
| 4 | 商品名称 | |
| 5 | 评价内容 | |
| 6 | 评分 | 整数，1-5（1-2 为差评） |

## 3. 数据处理规则（导入时清洗）

1. **时间 → 年月日**：两种格式都解析成 `DATE`（`YYYY-MM-DD`）
   - 文本 `2026年06月07日 12:39:34` → `2026-06-07`
   - Excel datetime → 取日期部分
   - 原始时间文本另存一列备查
2. **订单编号提取**：剥掉中文前缀 `订单编号：`（及可能的空格），只留真号；抖音无前缀则原样
   - 一律按**字符串**存（19 位长号，绝不转数字，防精度丢失）
   - 原始订单编号文本另存一列备查
3. **平台**：直接取「平台」列原值，与客服总览平台命名对齐（实现阶段核对：抖音/拼多多/天猫/淘宝/京东…）
4. 脏数据：缺列 / 时间解析失败 / 评分非数字 → 跳过该行并计入日志，不中断整份导入

## 4. 数据模型

新建表 `op_customer_comment`（明细表，非按天聚合；沿用 RPA `op_*` 前缀）：

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT PK AUTO | |
| platform | VARCHAR(32) | 平台 |
| shop_name | VARCHAR(128) | 店铺 |
| comment_date | DATE | 评论日期(年月日) |
| comment_time_raw | VARCHAR(64) | 原始时间文本(备查) |
| order_no | VARCHAR(64) | 提取后的订单编号(字符串) |
| order_no_raw | VARCHAR(128) | 原始订单编号文本(备查) |
| product_name | VARCHAR(255) | 商品名称 |
| comment_content | TEXT | 评价内容 |
| score | TINYINT | 评分 1-5 |
| source_file | VARCHAR(64) | 来源文件名 |
| content_hash | CHAR(32) | 去重键 = md5(platform|shop|order_no|comment_date|comment_content) |
| created_at / updated_at | DATETIME | |

- **去重**：`UNIQUE KEY uk_hash (content_hash)`，导入用 `INSERT ... ON DUPLICATE KEY UPDATE`（幂等，重复导入/重跑历史不会重复累加）。配合 [feedback_upsert_requires_uk]。
- 评论无天然主键，故用内容 hash 去重；若 RPA 文件是「累计快照」也安全（同一条多次出现只留一条）。
- 中文注释（[feedback_chinese_comments]）。
- 建表后必须重建 + 重启 bi-server 让 `docs.go` 映射生效（[feedback_new_rpa_table_checklist] 7 步）。

## 5. 组件

1. **导入工具** `server/cmd/import-comment/main.go`（参照 `import-customer`，用 excelize）
   - 入参：单份文件 / 整个目录全量；默认增量导最新，支持 `--all` 全量重跑
   - 流程：读 xlsx → 逐行清洗(时间/订单号) → 算 content_hash → upsert
   - `.xls` 跳过报错（[feedback_no_xls_parse]，但此目录全是 .xlsx）
   - 编译产物拷到 `server/` 根（[feedback_deploy_exe]）
2. **后端接口**（`internal/handler`，挂 `/api/customer/...`）
   - `GET /api/customer/comments`：按 platform / date_from / date_to / shop 筛选，返回明细（分页，按 comment_date 倒序）
   - `GET /api/customer/comment-options`：返回平台、店铺下拉选项（distinct）
3. **前端页面** `src/pages/customer/CommentData.tsx`
   - 顶部筛选：平台(下拉) + 日期范围(DatePicker) + 店铺(可搜下拉)
   - 下方 antd Table：平台 / 店铺 / 时间(年月日) / 订单编号 / 商品名称 / 评价内容 / 评分；分页；默认时间倒序
   - 视觉用 antd 默认，不自改字体/颜色（[feedback_kpi_card_no_decoration]）
   - 改完必须 `npm run build`（[feedback_bi_frontend_must_build]）
4. **菜单 + 权限**
   - `navigation.tsx`：客服部门 `/customer` 下新增 `/customer/comment`「评论数据」，权限 `customer.comment:view`
   - `auth_seed.go`：permissionSeeds 加 `{customer.comment:view, 客服-评论数据, page}`；roleDefaultPermissions 给 management / dept_manager / operator 加该权限（与 customer.overview 同组）
   - 路由 `App.tsx` + `cmd/server/main.go`
5. **定时任务**：`schtasks` 新增 `BI-ImportComment` 每日定时（SYSTEM 账户，.bat 包装；[feedback_scheduled_task_system] [feedback_schtasks_redirect_needs_bat]）；并可挂进现有 `/api/webhook/sync-ops` 触发链

## 6. 数据流

```
RPA → Z盘 xlsx → (schtasks 每日) import-comment.exe → 清洗 → op_customer_comment(upsert)
                                                                      ↓
客服浏览器 → /customer/comment 页 → /api/customer/comments(筛选) → 明细表
```

## 7. 错误处理

- Z 盘不可达 / 超时：导入工具报错退出，不写脏数据；所有 Z 盘访问带超时（[feedback_z_drive_hang_recovery]）
- 单行脏数据：跳过 + 日志计数，不中断整份
- 时间 / 订单号解析失败：记录原始值到 `*_raw` 列，comment_date 置空并日志告警
- 字段超长：建表时长度留足，`Data too long` 立即修（[feedback_data_too_long]）

## 8. 测试要点

- 单测：时间解析（抖音文本格式 + 拼多多 datetime 两种）→ 正确年月日
- 单测：订单号提取（带「订单编号：」前缀 + 纯数字两种）→ 正确号码、字符串不丢精度
- 单测：content_hash 幂等（同一行导两次只一条）
- 实测：导 2-3 份真实文件，核对入库条数 = Excel 行数（[feedback_test_and_verify]）；前端 playwright 走筛选 + 翻页

## 9. 默认决策（已采用，无异议即执行）

- 历史从 2025-12-29 全量导入 162 份
- 重复导入幂等去重（content_hash）
- 评分显示数字，不做星级/颜色
- 表格默认 comment_date 倒序 + 分页

## 10. 待实现阶段核实

- 平台命名是否与客服总览完全一致（淘宝 vs 天猫等）
- RPA 文件是「当日新增」还是「累计快照」（影响是否只需导最新一份；但 hash 去重对两者都安全）
- content_hash 字段组合是否足够区分（同订单同内容多条评价的极端情况）
