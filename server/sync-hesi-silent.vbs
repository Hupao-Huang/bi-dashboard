Set WshShell = CreateObject("WScript.Shell")
WshShell.CurrentDirectory = "C:\Users\Administrator\bi-dashboard\server"
' v1.57.1: 不再用 cmd 重定向 (exe 内部已 MultiWriter 写固定 sync-hesi.log + stdout)
' 直接 cmd /c 跑 exe; 同步等待 (True) 让 schtasks 拿到真实退出码
WshShell.Run "cmd /c .\sync-hesi.exe", 0, True
WScript.Quit 0
