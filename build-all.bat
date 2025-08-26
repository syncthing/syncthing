@echo off
REM Syncthing Complete Build Script for Windows
REM This script compiles all Syncthing binaries for Windows

echo =====================================================
echo Syncthing Complete Windows Build Script
echo =====================================================

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

REM Create bin directory if it doesn't exist
if not exist "bin" mkdir bin

echo.
echo Building all Syncthing binaries for Windows...
echo.

REM Build main Syncthing binary
echo 1. Building syncthing...
go build -o bin/syncthing.exe github.com/syncthing/syncthing/cmd/syncthing
if %errorlevel% neq 0 (
    echo Error building syncthing!
    pause
    exit /b %errorlevel%
)
echo    ^> Success!

REM Build discovery server
echo 2. Building stdiscosrv...
go build -o bin/stdiscosrv.exe github.com/syncthing/syncthing/cmd/stdiscosrv
if %errorlevel% neq 0 (
    echo Error building stdiscosrv!
    pause
    exit /b %errorlevel%
)
echo    ^> Success!

REM Build relay server
echo 3. Building strelaysrv...
go build -o bin/strelaysrv.exe github.com/syncthing/syncthing/cmd/strelaysrv
if %errorlevel% neq 0 (
    echo Error building strelaysrv!
    pause
    exit /b %errorlevel%
)
echo    ^> Success!

REM Build relay pool server
echo 4. Building strelaypoolsrv...
go build -o bin/strelaypoolsrv.exe github.com/syncthing/syncthing/cmd/infra/strelaypoolsrv
if %errorlevel% neq 0 (
    echo Error building strelaypoolsrv!
    pause
    exit /b %errorlevel%
)
echo    ^> Success!

REM Build upgrade server
echo 5. Building stupgrades...
go build -o bin/stupgrades.exe github.com/syncthing/syncthing/cmd/infra/stupgrades
if %errorlevel% neq 0 (
    echo Error building stupgrades!
    pause
    exit /b %errorlevel%
)
echo    ^> Success!

REM Build crash receiver
echo 6. Building stcrashreceiver...
go build -o bin/stcrashreceiver.exe github.com/syncthing/syncthing/cmd/infra/stcrashreceiver
if %errorlevel% neq 0 (
    echo Error building stcrashreceiver!
    pause
    exit /b %errorlevel%
)
echo    ^> Success!

REM Build usage reporting server
echo 7. Building ursrv...
go build -o bin/ursrv.exe github.com/syncthing/syncthing/cmd/infra/ursrv
if %errorlevel% neq 0 (
    echo Error building ursrv!
    pause
    exit /b %errorlevel%
)
echo    ^> Success!

echo.
echo =====================================================
echo All Syncthing binaries built successfully!
echo =====================================================
echo Binaries created in the bin directory:
echo   - syncthing.exe         (Main Syncthing application)
echo   - stdiscosrv.exe        (Discovery server)
echo   - strelaysrv.exe        (Relay server)
echo   - strelaypoolsrv.exe    (Relay pool server)
echo   - stupgrades.exe        (Upgrade server)
echo   - stcrashreceiver.exe   (Crash receiver)
echo   - ursrv.exe             (Usage reporting server)
echo =====================================================
echo.
echo Build completed successfully!
pause