@echo off
REM 每月7号跑：刷新上月月汇总账（最终确认）
cd /d C:\Users\Administrator\bi-dashboard\server
set REFRESH_LAST_MONTH=1
sync-summary-monthly.exe
