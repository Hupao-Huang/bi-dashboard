@echo off
REM Daily ops data sync (RPA fallback after RPA collects, fires from webhook handler at 13:00 if not already imported)
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

echo %date% %time% sync ops done >> %LOGFILE%