@echo off
chcp 65001 >nul
cd /d "%~dp0"
docker compose down
echo Остановлено. Данные сохранены; чтобы стереть и базу: docker compose down -v
pause
