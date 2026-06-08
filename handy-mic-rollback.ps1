# Full teardown — removes the scheduled task, kills the AHK daemon, and unmutes the cable.

# Remove scheduled task (ignore error if it was never registered)
Unregister-ScheduledTask -TaskName "HandyMicDaemon" -Confirm:$false -ErrorAction SilentlyContinue

# Kill any running AHK process hosting this script
Get-CimInstance Win32_Process |
    Where-Object { $_.CommandLine -like "*handy-mic-daemon.ahk*" } |
    ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }

# Safety unmute — ensure the cable feed is not left muted
& "C:\Tools\svcl.exe" /Unmute "CABLE Input (VB-Audio Virtual Cable)"

Write-Host "Rollback complete. You can leave VB-Cable / the Listen bridge in place, or remove the"
Write-Host "Listen route and reset Teams/Zoom/Discord mic back to your real microphone to fully undo."
