@echo off
REM v1.62.x: 周日凌晨跑 --full 全量补差 (覆盖 archived 单数据变更/凭证回写等增量遗漏)
cd /d C:\Users\Administrator\bi-dashboard\server
.\sync-hesi.exe --full
exit /b %ERRORLEVEL%
