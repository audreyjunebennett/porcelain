@echo off
title Locus - Start All
echo.
echo  Starting Porcelain stack (Locus + Gateway + Ingest)...
echo.

:: Clear stale processes so relaunching works even after a bad prior exit
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

:: PWA server (8080) is optional. The original PWA lives on Locus (11435).
:: If you want the legacy tabbed UI / Rebirth shell, uncomment this.
:: start "Claudia PWA" cmd /k "py -3.14 D:\Rebirth\pwa\server.py"

:: Locus - creative workspace server (PWA + conversations + auth), port 11435
start "Locus Server" cmd /k "py -3.14 D:\Rebirth\pwa\tools\run_mobile_orchestrator.py"

echo.
echo  All services started.
echo.
echo  Gateway panel:                              http://localhost:3000/ui/panel
echo  Original PWA (with sidebar/conversations):  https://localhost:11435/web
echo  PWA shell + APIs (optional):                http://localhost:8080
echo  Legacy tabbed app (optional):               http://localhost:8080/legacy-app
echo  iPhone: use Tailscale IP shown in PWA window.
echo.
pause
