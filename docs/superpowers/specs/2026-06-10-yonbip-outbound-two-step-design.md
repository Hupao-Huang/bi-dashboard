# 用友批量出库 改三步向导（转换 → 复查 → 出库）设计

2026-06-10 跑哥提出。当前出库工具是"一键全部执行"，内部批次转换(Phase1)+出库(Phase2)连着跑。
问题：批次转换是写用友、可能部分失败或有延迟，盲目接着出库会出错单。改成两步人工 gate，中间复查。

## 目标流程

```
粘贴出库行 →【生成/刷新计划】(export-plan, 只查库存不写)
  │
  ├─ 计划里有"批次转换"行(目标批次不够)
  │   →【①执行批次转换】(只转换) → 提示去复查
  │   →【刷新计划】复查 → 系统看新计划还有没有转换行:
  │        有 → 🔴还差货, 再点①  (②出库锁死)
  │        无 → 🟢都到货, 解锁②
  │
  └─ 计划本来没转换行(货够) → 直接解锁②

【②执行出库】(只出库, 复查全绿才点得动)
```

复查 = 重新点【生成/刷新计划】(复用现成 export-plan, 每次查实时库存)。
自动 gate: `plans` 里只要还有 `shipments[].convert_sources`(批次转换行) 就锁住出库按钮。

## 后端改动 (yonbip_outbound.go)

- `ybExecute(ctx, vouchdate, plans, groupByBill, phase)` 加 `phase` 参数:
  - `convert`: 只跑 Phase1 批次转换+审核, 跳过 Phase2
  - `out`: 跳过 Phase1, 只跑 Phase2 出库+审核
  - `all`/空: 旧行为(兼容)
  - `doConvert := phase != "out"` / `doOut := phase != "convert"`, 各包住对应阶段
  - phase=out 时 Phase1 没跑, `skipOut` 全 false → 全部出库(复查已确认够), 正确
- handler `YonbipExportExecute` 读 `req.Phase`(默认 "all"), 传入。清写超时/ctx/防重照旧。

## 前端改动 (YonbipOutbound.tsx)

- `hasConvert = plans?.some(p => p.shipments.some(s => s.convert_sources?.length > 0))`
- 按钮: 【①执行批次转换】disabled=!plans||!hasConvert; 【②执行出库】disabled=!plans||hasConvert
- 两个独立执行函数: doExecuteConvert(phase=convert) / doExecuteOut(phase=out)
- 两个结果区: 转换结果(只渲染 conversions 标签) / 出库结果(只渲染 out 标签), 各管各
- 转换完提示: "转换完成, 请点【刷新计划】复查目标批次是否到货(用友库存刷新可能要等几秒)"
- 复查后: hasConvert=true 红字"还差货,再点①"; false 绿字"都到货,可出库"
- 二次确认弹窗拆成两个: 转换确认 / 出库确认(都标不可逆)

## 边界与防呆

- 库存刷新延迟: 复查由跑哥手动点触发, 自掌时机, 延迟非阻塞; 仅加提示文案。
- 防重: ①②各自 10 分钟防重(conv_out / out 两种 kind 独立), 中间刷新计划是只读查询无副作用。
- 出库用最新一次刷新的 plans(反映转换后真实库存)。
- 一键 all 模式后端保留兼容, 前端不再用。

## 不做(本期外)

- 复查不自动轮询/不自动补转(只锁按钮+提示, 补转靠跑哥再点①)。
- 不做用友库存延迟的自动等待重试。
