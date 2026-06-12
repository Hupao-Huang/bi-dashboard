@echo off
rem BI sales forecast backtest - monthly auto-run for last month
rem schtasks: BI-RunForecastBacktest fires monthly on day 2 at 03:00
rem 4 algorithms in sequence: baseline (Go) + StatsForecast + Prophet + LightGBM
rem Supports manual month arg: run-forecast-backtest.bat 2026-04
setlocal
cd /d "%~dp0"

set "PY312=C:\Users\Administrator\AppData\Local\Programs\Python\Python312\python.exe"

set "TARGET_MONTH=%1"
if "%TARGET_MONTH%"=="" (
    echo [%date% %time%] === BI-RunForecastBacktest start (default: last month) ===
    set "MONTHS_FLAG="
    set "PY_FLAG="
) else (
    echo [%date% %time%] === BI-RunForecastBacktest start (target: %TARGET_MONTH%) ===
    set "MONTHS_FLAG=--months=%TARGET_MONTH%"
    set "PY_FLAG=--months=%TARGET_MONTH%"
)

echo.
echo --- 1/4 Baseline (Go, 4 algos: last_month/yoy/avg3m/wma3) ---
.\forecast-baseline-backtest.exe %MONTHS_FLAG%
if errorlevel 1 echo [WARN] baseline exit code %errorlevel%

echo.
echo --- 2/4 StatsForecast (Python 312) ---
"%PY312%" "cmd\prophet-backtest\statsforecast_backtest_v2.py" %PY_FLAG%
if errorlevel 1 echo [WARN] statsforecast exit code %errorlevel%

echo.
echo --- 3/4 Prophet (Python 312) ---
"%PY312%" "cmd\prophet-backtest\prophet_backtest.py" %PY_FLAG%
if errorlevel 1 echo [WARN] prophet exit code %errorlevel%

echo.
echo --- 4/4 LightGBM (Python 312) ---
"%PY312%" "cmd\lightgbm-backtest\lightgbm_backtest.py" %PY_FLAG%
if errorlevel 1 echo [WARN] lightgbm exit code %errorlevel%

echo.
echo [%date% %time%] === BI-RunForecastBacktest all done ===
endlocal
