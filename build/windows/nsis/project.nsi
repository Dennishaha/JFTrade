Unicode true
!include "wails_tools.nsh"

!ifndef OUTPUT_EXE
  !error "OUTPUT_EXE is required"
!endif
!ifndef JFTRADE_LICENSE_FILE
  !error "JFTRADE_LICENSE_FILE is required"
!endif
!ifndef JFTRADE_THIRD_PARTY_NOTICES_FILE
  !error "JFTRADE_THIRD_PARTY_NOTICES_FILE is required"
!endif

VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion "${INFO_PRODUCTVERSION}.0"
VIAddVersionKey "CompanyName" "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion" "${INFO_PRODUCTVERSION}"
VIAddVersionKey "ProductName" "${INFO_PRODUCTNAME}"
ManifestDPIAware true

!include "MUI2.nsh"
!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

Name "${INFO_PRODUCTNAME}"
OutFile "${OUTPUT_EXE}"
InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
ShowInstDetails show

Function .onInit
  !insertmacro wails.checkArchitecture
FunctionEnd

Section
  !insertmacro wails.setShellContext
  !insertmacro wails.webview2runtime
  SetOutPath $INSTDIR
  !insertmacro wails.files
  SetOutPath "$INSTDIR\licenses"
  File /oname=LICENSE "${JFTRADE_LICENSE_FILE}"
  File /oname=THIRD-PARTY-NOTICES.md "${JFTRADE_THIRD_PARTY_NOTICES_FILE}"
  SetOutPath $INSTDIR
  CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
  CreateShortcut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
  !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall"
  !insertmacro wails.setShellContext
  RMDir /r "$AppData\${PRODUCT_EXECUTABLE}"
  RMDir /r $INSTDIR
  Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
  Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"
  !insertmacro wails.deleteUninstaller
SectionEnd
