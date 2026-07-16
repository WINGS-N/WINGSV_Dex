; Universal NSIS installer for WINGS V Dex (Windows). One setup.exe serves both amd64
; and arm64: the 32-bit NSIS stub runs on amd64 (WoW64) and arm64 (x86 emulation), both
; payload sets are embedded, and only the one matching the host's native arch is
; extracted at install time. Build with:
;   makensis -DSRCDIR_AMD64=<dir> -DSRCDIR_ARM64=<dir> -DVERSION=x.y.z build/windows/installer.nsi
; Each SRCDIR holds that arch's wingsv-dex.exe, vkturn.exe, xray.exe, byedpi.exe and
; wintun.dll. The helpers are resolved next to the installed exe at runtime, so they
; must be installed into $INSTDIR alongside it. Cross-builds with makensis on Linux.

Unicode true
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "x64.nsh"

!ifndef VERSION
  !define VERSION "0.0.0"
!endif
!ifndef SRCDIR_AMD64
  !define SRCDIR_AMD64 "..\..\dist\windows-amd64"
!endif
!ifndef SRCDIR_ARM64
  !define SRCDIR_ARM64 "..\..\dist\windows-arm64"
!endif

!define APPNAME "WINGS V"
!define COMPANY "WINGS-N"
!define EXENAME "wingsv-dex.exe"
!define REGKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\WINGSV-Dex"

Name "${APPNAME}"
OutFile "wingsv-dex-setup.exe"
InstallDir "$PROGRAMFILES64\${APPNAME}"
InstallDirRegKey HKLM "Software\${COMPANY}\WINGSV-Dex" "InstallDir"
; Program Files + HKLM writes need elevation; the VPN also needs admin at runtime.
RequestExecutionLevel admin
SetCompressor /SOLID lzma

VIProductVersion "${VERSION}.0"
VIAddVersionKey "ProductName" "${APPNAME}"
VIAddVersionKey "CompanyName" "${COMPANY}"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "ProductVersion" "${VERSION}"
VIAddVersionKey "FileDescription" "${APPNAME} installer"
VIAddVersionKey "LegalCopyright" "${COMPANY}"

!define MUI_ICON "icon.ico"
!define MUI_UNICON "icon.ico"
!define MUI_ABORTWARNING

!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN "$INSTDIR\${EXENAME}"
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "Russian"
!insertmacro MUI_LANGUAGE "English"

; Warn (do not block) if the Edge WebView2 runtime is missing - the GUI needs it.
Function CheckWebView2
  ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${If} $0 == ""
    ReadRegStr $0 HKCU "SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${EndIf}
  ${If} $0 == ""
    MessageBox MB_ICONEXCLAMATION|MB_OK "Microsoft Edge WebView2 Runtime was not detected. The app UI needs it. Install it from https://go.microsoft.com/fwlink/p/?LinkId=2124703 if the app fails to open."
  ${EndIf}
FunctionEnd

Section "Install"
  SetShellVarContext all
  Call CheckWebView2

  SetOutPath "$INSTDIR"
  ; Both payload sets are embedded; only the matching arch is extracted.
  ${If} ${IsNativeARM64}
    File "${SRCDIR_ARM64}\${EXENAME}"
    File "${SRCDIR_ARM64}\vkturn.exe"
    File "${SRCDIR_ARM64}\xray.exe"
    File "${SRCDIR_ARM64}\byedpi.exe"
    File "${SRCDIR_ARM64}\wintun.dll"
  ${Else}
    File "${SRCDIR_AMD64}\${EXENAME}"
    File "${SRCDIR_AMD64}\vkturn.exe"
    File "${SRCDIR_AMD64}\xray.exe"
    File "${SRCDIR_AMD64}\byedpi.exe"
    File "${SRCDIR_AMD64}\wintun.dll"
  ${EndIf}

  WriteRegStr HKLM "Software\${COMPANY}\WINGSV-Dex" "InstallDir" "$INSTDIR"
  WriteUninstaller "$INSTDIR\uninstall.exe"

  ; Add/Remove Programs entry.
  WriteRegStr HKLM "${REGKEY}" "DisplayName" "${APPNAME}"
  WriteRegStr HKLM "${REGKEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "${REGKEY}" "Publisher" "${COMPANY}"
  WriteRegStr HKLM "${REGKEY}" "DisplayIcon" "$INSTDIR\${EXENAME}"
  WriteRegStr HKLM "${REGKEY}" "UninstallString" "$INSTDIR\uninstall.exe"
  WriteRegDWORD HKLM "${REGKEY}" "NoModify" 1
  WriteRegDWORD HKLM "${REGKEY}" "NoRepair" 1

  CreateDirectory "$SMPROGRAMS\${APPNAME}"
  CreateShortcut "$SMPROGRAMS\${APPNAME}\${APPNAME}.lnk" "$INSTDIR\${EXENAME}"
  CreateShortcut "$SMPROGRAMS\${APPNAME}\Uninstall ${APPNAME}.lnk" "$INSTDIR\uninstall.exe"
  CreateShortcut "$DESKTOP\${APPNAME}.lnk" "$INSTDIR\${EXENAME}"
SectionEnd

Section "Uninstall"
  SetShellVarContext all
  Delete "$INSTDIR\${EXENAME}"
  Delete "$INSTDIR\vkturn.exe"
  Delete "$INSTDIR\xray.exe"
  Delete "$INSTDIR\byedpi.exe"
  Delete "$INSTDIR\wintun.dll"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"

  Delete "$SMPROGRAMS\${APPNAME}\${APPNAME}.lnk"
  Delete "$SMPROGRAMS\${APPNAME}\Uninstall ${APPNAME}.lnk"
  RMDir "$SMPROGRAMS\${APPNAME}"
  Delete "$DESKTOP\${APPNAME}.lnk"

  DeleteRegKey HKLM "${REGKEY}"
  DeleteRegKey HKLM "Software\${COMPANY}\WINGSV-Dex"
SectionEnd
