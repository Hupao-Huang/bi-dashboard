@echo off
REM 用友 BIP 来料检验单同步 — schtasks 每天 09:40 调用 (默认拉最近3天~今天, 覆盖晚审批)
setlocal
set LOGDIR=C:\Users\Administrator\bi-dashboard\server\logs
if not exist "%LOGDIR%" mkdir "%LOGDIR%"
set TS=%date:~0,4%%date:~5,2%%date:~8,2%
set LOGFILE=%LOGDIR%\sync-ys-inspection-%TS%.log
echo. >> "%LOGFILE%"
echo ====== %date% %time% ====== >> "%LOGFILE%"
"C:\Users\Administrator\bi-dashboard\server\sync-yonsuite-inspection.exe" >> "%LOGFILE%" 2>&1
endlocal
