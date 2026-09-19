; WhatsApp Desk — Windows installer (NSIS, per-user, no admin required).
;
; Production installer for Windows 10 and 11 (x64). Differs from a plain
; portable exe by providing: branded wizard graphics, Start Menu + Desktop
; shortcuts, an Add/Remove Programs entry with proper metadata, registered
; capabilities so notifications and the taskbar group correctly, and a clean
; uninstaller that optionally removes user data.
;
; Design notes
;   * Per-user install ($LOCALAPPDATA) keeps the in-app self-updater able to
;     replace the binary without elevation; a Program Files install would need
;     UAC on every update.
;   * The engine is WebView2, which ships with Windows 11 and is present on
;     virtually all current Windows 10 machines.
;
; Build from the repo root:
;   python3 installer/windows/make_wizard_assets.py
;   makensis -DVERSION=1.5.9.7 -DAPPEXE_PATH=dist_win\WhatsAppDesk.exe installer\windows\WhatsAppDesk.nsi
; Output: WhatsApp-Desk-Windows-x64-Setup.exe (repo root).

Target x86-ansi

!define APPNAME "WhatsApp Desk"
!define APPID "WhatsAppDesk"
!define COMPANY "WhatsApp Desk Contributors"
!define APPURL "https://github.com/vianziro/Whatsapp-Dekstop"
!define APPEXE "WhatsAppDesk.exe"

!ifndef VERSION
!define VERSION "1.5.9.7"
!endif

; Path to the built binary. Overridable so release automation can point at the
; artifact directory instead of the working tree.
!ifndef APPEXE_PATH
!define APPEXE_PATH "../../${APPEXE}"
!endif

!ifndef ASSETS_DIR
!define ASSETS_DIR "assets"
!endif

Name "${APPNAME} ${VERSION}"
OutFile "../../WhatsApp-Desk-Windows-x64-Setup.exe"
InstallDir "$LOCALAPPDATA\${APPNAME}"
InstallDirRegKey HKCU "Software\${APPID}" "InstallDir"
RequestExecutionLevel user

; Modern wizard with an always-on header image, matching the app's quiet look.
!include "MUI2.nsh"
!include "FileFunc.nsh"
!include "LogicLib.nsh"

!define MUI_ABORTWARNING
!define MUI_ICON "${ASSETS_DIR}\icon.ico"
!define MUI_UNICON "${ASSETS_DIR}\icon.ico"

; Branded graphics: 164x314 sidebar on Welcome/Finish, 150x57 on the rest.
!define MUI_WELCOMEFINISHPAGE_BITMAP "${ASSETS_DIR}\welcome.bmp"
!define MUI_HEADERIMAGE
!define MUI_HEADERIMAGE_BITMAP "${ASSETS_DIR}\header.bmp"
!define MUI_HEADERIMAGE_RIGHT

!define MUI_WELCOMEPAGE_TITLE "${APPNAME} ${VERSION}"
!define MUI_WELCOMEPAGE_TEXT "This wizard installs ${APPNAME} on this computer.$\r$\n$\r$\n${APPNAME} is an independent desktop shell for WhatsApp Web. It creates Start Menu and desktop shortcuts and can be removed at any time from Apps & features.$\r$\n$\r$\nClick Next to continue."

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES

!define MUI_FINISHPAGE_RUN "$INSTDIR\${APPEXE}"
!define MUI_FINISHPAGE_RUN_TEXT "Run ${APPNAME} now"
!define MUI_FINISHPAGE_LINK "Open the project page"
!define MUI_FINISHPAGE_LINK_LOCATION "${APPURL}"
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "Indonesian"

; ---------------------------------------------------------------------------
; Install
; ---------------------------------------------------------------------------
Section "${APPNAME}" SEC_APP
  SectionIn RO

  ; Close a running instance so the binary is never replaced under a live
  ; process (a locked file would otherwise fail the install).
  DetailPrint "Closing a running ${APPNAME} instance, if any..."
  nsExec::ExecToLog 'taskkill /IM "${APPEXE}" /F'
  Pop $0
  Sleep 600

  SetOutPath "$INSTDIR"
  SetOverwrite on
  File "${APPEXE_PATH}"

  ; Ship the uninstaller and the icon next to the app so shortcuts and the
  ; Add/Remove entry can resolve them without depending on the source tree.
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  File "/oname=$INSTDIR\appicon.ico" "${ASSETS_DIR}\icon.ico"

  ; Registry: install location, uninstall metadata, and app capabilities.
  WriteRegStr HKCU "Software\${APPID}" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\${APPID}" "Version" "${VERSION}"

  !define UNINSTKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPID}"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayName" "${APPNAME}"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINSTKEY}" "Publisher" "${COMPANY}"
  WriteRegStr HKCU "${UNINSTKEY}" "URLInfoAbout" "${APPURL}"
  WriteRegStr HKCU "${UNINSTKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTKEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKCU "${UNINSTKEY}" "QuietUninstallString" '"$INSTDIR\Uninstall.exe" /S'
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayIcon" "$INSTDIR\appicon.ico"
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoRepair" 1
  ; EstimatedSize (KB) makes the Apps & features list show a real footprint.
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKCU "${UNINSTKEY}" "EstimatedSize" "$0"

  ; App capabilities: gives the app a proper identity for notifications and
  ; the shell (taskbar grouping, default-apps listing).
  WriteRegStr HKCU "Software\${APPID}\Capabilities" "ApplicationName" "${APPNAME}"
  WriteRegStr HKCU "Software\${APPID}\Capabilities" "ApplicationDescription" "Independent desktop shell for WhatsApp Web"
  WriteRegStr HKCU "Software\${APPID}\Capabilities" "ApplicationIcon" "$INSTDIR\appicon.ico"
  WriteRegStr HKCU "Software\RegisteredApplications" "${APPNAME}" "Software\${APPID}\Capabilities"

  ; Shortcuts: Start Menu (with uninstall entry) + Desktop.
  SetShellVarContext current
  CreateDirectory "$SMPROGRAMS\${APPNAME}"
  CreateShortcut "$SMPROGRAMS\${APPNAME}\${APPNAME}.lnk" "$INSTDIR\${APPEXE}" "" \
    "$INSTDIR\appicon.ico" 0 SW_SHOWNORMAL "" "Independent desktop shell for WhatsApp Web"
  CreateShortcut "$SMPROGRAMS\${APPNAME}\Uninstall ${APPNAME}.lnk" "$INSTDIR\Uninstall.exe"
  CreateShortcut "$DESKTOP\${APPNAME}.lnk" "$INSTDIR\${APPEXE}" "" \
    "$INSTDIR\appicon.ico" 0 SW_SHOWNORMAL "" "Independent desktop shell for WhatsApp Web"

  ; Refresh the shell so the new shortcuts and icon appear immediately.
  System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'
SectionEnd

; ---------------------------------------------------------------------------
; Uninstall
; ---------------------------------------------------------------------------
!define MUI_UNCONFIRMPAGE_TEXT_TOP "This removes ${APPNAME} from your computer. Your WhatsApp Desk chats and settings can be kept."

Section "Uninstall"
  ; Stop the app first: deleting a running exe leaves orphaned files.
  nsExec::ExecToLog 'taskkill /IM "${APPEXE}" /F'
  Pop $0
  Sleep 600

  SetShellVarContext current
  Delete "$SMPROGRAMS\${APPNAME}\${APPNAME}.lnk"
  Delete "$SMPROGRAMS\${APPNAME}\Uninstall ${APPNAME}.lnk"
  RMDir "$SMPROGRAMS\${APPNAME}"
  Delete "$DESKTOP\${APPNAME}.lnk"

  ; Installed payload (never touch the user's own downloads folder).
  Delete "$INSTDIR\${APPEXE}"
  Delete "$INSTDIR\appicon.ico"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"

  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPID}"
  DeleteRegValue HKCU "Software\RegisteredApplications" "${APPNAME}"
  DeleteRegKey HKCU "Software\${APPID}"

  ; Optional: delete the app profile (session, cache, settings). Kept intact by
  ; default so a reinstall stays logged in; the user opts in from the prompt.
  MessageBox MB_YESNO|MB_ICONQUESTION \
    "Also delete your WhatsApp Desk profile (login session, cache and settings)?$\r$\n$\r$\nChoose No to keep your session for a future reinstall." \
    IDNO skip_profile
    RMDir /r "$APPDATA\${APPID}"
    RMDir /r "$LOCALAPPDATA\${APPID}"
  skip_profile:

  System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'
SectionEnd

; ---------------------------------------------------------------------------
; Metadata
; ---------------------------------------------------------------------------
VIProductVersion "${VERSION}.0"
VIAddVersionKey "ProductName" "${APPNAME}"
VIAddVersionKey "CompanyName" "${COMPANY}"
VIAddVersionKey "FileDescription" "${APPNAME} Setup"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "ProductVersion" "${VERSION}"
VIAddVersionKey "LegalCopyright" "MIT License"
