@echo off
REM Simple Syncthing Windows Build Script
REM This script compiles Syncthing for Windows with default settings

echo ========================================
echo Syncthing Windows Build Script
echo ========================================

REM Check if Go is installed
go version >nul 2>&1
if %errorlevel% neq 0 (
    echo Error: Go is not installed or not in PATH
    echo Please install Go from https://golang.org/dl/
    pause
    exit /b 1
)

REM Install required tools
echo Installing required tools...
go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest
if %errorlevel% neq 0 (
    echo Warning: Failed to install goversioninfo. Windows binaries will not have file information.
)

REM Set environment variables for Windows build
echo Setting environment variables...
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64

echo Building for Windows 64-bit...

REM Create bin directory if it doesn't exist
if not exist "bin" mkdir bin

REM Build Syncthing
go build -o bin/syncthing.exe github.com/syncthing/syncthing/cmd/syncthing

if %errorlevel% equ 0 (
    echo.
    echo Build successful!
    echo Binary location: bin\syncthing.exe
    echo.
    echo To run Syncthing:
    echo   cd bin
    echo   syncthing.exe
    echo.
    echo Build completed successfully!
) else (
    echo.
    echo Build failed!
    echo Error code: %errorlevel%
    echo.
    pause
    exit /b %errorlevel%
)

pause