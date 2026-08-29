@echo off
setlocal
cd /d "%~dp0"

where make >nul 2>nul
if not errorlevel 1 (
    echo Starting Porcelain desktop launcher...
    make locus-desktop-run
    exit /b %errorlevel%
)

where bash >nul 2>nul
if not errorlevel 1 (
    echo Starting Porcelain desktop launcher via Git Bash...
    bash -lc "make locus-desktop-run"
    exit /b %errorlevel%
)

echo.
echo Porcelain launcher needs make or Git Bash on PATH.
echo Install Git for Windows, then re-run this file.
echo.
pause
exit /b 1
