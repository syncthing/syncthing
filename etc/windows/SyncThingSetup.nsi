;--------------------------------
;Include Modern UI

  !include "MUI2.nsh"

;--------------------------------
;General

  ;Name and file
  !define SOURCEPATH "C:\SourceCode\SyncThing\Binaries"
  
  Name "SyncThing Windows Service Install"
  OutFile "SyncThingSetup.exe"

  ;Default installation folder
  InstallDir "$PROGRAMFILES\SyncThing"
    
  ;Get installation folder from registry if available
  InstallDirRegKey HKCU "Software\SyncThing" ""

  ;Request application privileges for Windows Vista
  RequestExecutionLevel admin

;--------------------------------
;Interface Settings

  !define MUI_ABORTWARNING

;--------------------------------
;Pages

  !insertmacro MUI_PAGE_COMPONENTS
  !insertmacro MUI_PAGE_DIRECTORY
  !insertmacro MUI_PAGE_INSTFILES
  
  !insertmacro MUI_UNPAGE_CONFIRM
  !insertmacro MUI_UNPAGE_INSTFILES
  
;--------------------------------
;Languages
 
  !insertmacro MUI_LANGUAGE "English"

;--------------------------------
;Installer Sections

Section "SyncThing" SecSyncThing
  SectionIn RO
  SetOutPath "$INSTDIR"
  
  IfFileExists syncthingservice.exe 0 +2
	SimpleSC::StopService "SyncThing" 1 30	
  
  File /r "${SOURCEPATH}\syncthing.exe"
  File /r "${SOURCEPATH}\syncthing.exe.md5"
  File /r "${SOURCEPATH}\AUTHORS.txt"
  File /r "${SOURCEPATH}\LICENSE.txt"
  File /r "${SOURCEPATH}\README.txt"
  File /r "${SOURCEPATH}\FAQ.pdf"
  File /r "${SOURCEPATH}\Getting-Started.pdf"
    
  ;Store installation folder
  WriteRegStr HKCU "Software\SyncThing" "" $INSTDIR
  
  ;Create uninstaller
  WriteUninstaller "$INSTDIR\Uninstall.exe"

SectionEnd

Section "Command Line Interface" SecSyncThingCLI

  SetOutPath "$INSTDIR"
  
  File /r "${SOURCEPATH}\syncthing-cli.exe"  
  
SectionEnd

Section "Windows Service" SecSyncThingService

  SetOutPath "$INSTDIR"
    
  File /r "${SOURCEPATH}\syncthingservice.exe"  
  File /r "${SOURCEPATH}\syncthingservice.xml"  
  
  ExecWait 'syncthingservice.exe install'
  ExecWait 'syncthingservice.exe start'
 
SectionEnd

;--------------------------------
;Descriptions

  ;Language strings
  LangString DESC_SecSyncThing ${LANG_ENGLISH} "SyncThing"
  LangString DESC_SecSyncThingCLI ${LANG_ENGLISH} "Command Line Interface"
  LangString DESC_SecSyncThingService ${LANG_ENGLISH} "Windows Service"

  ;Assign language strings to sections
  !insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
    !insertmacro MUI_DESCRIPTION_TEXT ${SecSyncThing} $(DESC_SecSyncThing)
	!insertmacro MUI_DESCRIPTION_TEXT ${SecSyncThingCLI} $(DESC_SecSyncThingCLI)
	!insertmacro MUI_DESCRIPTION_TEXT ${SecSyncThingService} $(DESC_SecSyncThingService)
  !insertmacro MUI_FUNCTION_DESCRIPTION_END

;--------------------------------
;Uninstaller Section

Section "Uninstall"
  
  Delete "$INSTDIR\Uninstall.exe"

  RMDir "$INSTDIR"

  DeleteRegKey /ifempty HKCU "Software\SyncThing"

SectionEnd