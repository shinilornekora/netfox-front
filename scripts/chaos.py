"""Деградация: billing остановлен — playback живёт на кэше решений (stale-if-error).

Запуск: python3 scripts/chaos.py BASE  — скрипт сам попросит остановить billing.
"""
import sys
import uuid

from plenka_client import Client

BASE = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080"
c = Client(BASE)
c.ok("POST", "/api/auth/register", {"email": f"chaos-{uuid.uuid4().hex[:6]}@plenka.local", "password": "chaos-pass-2026",
                                    "display_name": "Хаос", "birth_date": "1990-01-01"})
c.ok("POST", "/api/billing/checkout", {"kind": "subscription", "plan_id": "month", "card_number": "4242424242424242"},
     headers={"Idempotency-Key": Client.key()})
items = c.ok("GET", "/api/titles?limit=50&access=subscription")["items"]
a, b = items[0], items[1]
c.ok("POST", "/api/playback/start", {"title_id": a["id"]})
input("Остановите billing и нажмите Enter… ") if sys.stdin.isatty() else sys.stdin.readline()
st, js = c.call("POST", "/api/playback/start", {"title_id": a["id"]})
print("уже открытый фильм:", st, js.get("via") if isinstance(js, dict) else js)
assert st == 200, "уже открытый фильм должен играть из кэша решений (stale-if-error)"
st, js = c.call("POST", "/api/playback/start", {"title_id": b["id"]})
print("новый фильм:", st, js.get("code") if isinstance(js, dict) else js)
assert st == 503 and js.get("code") == "access_unavailable", "новый фильм без billing — 503 (fail closed)"
print("CHAOS OK — 2 проверки")
