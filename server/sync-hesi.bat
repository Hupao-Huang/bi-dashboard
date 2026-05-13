@echo off
REM v1.62.1: schtasks 用 wscript+vbs 在 SYSTEM 账户下失败 (0x800710E0), 改 .bat 直调
REM 对齐其他 BI-* 任务的实现方式
cd /d C:\Users\Administrator\bi-dashboard\server
.\sync-hesi.exe
exit /b %ERRORLEVEL%
