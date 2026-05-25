@echo off
REM v1.74.3 拓范 (跑哥 5/25): 每天 03:00 刷未完成单状态
REM 解决 sync-allocate 默认 7 天窗口无法捕捉老单完成 (in_status 1→3) 的状态老化 bug
REM 跑 status!=20 OR in_status!=3 的所有未完成单 (从最早 audit_date 起到今天)
cd /d C:\Users\Administrator\bi-dashboard\server
if not exist logs mkdir logs
.\sync-allocate.exe -refresh-pending > logs\sync-allocate-refresh.log 2>&1
