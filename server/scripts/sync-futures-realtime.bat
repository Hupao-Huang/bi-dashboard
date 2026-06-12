@echo off
REM Futures intraday realtime snapshot sync - runs every 5 minutes
REM Tool has built-in trading-hours gate (auto-exits on weekends/nights), off-hours runs are no-ops
REM Called by schtasks under SYSTEM; absolute cd, exe needs .\ prefix
cd /d C:\Users\Administrator\bi-dashboard\server
.\sync-futures-realtime.exe
