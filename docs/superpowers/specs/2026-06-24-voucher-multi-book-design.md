# 凭证查询 多账簿合并查询 — 设计文档

- 日期: 2026-06-24
- 模块: 财务 → 凭证查询 (实时透传用友 YS 凭证, 不入库, 仅超管)
- 状态: 待跑哥审阅
- 关联代码: `src/pages/finance/VoucherQuery.tsx` / `server/internal/handler/ys_voucher.go` / `server/internal/yonsuite/voucher.go` / `server/internal/yonsuite/accbook.go` / `server/internal/yonsuite/client.go`

## 1. 背景与目标

凭证查询页(2026-06-16 上线)当前账簿是**单选**:选一本账 → 查一本账的凭证。要看另一本账,得重新选、重新查。

**目标**:账簿改**多选**,一次勾几本(或全选),凭证**合并成一张表**展示,加一列「账簿」标明每条属于哪本账。

**跑哥已拍板的两个口径(brainstorming 确认)**:
1. 结果**合成一张表 + 账簿列**(不按账簿分块)。
2. **默认快 + 手动拉全**:默认每本账只拉前 N 条(快),想看完整点「拉全」。

## 2. 非目标 (YAGNI)

- 不改凭证表的列结构/分录明细展开/金额格式(只加账簿维度)。
- 不入库(凭证查询本就是实时透传用友, 不落库, 保持)。
- 不动权限范围(仍仅超管)。
- 不按账簿分块/分页签展示(已否决, 选合并表)。
- 不做跨账簿的合计/对账汇总(本期只做"看", 不做"算")。
- 不改用友 YS client 的限流策略(1.1s/次保留, 见 §3 约束)。

## 3. 现状 / 关键约束(已查证)

### 3.1 现状(单账簿)

| 层 | 现状 | 证据 |
|----|------|------|
| 前端账簿框 | 单选 `<Select>`, `accbookCode: string`, 默认"浙江松鲜鲜自然调味品"或第一本 | `VoucherQuery.tsx:56,77-78,173-181` |
| 前端翻页 | **服务端翻页**: 翻页 onChange 重新打后端, 后端透传用友 pager | `VoucherQuery.tsx:238-246` |
| 后端入参 | `body.AccbookCode string`(单), 空则 400「请选择账簿」 | `ys_voucher.go:93,106-109` |
| 后端查询 | 1 本账 → 1 次 `QueryVoucherList` → 抽平 header+body 成 `voucherRow` | `ys_voucher.go:130-177` |
| YS client | `VoucherListReq.AccbookCode string` **必填且只能一个**, 空则报错 | `voucher.go:19,44` |
| 账簿清单 | `QueryAccbookList()` 拉全部账簿(code+name), 接口缓存 24h | `accbook.go:26` / `ys_voucher.go:22-49` |

### 3.2 硬约束:用友接口单账簿 + 全局限流(决定方案的根因)

- **用友 `queryVouchers` 接口一次只认一本账**(`accbookCode` 单值, 无"多账簿"参数)。→ 多账簿**只能后台逐本查再合并**, 没有"一次查多本"的捷径。
- **YS client 全局限流 1.1s/次**(`client.go:54` `defaultMinInterval=1100ms`, `client.go:87-99` `waitRateLimit`, 在 `postJSON`(`client.go:198`)和 token 刷新入口都调)。**单一共享限流器**(`rateMu`/`lastCallTime`), 并发也会被串行到 1.1s/次。
- **这条限流通道与所有 YS 同步共用**(采购/委外/材料出库/现存量/质检 同步都走同一个 `*Client`)。→ 一次"全量拉很多账簿"会占用通道, 拖慢/排队后台同步。
- **结论**:多账簿成本 = 调用次数 × 1.1s, 且省不掉。设计必须**最小化调用次数**, 并对"拉全"设兜底闸。

## 4. 设计

### 4.1 两种模式

| 模式 | 行为 | 调用次数 | 用途 |
|------|------|---------|------|
| **快(默认)** | 每本勾选账簿只打 1 次用友(拿前 `perBookLimit=20` 条), 合并 | = 账簿数 | 日常速览 |
| **拉全(手动)** | 每本账循环翻页(每页 200)拉到底, 合并 | = Σ各账簿页数 | 要看完整 |

- 快模式成本: 选 3 本≈3.3s, 选 14 本≈15s(用友限流硬地板, 无法再降)。
- 拉全设**兜底闸**:`maxCalls=30` 或 `maxRows=6000`, 先到先停 → 返回已拉到的 + `truncated=true` + 提示「结果太多已截断, 请缩小账期/账簿」。防一脚踩穿堵死后台同步。

### 4.2 后端流程(`ys_voucher.go` 的 `GetVoucherList` 改造)

```
入参(POST json):
  accbookCodes []string   // 多选, 空则 400「请选择账簿」(替换原 accbookCode 单值)
  periodStart / periodEnd / voucherStatus / billcodeMin / billcodeMax  // 不变, 对所有账簿统一生效
  full bool               // false=快(默认) / true=拉全

流程:
  accbookName = code→name 映射(复用账簿清单缓存)
  rows = []
  meta = []   // 每本账: {code, name, recordCount, fetched, error?}
  callCount = 0
  for code in accbookCodes:
     if !full:
        resp = QueryVoucherList(code, page1, size=perBookLimit)   // 1 次
        callCount++
        rows += flatten(resp).tag(code, name)
        meta += {code, name, recordCount: resp.RecordCount, fetched: len}
     else:
        pageIndex=1
        loop:
           if callCount >= maxCalls or len(rows) >= maxRows: truncated=true; break
           resp = QueryVoucherList(code, pageIndex, size=200); callCount++
           rows += flatten(resp).tag(code, name)
           if fetched_for_book >= resp.RecordCount: break
           pageIndex++
        meta += {code, name, recordCount, fetched}
     # 单本账查询失败: 记 meta[i].error, continue(不中断其他账簿)
  sort rows by (账簿在勾选中的顺序, period, voucherNo)
  return { list: rows, books: meta, truncated, full }
```

- **抽函数便于单测**:把"逐账簿拉取+合并"抽成 helper, 接收一个 `fetchPage(code, pageIndex, pageSize) (*VoucherListResp, error)` 函数注入, 单测用 mock 注入(避免直连用友)。`h.YS.QueryVoucherList` 作为生产实现传入。
- `voucherRow` 加两字段:`AccbookCode string` / `AccbookName string`(`ys_voucher.go:63-79`)。
- `voucher.go` / `accbook.go` **不动**(单账簿查询 + 账簿清单原样复用)。
- **不再向用友透传 pageIndex/pageSize 做服务端翻页**;翻页移到前端(见 4.3 行为变化)。

### 4.3 前端(`VoucherQuery.tsx`)

- 账簿框 `<Select>` → `mode="multiple"`, `accbookCodes: string[]`, 默认 `["浙江松鲜鲜的 code"]`(保持现状只选一本), `showSearch` + `maxTagCount="responsive"`。可加"全选"快捷。
- 表格**最前面加「账簿」列**(显示账簿名), 其余列不变。
- **翻页改前端本地翻**:`dataSource=rows` 全量交给 antd Table 默认分页(本地), 去掉 onChange 回后端重查(`VoucherQuery.tsx:238-246` 改造)。
- **截断提示条**:任一本账 `recordCount > fetched` → 顶部 antd Alert「账簿X还有N条未显示, 点『拉全』或缩小账期/凭证号」。
- **「拉全」按钮**:在「查询」旁;点了带 `full=true` 重查。多账簿或全量前弹 `Modal.confirm`「可能要等几十秒, 确定?」。拉取中显示 loading 文案「正在逐本账簿拉取…」。
- **单本账失败**:meta 里带 error 的账簿, 用 message.warning 列出「账簿X查询失败」, 其余正常展示。
- 账期/状态/凭证号筛选不变, 对所有勾选账簿统一生效。

### 4.4 错误处理

- `accbookCodes` 为空 → 400「请选择账簿」。
- 单本账用友调用失败 → 不中断, 记入该账簿 meta.error, 前端 warning 提示。
- 全部账簿都失败 → 返回空 list + 各自 error, 前端提示「查询失败, 请确认用友连接」。
- 拉全触顶 `maxCalls`/`maxRows` → `truncated=true` + 提示缩范围。
- 前端 AbortController 保留(重查 abort 上一次, `VoucherQuery.tsx:85-87`)。

## 5. 测试

- **后端单测**(注入 mock `fetchPage`, 不连用友):
  - 2 本账, 快模式各拉前 20 → 合并行数 = 两本 fetched 之和, 每行带对的账簿名。
  - 某本账 recordCount > 20 → meta.recordCount 正确, truncated 标记符合预期。
  - 拉全模式翻页到底(模拟 3 页)→ 全量合并。
  - 触顶 maxCalls/maxRows → truncated=true 且停在闸值。
  - 某本账 fetchPage 返回 error → 该本 meta.error 有值, 其余账簿照常合并。
- **前端**:`tsc` 0 报错 + `npm run build` 过 + playwright 真点(多选两本账 → 看账簿列 → 制造截断看黄条 → 点拉全验完整)。

## 6. 行为变化 / 已知取舍(已与跑哥确认)

- **行为变化**:改成"默认快+拉全"后, 即使**单选一本账**, 想看第 N 条以后也要点「拉全」(不再像现在逐页服务端翻页)。跑哥选的就是这个模型, 已确认接受。
- **快模式选多账簿仍不是秒级**:成本 = 账簿数 × 1.1s(用友限流硬地板)。选十几本约 15s, 已告知。
- **拉全占用 YS 同步通道**:大批量拉全期间可能拖慢后台同步(采购/库存等), 兜底闸(30 次/6000 行)限制最坏情况。

## 7. 默认参数(跑哥已默认通过, 落地前可再调)

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `perBookLimit` | 20 | 快模式每本账拉前几条(对齐现有默认页大小) |
| `maxCalls` | 30 | 拉全总调用上限(≈33s 封顶) |
| `maxRows` | 6000 | 拉全总行数上限 |
| 账簿列位置 | 最前 | 表格第一列 |
| 默认勾选 | 浙江松鲜鲜 1 本 | 保持现状习惯 |
| 权限 | 仅超管 | 不变 |
