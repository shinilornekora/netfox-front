@echo off
chcp 65001 >nul
cd /d "%~dp0"
echo === Netfox: полный сброс — база и Kafka будут очищены ===
docker compose down -v
call "%~dp0start.bat"
