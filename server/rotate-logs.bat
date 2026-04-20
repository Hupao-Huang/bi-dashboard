@echo off
REM 每周日志轮转：
REM 1. 活跃 .log 超过 10MB 则 rename 为 .1 并清空
REM 2. 30 天以前的 manual-*.log 归档到 archive/
REM 3. 超过 90 天的归档压缩后删除原文件

cd /d C:\Users\Administrator\bi-dashboard\server

set LOGFILE=C:\Users\Administrator\bi-dashboard\server\rotate-logs.log
echo =============================================== >> %LOGFILE%
echo %date% %time% start log rotation >> %LOGFILE%

REM 1. 轮转活跃日志（>10MB）
for %%f in (sync-stock.log sync-batch-stock.log sync-daily-trades.log sync-daily-summary.log sync-daily.log server.log server-stderr.log bi-server.log sync-hesi.log sync-modified.log sync-summary-2024.log sync-ops-daily.log) do (
    if exist "%%f" (
        for %%a in ("%%f") do (
            if %%~za GTR 10485760 (
                if exist "%%f.1" del /q "%%f.1"
                move /y "%%f" "%%f.1" >nul
                echo.>"%%f"
                echo %date% %time% rotated %%f >> %LOGFILE%
            )
        )
    )
)

REM 2. 30天前的 manual-* 归档
if not exist archive mkdir archive
forfiles /P . /M "manual-*.log" /D -30 /C "cmd /c move @file archive\ >nul" 2>nul
forfiles /P . /M "monitor*.log" /D -30 /C "cmd /c move @file archive\ >nul" 2>nul
forfiles /P . /M "fix-*.log" /D -30 /C "cmd /c move @file archive\ >nul" 2>nul

REM 3. 90天前的归档直接删
forfiles /P archive /M "*.log" /D -90 /C "cmd /c del @file" 2>nul

echo %date% %time% done >> %LOGFILE%
