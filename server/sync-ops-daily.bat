@echo off
REM 每日运营数据自动导入（RPA抓完后执行）
REM 导入昨日所有运营平台数据

cd /d C:\Users\Administrator\bi-dashboard\server

for /f %%i in ('powershell -Command "(Get-Date).AddDays(-1).ToString('yyyyMMdd')"') do set YESTERDAY=%%i

set LOGFILE=C:\Users\Administrator\bi-dashboard\server\sync-ops-daily.log
echo =============================================== >> %LOGFILE%
echo %date% %time% start sync ops data for %YESTERDAY% >> %LOGFILE%

REM 天猫
C:\Users\Administrator\bi-dashboard\server\import-tmall.exe %YESTERDAY% %YESTERDAY% >> %LOGFILE% 2>&1

REM 拼多多
C:\Users\Administrator\bi-dashboard\server\import-pdd.exe %YESTERDAY% %YESTERDAY% >> %LOGFILE% 2>&1

REM 京东
C:\Users\Administrator\bi-dashboard\server\import-jd.exe %YESTERDAY% %YESTERDAY% >> %LOGFILE% 2>&1

REM 京东推广（联盟/京准通）
C:\Users\Administrator\bi-dashboard\server\import-promo.exe %YESTERDAY% %YESTERDAY% >> %LOGFILE% 2>&1

REM 唯品会
C:\Users\Administrator\bi-dashboard\server\import-vip.exe %YESTERDAY% %YESTERDAY% >> %LOGFILE% 2>&1

REM 天猫客服
C:\Users\Administrator\bi-dashboard\server\import-tmallcs.exe %YESTERDAY% %YESTERDAY% >> %LOGFILE% 2>&1

REM 抖音自营（新加）
C:\Users\Administrator\bi-dashboard\server\import-douyin.exe %YESTERDAY% %YESTERDAY% >> %LOGFILE% 2>&1

REM 抖音分销（新加）
C:\Users\Administrator\bi-dashboard\server\import-douyin-dist.exe %YESTERDAY% %YESTERDAY% >> %LOGFILE% 2>&1

REM 客服: 京东自营/飞鸽/快手/小红书（新加）
C:\Users\Administrator\bi-dashboard\server\import-customer.exe %YESTERDAY% %YESTERDAY% >> %LOGFILE% 2>&1

REM 飞瓜
C:\Users\Administrator\bi-dashboard\server\import-feigua.exe %YESTERDAY% %YESTERDAY% >> %LOGFILE% 2>&1

echo %date% %time% sync ops done >> %LOGFILE%
