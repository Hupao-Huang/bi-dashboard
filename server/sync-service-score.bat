@echo off
REM Customer service score daily sync (RPA writes Z: in the afternoon, 19:30 fallback import; full idempotent scan)
cd /d C:\Users\Administrator\bi-dashboard\server

set LOGFILE=C:\Users\Administrator\bi-dashboard\server\sync-service-score.log
echo =============================================== >> %LOGFILE%
echo %date% %time% start sync service score >> %LOGFILE%

.\import-service-score.exe >> %LOGFILE% 2>&1

echo %date% %time% sync service score done >> %LOGFILE%
