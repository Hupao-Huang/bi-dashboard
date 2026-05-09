@echo off
REM Daily build of warehouse_flow_summary materialized table (current month)
REM On day 03 each month, also rebuild last month for cleanup
cd /d C:\Users\Administrator\bi-dashboard\server
.\build-warehouse-flow-summary.exe >> .\build-warehouse-flow-summary.log 2>&1
if "%date:~8,2%"=="03" (
  for /f %%b in ('powershell -NoProfile -Command "(Get-Date).AddMonths(-1).ToString('yyyy-MM')"') do (
    .\build-warehouse-flow-summary.exe --ym=%%b >> .\build-warehouse-flow-summary.log 2>&1
  )
)