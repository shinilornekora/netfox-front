#!/usr/bin/env sh
# Netfox одной командой (Linux, macOS): ./start.sh
set -e
cd "$(dirname "$0")"
docker version >/dev/null 2>&1 || { echo "Docker не запущен — запустите Docker и повторите."; exit 1; }
echo "=== Сборка и запуск (первый раз 3-6 минут) ==="
docker compose up -d --build
echo "=== Жду, пока поднимется сайт и наполнится каталог ==="
i=0
until curl -sf 'http://localhost:8080/api/titles?limit=1' 2>/dev/null | grep -q '"id"'; do
  i=$((i + 1))
  if [ "$i" -gt 100 ]; then echo "Сайт не ответил за 5 минут. Логи: docker compose logs gateway seed"; exit 1; fi
  sleep 3
done
echo
echo "Готово: http://localhost:8080"
echo "Зритель: зарегистрируйтесь на сайте. Админ: admin@plenka.local / admin12345"
echo "Карта для оплаты: 4242 4242 4242 4242"
