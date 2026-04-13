 = 4216
while (Get-Process -Id  -ErrorAction SilentlyContinue) {
  Start-Sleep -Seconds 15
}
 = '2025-10-01'
 = '2025-10-31'
& 'C:\Users\Administrator\bi-dashboard\server\sync-trades-v2.exe' 1>> 'C:\Users\Administrator\bi-dashboard\server\sync-trades-v2-202510.out.log' 2>> 'C:\Users\Administrator\bi-dashboard\server\sync-trades-v2-202510.err.log'
