@echo off
title Claudia - Stop
echo.
echo  Stopping Claudia processes...
echo.

:: Close the specific cmd windows started by our launchers (least destructive).
taskkill /f /fi "WINDOWTITLE eq Claudia PWA*" >nul 2>nul
taskkill /f /fi "WINDOWTITLE eq Claudia Orchestrator*" >nul 2>nul
taskkill /f /fi "WINDOWTITLE eq Claudia Ingest*" >nul 2>nul
taskkill /f /fi "WINDOWTITLE eq Claudia Receiver*" >nul 2>nul

:: Stop gateway native processes if running.
taskkill /f /im claudia-desktop.exe >nul 2>nul
taskkill /f /im claudia.exe >nul 2>nul

echo  Done.
echo.
pause

testing <3