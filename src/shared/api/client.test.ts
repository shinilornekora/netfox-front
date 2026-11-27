import { describe, expect, it, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/setup';
import { ApiError, onUnauthorized, request, requestRaw } from './client';

describe('HTTP-клиент', () => {
  it('шлёт X-Plenka-Client на каждый запрос (CSRF-защита gateway)', async () => {
    let header: string | null = null;
    server.use(
      http.post('*/api/ping', ({ request: r }) => {
        header = r.headers.get('X-Plenka-Client');
        return HttpResponse.json({ ok: true });
      }),
    );
    await request('/ping', { method: 'POST', body: {} });
    expect(header).toBe('netfox/1');
  });

  it('разбирает problem+json: код, текст и ошибки полей', async () => {
    server.use(
      http.post('*/api/ping', () =>
        HttpResponse.json(
          {
            status: 422,
            code: 'invalid',
            title: 'Проверьте данные формы',
            errors: { email: 'Неверный email' },
          },
          { status: 422 },
        ),
      ),
    );
    const e = await request('/ping', { method: 'POST' }).catch((x: unknown) => x);
    expect(e).toBeInstanceOf(ApiError);
    expect(e).toMatchObject({
      status: 422,
      code: 'invalid',
      message: 'Проверьте данные формы: Неверный email',
    });
    expect((e as ApiError).fields).toEqual({ email: 'Неверный email' });
  });

  it('добавляет detail к тексту ошибки', async () => {
    server.use(
      http.get('*/api/ping', () =>
        HttpResponse.json(
          { status: 503, code: 'down', title: 'Сервис недоступен', detail: 'billing' },
          { status: 503 },
        ),
      ),
    );
    await expect(request('/ping')).rejects.toThrow('Сервис недоступен. billing');
  });

  it('не-JSON ошибка и обрыв сети', async () => {
    server.use(http.get('*/api/ping', () => new HttpResponse('oops', { status: 500 })));
    await expect(request('/ping')).rejects.toMatchObject({ status: 500, code: 'unknown' });
    server.use(http.get('*/api/ping', () => HttpResponse.error()));
    await expect(request('/ping')).rejects.toMatchObject({ status: 0, code: 'network' });
  });

  it('401 вне /auth — сигнал о потере сессии', async () => {
    const lost = vi.fn();
    onUnauthorized(lost);
    server.use(
      http.get('*/api/me', () =>
        HttpResponse.json({ status: 401, code: 'unauthenticated', title: 'Войдите' }, { status: 401 }),
      ),
    );
    await expect(request('/me')).rejects.toBeInstanceOf(ApiError);
    expect(lost).toHaveBeenCalledOnce();
  });

  it('requestRaw отдаёт статус, 204 — без тела, query без пустых значений', async () => {
    let url = '';
    server.use(
      http.get('*/api/list', ({ request: r }) => {
        url = r.url;
        return HttpResponse.json([], { status: 202 });
      }),
      http.delete('*/api/x', () => new HttpResponse(null, { status: 204 })),
    );
    const r = await requestRaw('/list', { query: { q: 'a b', empty: '', no: undefined, off: false, n: 0 } });
    expect(r.status).toBe(202);
    expect(new URL(url).search).toBe('?q=a+b&n=0');
    await expect(request('/x', { method: 'DELETE' })).resolves.toBeUndefined();
  });
});
