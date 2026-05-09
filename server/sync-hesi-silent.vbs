Set WshShell = CreateObject("WScript.Shell")
WshShell.CurrentDirectory = "C:\Users\Administrator\bi-dashboard\server"
' Redirect output to sync-hesi.log for ops monitoring
WshShell.Run "cmd /c .\sync-hesi.exe > sync-hesi.log 2>&1", 0, True