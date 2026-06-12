@echo off
REM Daily ops data sync (RPA fallback after RPA collects, fires at 13:00 if not already imported)
REM IMPORTANT: keep this file pure ASCII with CRLF line endings.
REM cmd.exe (codepage 936) misparses UTF-8 Chinese comments + LF-only endings -> instant exit 255 (2026-06-12 incident).
cd /d C:\Users\Administrator\bi-dashboard\server

for /f %%i in ('powershell -Command "(Get-Date).AddDays(-1).ToString('yyyyMMdd')"') do set YESTERDAY=%%i

set LOGFILE=C:\Users\Administrator\bi-dashboard\server\sync-ops-daily.log
echo =============================================== >> %LOGFILE%
echo %date% %time% start sync ops data for %YESTERDAY% >> %LOGFILE%

.\import-tmall.exe %YESTERDAY% %YESTERDAY% >> %LOGFILE% 2>&1
.\import-pdd.exe %YESTERDAY% %YESTERDAY% >> %LOGFILE% 2>&1
.\import-jd.exe %YESTERDAY% %YESTERDAY% >> %LOGFILE% 2>&1
.\import-promo.exe %YESTERDAY% %YESTERDAY% >> %LOGFILE% 2>&1
.\import-vip.exe %YESTERDAY% %YESTERDAY% >> %LOGFILE% 2>&1
.\import-tmallcs.exe %YESTERDAY% %YESTERDAY% >> %LOGFILE% 2>&1
.\import-douyin.exe %YESTERDAY% %YESTERDAY% >> %LOGFILE% 2>&1
.\import-douyin-dist.exe %YESTERDAY% %YESTERDAY% >> %LOGFILE% 2>&1
.\import-customer.exe %YESTERDAY% %YESTERDAY% >> %LOGFILE% 2>&1
.\import-feigua.exe %YESTERDAY% %YESTERDAY% >> %LOGFILE% 2>&1
REM comment data: full idempotent scan (files may arrive late, daily full scan is safest; small files, seconds)
.\import-comment.exe >> %LOGFILE% 2>&1
REM service score: full idempotent scan (daily file covers whole month, full scan survives month rollover; small files, seconds)
.\import-service-score.exe >> %LOGFILE% 2>&1

echo %date% %time% sync ops done >> %LOGFILE%
