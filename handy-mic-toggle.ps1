param([ValidateSet("mute", "unmute")][string]$Action)  # manual test / fallback helper

# The hotkey daemon calls svcl.exe directly for speed; this script is for manual testing
# and verification. It mutes the VB-Cable render endpoint that feeds the call apps.

$svcl   = "C:\Tools\svcl.exe"
$device = "CABLE Input (VB-Audio Virtual Cable)"

if ($Action -eq "mute") {
    & $svcl /Mute $device
} elseif ($Action -eq "unmute") {
    & $svcl /Unmute $device
}
