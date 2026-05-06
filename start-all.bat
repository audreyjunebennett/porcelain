@echo off
title Claudia - Start All
echo.
echo  Starting Claudia stack...
echo.

:: Clear stale Claudia processes so relaunching works even after a bad prior exit
taskkill /f /im claudia-desktop.exe >nul 2>nul
taskkill /f /im claudia.exe >nul 2>nul

:: Receiver - transcription, port 8765
start "Claudia Receiver" cmd /k py -3.14 "D:\Rebirth\Moto X\receiver.py"

:: Small delay so receiver loads first (Whisper model takes a moment)
timeout /t 3 /nobreak >nul

:: Gateway - claudia-desktop.exe opens its own native window (no terminal)
:: Supervises Bifrost + Qdrant internally; keep BiFrost on 8090 so the PWA can stay on 8080
start "" /d "D:\Rebirth\claudia-gateway" claudia-desktop.exe desktop ^
  --bifrost-bin "bin\bifrost-http.exe" ^
  --bifrost-config "config\bifrost.config.json" ^
  --bifrost-data-dir "data\bifrost" ^
  --bifrost-port 8090 ^
  --bifrost-bind 127.0.0.1 ^
  --upstream-host 127.0.0.1 ^
  --qdrant-bin "bin\qdrant.exe" ^
  --qdrant-storage "data\qdrant" ^
  --qdrant-bind 127.0.0.1 ^
  --qdrant-http-port 6333 ^
  --config "config\gateway.yaml"

:: Ingest watcher - polls every 30s, pushes Moto X transcripts + D:\Notes into Qdrant
start "Claudia Ingest" cmd /k "py -3.14 D:\Rebirth\ingest_watcher.py"

:: PWA server - legacy tabbed app + APIs, port 8080
start "Claudia PWA" cmd /k "py -3.14 D:\Rebirth\pwa\server.py"

:: Orchestrator - original PWA + conversations + auth, port 11435
start "Claudia Orchestrator" cmd /k "cd /d ""D:\Rebirth\Previously Claudia Core"" && py -3.14 Scripts\mobile_orchestrator_api.py"

echo.
echo  All services started.
echo.
echo  Gateway panel:                              http://localhost:3000/ui/panel
echo  Original PWA (with sidebar/conversations):  https://localhost:11435/web
echo  Legacy tabbed app:                          http://localhost:8080/legacy-app
echo  iPhone: use Tailscale IP shown in PWA window.
echo.
pause
