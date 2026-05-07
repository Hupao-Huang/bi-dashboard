# Agent 误报核实清单 (Sprint 2 Pre-flight 抓到的)

跑哥批评 lens "agent 误报" + "memory 已记录的 8 条" + 本次新增的 3 条.

执行 Sprint 2 前必做的实际验证, 防止"按 agent 报告盲改"导致改错或白干.

---

## 新增 3 条 (Sprint 2 验证发现)

### 误报 1: op_douyin_live_daily 无 UNIQUE KEY (B-P0-3 + E-P0-3)
**agent 报告**: B 路 + E 路双确认表无 UK, REPLACE INTO 重跑必累加, 是 P0
**实际验证**: 表已有 `UNIQUE KEY uk_live (stat_date, shop_name, anchor_name(50), start_time)`
**重复检测**: 76711 行, 按 UK 4 字段 group by 后 0 个 dup_groups
**结论**: agent 错读 auth.go:671-707 建表代码, 实际生产 DB 早已 ALTER 加 UK. 跟跑哥之前的 op_douyin_ad_material_daily 误报同模式 (建表 SQL 没声明但实际加过)
**动作**: 不修, 加入 memory 误报清单

### 误报 2: op_douyin_anchor_daily UK 缺 account 同名主播覆盖 (B-P0-4 + E-P2-6)
**agent 报告**: UK (stat_date, shop_name, anchor_name) 过窄, 同店铺不同账号同名主播会撞 UK 互相覆盖, 是 P0
**实际验证**:
- account 字段 100% 填充 (1942/1942)
- 同 (stat_date, shop_name, anchor_name) 不同 account 的案例 = **0 个**
**结论**: 理论风险存在, 但实际从未发生. 改 UK 是纯防御性, ROI 低 + 改 UK 期间锁表风险
**动作**: 不改, 留 follow-up. 等真出现同名跨账号才改

### 误报 3: hesi_flow_attachment 加 UK (E-P0-2)
**agent 报告**: 表无 UK, 已发现 1 条重复, 加 UK
**实际验证**: 重复 1 条属实
**阻塞点**: sync-hesi 写入是**裸 INSERT** (3 处, sync-hesi/main.go:435/449/464), 加 UK 后 sync-hesi 会撞 UK 报错挂掉
**结论**: 必须先改 sync-hesi 用 INSERT IGNORE 或 ON DUPLICATE KEY + build + 拷贝 exe + 验证, 才能 ALTER. 跑哥不在线时风险高
**动作**: 跳过, 等明天讨论. 1 条重复影响微小

### 半误报: D-P1-6 RPAMonitor.tsx:424 用 data.error
**agent 报告**: 多文件用 data.error 不是 data.msg, 含 RPAMonitor.tsx:424
**实际验证**: 该行已经是 `json.msg || json.error || '导入请求失败'`, 是好的写法
**结论**: D 路报告这一条不准, RPAMonitor 不需要改

---

## 已知误报清单 (memory 已挂的 8 条)

跑哥之前已确认, agent 仍可能再报, 不要再改:
1. offline_region_target 已有 UK (year, month, region)
2. op_douyin_ad_material_daily 已有 UK (stat_date, shop_name, material_id)
3. trade_package_YYYYMM 已有 UK (trade_id, logistic_no, barcode)
4. sync-daily-trades 504/429 重试 — 已有 5 次 HTTP 重试
5. AuthContext 仅校验 response.ok — 后端 ok 与 body.code 同步
6. 钉钉 OAuth state 简单字符串 — 非高危
7. 前端 pendingToken 明文 — 一次性 nonce 是标准
8. finance_report 缺复合索引 — 现有 idx_dept_year 已覆盖

---

## 教训

跑哥批评 lens 应用:
- **不主动 grep 现有功能就动手**: agent 报"无 UK"前必须 SHOW CREATE TABLE 确认实际现状, 不能只看 .sql 文件
- **范围思维狭窄**: agent 报"加 UK 修 UK 缺失" 时必须查写入工具是否兼容 (REPLACE INTO ✓ / 裸 INSERT ✗)
- **闭环**: 改 DB DDL → 改写入代码 → build → 部署 → 验证. 任何一步缺失 = 上线挂

下次审查启动 agent 时, 在 prompt 加一句: "**报 UK 缺失前必须 SHOW CREATE TABLE 实际验证, 不能只看 sql 目录文件**".
