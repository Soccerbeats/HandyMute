# Run once as Administrator to register the AHK daemon at login via Task Scheduler.

$ahkExe    = "C:\Program Files\AutoHotkey\v2\AutoHotkey64.exe"
$ahkScript = "C:\Tools\handy-mic-daemon.ahk"

$action   = New-ScheduledTaskAction -Execute $ahkExe -Argument "`"$ahkScript`""
$trigger  = New-ScheduledTaskTrigger -AtLogOn
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries

Register-ScheduledTask `
    -TaskName "HandyMicDaemon" `
    -Action   $action `
    -Trigger  $trigger `
    -Settings $settings `
    -RunLevel Limited `
    -Force

Write-Host "Task registered. Verify with: Get-ScheduledTask -TaskName 'HandyMicDaemon'"
