@echo off
REM 期货日线每日同步 - 每天 17:30 跑（日盘收盘 15:00 后 + 夜盘前）
REM SYSTEM 账户 schtasks 调用，不能用相对 cd 因为工作目录不可控
cd /d C:\Users\Administrator\bi-dashboard\server
.\sync-futures.exe
