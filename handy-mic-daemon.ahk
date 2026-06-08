#Requires AutoHotkey v2.0

; Mutes the VB-Cable feed while ctrl+space is held so call participants (Teams/Zoom/
; Discord) hear silence during Handy dictation. Handy reads the physical mic directly,
; so it is unaffected. svcl.exe is called directly (no PowerShell) for instant, device-
; specific muting. The ~ prefix passes ctrl+space through so Handy still receives it.

svcl   := "C:\Tools\svcl.exe"
device := "CABLE Input (VB-Audio Virtual Cable)"   ; render endpoint feeding the cable

; ctrl+space held — mute the cable feed; call participants hear nothing
~^Space:: Run('"' svcl '" /Mute "' device '"', , "Hide")

; ctrl+space released — unmute; call participants hear the user again
~^Space up:: Run('"' svcl '" /Unmute "' device '"', , "Hide")
