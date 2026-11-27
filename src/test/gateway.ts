// Фейковый gateway Плёнки на MSW: in-memory состояние и обработчики,
// повторяющие контракты бэкенда (plenka/services/*/internal/adapter/rest).
// Используется в интеграционных тестах и в демо-режиме (GitHub Pages).
import { http, HttpResponse } from 'msw';
import type {
  Device,
  Episode,
  Offer,
  Order,
  Plan,
  Progress,
  Purchase,
  Subscription,
  Title,
  TitleFields,
  TitleRef,
  User,
} from '@/shared/api/types';
import { GENRES } from '@/shared/lib/genres';
import { cardNumbers as cards, catalog, plans, users } from './fixtures';

type DbUser = User & { password: string };
interface DbDevice extends Device {
  user_id: string;
}
interface View {
  id: string;
  user_id: string;
  title_id: string;
  episode_id?: string;
  via: string;
  started_at: string;
}

export interface Db {
  users: DbUser[];
  devices: DbDevice[];
  /** Cookie plenka_sid «в браузере»: id устройства (сессии) или null. */
  sessionDevice: string | null;
  titles: Title[];
  plans: Plan[];
  orders: (Order & { user_id: string; idem_key: string })[];
  purchases: (Purchase & { user_id: string })[];
  subscriptions: (Subscription & { user_id: string })[];
  progress: (Progress & { user_id: string })[];
  watchlist: { user_id: string; title_id: string; added_at: string }[];
  views: View[];
  /** Сколько раз опросили «потерянный» заказ: на втором опросе recovery доводит его до конца. */
  polls: Record<string, number>;
  /** Видео для демо: в тестах не грузится. */
  demoStream: string;
  seq: number;
}

const at = (daysAgo: number) => new Date(Date.now() - daysAgo * 86_400_000).toISOString();

export function createDb(): Db {
  const d: Db = {
    users: Object.values(users).map((u) => ({ ...u })),
    devices: [
      {
        id: 'd-anna-tv',
        user_id: 'u-anna',
        user_agent: 'Smart TV / Tizen',
        ip: '10.0.0.15',
        created_at: at(20),
        last_seen_at: at(2),
        current: false,
      },
    ],
    sessionDevice: null,
    titles: catalog(),
    plans: structuredClone(plans),
    orders: [
      {
        id: 'o-1',
        user_id: 'u-anna',
        idem_key: 'seed-1',
        kind: 'purchase',
        title_id: 't-general',
        title_name: 'Генерал',
        amount_kopecks: 9900,
        status: 'completed',
        card_last4: '4242',
        created_at: at(10),
        updated_at: at(10),
      },
    ],
    purchases: [
      {
        user_id: 'u-anna',
        title_id: 't-general',
        title_name: 'Генерал',
        purchased_at: at(10),
        amount_kopecks: 9900,
      },
    ],
    subscriptions: [],
    progress: [
      {
        user_id: 'u-anna',
        title_id: 't-general',
        position_sec: 1800,
        duration_sec: 4680,
        finished: false,
        updated_at: at(1),
      },
    ],
    watchlist: [{ user_id: 'u-anna', title_id: 't-nosferatu', added_at: at(3) }],
    views: [{ id: 'v-1', user_id: 'u-anna', title_id: 't-general', via: 'purchase', started_at: at(1) }],
    polls: {},
    demoStream: '',
    seq: 100,
  };
  return d;
}

export let db = createDb();
export function resetDb() {
  db = createDb();
}
/** Подменяет состояние целиком (демо-режим восстанавливает его из localStorage). */
export function replaceDb(next: Db) {
  db = next;
}

const API = '*/api';
const id = (p: string) => `${p}-${++db.seq}`;
const nowIso = () => new Date().toISOString();

/** Ответ об ошибке в формате RFC 7807, как у kit/problem. */
const problem = (status: number, code: string, title: string, errors?: Record<string, string>) =>
  HttpResponse.json(
    { status, code, title, errors },
    { status, headers: { 'Content-Type': 'application/problem+json' } },
  );
const invalid = (errors: Record<string, string>) => problem(422, 'invalid', 'Проверьте данные формы', errors);
const notFound = (what: string) => problem(404, 'not_found', `${what} не найден`);
const noContent = () => new HttpResponse(null, { status: 204 });

const strip = ({ password: _p, ...u }: DbUser): User => u;

function current(): DbUser | null {
  const dev = db.devices.find((d) => d.id === db.sessionDevice);
  return dev ? (db.users.find((u) => u.id === dev.user_id) ?? null) : null;
}

/** Как gateway: CSRF-заголовок на изменяющих запросах, затем сессия и роль. */
function guard(
  req: Request,
  roles: User['role'][] | null,
  fn: (u: DbUser) => Response | Promise<Response>,
): Response | Promise<Response> {
  if (req.method !== 'GET' && !req.headers.get('X-Plenka-Client'))
    return problem(403, 'csrf', 'Запрос отклонён: нет заголовка X-Plenka-Client');
  const u = current();
  if (!u) return problem(401, 'unauthenticated', 'Войдите, чтобы продолжить');
  if (roles && !roles.includes(u.role)) return problem(403, 'forbidden', 'Недостаточно прав');
  return fn(u);
}

function openSession(u: DbUser) {
  const dev: DbDevice = {
    id: id('d'),
    user_id: u.id,
    user_agent: typeof navigator !== 'undefined' ? navigator.userAgent : 'test',
    ip: '127.0.0.1',
    created_at: nowIso(),
    last_seen_at: nowIso(),
    current: false,
  };
  db.devices.push(dev);
  db.sessionDevice = dev.id;
}

const ref = (t: Title): TitleRef => ({
  id: t.id,
  name: t.name,
  kind: t.kind,
  year: t.year,
  age_rating: t.age_rating,
  poster_hue: t.poster_hue,
  poster_url: t.poster_url,
  available: t.status === 'published',
});
const titleById = (tid: string) => db.titles.find((t) => t.id === tid);
const published = (tid: string) => {
  const t = titleById(tid);
  return t && t.status === 'published' ? t : undefined;
};
const listView = ({ episodes: _e, ...t }: Title): Title => t;

function activeSub(uid: string) {
  return db.subscriptions.find((s) => s.user_id === uid && s.current_period_end > nowIso());
}
function offer(u: DbUser, t: Title): Offer {
  const owned = db.purchases.some((p) => p.user_id === u.id && p.title_id === t.id);
  const sub = activeSub(u.id);
  const viaSub = Boolean(sub && t.in_subscription);
  return {
    known: true,
    owned,
    subscription_active: Boolean(sub),
    in_subscription: t.in_subscription,
    price_kopecks: t.price_kopecks,
    can_watch: owned || viaSub,
    via: owned ? 'purchase' : viaSub ? 'subscription' : undefined,
    valid_until: viaSub && !owned ? sub?.current_period_end : undefined,
  };
}

function age(birth: string): number {
  const b = new Date(birth);
  const n = new Date();
  let a = n.getFullYear() - b.getFullYear();
  if (n.getMonth() < b.getMonth() || (n.getMonth() === b.getMonth() && n.getDate() < b.getDate())) a -= 1;
  return a;
}

/** Выдача прав по оплаченному заказу (fulfil саги). */
function fulfil(o: Db['orders'][number]) {
  o.status = 'completed';
  o.updated_at = nowIso();
  if (o.kind === 'purchase' && o.title_id) {
    db.purchases.push({
      user_id: o.user_id,
      title_id: o.title_id,
      title_name: o.title_name ?? '',
      purchased_at: nowIso(),
      amount_kopecks: o.amount_kopecks,
    });
  } else if (o.plan_id) {
    const plan = db.plans.find((p) => p.id === o.plan_id)!;
    const end = new Date(Date.now() + plan.period_days * 86_400_000).toISOString();
    db.subscriptions.push({
      id: id('s'),
      user_id: o.user_id,
      plan_id: plan.id,
      plan_name: plan.name,
      started_at: nowIso(),
      current_period_end: end,
      cancel_at_period_end: false,
    });
  }
}

const publicOrder = ({ idem_key: _k, ...o }: Db['orders'][number]): Order => o;

function validateTitle(b: TitleFields): Record<string, string> {
  const e: Record<string, string> = {};
  if (!b.name?.trim() || b.name.length > 200) e.name = 'Название — от 1 до 200 символов';
  if (!(b.year >= 1895 && b.year <= new Date().getFullYear() + 3)) e.year = 'Год — от 1895 до ближайших лет';
  if (![0, 6, 12, 16, 18].includes(b.age_rating))
    e.age_rating = 'Возрастной рейтинг: 0+, 6+, 12+, 16+ или 18+';
  if (b.genres.some((g) => !GENRES.includes(g))) e.genres = 'Неизвестный жанр';
  if (b.kind === 'film' && (b.duration_sec < 60 || b.duration_sec > 6 * 3600))
    e.duration_sec = 'Длительность фильма — от 1 минуты до 6 часов';
  if (b.genres.length > 4) e.genres = 'Не больше 4 жанров';
  if (b.kind === 'series' && b.price_kopecks > 0) e.price_kopecks = 'Сериалы доступны только по подписке';
  else if (b.price_kopecks < 0 || b.price_kopecks > 10_000_000) e.price_kopecks = 'Цена — от 0 до 100 000 ₽';
  else if (b.price_kopecks > 0 && b.price_kopecks < 4900) e.price_kopecks = 'Минимальная цена покупки — 49 ₽';
  return e;
}

/** Правила пароля identity (core.Account). */
function passwordError(pw: string, email: string): string | null {
  if (pw.length < 10) return 'Пароль — минимум 10 символов';
  if (pw.length > 128) return 'Пароль — не длиннее 128 символов';
  if (!/\p{L}/u.test(pw) || !/\d/.test(pw)) return 'В пароле нужны буквы и цифры';
  if (pw.toLowerCase() === email.toLowerCase()) return 'Пароль не должен совпадать с email';
  return null;
}

/** Ревизия: чужая правка между чтением и сохранением → 409. */
const stale = () => problem(409, 'stale', 'Тайтл изменили в другой вкладке — обновите страницу');

export const handlers = [
  // ---------- identity ----------
  http.get(`${API}/auth/session`, () => {
    const u = current();
    return HttpResponse.json({ user: u ? strip(u) : null });
  }),
  http.post(`${API}/auth/login`, async ({ request }) => {
    const { email, password } = (await request.json()) as { email: string; password: string };
    const u = db.users.find((x) => x.email === email.trim().toLowerCase() && x.password === password);
    if (!u) return problem(401, 'invalid_credentials', 'Неверный email или пароль');
    openSession(u);
    return HttpResponse.json(strip(u));
  }),
  http.post(`${API}/auth/register`, async ({ request }) => {
    const b = (await request.json()) as {
      email: string;
      password: string;
      display_name: string;
      birth_date: string;
    };
    const e: Record<string, string> = {};
    if (!/^\S+@\S+\.\S+$/.test(b.email)) e.email = 'Неверный email';
    const pw = passwordError(b.password, b.email);
    if (pw) e.password = pw;
    const dn = b.display_name.trim().length;
    if (dn < 2 || dn > 60) e.display_name = 'Имя — от 2 до 60 символов';
    if (Object.keys(e).length) return invalid(e);
    if (db.users.some((u) => u.email === b.email.toLowerCase()))
      return problem(409, 'email_taken', 'Этот email уже зарегистрирован — попробуйте войти');
    const u: DbUser = {
      id: id('u'),
      email: b.email.toLowerCase(),
      display_name: b.display_name.trim(),
      birth_date: b.birth_date || null,
      role: 'viewer',
      created_at: nowIso(),
      revision: 1,
      password: b.password,
    };
    db.users.push(u);
    openSession(u);
    return HttpResponse.json(strip(u), { status: 201 });
  }),
  http.post(`${API}/auth/logout`, () => {
    db.devices = db.devices.filter((d) => d.id !== db.sessionDevice);
    db.sessionDevice = null;
    return noContent();
  }),
  http.get(`${API}/me`, ({ request }) => guard(request, null, (u) => HttpResponse.json(strip(u)))),
  http.patch(`${API}/me`, ({ request }) =>
    guard(request, null, async (u) => {
      const b = (await request.json()) as { display_name: string; birth_date: string; revision: number };
      if (b.revision !== u.revision)
        return problem(409, 'stale', 'Профиль изменён на другом устройстве — обновите страницу');
      const dn = b.display_name.trim().length;
      if (dn < 2 || dn > 60) return invalid({ display_name: 'Имя — от 2 до 60 символов' });
      Object.assign(u, {
        display_name: b.display_name.trim(),
        birth_date: b.birth_date || null,
        revision: u.revision + 1,
      });
      return HttpResponse.json(strip(u));
    }),
  ),
  http.post(`${API}/me/password`, ({ request }) =>
    guard(request, null, async (u) => {
      const b = (await request.json()) as { current_password: string; new_password: string };
      if (b.current_password !== u.password) return invalid({ current_password: 'Текущий пароль неверен' });
      const pw = passwordError(b.new_password, u.email);
      if (pw) return invalid({ new_password: pw });
      u.password = b.new_password;
      // Смена пароля завершает сессии на других устройствах.
      db.devices = db.devices.filter((d) => d.user_id !== u.id || d.id === db.sessionDevice);
      return noContent();
    }),
  ),
  http.get(`${API}/me/devices`, ({ request }) =>
    guard(request, null, (u) =>
      HttpResponse.json({
        items: db.devices
          .filter((d) => d.user_id === u.id)
          .map(({ user_id: _u, ...d }) => ({ ...d, current: d.id === db.sessionDevice })),
      }),
    ),
  ),
  http.delete(`${API}/me/devices/:id`, ({ request, params }) =>
    guard(request, null, (u) => {
      const dev = db.devices.find((d) => d.id === params.id && d.user_id === u.id);
      if (!dev) return notFound('Устройство');
      db.devices = db.devices.filter((d) => d !== dev);
      if (dev.id === db.sessionDevice) db.sessionDevice = null;
      return noContent();
    }),
  ),

  // ---------- catalog ----------
  http.get(`${API}/titles`, ({ request }) => {
    const p = new URL(request.url).searchParams;
    const q = (p.get('q') ?? '').trim().toLowerCase();
    const kind = p.get('kind');
    const genre = (p.get('genre') ?? '').toLowerCase();
    const access = p.get('access');
    const sort = p.get('sort') || 'new';
    if (!['new', 'year', 'name'].includes(sort)) return invalid({ sort: 'Сортировка: new, year или name' });
    const limit = Math.min(50, Math.max(1, Number(p.get('limit')) || 24));
    const offset = Number(p.get('cursor')) || 0;
    const items = db.titles
      .filter((t) => t.status === 'published')
      .filter((t) => !q || `${t.name} ${t.original_name}`.toLowerCase().includes(q))
      .filter((t) => !kind || t.kind === kind)
      .filter((t) => !genre || t.genres.includes(genre))
      .filter((t) => access !== 'subscription' || t.in_subscription)
      .filter((t) => access !== 'purchase' || t.price_kopecks > 0)
      .sort((a, b) =>
        sort === 'name'
          ? a.name.localeCompare(b.name, 'ru')
          : sort === 'year'
            ? b.year - a.year
            : (b.published_at ?? '').localeCompare(a.published_at ?? ''),
      );
    const page = items.slice(offset, offset + limit).map(listView);
    const next = offset + limit < items.length ? String(offset + limit) : undefined;
    return HttpResponse.json({ items: page, next_cursor: next });
  }),
  // Неопубликованное видит только администратор (catalog usecase.Title).
  http.get(`${API}/titles/:id`, ({ params }) => {
    const t = current()?.role === 'admin' ? titleById(String(params.id)) : published(String(params.id));
    return t ? HttpResponse.json(t) : notFound('Тайтл');
  }),
  // Фиксированный словарь жанров (catalog core.Genres) со счётчиками опубликованного.
  http.get(`${API}/genres`, () => {
    const pub = db.titles.filter((t) => t.status === 'published');
    const items = GENRES.map((name) => ({ name, count: pub.filter((t) => t.genres.includes(name)).length }));
    return HttpResponse.json({ items });
  }),

  http.get(`${API}/admin/titles`, ({ request }) =>
    guard(request, ['admin'], () => {
      const p = new URL(request.url).searchParams;
      const q = (p.get('q') ?? '').toLowerCase();
      const status = p.get('status');
      const items = db.titles
        .filter((t) => !status || t.status === status)
        .filter((t) => !q || t.name.toLowerCase().includes(q))
        .sort((a, b) => b.updated_at.localeCompare(a.updated_at))
        .map(listView);
      return HttpResponse.json({ items });
    }),
  ),
  http.post(`${API}/admin/titles`, ({ request }) =>
    guard(request, ['admin'], async () => {
      const b = (await request.json()) as TitleFields;
      const e = validateTitle(b);
      if (Object.keys(e).length) return invalid(e);
      const t: Title = {
        ...b,
        id: id('t'),
        genres: b.genres.map((g) => g.toLowerCase()),
        status: 'draft',
        published_at: null,
        updated_at: nowIso(),
        revision: 1,
        episode_count: 0,
        episodes: b.kind === 'series' ? [] : undefined,
      };
      db.titles.push(t);
      return HttpResponse.json(t, { status: 201 });
    }),
  ),
  http.put(`${API}/admin/titles/:id`, ({ request, params }) =>
    guard(request, ['admin'], async () => {
      const t = titleById(String(params.id));
      if (!t) return notFound('Тайтл');
      const b = (await request.json()) as TitleFields & { revision: number };
      if (b.revision !== t.revision) return stale();
      const e = validateTitle(b);
      if (Object.keys(e).length) return invalid(e);
      const { revision: _r, ...fields } = b;
      Object.assign(t, fields, {
        genres: b.genres.map((g) => g.toLowerCase()),
        revision: t.revision + 1,
        updated_at: nowIso(),
      });
      return HttpResponse.json(t);
    }),
  ),
  http.delete(`${API}/admin/titles/:id`, ({ request, params }) =>
    guard(request, ['admin'], () => {
      const t = titleById(String(params.id));
      if (!t) return notFound('Тайтл');
      if (t.status !== 'draft')
        return problem(
          409,
          'not_draft',
          'Удалить можно только черновик — опубликованный тайтл снимите с показа',
        );
      db.titles = db.titles.filter((x) => x !== t);
      return noContent();
    }),
  ),
  http.post(`${API}/admin/titles/:id/publish`, ({ request, params }) =>
    guard(request, ['admin'], async () => {
      const t = titleById(String(params.id));
      if (!t) return notFound('Тайтл');
      const { revision } = (await request.json()) as { revision: number };
      if (revision !== t.revision) return stale();
      // readiness (catalog core.Title): что мешает опубликовать.
      const e: Record<string, string> = {};
      if ([...t.description].length < 20)
        e.description = 'Перед публикацией добавьте описание (от 20 символов)';
      if (!t.in_subscription && t.price_kopecks === 0)
        e.in_subscription = 'Включите подписку или задайте цену покупки';
      if (t.kind === 'series' && !t.episodes?.length) e.episodes = 'Добавьте хотя бы одну серию';
      if (Object.keys(e).length) return invalid(e);
      Object.assign(t, {
        status: 'published',
        withdraw_reason: undefined,
        published_at: nowIso(),
        revision: t.revision + 1,
        updated_at: nowIso(),
      });
      return HttpResponse.json(t);
    }),
  ),
  http.post(`${API}/admin/titles/:id/withdraw`, ({ request, params }) =>
    guard(request, ['admin'], async () => {
      const t = titleById(String(params.id));
      if (!t) return notFound('Тайтл');
      const b = (await request.json()) as { revision: number; reason: string };
      if (b.revision !== t.revision) return stale();
      const len = (b.reason ?? '').trim().length;
      if (len < 3 || len > 300) return invalid({ reason: 'Укажите причину (3–300 символов)' });
      Object.assign(t, {
        status: 'withdrawn',
        withdraw_reason: b.reason.trim(),
        revision: t.revision + 1,
        updated_at: nowIso(),
      });
      return HttpResponse.json(t);
    }),
  ),
  http.put(`${API}/admin/titles/:id/episodes`, ({ request, params }) =>
    guard(request, ['admin'], async () => {
      const t = titleById(String(params.id));
      if (!t) return notFound('Тайтл');
      if (t.kind !== 'series') return problem(409, 'not_series', 'Серии бывают только у сериалов');
      const b = (await request.json()) as Partial<Episode>;
      if (!b.name?.trim()) return invalid({ name: 'Укажите название серии' });
      const eps = (t.episodes ??= []);
      if (eps.some((e) => e.id !== b.id && e.season === b.season && e.number === b.number))
        return problem(409, 'episode_exists', `Серия ${b.season}×${b.number} уже есть`);
      let ep = eps.find((e) => e.id === b.id);
      if (ep) Object.assign(ep, b);
      else {
        ep = {
          id: id('e'),
          season: b.season ?? 1,
          number: b.number ?? eps.length + 1,
          name: b.name.trim(),
          duration_sec: b.duration_sec ?? 0,
        };
        eps.push(ep);
      }
      eps.sort((a, c) => a.season - c.season || a.number - c.number);
      t.episode_count = eps.length;
      t.updated_at = nowIso();
      return HttpResponse.json(ep);
    }),
  ),
  http.delete(`${API}/admin/titles/:id/episodes/:eid`, ({ request, params }) =>
    guard(request, ['admin'], () => {
      const t = titleById(String(params.id));
      if (!t) return notFound('Тайтл');
      if (t.status === 'published' && (t.episodes ?? []).length <= 1)
        return problem(409, 'last_episode', 'У опубликованного сериала должна остаться хотя бы одна серия');
      t.episodes = (t.episodes ?? []).filter((e) => e.id !== params.eid);
      t.episode_count = t.episodes.length;
      return noContent();
    }),
  ),

  // ---------- billing ----------
  http.get(`${API}/billing/plans`, () => HttpResponse.json({ items: db.plans })),
  http.post(`${API}/billing/checkout`, ({ request }) =>
    guard(request, ['viewer'], async (u) => {
      const key = request.headers.get('Idempotency-Key') ?? '';
      if (!key) return problem(400, 'no_idempotency_key', 'Нет заголовка Idempotency-Key');
      const b = (await request.json()) as {
        kind: 'subscription' | 'purchase';
        plan_id?: string;
        title_id?: string;
        card_number: string;
      };
      const replay = db.orders.find((o) => o.user_id === u.id && o.idem_key === key);
      if (replay) {
        if (replay.kind !== b.kind || (replay.title_id ?? '') !== (b.title_id ?? ''))
          return problem(409, 'idempotency_mismatch', 'Этот ключ уже использован для другой покупки');
        return HttpResponse.json(publicOrder(replay), { headers: { 'Idempotent-Replayed': 'true' } });
      }
      const card = b.card_number.replace(/\D/g, '');
      if (card.length < 12) return invalid({ card_number: 'Номер карты — от 12 цифр' });
      if (db.orders.some((o) => o.user_id === u.id && o.status === 'charging'))
        return problem(
          409,
          'order_in_progress',
          'Предыдущая оплата ещё обрабатывается — подождите несколько секунд',
        );
      let amount = 0;
      let titleName: string | undefined;
      if (b.kind === 'subscription') {
        const plan = db.plans.find((p) => p.id === b.plan_id);
        if (!plan) return notFound('Тариф');
        const sub = activeSub(u.id);
        if (sub)
          return problem(
            409,
            'already_subscribed',
            `Подписка уже действует до ${new Date(sub.current_period_end).toLocaleDateString('ru-RU')}`,
          );
        amount = plan.price_kopecks;
      } else {
        const t = published(b.title_id ?? '');
        if (!t) return notFound('Тайтл');
        if (t.kind !== 'film' || t.price_kopecks <= 0)
          return problem(409, 'not_for_sale', 'Этот фильм сейчас не продаётся');
        if (db.purchases.some((p) => p.user_id === u.id && p.title_id === t.id))
          return problem(409, 'already_owned', 'Фильм уже куплен — он есть в разделе «Покупки»');
        amount = t.price_kopecks;
        titleName = t.name;
      }
      const o: Db['orders'][number] = {
        id: id('o'),
        user_id: u.id,
        idem_key: key,
        kind: b.kind,
        plan_id: b.plan_id,
        title_id: b.title_id,
        title_name: titleName,
        amount_kopecks: amount,
        status: 'charging',
        card_last4: card.slice(-4),
        created_at: nowIso(),
        updated_at: nowIso(),
      };
      db.orders.push(o);
      if (card === cards.lost)
        return HttpResponse.json(publicOrder(o), {
          status: 202,
          headers: { Location: `/api/billing/orders/${o.id}` },
        });
      if (card === cards.declined || card === cards.insufficient) {
        o.status = 'failed';
        o.failure = card === cards.declined ? 'Банк отклонил операцию' : 'Недостаточно средств на карте';
        return HttpResponse.json(publicOrder(o), { status: 402 });
      }
      fulfil(o);
      return HttpResponse.json(publicOrder(o), { status: 201 });
    }),
  ),
  http.get(`${API}/billing/orders/:id`, ({ request, params }) =>
    guard(request, null, (u) => {
      const o = db.orders.find((x) => x.id === params.id && (x.user_id === u.id || u.role === 'admin'));
      if (!o) return notFound('Заказ');
      // recovery: «банк молчит» → со второго опроса сверка с банком доводит заказ до конца.
      if (o.status === 'charging') {
        db.polls[o.id] = (db.polls[o.id] ?? 0) + 1;
        if (db.polls[o.id]! >= 2) fulfil(o);
      }
      return HttpResponse.json(publicOrder(o));
    }),
  ),
  http.get(`${API}/billing/wallet`, ({ request }) =>
    guard(request, null, (u) => {
      const sub = activeSub(u.id);
      return HttpResponse.json({
        subscription: sub ? (({ user_id: _u, ...s }) => s)(sub) : null,
        purchases: db.purchases.filter((p) => p.user_id === u.id).map(({ user_id: _u, ...p }) => p),
        orders: db.orders
          .filter((o) => o.user_id === u.id)
          .sort((a, b) => b.created_at.localeCompare(a.created_at))
          .map(publicOrder),
      });
    }),
  ),
  http.post(`${API}/billing/subscription/auto-renew`, ({ request }) =>
    guard(request, ['viewer'], async (u) => {
      const { enabled } = (await request.json()) as { enabled: boolean };
      const sub = activeSub(u.id);
      if (!sub) return problem(409, 'no_subscription', 'Активной подписки нет');
      sub.cancel_at_period_end = !enabled;
      return noContent();
    }),
  ),
  http.get(`${API}/billing/offers/:title`, ({ request, params }) =>
    guard(request, null, (u) => {
      const t = titleById(String(params.title));
      if (!t) return HttpResponse.json({ known: false } as Offer);
      return HttpResponse.json(offer(u, t));
    }),
  ),
  http.get(`${API}/billing/admin/orders`, ({ request }) =>
    guard(request, ['admin'], () => {
      const p = new URL(request.url).searchParams;
      const status = p.get('status');
      const before = p.get('before');
      const all = db.orders
        .filter((o) => !status || o.status === status)
        .filter((o) => !before || o.created_at < before)
        .sort((a, b) => b.created_at.localeCompare(a.created_at));
      const page = all.slice(0, 30);
      const next = page.length === 30 ? page[29]!.created_at : '';
      return HttpResponse.json({ items: page.map(publicOrder), next_before: next });
    }),
  ),
  http.post(`${API}/billing/admin/orders/:id/refund`, ({ request, params }) =>
    guard(request, ['admin'], () => {
      const o = db.orders.find((x) => x.id === params.id);
      if (!o) return notFound('Заказ');
      if (o.status !== 'completed')
        return problem(409, 'not_refundable', 'Вернуть можно только оплаченный заказ');
      o.status = 'refunded';
      o.updated_at = nowIso();
      if (o.kind === 'purchase')
        db.purchases = db.purchases.filter((p) => !(p.user_id === o.user_id && p.title_id === o.title_id));
      else db.subscriptions = db.subscriptions.filter((s) => s.user_id !== o.user_id);
      return HttpResponse.json(publicOrder(o));
    }),
  ),
  http.get(`${API}/billing/admin/revenue`, ({ request }) =>
    guard(request, ['admin'], () => {
      const done = db.orders.filter((o) => o.status === 'completed');
      const byDay = new Map<string, { kopecks: number; orders: number }>();
      done.forEach((o) => {
        const d = o.created_at.slice(0, 10);
        const v = byDay.get(d) ?? { kopecks: 0, orders: 0 };
        byDay.set(d, { kopecks: v.kopecks + o.amount_kopecks, orders: v.orders + 1 });
      });
      return HttpResponse.json({
        total_kopecks: done.reduce((s, o) => s + o.amount_kopecks, 0),
        completed_orders: done.length,
        refunded_orders: db.orders.filter((o) => o.status === 'refunded').length,
        failed_orders: db.orders.filter((o) => o.status === 'failed').length,
        active_subscriptions: db.subscriptions.filter((s) => s.current_period_end > nowIso()).length,
        days: [...byDay].sort(([a], [b]) => a.localeCompare(b)).map(([day, v]) => ({ day, ...v })),
      });
    }),
  ),

  // ---------- playback ----------
  http.post(`${API}/playback/start`, ({ request }) =>
    guard(request, ['viewer'], async (u) => {
      const b = (await request.json()) as { title_id: string; episode_id?: string };
      const t = titleById(b.title_id);
      if (!t || t.status !== 'published')
        return problem(409, 'not_available', 'Тайтл сейчас недоступен для просмотра');
      if (t.age_rating >= 18 && !u.birth_date)
        return problem(403, 'forbidden', 'Для фильмов 18+ укажите дату рождения в профиле');
      if (u.birth_date && age(u.birth_date) < t.age_rating)
        return problem(403, 'forbidden', 'Этот тайтл недоступен по возрасту');
      const o = offer(u, t);
      if (!o.can_watch)
        return problem(402, 'not_entitled', 'Оформите подписку или купите фильм, чтобы смотреть');
      let ep: Episode | undefined;
      if (t.kind === 'series') {
        if (!t.episodes?.length) return problem(409, 'no_episodes', 'У сериала пока нет серий');
        ep = t.episodes.find((e) => e.id === b.episode_id) ?? t.episodes[0];
      }
      const pr = db.progress.find(
        (p) => p.user_id === u.id && p.title_id === t.id && (!ep || p.episode_id === ep.id),
      );
      db.views.unshift({
        id: id('v'),
        user_id: u.id,
        title_id: t.id,
        episode_id: ep?.id,
        via: o.via ?? 'subscription',
        started_at: nowIso(),
      });
      return HttpResponse.json({
        stream_url: db.demoStream || `/api/playback/stream/tok-${t.id}`,
        resume_at_sec: pr && !pr.finished ? pr.position_sec : 0,
        duration_sec: ep?.duration_sec ?? t.duration_sec,
        via: o.via,
        episode: ep,
        expires_at: new Date(Date.now() + 4 * 3_600_000).toISOString(),
        demo: true,
      });
    }),
  ),
  http.put(`${API}/playback/progress`, ({ request }) =>
    guard(request, ['viewer'], async (u) => {
      const b = (await request.json()) as {
        title_id: string;
        episode_id?: string;
        position_sec: number;
        duration_sec: number;
      };
      let p = db.progress.find((x) => x.user_id === u.id && x.title_id === b.title_id);
      if (!p) {
        p = {
          user_id: u.id,
          title_id: b.title_id,
          position_sec: 0,
          duration_sec: 0,
          finished: false,
          updated_at: '',
        };
        db.progress.push(p);
      }
      Object.assign(p, {
        episode_id: b.episode_id,
        position_sec: b.position_sec,
        duration_sec: b.duration_sec,
        finished: b.duration_sec > 0 && b.position_sec >= b.duration_sec * 0.95,
        updated_at: nowIso(),
      });
      return HttpResponse.json({ applied: true });
    }),
  ),
  http.get(`${API}/playback/titles/:id`, ({ request, params }) =>
    guard(request, null, (u) => {
      const p = db.progress.find((x) => x.user_id === u.id && x.title_id === params.id);
      return HttpResponse.json({
        progress: p ? (({ user_id: _u, ...r }) => r)(p) : null,
        in_watchlist: db.watchlist.some((w) => w.user_id === u.id && w.title_id === params.id),
      });
    }),
  ),
  http.get(`${API}/playback/continue`, ({ request }) =>
    guard(request, null, (u) => {
      const items = db.progress
        .filter((p) => p.user_id === u.id && !p.finished && titleById(p.title_id))
        .sort((a, b) => b.updated_at.localeCompare(a.updated_at))
        .map(({ user_id: _u, ...p }) => ({ ...p, title: ref(titleById(p.title_id)!) }));
      return HttpResponse.json({ items });
    }),
  ),
  http.get(`${API}/playback/history`, ({ request }) =>
    guard(request, null, (u) => {
      const p = new URL(request.url).searchParams;
      const before = p.get('before');
      const all = db.views.filter((v) => v.user_id === u.id && titleById(v.title_id));
      const start = before ? all.findIndex((v) => v.id === p.get('before_id')) + 1 : 0;
      const page = all.slice(start, start + 20);
      const last = page.at(-1);
      return HttpResponse.json({
        items: page.map(({ user_id: _u, ...v }) => ({ ...v, title: ref(titleById(v.title_id)!) })),
        next: start + 20 < all.length && last ? { before: last.started_at, before_id: last.id } : {},
      });
    }),
  ),
  http.get(`${API}/playback/watchlist`, ({ request }) =>
    guard(request, null, (u) =>
      HttpResponse.json({
        items: db.watchlist
          .filter((w) => w.user_id === u.id && titleById(w.title_id))
          .sort((a, b) => b.added_at.localeCompare(a.added_at))
          .map(({ user_id: _u, ...w }) => ({ ...w, title: ref(titleById(w.title_id)!) })),
      }),
    ),
  ),
  http.put(`${API}/playback/watchlist/:id`, ({ request, params }) =>
    guard(request, ['viewer'], () => {
      const tid = String(params.id);
      if (!titleById(tid)) return notFound('Тайтл');
      if (!db.watchlist.some((w) => w.user_id === current()!.id && w.title_id === tid))
        db.watchlist.push({ user_id: current()!.id, title_id: tid, added_at: nowIso() });
      return noContent();
    }),
  ),
  http.delete(`${API}/playback/watchlist/:id`, ({ request, params }) =>
    guard(request, ['viewer'], (u) => {
      db.watchlist = db.watchlist.filter((w) => !(w.user_id === u.id && w.title_id === params.id));
      return noContent();
    }),
  ),
];

/** Для тестов: «браузер с живой сессией» этого пользователя. */
export function signIn(userId: string) {
  const u = db.users.find((x) => x.id === userId)!;
  openSession(u);
}
