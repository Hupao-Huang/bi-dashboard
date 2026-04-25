Set WshShell = CreateObject("WScript.Shell")
WshShell.CurrentDirectory = "C:\Users\Administrator\bi-dashboard\server"
' 重定向输出到 sync-hesi.log，便于运维监控
WshShell.Run "cmd /c sync-hesi.exe > sync-hesi.log 2>&1", 0, True
