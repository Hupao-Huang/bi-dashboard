Set WshShell = CreateObject("WScript.Shell")
WshShell.CurrentDirectory = "C:\Users\Administrator\bi-dashboard\server"
WshShell.Run "sync-hesi.exe", 0, True
