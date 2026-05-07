@echo off
REM 每天 02:00 拉吉客云调拨单(默认最近 7 天滚动覆盖,捕在途单状态变更)
cd /d C:\Users\Administrator\bi-dashboard\server
sync-allocate.exe --days=7 >> sync-allocate.log 2>&1
