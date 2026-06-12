@echo off
REM Hesi travel orders daily sync (orders created in last 90 days, idempotent upsert; status fields get updated)
cd /d C:\Users\Administrator\bi-dashboard\server
echo =============================================== >> sync-hesi-orders.log
echo %date% %time% start >> sync-hesi-orders.log
.\sync-hesi-orders.exe >> sync-hesi-orders.log 2>&1
echo %date% %time% done >> sync-hesi-orders.log
