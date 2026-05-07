# A 路: 后端 handler 安全审查 (2026-05-08)

agent: a883457b52f1e5535
基线: cdb60b1 (v0.83)

## 总评
- **没有 SQL 注入** (所有动态拼接都用 placeholder + args)
- **没有 RCE / 鉴权绕过 P0**
- 主要风险是**信息泄露** (err.Error() 直暴露) 和 **1 个权限漏配**

---

## P0: 无

---

## P1 (中危, 应修)

### P1-1 special_channel 路由漏配 page 权限 ⭐⭐⭐
- 文件: server/cmd/server/main.go:261-262
- 问题: 任何登录用户都能查 京东/猫超/朴朴 销售汇总+单据明细 (含金额)
- 修复: 加 pageProtected("finance.report:view", ...) 或新建 special_channel:view

### P1-2 大量 err.Error() 直接返回前端 ⭐⭐
- 文件: feedback.go:120,163,263 / finance_report.go:617,679+ / notice.go / profile.go / hesi.go / supply_chain.go:136 / warehouse_flow.go / task_monitor.go / admin.go / auth.go:1240 等 30+ 处
- 问题: DB 表名/字段名/SQL/Windows 文件路径暴露给前端
- 修复: 沿用 writeDatabaseError, generic message + 内部 log

### P1-3 auth.go:1758 dtID[:12] 越界 panic 风险
- 文件: server/internal/handler/auth.go:1758
- 问题: 钉钉返回短字符串 < 12 时 panic (Go http server 会 recover, 但是观测面)
- 修复: dtID[:min(12, len(dtID))]

### P1-4 钉钉手机号自动绑定无老账号确认
- 文件: server/internal/handler/auth.go:1858-1873
- 问题: 钉钉登录用 mobile 匹配 users.phone 自动绑定+登录, 不需要老账号密码确认
- 风险: 钉钉账号被改/离职号留存场景下可接管同手机号 BI 账号
- 修复: 加老账号密码确认; 或归为设计选择(跑哥拍板)

### P1-5 task_monitor.go:614-644 import-ops webhook 自调用漏 secret
- 文件: server/internal/handler/task_monitor.go:623
- 问题: 主程序调自己 /api/webhook/sync-ops 没带 X-Webhook-Secret → 必返 403
- 不算安全, 是功能 bug, 说明这条路径未经测试
- 修复: 改成直接调 sync.go runSync() 绕过 webhook 自调用

### P1-6 notice.go/feedback.go 用户输入入库不 HTML escape
- 文件: notice.go:113 / feedback.go:113-118
- 问题: 取决于前端是否用 dangerouslySetInnerHTML/markdown 渲染
- 修复: 入库前 escape 或确认前端不渲染 HTML (D 路前端审查会确认)

---

## P2 (低危)

P2-1 feedback.go:75-80 / profile.go:117-122 文件上传只校验后缀不校验 MIME 魔数
P2-2 auth.go:1769 log.Printf 完整手机号 (PII)
P2-3 sync.go:96 / task_monitor.go:332 date 参数只校验 len(8) 不校验数字
P2-4 admin.go:1457-1463 批量导入 bcrypt 共用同一个 hash
P2-5 stock.go:206 / channel.go:41 / hesi.go:85 / warehouse_flow.go:195 LIKE 查询不 escape %/_
P2-6 sync.go hmac.Equal 是常时间比较, 不是 HMAC 校验 (变量名误导, 跑哥已搁置 webhook 防重放)
P2-7 sync.go 文件上传 ParseMultipartForm 内存限制 vs MaxBytesReader 边界

---

## 误报澄清 (跑哥已知/不修)

1. finance_report.go fmt.Sprintf 拼 SQL — placeholder 串生成器, args 占位, 不是注入
2. audit.go/hesi.go/admin.go/warehouse_flow.go fmt.Sprintf 拼 SQL — 同上
3. profile.go/notice.go/auth.go strings.Join 拼 SET — 列名硬编码不可注入
4. warehouse_flow.go ym 已 time.Parse 校验
5. supply_chain.go 1372-1373 purchaseStart 来自 MIN(vouchdate) 非用户输入
6. sync.go hmac.Equal 等价 subtle.ConstantTimeCompare, 不是 SQL 注入或鉴权绕过

---

## agent 推荐 5 条优先 (ROI 排序)

1. P1-1 special_channel 路由权限 (1 行改动)
2. P1-2 err.Error() 集中清理 (改动量大但 ROI 高)
3. P1-3 dtID[:12] 长度校验 (1 行)
4. P1-5 task_monitor webhook 自调用 (功能 bug)
5. P2-1 上传 MIME 校验 + nosniff
