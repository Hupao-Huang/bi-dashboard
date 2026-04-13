@echo off
cd /d C:\Users\Administrator\bi-dashboard\server

for /f %%i in ('powershell -Command "(Get-Date).AddDays(-1).ToString('yyyyMMdd')"') do set YESTERDAY=%%i

echo %date% %time% sync ops data for %YESTERDAY%

C:\Users\Administrator\bi-dashboard\server\import-tmall.exe %YESTERDAY% %YESTERDAY%
C:\Users\Administrator\bi-dashboard\server\import-pdd.exe %YESTERDAY% %YESTERDAY%
C:\Users\Administrator\bi-dashboard\server\import-jd.exe %YESTERDAY% %YESTERDAY%
C:\Users\Administrator\bi-dashboard\server\import-vip.exe %YESTERDAY% %YESTERDAY%
C:\Users\Administrator\bi-dashboard\server\import-tmallcs.exe %YESTERDAY% %YESTERDAY%
C:\Users\Administrator\bi-dashboard\server\import-promo.exe %YESTERDAY% %YESTERDAY%
C:\Users\Administrator\bi-dashboard\server\import-feigua.exe %YESTERDAY% %YESTERDAY%

echo %date% %time% done
