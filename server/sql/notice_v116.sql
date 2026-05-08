-- v1.16 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v1.16 运维监控 重写 - 全量实时读 schtasks',
'本次完成内容:

【运维监控页面 重大改造】
之前运维监控页只显示 6 个任务, 但实际后台有 19 个 BI 定时任务在跑,
13 个任务永远 "看不见", 失败也不报警(状态判断只看 log mtime, 不读真
实退出码)。

本次后端重写 GetTaskStatus 接口:
- 不再硬编码任务列表, 改成实时调用 PowerShell Get-ScheduledTask
- 拉到全部 BI-* 任务 (19 个) + API 服务端口探测 = 20 项
- 状态来源 = schtasks 真实 LastTaskResult 退出码 (0=成功, 其他=失败)
- 卡死检测: State=Running 但 LastRunTime > 1 小时前 → 标 "stuck"

【收益对比】
改前 (v1.15 之前):
- 任务总数 6 / 运行中 1 / 成功 3 / 失败 0
- 实际有 13 个任务被遗漏在外
- 失败任务因为 log 文件还在就被误判 "成功"

改后 (v1.16):
- 任务总数 20 / 运行中 1 / 成功 15 / 失败 2 / 等待 2
- 凡是 schtasks 里 BI-* 开头的全自动出现
- 真实失败暴露: 日志轮转(5/3) + 合思费控(刚才卡死后被 kill)
- 等待任务暴露: 商品资料同步还没首次跑

【新增能力】
- 自动反映新 schtasks: 加任务不用改 BI 看板代码, 自动出现
- 失败状态实时: 任务 fail 立刻在 UI "失败" 卡片+1
- 卡死预警: 任务挂 1+ 小时自动标 "stuck", 提醒去 kill

【今天顺手修的真问题】
排查时发现并已修复:
- BI-Build-WarehouseFlowSummary 物化表构建持续 fail (Win11 22H2 + RunLevel 坑)
- BI-SyncOpsFallback 运营数据导入 fail (同上)
- BI-SyncHesi 合思费控同步卡死 13 小时 (vbs 脚本问题)
- 物化表 5 月数据已自动补齐 (3483 行)

不影响数据口径与定时任务运行',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS '时间'
FROM notices WHERE is_pinned=1;
