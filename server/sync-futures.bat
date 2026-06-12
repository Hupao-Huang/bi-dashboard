@echo off
REM Futures daily-bar sync - runs 17:30 (after day session close 15:00, before night session)
REM Called by schtasks under SYSTEM; must use absolute cd (working dir not controllable)
cd /d C:\Users\Administrator\bi-dashboard\server
.\sync-futures.exe
