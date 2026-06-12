@echo off
REM Customer service score + comment daily sync (RPA writes Z: in the afternoon 15:00-17:30,
REM 19:30 fallback import catches same-day files that the 13:00 ops run is too early for; full idempotent scan)
REM IMPORTANT: keep this file pure ASCII with CRLF line endings (cmd misparses UTF-8/LF, exit 255).
cd /d C:\Users\Administrator\bi-dashboard\server

set LOGFILE=C:\Users\Administrator\bi-dashboard\server\sync-service-score.log
echo =============================================== >> %LOGFILE%
echo %date% %time% start sync service score + comment >> %LOGFILE%

.\import-service-score.exe >> %LOGFILE% 2>&1
REM comment files land 14:53-17:24 daily, after the 13:00 ops fallback -> without this line they are T+1 (2026-06-12 incident)
.\import-comment.exe >> %LOGFILE% 2>&1

echo %date% %time% sync service score and comment done >> %LOGFILE%
