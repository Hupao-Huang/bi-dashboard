@echo off
cd /d C:\Users\Administrator\bi-dashboard
"C:\Program Files\Python314\python.exe" server\cmd\prophet-train\prophet_train.py >> logs\prophet_train.log 2>&1
