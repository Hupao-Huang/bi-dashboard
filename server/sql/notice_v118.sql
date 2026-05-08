-- v1.18 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v1.18 定时任务失败 钉钉告警',
'本次完成内容:

【定时任务失败 自动钉钉告警】
跑哥反馈："定时任务失败给我发通知吧"。

之前定时任务失败要主动打开运维监控页才能看到, 没人盯就会像 5 月 3 日
日志轮转那样, 一周才被发现。本次后端起一个常驻巡检 goroutine, 自动
监控所有 BI-* 任务, 发现失败/卡死立刻推送钉钉给 admin。

巡检规则:
- 频率: 每 30 分钟一次 (启动后 2 分钟首次)
- 数据源: 实时调 PowerShell Get-ScheduledTask 拉所有 BI-* 任务
- 状态判断: 退出码非 0 = 失败 / Running 超 1 小时 = 卡死
- 去重: 用 lastRun + status + output 做指纹, 同一个失败不重复推
- 状态恢复正常 (变绿) 后下次再失败仍会推, 不会哑火

推送内容示例:
"【BI 看板·定时任务异常告警】
任务: 合思费控同步
状态: 失败
上次运行: 2026-05-08 10:30:01
schtasks 退出码 = 4294967295
打开运维监控查看详情 → /system/ops"

多个任务同时失败时合并成一条消息, 不会刷屏。

【启用条件】
依赖 v1.14 钉钉 Notifier (chatbotToOne 推送), 凭证未配置时巡检自动
跳过, 不影响其他功能。

【联调验证】
启动后立刻收到 1 条告警 (BI-SyncHesi 显示 fail, 因为 v1.17 时被
强制 kill, 退出码 4294967295 还在 schtasks 里)。这条告警证明链路完整
打通: schtasks → 巡检发现 → 钉钉推 → 跑哥手机。

不影响数据口径与定时任务运行',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS '时间'
FROM notices WHERE is_pinned=1;
