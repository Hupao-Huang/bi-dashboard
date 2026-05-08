-- v1.17 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v1.17 修两个真定时任务问题 - 日志轮转 + 合思费控',
'本次完成内容:

【日志轮转 BI-RotateLogs 修复】
之前 5 月 3 日开始持续失败 (退出码 255), 一周没人发现, v1.16 运维监控
重写后才暴露出来。

真因: bat 脚本里的中文 REM 注释是 UTF-8 编码, Windows cmd 默认 GBK
读时把中文识别成命令导致解析失败 ("''.log'' is not recognized as an
internal or external command")。还有 forfiles 命令在没找到匹配文件
时会返回非 0 退出码, 进一步触发 schtasks 标失败。

修复:
- bat 改成纯 ANSI / 英文注释 (告别 UTF-8 中文乱码)
- forfiles 全部 >nul 2>&1 抑制错误输出
- bat 末尾加 exit /b 0 兜底, forfiles 空匹配不再让任务标失败
- 手动测试 + schtasks 触发, LastResult=0 全绿

【合思费控同步 BI-SyncHesi 加超时保护】
之前合思 API 卡死会让 sync-hesi 卡 13 小时不退出, vbs 同步等待无法回收,
schtasks 一直显示 Running, 影响下次定时调度, 也让运维监控页 "运行中"
始终非零, 误导排查。

修复:
- sync-hesi/main.go 入口加整体超时保护 goroutine
- 30 分钟未结束 → log.Fatalf 强制退出 (退出码 1)
- 下次 schtasks 触发会重新跑, 不会再被卡死的实例堵住
- 触发场景: 合思 API 长时间无响应 / 死循环 / 数据库连接池耗尽

【现状汇总 (本次修完后)】
今天定时任务从 19 个的 3 红点 → 0 红点全绿:
- BI-Build-WarehouseFlowSummary 已修 (v1.16 时)
- BI-SyncOpsFallback 已修 (v1.16 时)
- BI-SyncHesi 卡死进程已 kill, 增加超时保护 (本版)
- BI-RotateLogs 修复 bat 编码与 forfiles 错误码 (本版)

不影响数据口径与定时任务运行',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS '时间'
FROM notices WHERE is_pinned=1;
