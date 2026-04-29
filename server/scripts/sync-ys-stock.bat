@echo off
REM 用友 BIP 现存量同步 — schtasks 09:30/14:00/18:00 调用
REM 日志按日切分, 完成后由 exe 自行调 bi-server webhook 清缓存
setlocal
set LOGDIR=C:\Users\Administrator\bi-dashboard\server\logs
if not exist "%LOGDIR%" mkdir "%LOGDIR%"
set TS=%date:~0,4%%date:~5,2%%date:~8,2%
set LOGFILE=%LOGDIR%\sync-ys-stock-%TS%.log
echo. >> "%LOGFILE%"
echo ====== %date% %time% ====== >> "%LOGFILE%"
"C:\Users\Administrator\bi-dashboard\server\sync-yonsuite-stock.exe" >> "%LOGFILE%" 2>&1
endlocal
