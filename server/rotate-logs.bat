@echo off
REM Weekly log rotation
REM 1. Active log over 10MB rename to .1 and clear
REM 2. manual/monitor/fix log older than 30 days move to archive
REM 3. archive log older than 90 days remove

cd /d C:\Users\Administrator\bi-dashboard\server

set LOGFILE=C:\Users\Administrator\bi-dashboard\server\rotate-logs.log
echo =============================================== >> %LOGFILE%
echo %date% %time% start log rotation >> %LOGFILE%

for %%f in (sync-stock.log sync-batch-stock.log sync-daily-trades.log sync-daily-summary.log sync-daily.log server.log server-stderr.log bi-server.log sync-hesi.log sync-summary-2024.log sync-ops-daily.log) do (
    if exist "%%f" (
        for %%a in ("%%f") do (
            if %%~za GTR 10485760 (
                if exist "%%f.1" erase /q "%%f.1"
                move /y "%%f" "%%f.1" >nul
                echo.>"%%f"
                echo %date% %time% rotated %%f >> %LOGFILE%
            )
        )
    )
)

if not exist archive mkdir archive
forfiles /P . /M "manual-*.log" /D -30 /C "cmd /c move @file archive\ >nul" >nul 2>&1
forfiles /P . /M "monitor*.log" /D -30 /C "cmd /c move @file archive\ >nul" >nul 2>&1
forfiles /P . /M "fix-*.log" /D -30 /C "cmd /c move @file archive\ >nul" >nul 2>&1

forfiles /P archive /M "*.log" /D -90 /C "cmd /c erase @file" >nul 2>&1

echo %date% %time% done >> %LOGFILE%

REM Force exit 0 to avoid forfiles no-match errorlevel failing the task
exit /b 0
