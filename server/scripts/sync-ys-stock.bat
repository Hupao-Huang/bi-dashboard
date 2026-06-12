@echo off
REM YonBIP current stock sync - schtasks 09:30/14:00/18:00
REM Logs split by day; exe calls bi-server webhook to clear cache when done
setlocal
set LOGDIR=C:\Users\Administrator\bi-dashboard\server\logs
if not exist "%LOGDIR%" mkdir "%LOGDIR%"
set TS=%date:~0,4%%date:~5,2%%date:~8,2%
set LOGFILE=%LOGDIR%\sync-ys-stock-%TS%.log
echo. >> "%LOGFILE%"
echo ====== %date% %time% ====== >> "%LOGFILE%"
"C:\Users\Administrator\bi-dashboard\server\sync-yonsuite-stock.exe" >> "%LOGFILE%" 2>&1
endlocal
