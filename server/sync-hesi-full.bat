@echo off
REM v1.62.x: Sunday night --full resync (covers archived doc changes / voucher writebacks missed by incremental)
cd /d C:\Users\Administrator\bi-dashboard\server
.\sync-hesi.exe --full
exit /b %ERRORLEVEL%
