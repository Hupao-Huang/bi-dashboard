@echo off
REM v1.62.1: wscript+vbs fails under SYSTEM schtasks (0x800710E0), switched to direct .bat call
REM Aligned with other BI-* task implementations
cd /d C:\Users\Administrator\bi-dashboard\server
.\sync-hesi.exe
exit /b %ERRORLEVEL%
