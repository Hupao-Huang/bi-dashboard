# D 路: 前端代码审查 (2026-05-08)

agent: aeec0ed305a29b44f
基线: cdb60b1 (v0.83)
范围: src/ 全部 .tsx/.ts (35+ 业务页 + 20+ 组件)

## 总评
**3 个 P0 是真问题, 大部分 P2 是锦上添花**. 跑哥 lens "基础 UI 问题没自检" 在 placeholder 处理上**已修干净** (大量 `value={x || undefined}` 已铺开, 跟 v0.80.1 同模式).

---

## P0 (必须立刻修)

### P0-1 ⭐⭐⭐ SpecialChannelAllot 暴露 exe 文件名给业务用户
- 文件: src/pages/ecommerce/SpecialChannelAllot.tsx:229
- 问题: Alert description 写 "价格体系.xlsx 漏维护这些 SKU, 跑哥补完后跑 import-channel-price.exe + sync-allocate.exe 即可"
- **违反 feedback_tooltip_no_table_name 红线**
- 修复: 改业务话术 "价格表里漏配了这些商品, 请联系数据组补上后会自动同步"

### P0-2 ⭐⭐ BusinessReport Tooltip 暴露 CLI
- 文件: src/pages/finance/BusinessReport.tsx:139
- 问题: Tooltip "财务自助上传业务预决算 xlsx — 下个版本上线 (当前由数据团队 CLI 导入)"
- 修复: 改成 "下个版本支持自助上传, 目前请联系数据组导入"

### P0-3 ⭐⭐ ChannelManagement 保存/同步成功判定写错
- 文件: src/pages/system/ChannelManagement.tsx:72-78, 94-99
- 问题: `if (res.data?.message || res.message)` 当成功判定. 后端 `{code:200, data:{}}` 无 message 时**永远显示失败**, 但其实保存成功了
- 修复: 改成 `if (res.code === 200)`

---

## P1 (重要, 尽快修)

### P1-1 AIToolboxDrawer target="_blank" 缺 rel="noopener noreferrer"
- 文件: src/components/AIToolboxDrawer.tsx:394, 479
- 修复: antd Button 不支持 rel, 改成 `<a>` 或 onClick + window.open(url, '_blank', 'noopener,noreferrer')

### P1-2 Notices 删除/切换无错误提示 (静默失败)
- 文件: src/pages/system/Notices.tsx:85-94, 96-104, 106-114
- 修复: 每个加 try/catch + fail message.error

### P1-3 Profile 解绑钉钉静默失败
- 文件: src/pages/system/Profile.tsx:134-148
- 修复: 补 else 分支 message.error

### P1-4 AuditLog 翻页 useEffect 依赖不全 (eslint warning)
- 文件: src/pages/system/AuditLog.tsx:55-57

### P1-5 AuditLog 后端响应取值不一致 (json.list 而不是 json.data.list)
- 文件: src/pages/system/AuditLog.tsx:45-46
- 修复: 跟后端协议 {code, data} 对齐

### P1-6 ⭐ 多处用 data.error 但后端是 data.msg, 永远拿不到具体报错
- 文件: Profile.tsx:47/67/234, Notices.tsx:80, ChannelManagement.tsx:77/98, RPAMonitor.tsx:424
- 问题: 用 `data.error || '保存失败'`, 但后端协议是 `{code, msg, data}`, 没有 error 字段 → 永远兜底文案
- 修复: 统一改 `data.msg || data.error || '保存失败'` (msg 优先, error 兼容旧)

### P1-7 customer/Overview Tooltip 写 "RPA 采集"
- 文件: src/pages/customer/Overview.tsx:13
- 修复: 改 "数据从生意参谋采集回来, 通常会延迟 3 天左右", 把 RPA 字眼拿掉

---

## P2 (28 个, 锦上添花)

精选 10 个值得做的:

P2-1 Notices catch {} 完全吞异常
P2-3 多处 fetch 没 catch, 网络故障静默
P2-5 ProductDashboard 商品表无 defaultSortOrder='descend'
P2-8 多页面 rowKey={i} 用 index, attachments 数组场景会 reconcile 错
P2-10 ⚠️ PurchasePlan v0.74 留下来的 console.log 调试日志没清 (生产可见)
P2-12 SpecialChannelAllot 用裸 `<a onClick>` 没 href, a11y 警告
P2-15 Login/Profile 钉钉 url 没白名单校验直接 location.href = (理论 javascript: 风险)
P2-19 BusinessReport useEffect fetch 没 abort, unmount 后 setState 警告
P2-24 ErrorBoundary 直接显示 error.message 给用户 (生产 minify 后无意义)
P2-27 多页面 if (!data) return "加载失败" — 没重试按钮

完整 28 项见 agent 原报告.

---

## 跑哥 lens 验证

### "基础 UI 问题没自检" — ✅ 已修干净
placeholder 处理大量 `value={x || undefined}` 已铺开, 跟 v0.80.1 同模式. **没有发现 placeholder 不显示的隐患**.

### "范围思维狭窄" — ⚠️ 1 处
- P1-6 res.error vs res.msg 协议不一致**散落 6 个文件**, 是范围思维狭窄的具象 (一处错误, 跨多处复制)

---

## agent 推荐 Top 3

1. **P0-1 SpecialChannelAllot** Alert 暴露 exe 文件名 (违反红线)
2. **P0-3 ChannelManagement** 成功判定永远失败 (功能 bug)
3. **P1-6** res.msg vs res.error 6 处统一 (范围思维狭窄)
