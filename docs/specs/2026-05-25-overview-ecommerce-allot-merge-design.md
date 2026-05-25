# 综合看板·电商部门 KPI 合并调拨金额 (v1.74.3)

**Status**: Draft for review
**Date**: 2026-05-25
**Author**: 跑哥 (via Claude brainstorming)
**Scope**: 单轮迭代, 业务反馈后决定下一轮联动范围

---

## 1. 背景

### 1.1 业务问题

BI 看板综合看板 (`/overview`) 显示各部门销售额. 电商部门下两个特殊渠道:

- **ds-京东-清心湖自营** (channel_id `1819610592561398400`)
- **ds-天猫超市-寄售** (channel_id `1819610591915475584`)

业务上**不按销售单算销售额**, 而是**按调拨入库时间统计** (跑哥 v0.62 拍板, 2026-05-08 上线 `/ecommerce/special-channel-allot` 对账页).

### 1.2 现状 gap

- `dashboard_overview.go:27` 的 SQL 仍从 `sales_goods_summary` 聚合, 包含了这 2 渠道的销售单口径数据
- 调拨金额 (在 `allocate_orders` + `allocate_details`) **未合并进综合看板 KPI**
- 代码 line 535-541 加了一个 "不含调拨" Tooltip 提示, 但**没真合并数据**
- 业务读综合看板拿到的电商部金额, 跟实际口径偏低 (4 月起样本: 销售单 ¥513 万 vs 调拨 ¥732 万, 差 +¥219 万 / +42%)

### 1.3 触发

跑哥 2026-05-25 在 Beta 实测中提出: "综合看板, 电商部门的金额, 是不是没有计算这 2 渠道调拨的金额, 能不能再 KPI 卡片里面增加调拨金额/销售金额/相加的金额."

---

## 2. 目标

### 2.1 数据口径

综合看板电商部门 KPI 改为:

```
电商部门金额 = (sales_goods_summary 销售单口径, 排除 2 调拨渠道)
             + (allocate_details.excel_amount 调拨口径, 限这 2 调拨渠道)
```

### 2.2 UI (跑哥定方案 C)

电商部 mini 卡 (`/overview` 下方部门卡片) 改为:

```
┌────────────────────────┐
│ 🛒 电商部门            │
│  ¥7,313.20 万         │  ← 主字 (总和)
│  销售额 ¥6,581.40 万  │  ← 排除 2 调拨渠道
│  调拨额 ¥  731.80 万  │  ← 这 2 调拨渠道
│  总额   ¥7,313.20 万  │  ← 销售 + 调拨
│  ─ 货品 / 客单价      │
└────────────────────────┘
```

顶部 "总销售额" 主卡 + 右上角 "电商部门" tag 自动跟着新口径 (因为 totalSales = SUM(各部门 sales)).

### 2.3 联动决策 (本轮不动)

明确**不在本轮范围**:
- 即时零售部 / 朴朴渠道 (同性质但跑哥决定本轮不动)
- /ecommerce 部门主页
- 店铺排行 / 商品排行 / 趋势图 / 趋势对比
- sales_goods_summary 数据表 (这 2 渠道历史数据保留)
- allocate_orders 数据表
- /ecommerce/special-channel-allot 对账页

本轮上线后, 业务反馈再决定下一轮联动范围.

---

## 3. 设计

### 3.1 后端: `server/internal/handler/dashboard_overview.go`

#### 新增 helper

```go
// loadEcommerceAllotAdjustment 加载电商部 2 调拨渠道的双口径金额
// 返回 (salesExcluded float64, allotAmt float64, err error)
//   salesExcluded: 这 2 渠道在 sales_goods_summary 的销售单口径 SUM (要从 dept.sales 减掉)
//   allotAmt: 这 2 渠道在 allocate_details 的 excel_amount SUM (要加到 dept.sales)
func (h *DashboardHandler) loadEcommerceAllotAdjustment(
    ctx context.Context,
    start, end string,
    scopeCond string, scopeArgs []interface{},
) (salesExcluded, allotAmt float64, err error) {
    // 2 个固定渠道 ID (硬编码, 跟 special_channel.go 一致)
    const jdShopID = "1819610592561398400"   // ds-京东-清心湖自营
    const tmcsShopID = "1819610591915475584" // ds-天猫超市-寄售

    // query 1: 这 2 渠道的销售单口径 (要从 dept.sales 减)
    salesArgs := append([]interface{}{start, end, jdShopID, tmcsShopID}, scopeArgs...)
    err = h.DB.QueryRowContext(ctx, `
        SELECT IFNULL(SUM(IFNULL(local_goods_amt, goods_amt)), 0)
        FROM sales_goods_summary
        WHERE stat_date BETWEEN ? AND ?
          AND shop_id IN (?, ?)
        ` + scopeCond, salesArgs...).Scan(&salesExcluded)
    if err != nil {
        return 0, 0, fmt.Errorf("查 2 渠道销售单口径失败: %w", err)
    }

    // query 2: 这 2 渠道的调拨口径 (要加进 dept.sales)
    // channel_key 在 allocate_orders 是 '京东' / '猫超' (special_channel.go 一致)
    allotArgs := []interface{}{start, end}
    err = h.DB.QueryRowContext(ctx, `
        SELECT IFNULL(SUM(d.excel_amount), 0)
        FROM allocate_orders o
        JOIN allocate_details d ON d.allocate_no = o.allocate_no
        WHERE o.stat_date BETWEEN ? AND ?
          AND o.channel_key IN ('京东', '猫超')
    `, allotArgs...).Scan(&allotAmt)
    if err != nil {
        return 0, 0, fmt.Errorf("查 2 渠道调拨口径失败: %w", err)
    }

    return salesExcluded, allotAmt, nil
}
```

#### GetOverview 改造点

```go
// 现状 deptList 取到后, 在 line 84 之前插入:

// v1.74.3: 电商部门 KPI 合并调拨金额 (排除 2 调拨渠道销售单 + 加调拨)
salesExcluded, allotAmt, allotErr := h.loadEcommerceAllotAdjustment(
    r.Context(), start, end, scopeCond, scopeArgs)
if allotErr != nil {
    // 兜底: 不阻塞主流程, 用原 dept.sales (回落到改前行为, 多个 log)
    log.Printf("[overview] 调拨口径加载失败, 用原口径: %v", allotErr)
} else {
    for i, d := range deptList {
        if d.Department != "ecommerce" {
            continue
        }
        salesAmt := d.Sales - salesExcluded  // 其它电商渠道销售单
        if salesAmt < 0 {
            salesAmt = 0  // 容错 (理论不应负)
        }
        deptList[i].SalesAmt = salesAmt
        deptList[i].AllotAmt = allotAmt
        deptList[i].Sales = salesAmt + allotAmt  // 主字 = 新总和, totalSales 自动跟着
        break
    }
}
```

#### DeptSummary 加 2 字段

```go
type DeptSummary struct {
    Department string  `json:"department"`
    Sales      float64 `json:"sales"`
    Qty        float64 `json:"qty"`
    Profit     float64 `json:"profit"`
    Cost       float64 `json:"cost"`
    SkuCount   int     `json:"skuCount"`
    SalesAmt   float64 `json:"salesAmt,omitempty"` // v1.74.3 新增: 排除 2 调拨渠道的销售口径
    AllotAmt   float64 `json:"allotAmt,omitempty"` // v1.74.3 新增: 这 2 调拨渠道的调拨口径
}
```

### 3.2 前端: `src/pages/overview/index.tsx`

#### 改动 1: 删 line 535-541 "不含调拨" Tooltip

业务已对齐, 不再需要提示.

#### 改动 2: 电商部 mini 卡渲染 (line 543-558)

```tsx
{/* 检测 allotAmt > 0 才显示 3 数据拆解 (兼容其它部门) */}
{(dept as any).allotAmt > 0 ? (
  <>
    <div style={{ fontSize: 24, fontWeight: 700, color: '#1e293b', fontVariantNumeric: 'tabular-nums' }}>
      ¥{dept.sales?.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
    </div>
    {formatWanHint(dept.sales || 0) && (
      <div style={{ fontSize: 13, color: '#64748b', marginTop: 2, fontVariantNumeric: 'tabular-nums', fontWeight: 400 }}>
        {formatWanHint(dept.sales || 0).replace('约', '≈ ')}
      </div>
    )}
    <div style={{ marginTop: 8, display: 'flex', flexDirection: 'column', gap: 2 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12, color: '#64748b' }}>
        <span>销售额</span>
        <span style={{ fontVariantNumeric: 'tabular-nums' }}>¥{((dept as any).salesAmt / 10000).toFixed(2)} 万</span>
      </div>
      <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12, color: '#64748b' }}>
        <span>调拨额</span>
        <span style={{ fontVariantNumeric: 'tabular-nums' }}>¥{((dept as any).allotAmt / 10000).toFixed(2)} 万</span>
      </div>
      <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12, color: '#64748b' }}>
        <span>总额</span>
        <span style={{ fontVariantNumeric: 'tabular-nums', fontWeight: 600 }}>¥{(dept.sales / 10000).toFixed(2)} 万</span>
      </div>
    </div>
  </>
) : (
  /* 原渲染保留, 给其它部门用 */
  <>... 原 line 543-558 ...</>
)}
```

### 3.3 错误处理

| 场景 | 行为 |
|---|---|
| helper query 1 失败 | log warn, salesExcluded=0, allotAmt=0, 走兜底 (dept.sales 不变) |
| helper query 2 失败 | 同上 |
| 主流程 SQL 失败 | 原有 writeServerError, 不变 |
| 2 渠道在 sales_goods_summary 无数据 (=0) | salesExcluded=0, dept.SalesAmt = dept.Sales (符合预期) |
| 2 渠道在 allocate 无数据 (=0) | allotAmt=0, dept.AllotAmt=0, 前端 fallback 回原渲染 (不显示拆解) |

### 3.4 测试

#### 单元测试 (sqlmock)

新增文件 `server/internal/handler/dashboard_overview_test.go` (或加入 misc_handlers_test.go):

- `TestGetOverviewWithAllotAdjustment_HappyPath`: mock 2 渠道销售 ¥513 万 + 调拨 ¥732 万, 验证 dept.sales 从 ¥7094 → ¥7313 万 + salesAmt + allotAmt 返回正确
- `TestGetOverviewWithAllotAdjustment_HelperError`: mock query 1 失败, 验证主流程不挂 + dept.sales 不变 + log warn
- `TestGetOverviewWithAllotAdjustment_ZeroAllot`: mock 调拨 0, 验证 allotAmt=0 + salesAmt=dept.sales

#### 实测

跑哥点 `/overview`:
- 电商部 mini 卡显示主字 + 3 行拆解 (销售/调拨/总额对得上)
- 顶部"总销售额"卡数字跟改前比 +¥219 万 (4 月起样本)
- 右上角"电商部门"tag 数字跟 mini 卡主字一致

---

## 4. 影响 / 兼容

### 4.1 数据库

不动:
- `sales_goods_summary` (历史数据保留)
- `allocate_orders` / `allocate_details` (只读)

### 4.2 API 兼容

`/api/overview` 响应增加 2 个 optional 字段 (`salesAmt`, `allotAmt`), 默认 omitempty. 旧前端代码不受影响.

### 4.3 缓存

`buildOverviewCacheKey` 不需要改 (新口径直接生效, 没切换开关). 现有 cache 自动失效后下次重算用新口径.

### 4.4 影响估算

| 指标 | 估算 |
|---|---|
| 后端代码改动 | +60 行 (helper 30 + GetOverview 修改 10 + DeptSummary 字段 2 + 单测 20) |
| 前端代码改动 | +25 行 (mini 卡拆解逻辑 + 删 Tooltip) |
| 测试覆盖 | 3 个新单元测试 |
| 部署影响 | 综合看板 KPI 数字变化, 业务感知明显 (¥7094 → ¥7313 万 量级) |
| 风险 | 低: helper 失败兜底回原口径, 不影响主流程 |

### 4.5 发版

- 版本号: **v1.74.3** (PATCH, hotfix 性质: 业务正在用的功能口径不准)
- 用户感知: 综合看板电商部金额变化, 需 notice 告知财务/管理层

---

## 5. 不在本轮范围 (业务反馈后决定下轮)

- 朴朴 / 即时零售部门 KPI 同样调整
- /ecommerce 部门主页同步
- 店铺排行 / 商品排行 / 趋势图 / 趋势对比 排除这 2 渠道销售单口径
- sales_goods_summary 这 2 渠道历史数据是否归档

---

## 6. 关键 commit (待 v1.74.3 发版)

```
fix(overview): 电商部门 KPI 合并 2 调拨渠道金额 (口径修正)
- 新增 helper loadEcommerceAllotAdjustment 查 2 渠道双口径
- GetOverview 替换 dept[ecommerce].sales 为 (其它销售 + 调拨)
- DeptSummary 加 SalesAmt/AllotAmt 字段供前端拆解
- 前端 mini 卡显示主字 + 3 数据拆解 (销售/调拨/总额)
- 删除 line 535-541 "不含调拨" Tooltip
- 3 个 sqlmock 单测 + 跑哥实测
```
