SET NET_FRAMEWORK_DIR=%WINDIR%\Microsoft.NET\Framework\v4.0.30319
CALL "%VS120COMNTOOLS%..\..\VC\vcvarsall.bat"
msbuild.exe msbuild.proj /t:BuildMsiPack
pause