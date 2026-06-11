@echo off
REM 客服服务分每日同步 (RPA 当天下午写 Z 盘, 19:30 兜底导入; 全量幂等扫描)
cd /d C:\Users\Administrator\bi-dashboard\server

set LOGFILE=C:\Users\Administrator\bi-dashboard\server\sync-service-score.log
echo =============================================== >> %LOGFILE%
echo %date% %time% start sync service score >> %LOGFILE%

.\import-service-score.exe >> %LOGFILE% 2>&1

echo %date% %time% sync service score done >> %LOGFILE%
