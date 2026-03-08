@echo off
setlocal EnableExtensions EnableDelayedExpansion
cd /d "%~dp0"
title Codex Overview - Stop Service

set "APP_ROOT=%CD%"
set "PID_FILE=%APP_ROOT%\.backend.pid"
set "PORT=8787"
set "STOPPED="
set "FOUND_PID="

call :print_header

if exist "%PID_FILE%" (
  set /p FOUND_PID=<"%PID_FILE%"
  if defined FOUND_PID (
    echo [STEP 1/3] Trying PID from file: !FOUND_PID!
    powershell -NoProfile -ExecutionPolicy Bypass -Command "try { Stop-Process -Id !FOUND_PID! -Force -ErrorAction Stop; exit 0 } catch { exit 1 }" >nul 2>nul
    if not errorlevel 1 (
      echo [INFO] Process !FOUND_PID! was stopped.
      set "STOPPED=1"
    ) else (
      echo [INFO] PID file exists, but the process is no longer running.
    )
  )
  del "%PID_FILE%" >nul 2>nul
) else (
  echo [STEP 1/3] No PID file was found.
)

echo [STEP 2/3] Checking port %PORT% for any remaining listener.
for /f "tokens=5" %%P in ('netstat -ano ^| findstr /R /C:":%PORT% .*LISTENING"') do (
  echo [INFO] Found listener on port %PORT% with PID %%P.
  powershell -NoProfile -ExecutionPolicy Bypass -Command "try { Stop-Process -Id %%P -Force -ErrorAction Stop; exit 0 } catch { exit 1 }" >nul 2>nul
  if not errorlevel 1 (
    echo [INFO] Process %%P was stopped.
    set "STOPPED=1"
  ) else (
    echo [WARN] Failed to stop process %%P.
  )
)

echo [STEP 3/3] Final result.
if defined STOPPED (
  echo [DONE] The service has been stopped.
) else (
  echo [DONE] No running service was found.
)

echo.
echo Press any key to close this window.
pause >nul
exit /b 0

:print_header
echo ==========================================
echo Codex Overview - Stop Service
echo ==========================================
echo.
exit /b 0
