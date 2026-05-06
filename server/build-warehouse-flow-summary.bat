@echo off
REM 每天凌晨跑：构建快递仓储分析物化表(当月)
REM 每月3号顺带刷上月一次(确保收尾)
cd /d C:\Users\Administrator\bi-dashboard\server
build-warehouse-flow-summary.exe >> build-warehouse-flow-summary.log 2>&1
if "%date:~8,2%"=="03" (
  for /f "tokens=2 delims= " %%a in ('date /t') do (
    REM 简单算上月: 使用 powershell 取上月
    for /f %%b in ('powershell -NoProfile -Command "(Get-Date).AddMonths(-1).ToString('yyyy-MM')"') do (
      build-warehouse-flow-summary.exe --ym=%%b >> build-warehouse-flow-summary.log 2>&1
    )
  )
)
