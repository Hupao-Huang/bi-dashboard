# C 路: 后端 sync/import 工具审查 (2026-05-08)

agent: ad908b5b2d8f008c6
基线: cdb60b1 (v0.83)
范围: server/cmd/*/main.go 40+ 个 + internal/jackyun + yonsuite + finance + business + importutil

---

## P0 (6 项 - 数据可能丢/错)

### P0-1 sync-stock 翻页游标可能漏数据
- 文件: server/cmd/sync-stock/main.go:114-130
- 问题: maxQuantityId 用 `lastQid` (循环最后一条), 不是 MAX. 如果 page 内 quantityId 不单调, 中间数据被跳过.
- 修复: `lastQid` 改 `MAX(quantityId of all items)`.

### P0-2 6 个 import 工具吞 config/sql.Open 错误
- 文件: import-vip/import-tmallcs/import-douyin-dist/import-jd/import-pdd/import-douyin 各 main.go
- 问题: `cfg, _ := config.Load(...)` + `db, _ := sql.Open(...)` 错误全 _
- 修复: 改 log.Fatalf, 否则 cfg.Load 失败时 DSN=":@tcp(:0)/" 静默走完逻辑

### P0-3 sync-trades-v2 / sync-half-day / sync-daily-trades 翻页 scrollId 缺失直接 break, 漏数据
- 文件: sync-trades-v2:390 / sync-half-day:161 / sync-daily-trades:355
- 问题: 没 scrollId 就 break, 但接口偶发不返
- 修复: fallback 用 pageIndex 翻页 + 打日志保留时间窗

### P0-4 sync-channels 部门映射写死 (10 个未映射)
- 已在 memory project_unmapped_channels.md
- 修复: 跑哥确认后搬到 channel_dept_mapping 表

### P0-5 sync-stock-io 先 DELETE 再拉 API, 失败时空窗
- 文件: server/cmd/sync-stock-io/main.go:60-104
- 问题: API 失败 fall through 不 fatal, 当天数据被删了没补上
- 修复: 照 sync-daily-summary 用事务: collect 先 → tx.Begin → DELETE → INSERT → Commit

### P0-6 import-finance 无 file lock
- 文件: server/cmd/import-finance/main.go
- 问题: 没 importutil.AcquireLock, 重复触发可能脏写
- 修复: 加锁

---

## P1 (15 项 - 该修)

P1-1 fetch-channels 硬编码生产 AppKey+Secret (server/cmd/fetch-channels/main.go:11-15)
P1-2 sync-summary 默认日期写死 2025-01-01~2026-03-25, 不传环境变量直接跑全量
P1-3 sync-trades 日期+tableMonth 写死 (建议删, sync-trades-v2 已覆盖)
P1-4 sync-detail tableMonth 写死 "202601"
P1-5 sync-daily-trades 伪循环 dayOffset 1→1, 改 --days=N 参数
P1-6 sync-yonsuite-stock lastErr 失败 log.Fatalf 中断, 没保存进度
P1-7 大量 import-* / sync-* 没 file lock (P0-6 详单)
P1-8 sync-yonsuite-purchase/subcontract/materialout 默认拉昨天-今天, 应拉前 3 天
P1-9 import-tmall isLatest 选最新文件可能错过中间日期
P1-10 sync-stock saveDetailSnapshot 死代码, 删
P1-11 sync-yonsuite-purchase 翻页死循环风险, 加 pageIndex>100 兜底
P1-12 sync-summary/fresh/daily-summary 三套 SQL UPSERT 不一致, 抽 helper
P1-13 sync-half-day/sync-trades-v2 trade 三表写入无事务
P1-14 sync-trades-v2 wrapper.ScrollId 只第一页打印
P1-15 sync-batch-stock 仓库列表空时静默退出

---

## P2 (14 项 - 锦上添花)

P2-1 sync-detail orderMap 200 万订单全内存
P2-2 import-pdd 用 xls 库 (10 年没更新)
P2-3 import-tmall 正则保留 .xls 但 excelize 不支持 (跟 memory feedback_no_xls_parse 冲突)
P2-4 import-business-report / sync-allocate 自定义 Database 配置 (重复造轮子, 用 internal/config)
P2-5 sync-yonsuite-* 4 工具 helper 函数复制 5 遍 (200 行 ×4)
P2-6 jackyun.Client.HTTP timeout 120s 太长, 调 60s
P2-7 sync-allocate stdout 大量 emoji (Windows codepage 乱码)
P2-8 importutil.AcquireLock 用文件存在性判断, 不防进程崩溃残留
P2-9 sync-stock-io INSERT 没 ON DUPLICATE KEY UPDATE 兜底
P2-10 sync-half-day/v2/daily-trades fields 字段定义复制三遍
P2-11 import-douyin/pdd 等 dateStr 不验证格式直接 SQL 日期
P2-12 sync-summary-monthly MAX(shop_name) 月内改名取错
P2-13 import-finance/business-report 没文件锁+没幂等
P2-14 sync-yonsuite-* ClearBIServerCache 失败静默 log.Printf

---

## agent 推荐优先动手 3 件

1. P0-2: 6 个 import 工具加 log.Fatalf 错误检查 (改动小, 风险低)
2. P0-1: sync-stock 翻页游标 (直接关系数据准确性)
3. P0-5: sync-stock-io 事务化 (避免空窗)
