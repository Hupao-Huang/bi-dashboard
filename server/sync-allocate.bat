@echo off
REM Daily 02:00 pull Jackyun allot orders (rolling 7-day window to catch in-transit status changes)
cd /d C:\Users\Administrator\bi-dashboard\server
sync-allocate.exe --days=7 >> sync-allocate.log 2>&1
