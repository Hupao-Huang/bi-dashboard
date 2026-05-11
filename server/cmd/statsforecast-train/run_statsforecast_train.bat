@echo off
cd /d C:\Users\Administrator\bi-dashboard
"C:\Users\Administrator\AppData\Local\Programs\Python\Python312\python.exe" server\cmd\statsforecast-train\statsforecast_train.py >> logs\statsforecast_train.log 2>&1
