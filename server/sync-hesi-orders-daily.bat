@echo off
REM 合思商旅行程/订单每日同步 (近90天创建的单, 幂等覆盖; 报销状态等字段会更新)
cd /d C:\Users\Administrator\bi-dashboard\server
echo =============================================== >> sync-hesi-orders.log
echo %date% %time% start >> sync-hesi-orders.log
.\sync-hesi-orders.exe >> sync-hesi-orders.log 2>&1
echo %date% %time% done >> sync-hesi-orders.log
