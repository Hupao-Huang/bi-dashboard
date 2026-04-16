Set WshShell = CreateObject("WScript.Shell")
WshShell.CurrentDirectory = "C:\Users\Administrator\bi-dashboard\server"
WshShell.Run "bi-server.exe", 0, False
