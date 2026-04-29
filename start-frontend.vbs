Set WshShell = CreateObject("WScript.Shell")
WshShell.CurrentDirectory = "C:\Users\Administrator\bi-dashboard"
WshShell.Run "cmd /c ""C:\Users\Administrator\AppData\Roaming\npm\serve.cmd"" -s build -l 3000 > frontend-serve.log 2>&1", 0, False
