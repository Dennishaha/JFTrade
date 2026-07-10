Unicode True
!include "MUI2.nsh"
!include "x64.nsh"

!ifndef VERSION
  !error "VERSION is required"
!endif
!ifndef SOURCE_EXE
  !error "SOURCE_EXE is required"
!endif
!ifndef OUTPUT_EXE
  !error "OUTPUT_EXE is required"
!endif

Name "JFTrade"
OutFile "${OUTPUT_EXE}"
InstallDir "$LOCALAPPDATA\Programs\JFTrade"
InstallDirRegKey HKCU "Software\JFTrade" "InstallDir"
RequestExecutionLevel user
SetCompressor /SOLID lzma

!define MUI_ABORTWARNING
!define MUI_ICON "${__FILEDIR__}\icon.ico"
!define MUI_UNICON "${__FILEDIR__}\icon.ico"
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

VIProductVersion "${VERSION}.0"
VIAddVersionKey /LANG=2052 "ProductName" "JFTrade"
VIAddVersionKey /LANG=2052 "ProductVersion" "${VERSION}"
VIAddVersionKey /LANG=2052 "CompanyName" "JFTrade"
VIAddVersionKey /LANG=2052 "FileDescription" "JFTrade per-user installer"

Section "JFTrade" SEC_MAIN
  SetShellVarContext current
  SetOutPath "$INSTDIR"
  File /oname=JFTrade.exe "${SOURCE_EXE}"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  WriteRegStr HKCU "Software\JFTrade" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\JFTrade" "DisplayName" "JFTrade"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\JFTrade" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\JFTrade" "Publisher" "JFTrade"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\JFTrade" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\JFTrade" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  CreateDirectory "$SMPROGRAMS\JFTrade"
  CreateShortcut "$SMPROGRAMS\JFTrade\JFTrade.lnk" "$INSTDIR\JFTrade.exe"
  CreateShortcut "$DESKTOP\JFTrade.lnk" "$INSTDIR\JFTrade.exe"
SectionEnd

Section "Uninstall"
  SetShellVarContext current
  Delete "$DESKTOP\JFTrade.lnk"
  Delete "$SMPROGRAMS\JFTrade\JFTrade.lnk"
  RMDir "$SMPROGRAMS\JFTrade"
  Delete "$INSTDIR\JFTrade.exe"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\JFTrade"
  DeleteRegKey HKCU "Software\JFTrade"
SectionEnd
