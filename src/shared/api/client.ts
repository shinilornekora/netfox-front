// HTTP-клиент к gateway Плёнки (/api).
//
// Сессия — HttpOnly-cookie plenka_sid: JS её не видит и не хранит, поэтому
// в приложении нет «токена», а состояние входа — это ответ /auth/session.
// Каждый запрос несёт X-Plenka-Client: без него gateway отклоняет изменяющие
// запросы (защита от CSRF). Если сервер ответил 401 на обычный запрос, значит
// сессия истекла или отозвана с другого устройства — сообщаем модели session.
import type { Problem } from './types';

export const API_BASE = '/api';
export const CLIENT_HEADER = 'X-Plenka-Client';
export const CLIENT_ID = 'netfox/1';
const TIMEOUT_MS = 15_000;

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly fields: Record<string, string> = {},
    /** Сырое тело ответа: у 402 на оплате там заказ, а не Problem. */
    public readonly body: unknown = undefined,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

const listeners = new Set<() => void>();

/** Подписка на потерю сессии (401 на запросе, который её требовал). */
export function onUnauthorized(l: () => void): () => void {
  listeners.add(l);
  return () => listeners.delete(l);
}

export function apiUrl(path: string, query?: Record<string, unknown>): string {
  const base = typeof window !== 'undefined' ? window.location.origin : 'http://localhost';
  const u = new URL(API_BASE + path, base);
  if (query) {
    for (const [k, v] of Object.entries(query)) {
      if (v === undefined || v === null || v === '' || v === false) continue;
      u.searchParams.set(k, String(v));
    }
  }
  return u.toString();
}

export interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
  body?: unknown;
  query?: Record<string, unknown>;
  headers?: Record<string, string>;
}

function isProblem(b: unknown): b is Problem {
  return typeof b === 'object' && b !== null && 'code' in b && 'title' in b;
}

/**
 * Текст ошибки для человека: «Заголовок. Подробности». У 422 сервер кладёт
 * причины в errors по полям; не у каждого поля есть место в форме
 * (например, «Добавьте хотя бы одну серию» при публикации), поэтому
 * причины дописываются в сам текст.
 */
export function problemText(p: Problem): string {
  const base = p.detail ? `${p.title}. ${p.detail}` : p.title;
  const reasons = Object.values(p.errors ?? {});
  return reasons.length ? `${base}: ${reasons.join('; ')}` : base;
}

export interface RawResponse<T> {
  status: number;
  data: T;
}

async function parse(res: Response): Promise<unknown> {
  if (res.status === 204) return undefined;
  const text = await res.text();
  if (!text) return undefined;
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
}

async function send(path: string, opts: RequestOptions): Promise<Response> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    [CLIENT_HEADER]: CLIENT_ID,
    ...opts.headers,
  };
  if (opts.body !== undefined) headers['Content-Type'] = 'application/json';
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), TIMEOUT_MS);
  try {
    return await fetch(apiUrl(path, opts.query), {
      method: opts.method ?? 'GET',
      headers,
      body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
      credentials: 'same-origin',
      signal: ctrl.signal,
    });
  } catch (e) {
    if (e instanceof DOMException && e.name === 'AbortError')
      throw new ApiError(0, 'timeout', 'Сервер долго не отвечает. Попробуйте ещё раз');
    throw new ApiError(0, 'network', 'Нет связи с сервером. Проверьте интернет');
  } finally {
    clearTimeout(timer);
  }
}

/** Запрос, который возвращает и статус: оплата различает 201, 202 и 402. */
export async function requestRaw<T>(path: string, opts: RequestOptions = {}): Promise<RawResponse<T>> {
  const res = await send(path, opts);
  const body = await parse(res);
  if (!res.ok) {
    if (res.status === 401 && !path.startsWith('/auth/')) listeners.forEach((l) => l());
    if (isProblem(body))
      throw new ApiError(res.status, body.code, problemText(body), body.errors ?? {}, body);
    throw new ApiError(res.status, 'unknown', `Ошибка запроса (${res.status})`, {}, body);
  }
  return { status: res.status, data: body as T };
}

export async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  return (await requestRaw<T>(path, opts)).data;
}

/** Для тестов: сбросить состояние клиента между кейсами. */
export function __resetClient() {
  listeners.clear();
}
