@echo off
REM YonBIP incoming inspection sync - schtasks daily 09:40 (default last 3 days to today, covers late approvals)
setlocal
set LOGDIR=C:\Users\Administrator\bi-dashboard\server\logs
if not exist "%LOGDIR%" mkdir "%LOGDIR%"
set TS=%date:~0,4%%date:~5,2%%date:~8,2%
set LOGFILE=%LOGDIR%\sync-ys-inspection-%TS%.log
echo. >> "%LOGFILE%"
echo ====== %date% %time% ====== >> "%LOGFILE%"
"C:\Users\Administrator\bi-dashboard\server\sync-yonsuite-inspection.exe" >> "%LOGFILE%" 2>&1
endlocal
