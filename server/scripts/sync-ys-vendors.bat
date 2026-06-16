@echo off
REM YonBIP vendor dict sync - schtasks daily 05:00 (full refresh ys_vendor_dict)
setlocal
set LOGDIR=C:\Users\Administrator\bi-dashboard\server\logs
if not exist "%LOGDIR%" mkdir "%LOGDIR%"
set TS=%date:~0,4%%date:~5,2%%date:~8,2%
set LOGFILE=%LOGDIR%\sync-ys-vendors-%TS%.log
echo. >> "%LOGFILE%"
echo ====== %date% %time% ====== >> "%LOGFILE%"
"C:\Users\Administrator\bi-dashboard\server\sync-ys-vendors.exe" >> "%LOGFILE%" 2>&1
endlocal
