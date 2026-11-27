@echo off
chcp 65001 >nul
title Netfox
cd /d "%~dp0"
echo === Netfox: проверяю Docker ===
docker version >nul 2>&1
if errorlevel 1 (
  echo Docker не запущен. Откройте Docker Desktop, дождитесь зелёного статуса и запустите этот файл снова.
  pause
  exit /b 1
)
echo === Сборка и запуск (первый раз 3-6 минут) ===
docker compose up -d --build
if errorlevel 1 (
  echo Не удалось поднять контейнеры, см. ошибку выше.
  pause
  exit /b 1
)
echo === Жду, пока поднимется сайт и наполнится каталог ===
set n=0
:wait
set /a n+=1
if %n% gtr 100 (
  echo Сайт не ответил за 5 минут. Логи: docker compose logs gateway seed
  pause
  exit /b 1
)
curl -sf http://localhost:8080/api/titles?limit=1 2>nul | findstr /c:"\"id\"" >nul
if errorlevel 1 (
  timeout /t 3 >nul
  goto wait
)
echo.
echo Готово: http://localhost:8080
echo Зритель: зарегистрируйтесь на сайте. Админ: admin@plenka.local / admin12345
echo Карта для оплаты: 4242 4242 4242 4242
start "" http://localhost:8080
pause
