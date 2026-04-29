@echo off
REM 用友 BIP 委外订单同步 — schtasks 09:10 调用 (默认拉昨天~今天)
setlocal
set LOGDIR=C:\Users\Administrator\bi-dashboard\server\logs
if not exist "%LOGDIR%" mkdir "%LOGDIR%"
set TS=%date:~0,4%%date:~5,2%%date:~8,2%
set LOGFILE=%LOGDIR%\sync-ys-subcontract-%TS%.log
echo. >> "%LOGFILE%"
echo ====== %date% %time% ====== >> "%LOGFILE%"
"C:\Users\Administrator\bi-dashboard\server\sync-yonsuite-subcontract.exe" >> "%LOGFILE%" 2>&1
endlocal
