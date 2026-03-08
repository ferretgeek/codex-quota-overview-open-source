@echo off
setlocal EnableExtensions EnableDelayedExpansion
cd /d "%~dp0"
title Codex Overview - Install Environment

set "APP_ROOT=%CD%"
set "WEB_DIR=%APP_ROOT%\web"
set "BACKEND_DIR=%APP_ROOT%\backend"
set "BIN_DIR=%APP_ROOT%\bin"
set "LOG_DIR=%APP_ROOT%\logs"
set "SERVER_EXE=%BIN_DIR%\codex-overview-server.exe"
set "GOROOT="

call :print_header

if not exist "%WEB_DIR%" (
  echo [ERROR] Web directory was not found.
  goto :error
)

if not exist "%BACKEND_DIR%" (
  echo [ERROR] Backend directory was not found.
  goto :error
)

call :require_command go "Go is required before installation can continue."
if errorlevel 1 goto :error

call :require_command npm "Node.js and npm are required before installation can continue."
if errorlevel 1 goto :error

if not exist "%BIN_DIR%" mkdir "%BIN_DIR%"
if not exist "%LOG_DIR%" mkdir "%LOG_DIR%"

echo [STEP 1/5] Checking Go.
go version
if errorlevel 1 goto :error

echo [STEP 2/5] Checking npm.
call npm --version
if errorlevel 1 goto :error

echo [STEP 3/5] Installing frontend dependencies.
pushd "%WEB_DIR%"
if exist package-lock.json (
  call npm ci
) else (
  call npm install
)
if errorlevel 1 (
  popd
  echo [ERROR] Frontend dependency installation failed.
  goto :error
)

echo [STEP 4/5] Building the frontend.
call npm run build
if errorlevel 1 (
  popd
  echo [ERROR] Frontend build failed.
  goto :error
)
popd

echo [STEP 5/5] Building the backend executable.
pushd "%BACKEND_DIR%"
go build -o "%SERVER_EXE%" ./cmd/server
if errorlevel 1 (
  popd
  echo [ERROR] Backend build failed.
  goto :error
)
popd

echo.
echo [DONE] Environment setup completed successfully.
echo [INFO] Frontend build: %WEB_DIR%\dist
echo [INFO] Backend executable: %SERVER_EXE%
echo [INFO] Next step: run the start BAT file in this folder.
echo.
echo Press any key to close this window.
pause >nul
exit /b 0

:require_command
where %~1 >nul 2>nul
if errorlevel 1 (
  echo [ERROR] %~2
  exit /b 1
)
exit /b 0

:error
echo.
echo Press any key to close this window.
pause >nul
exit /b 1

:print_header
echo ==========================================
echo Codex Overview - Install Environment
echo ==========================================
echo.
exit /b 0
