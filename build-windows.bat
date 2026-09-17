@echo off
setlocal EnableExtensions

cd /d "%~dp0"
title glider - Windows Build

echo ============================================================
echo  glider - Windows Build
echo ============================================================
echo.

where go >nul 2>nul
if errorlevel 1 (
    echo [ERROR] Go was not found in PATH.
    echo Please install Go or add it to PATH, then run this file again.
    goto :FAIL
)

for /f "delims=" %%V in ('go version 2^>nul') do set "GO_VERSION=%%V"
echo [INFO] %GO_VERSION%

set "GIT_BRANCH=unknown"
set "GIT_COMMIT=unknown"
where git >nul 2>nul
if not errorlevel 1 (
    for /f "delims=" %%B in ('git branch --show-current 2^>nul') do set "GIT_BRANCH=%%B"
    for /f "delims=" %%C in ('git rev-parse --short HEAD 2^>nul') do set "GIT_COMMIT=%%C"
)

echo [INFO] Branch : %GIT_BRANCH%
echo [INFO] Commit : %GIT_COMMIT%
echo.

if not exist "build" mkdir "build"
if errorlevel 1 (
    echo [ERROR] Could not create the build directory.
    goto :FAIL
)

if exist "build\glider.exe" (
    echo [INFO] Removing previous build\glider.exe ...
    del /f /q "build\glider.exe" >nul 2>nul
    if exist "build\glider.exe" (
        echo [ERROR] Could not remove the old glider.exe.
        echo [HINT] Make sure glider.exe is not still running.
        goto :FAIL
    )
)

echo [BUILD] Compiling glider.exe ...
echo.
go build -trimpath -ldflags="-s -w" -o "build\glider.exe" .
if errorlevel 1 goto :FAIL

if not exist "build\glider.exe" (
    echo [ERROR] Go reported success but build\glider.exe was not found.
    goto :FAIL
)

for %%F in ("build\glider.exe") do set "EXE_SIZE=%%~zF"

echo.
echo ============================================================
echo  BUILD SUCCESS
echo ============================================================
echo  Branch : %GIT_BRANCH%
echo  Commit : %GIT_COMMIT%
echo  Output : %CD%\build\glider.exe
echo  Size   : %EXE_SIZE% bytes
echo ============================================================
echo.
pause
exit /b 0

:FAIL
echo.
echo ============================================================
echo  BUILD FAILED
echo ============================================================
echo Copy or screenshot the error above for troubleshooting.
echo ============================================================
echo.
pause
exit /b 1
