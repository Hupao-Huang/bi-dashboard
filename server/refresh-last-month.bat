@echo off
REM Runs on day 7 monthly: refresh last-month summary (final confirmation)
cd /d C:\Users\Administrator\bi-dashboard\server
set REFRESH_LAST_MONTH=1
sync-summary-monthly.exe
