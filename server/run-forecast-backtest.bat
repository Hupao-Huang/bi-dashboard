@echo off
rem BI 销量预测算法回测 — 月度自动跑上月
rem schtasks: BI-RunForecastBacktest 每月 2 号 03:00 自动触发
rem 4 算法串行: baseline (Go) + StatsForecast + Prophet + LightGBM
rem 单独支持手动指定月份: run-forecast-backtest.bat 2026-04 (跑指定月份)
setlocal
cd /d "%~dp0"

set "PY312=C:\Users\Administrator\AppData\Local\Programs\Python\Python312\python.exe"

set "TARGET_MONTH=%1"
if "%TARGET_MONTH%"=="" (
    echo [%date% %time%] === BI-RunForecastBacktest 开始 (默认: 上月) ===
    set "MONTHS_FLAG="
    set "PY_FLAG="
) else (
    echo [%date% %time%] === BI-RunForecastBacktest 开始 (指定: %TARGET_MONTH%) ===
    set "MONTHS_FLAG=--months=%TARGET_MONTH%"
    set "PY_FLAG=--months=%TARGET_MONTH%"
)

echo.
echo --- 1/4 Baseline (Go, 4 算法: last_month/yoy/avg3m/wma3) ---
.\forecast-baseline-backtest.exe %MONTHS_FLAG%
if errorlevel 1 echo [WARN] baseline 退出码 %errorlevel%

echo.
echo --- 2/4 StatsForecast (Python 312) ---
"%PY312%" "cmd\prophet-backtest\statsforecast_backtest_v2.py" %PY_FLAG%
if errorlevel 1 echo [WARN] statsforecast 退出码 %errorlevel%

echo.
echo --- 3/4 Prophet (Python 312) ---
"%PY312%" "cmd\prophet-backtest\prophet_backtest.py" %PY_FLAG%
if errorlevel 1 echo [WARN] prophet 退出码 %errorlevel%

echo.
echo --- 4/4 LightGBM (Python 312) ---
"%PY312%" "cmd\lightgbm-backtest\lightgbm_backtest.py" %PY_FLAG%
if errorlevel 1 echo [WARN] lightgbm 退出码 %errorlevel%

echo.
echo [%date% %time%] === BI-RunForecastBacktest 全部完成 ===
endlocal
