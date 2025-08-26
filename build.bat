@echo off
REM Syncthing Windows Build Script
REM This script compiles Syncthing for Windows

SETLOCAL

:menu
cls
echo ========================================
echo Syncthing Windows Build Script
echo ========================================
echo.
echo Select an option:
echo 1. Build for Windows 64-bit (default)
echo 2. Build for Windows 32-bit
echo 3. Build all Windows versions
echo 4. Run tests
echo 5. Clean build directory
echo 6. Exit
echo.

choice /c 123456 /m "Enter your choice"

if errorlevel 6 goto :exit
if errorlevel 5 goto :clean
if errorlevel 4 goto :test
if errorlevel 3 goto :buildall
if errorlevel 2 goto :build32
if errorlevel 1 goto :build64

goto :menu

:build64
echo Building for Windows 64-bit (amd64)...
call :build amd64
if %errorlevel% neq 0 (
    echo Build failed!
    pause
    exit /b %errorlevel%
)
goto :menu

:build32
echo Building for Windows 32-bit (386)...
call :build 386
if %errorlevel% neq 0 (
    echo Build failed!
    pause
    exit /b %errorlevel%
)
goto :menu

:buildall
echo Building all Windows versions...
call :build amd64
call :build 386
goto :menu

:test
echo Running tests...
set CGO_ENABLED=0
set GOOS=
set GOARCH=
go test ./...
goto :menu

:clean
echo Cleaning build directory...
if exist "bin" (
    rmdir /s /q "bin"
    echo Build directory cleaned.
) else (
    echo Build directory does not exist.
)
goto :menu

:build
set GOARCH=%1
set CGO_ENABLED=0
set GOOS=windows

echo Installing required tools...
go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest >nul 2>&1
if %errorlevel% neq 0 (
    echo Warning: Failed to install goversioninfo. Windows binaries will not have file information.
)

echo Building syncthing for GOOS=%GOOS% GOARCH=%GOARCH%...

REM Create bin directory if it doesn't exist
if not exist "bin" mkdir bin

REM Build Syncthing
if "%GOARCH%"=="386" (
    go build -o bin/syncthing-x86.exe github.com/syncthing/syncthing/cmd/syncthing
    if %errorlevel% equ 0 (
        echo Build successful! Binary: bin/syncthing-x86.exe
    ) else (
        echo Build failed for 386 architecture!
        exit /b %errorlevel%
    )
) else (
    go build -o bin/syncthing.exe github.com/syncthing/syncthing/cmd/syncthing
    if %errorlevel% equ 0 (
        echo Build successful! Binary: bin/syncthing.exe
    ) else (
        echo Build failed for amd64 architecture!
        exit /b %errorlevel%
    )
)

exit /b 0

:exit
echo Exiting...
ENDLOCAL
exit /b 0