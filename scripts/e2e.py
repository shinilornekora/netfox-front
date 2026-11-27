"""Сквозной сценарий через gateway: зритель, подписка, покупка, просмотр, админ.

python3 scripts/e2e.py http://localhost:8080   (после seed.py)
"""
import sys
import time
import uuid

from plenka_client import Client

BASE = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080"
OK_CARD, DECLINED = "4242424242424242", "4000000000000002"
step = 0


def check(cond, msg):
    global step
    step += 1
    if not cond:
        raise SystemExit(f"FAIL {step}: {msg}")
    print(f"ok {step:2d}  {msg}")


def find(c, name):
    items = c.ok("GET", "/api/titles?limit=50&q=" + urllib_q(name))["items"]
    return next(t for t in items if t["name"] == name)


def urllib_q(s):
    import urllib.parse
    return urllib.parse.quote(s)


def waited(fn, timeout=10):
    """Секунды до выполнения условия или None, если не дождались."""
    start = time.time()
    while time.time() - start < timeout:
        if fn():
            return time.time() - start
        time.sleep(0.3)
    return None


def eventually(fn, what, timeout=10):
    end = time.time() + timeout
    while time.time() < end:
        if fn():
            return True
        time.sleep(0.3)
    raise SystemExit(f"FAIL: timeout waiting for {what}")


anon = Client(BASE)
st, _ = anon.call("POST", "/api/auth/login", {"email": "x@y.z", "password": "x"}, headers={"X-Plenka-Client": ""})
check(st == 403, "POST без X-Plenka-Client отклонён (CSRF)")
st, js = anon.call("GET", "/api/me")
check(st == 401, "аноним не видит кабинет")
cat = anon.ok("GET", "/api/titles?limit=6")
check(len(cat["items"]) == 6 and cat.get("next_cursor"), "витрина отдаёт страницу и курсор")
cat2 = anon.ok("GET", "/api/titles?limit=6&cursor=" + cat["next_cursor"])
check(not {t["id"] for t in cat["items"]} & {t["id"] for t in cat2["items"]}, "вторая страница не повторяет первую")

v = Client(BASE)
email = f"viewer-{uuid.uuid4().hex[:8]}@plenka.local"
st, js = v.call("POST", "/api/auth/register", {"email": email, "password": "short", "display_name": "Зритель", "birth_date": "1998-04-02"})
check(st == 422 and "password" in (js.get("errors") or {}), "слабый пароль: 422 с ошибкой по полю")
v.ok("POST", "/api/auth/register", {"email": email, "password": "kino-2026-pass", "display_name": "Зритель", "birth_date": "1998-04-02"})
me = v.ok("GET", "/api/me")
check(me["email"] == email and me["role"] == "viewer", "регистрация сразу открывает сессию")

film = find(v, "Генерал")  # в подписке, 78 минут
# Отдельный платный фильм на каждый прогон: сценарий в конце снимает его с показа.
a = Client(BASE)
a.ok("POST", "/api/auth/login", {"email": "admin@plenka.local", "password": "admin12345"})
t = a.ok("POST", "/api/admin/titles", {"kind": "film", "name": f"E2E {uuid.uuid4().hex[:6]}", "year": 2026, "age_rating": 16,
         "genres": ["триллер"], "duration_sec": 5400, "in_subscription": False, "price_kopecks": 29900,
         "poster_hue": 10, "description": "Тестовый фильм сквозного сценария."})
a.ok("POST", f"/api/admin/titles/{t['id']}/publish", {"revision": t["revision"]})
paid = t
eventually(lambda: v.call("GET", f"/api/billing/offers/{paid['id']}")[1].get("known"), "тайтл дошёл до billing")
st, js = v.call("POST", "/api/playback/start", {"title_id": film["id"]})
check(st == 402 or (js and js.get("code") == "not_entitled"), f"без подписки фильм не стартует ({st})")

plans = v.ok("GET", "/api/billing/plans")["items"]
month = next(p for p in plans if p["period_days"] <= 31)
st, js = v.call("POST", "/api/billing/checkout", {"kind": "subscription", "plan_id": month["id"], "card_number": DECLINED},
                headers={"Idempotency-Key": Client.key()})
check(st == 402 and js["status"] == "failed", "отклонённая карта: 402, заказ в статусе failed")
key = Client.key()
o1 = v.ok("POST", "/api/billing/checkout", {"kind": "subscription", "plan_id": month["id"], "card_number": OK_CARD},
          headers={"Idempotency-Key": key})
o2 = v.ok("POST", "/api/billing/checkout", {"kind": "subscription", "plan_id": month["id"], "card_number": OK_CARD},
          headers={"Idempotency-Key": key})
check(o1["status"] == "completed" and o1["id"] == o2["id"], "оплата подписки; повтор с тем же ключом не списывает второй раз")

s = v.ok("POST", "/api/playback/start", {"title_id": film["id"]})
check(s["via"] == "subscription" and s["resume_at_sec"] == 0, "подписка открыла фильм, старт с начала")
st, body, hdrs = v.call("GET", s["stream_url"], headers={"Range": "bytes=0-1023"}, raw=True)
check(st == 206 and len(body) == 1024 and hdrs["Content-Range"].startswith("bytes 0-1023/"), "поток отдаёт Range 206")
st, _, _ = v.call("GET", s["stream_url"][:-3] + "xyz", raw=True)
check(st in (401, 403, 404), "подделанный токен потока не принимается")

now = int(time.time() * 1000)
v.ok("PUT", "/api/playback/progress", {"title_id": film["id"], "position_sec": 1500, "duration_sec": 4680, "client_ms": now})
v.ok("PUT", "/api/playback/progress", {"title_id": film["id"], "position_sec": 30, "duration_sec": 4680, "client_ms": now - 5000})
s = v.ok("POST", "/api/playback/start", {"title_id": film["id"]})
check(s["resume_at_sec"] == 1495, "опоздавший пинг не откатил позицию; продолжение за 5 секунд до остановки")
cont = v.ok("GET", "/api/playback/continue")["items"]
check(any(i["title"]["id"] == film["id"] for i in cont), "фильм в «Продолжить просмотр»")

st, js = v.call("POST", "/api/playback/start", {"title_id": paid["id"]})
check(st == 402 and js.get("code") == "not_entitled", f"фильм вне подписки не открывается подпиской: 402 not_entitled ({st})")
offer = v.ok("GET", f"/api/billing/offers/{paid['id']}")
check(not offer["can_watch"] and offer["price_kopecks"] == 29900, "оффер: купить за 299 ₽")
o = v.ok("POST", "/api/billing/checkout", {"kind": "purchase", "title_id": paid["id"], "card_number": OK_CARD},
         headers={"Idempotency-Key": Client.key()})
check(o["status"] == "completed", "покупка фильма")
s = v.ok("POST", "/api/playback/start", {"title_id": paid["id"]})
check(s["via"] == "purchase", "купленный фильм открылся по покупке")

v.ok("PUT", f"/api/playback/watchlist/{paid['id']}")
v.ok("PUT", f"/api/playback/watchlist/{paid['id']}")
wl = v.ok("GET", "/api/playback/watchlist")["items"]
check(len(wl) == 1, "«Смотреть позже»: добавление идемпотентно")
w = v.ok("GET", "/api/billing/wallet")
check(w["subscription"] and len(w["purchases"]) == 1 and len(w["orders"]) >= 3, "кабинет: подписка, покупка, все заказы")
h = v.ok("GET", "/api/playback/history")["items"]
check(len(h) >= 3, "история просмотров")

# Возврат оплаченной покупки: billing забирает право в одной транзакции с
# completed → refunding, событие через outbox и Kafka сбрасывает кэш
# решений playback — без ожидания 5-минутного TTL.
r = Client(BASE)
r.ok("POST", "/api/auth/register", {"email": f"refund-{uuid.uuid4().hex[:6]}@plenka.local", "password": "refund-pass-2026",
                                    "display_name": "Возврат", "birth_date": "1990-01-01"})
ro = r.ok("POST", "/api/billing/checkout", {"kind": "purchase", "title_id": paid["id"], "card_number": OK_CARD},
          headers={"Idempotency-Key": Client.key()})
check(r.ok("POST", "/api/playback/start", {"title_id": paid["id"]})["via"] == "purchase", "второй зритель купил и открыл фильм")
st, _ = r.call("POST", f"/api/billing/admin/orders/{ro['id']}/refund")
check(st == 403, "зритель не может вернуть заказ сам")
back = a.ok("POST", f"/api/billing/admin/orders/{ro['id']}/refund", {"reason": "Жалоба на качество"})
check(back["status"] == "refunded", "админ вернул деньги: заказ refunded")
def denied(c, code, status):
    st, js = c.call("POST", "/api/playback/start", {"title_id": paid["id"]})
    return st == status and isinstance(js, dict) and js.get("code") == code


el = waited(lambda: denied(r, "not_entitled", 402), timeout=8)
check(el is not None, f"после возврата фильм не стартует: 402 not_entitled через {el or 0:.1f} с (ждём не дольше 8 с) — кэш сброшен событием, а не 5-минутным TTL")
check(not r.ok("GET", "/api/billing/wallet")["purchases"], "покупка исчезла из кабинета")

teen = Client(BASE)
teen.ok("POST", "/api/auth/register", {"email": f"teen-{uuid.uuid4().hex[:6]}@plenka.local", "password": "teen-pass-2026",
                                       "display_name": "Школьник", "birth_date": f"{time.localtime().tm_year - 14}-01-01"})
ages = {t["age_rating"] for t in teen.ok("GET", "/api/titles?limit=50")["items"]}
check(max(ages) <= 12, f"подростку 14 лет витрина не показывает 16+/18+ (макс {max(ages)})")

st, _ = v.call("GET", "/api/admin/titles")
check(st == 403, "зритель не попадает в админку")
t = a.ok("GET", f"/api/titles/{paid['id']}")
st, js = a.call("POST", f"/api/admin/titles/{paid['id']}/withdraw", {"reason": "Истекла лицензия", "revision": t["revision"] - 1})
check(st == 409, "снятие по устаревшей ревизии: 409")
a.ok("POST", f"/api/admin/titles/{paid['id']}/withdraw", {"reason": "Истекла лицензия", "revision": t["revision"]})
el = waited(lambda: denied(v, "not_available", 409))
check(el is not None, f"снятый с показа фильм не смотрится даже купившим: 409 not_available через {el or 0:.1f} с (событие через Kafka)")
st, _ = anon.call("GET", f"/api/titles/{paid['id']}")
check(st == 404, "снятый тайтл скрыт с витрины")
rev = a.ok("GET", "/api/billing/admin/revenue")
check(rev["total_kopecks"] >= 39900 + 29900, "выручка в админке учитывает оплаты")
v.ok("POST", "/api/auth/logout")
st, _ = v.call("GET", "/api/me")
check(st == 401, "выход: сессия сразу недействительна (кэш gateway сброшен)")
print(f"\nE2E OK — {step} проверок")
