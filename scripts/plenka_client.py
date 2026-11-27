"""Минимальный HTTP-клиент для сид- и e2e-скриптов: cookie-сессия + заголовок клиента."""
import http.cookiejar
import json
import urllib.error
import urllib.request
import uuid


class Client:
    def __init__(self, base):
        self.base = base.rstrip("/")
        self.jar = http.cookiejar.CookieJar()
        self.op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.jar))

    def call(self, method, path, body=None, headers=None, raw=False, data=None):
        h = {"X-Plenka-Client": "script", "Accept": "application/json"}
        if data is not None:
            h["Content-Type"] = "application/octet-stream"
        if body is not None:
            data = json.dumps(body).encode()
            h["Content-Type"] = "application/json"
        h.update(headers or {})
        req = urllib.request.Request(self.base + path, data=data, method=method, headers=h)
        try:
            with self.op.open(req, timeout=30) as r:
                payload = r.read()
                status, hdrs = r.status, r.headers
        except urllib.error.HTTPError as e:
            payload, status, hdrs = e.read(), e.code, e.headers
        if raw:
            return status, payload, hdrs
        try:
            js = json.loads(payload) if payload else None
        except ValueError:
            js = payload.decode(errors="replace")
        return status, js

    def ok(self, method, path, body=None, expect=(200, 201, 204), headers=None):
        st, js = self.call(method, path, body, headers)
        if st not in expect:
            raise AssertionError(f"{method} {path} -> {st}: {js}")
        return js

    @staticmethod
    def key():
        return str(uuid.uuid4())
