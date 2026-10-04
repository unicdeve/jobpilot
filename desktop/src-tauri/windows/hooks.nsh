; JobPilot's hooks into Tauri's NSIS installer (ADR-0029 §5). The installer
; is per user, so these run as the user whose daemon, data and scheduled
; tasks they are.

; An upgrade cannot replace a jobpilot.exe that is running, so the installed
; daemon is stopped first. `stop` waits for its process to exit.
!macro NSIS_HOOK_PREINSTALL
  ${If} ${FileExists} "$INSTDIR\jobpilot.exe"
    nsExec::Exec '"$INSTDIR\jobpilot.exe" stop'
    Pop $0
  ${EndIf}
!macroend

; Upgrading runs the old version's uninstaller first, by default, whose
; hook below cannot tell an upgrade from an uninstall and removes every
; scheduled scout's task. The new jobpilot.exe schedules them again from the
; database, and on a first install, with no database, does nothing.
!macro NSIS_HOOK_POSTINSTALL
  nsExec::Exec '"$INSTDIR\jobpilot.exe" alarms restore'
  Pop $0
!macroend

; Uninstalling stops the daemon, then removes every scheduled scout's task,
; which would otherwise start a jobpilot.exe that is no longer there at
; every scheduled time. The user's data is left where it is (ADR-0021).
!macro NSIS_HOOK_PREUNINSTALL
  ${If} ${FileExists} "$INSTDIR\jobpilot.exe"
    nsExec::Exec '"$INSTDIR\jobpilot.exe" stop'
    Pop $0
    nsExec::Exec '"$INSTDIR\jobpilot.exe" alarms remove'
    Pop $0
  ${EndIf}
!macroend
