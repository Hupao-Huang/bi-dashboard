@echo off
REM 期货盘中实时快照同步 - 每5分钟跑一次
REM 工具内部自带交易时段闸门(周末/深夜自动退出), 非交易时段空跑也无副作用
REM SYSTEM 账户 schtasks 调用, 用绝对 cd, exe 必须 .\ 前缀
cd /d C:\Users\Administrator\bi-dashboard\server
.\sync-futures-realtime.exe
