@echo off
REM v1.74.3 (2026-05-25): daily 03:00 refresh of pending allot order status
REM Fixes stale-status bug: default 7-day window misses old orders completing (in_status 1->3)
REM Processes all pending orders where status!=20 OR in_status!=3 (from earliest audit_date to today)
cd /d C:\Users\Administrator\bi-dashboard\server
if not exist logs mkdir logs
.\sync-allocate.exe -refresh-pending > logs\sync-allocate-refresh.log 2>&1
