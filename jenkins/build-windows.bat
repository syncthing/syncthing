@echo off

rem Copyright (C) 2016 The Syncthing Authors.
rem
rem This Source Code Form is subject to the terms of the Mozilla Public
rem License, v. 2.0. If a copy of the MPL was not distributed with this file,
rem You can obtain one at https://mozilla.org/MPL/2.0/.

rem This batch file should be run from the GOPATH.
rem It expects to run on amd64, for windows-amd64 Go to be installed in C:\go
rem and for windows-386 Go to be installed in C:\go-386.

rem cURL should be installed in C:\Program Files\cURL.

set ORIGPATH="C:\Program Files\cURL\bin";%PATH%
set PATH=c:\go\bin;%ORIGPATH%
set GOROOT=c:\go

cd >gopath
set /p GOPATH= <gopath

cd src\github.com\syncthing\syncthing

echo Initializing ^& cleaning
go version
git clean -fxd || goto error
go run build.go version
echo.

echo Fetching extras
mkdir extra
curl -s -L -o extra/Getting-Started.pdf https://docs.syncthing.net/pdf/Getting-Started.pdf || goto :error
curl -s -L -o extra/FAQ.pdf https://docs.syncthing.net/pdf/FAQ.pdf || goto :error
echo.

echo Testing
go run build.go test || goto :error
echo.

echo Building (amd64)
go run build.go zip || goto :error
echo.

set PATH=c:\go-386\bin;%ORIGPATH%
set GOROOT=c:\go-386

echo building (386)
go run build.go zip || goto :error
echo.

goto :EOF

:error
echo code #%errorlevel%.
exit /b %errorlevel%