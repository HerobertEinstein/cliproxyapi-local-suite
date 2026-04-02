Option Explicit

Dim fso, scriptDir, ps1Path, psExe, cmd, shell
Set fso = CreateObject("Scripting.FileSystemObject")
scriptDir = fso.GetParentFolderName(WScript.ScriptFullName)
ps1Path = scriptDir & "\\Open-CLIProxyAPI-GUI.ps1"

psExe = CreateObject("WScript.Shell").ExpandEnvironmentStrings("%SystemRoot%") & "\\System32\\WindowsPowerShell\\v1.0\\powershell.exe"
cmd = psExe & " -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File """ & ps1Path & """"

Set shell = CreateObject("WScript.Shell")
shell.Run cmd, 0, False
