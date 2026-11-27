// Все эндпоинты gateway в одном месте. Страницы и модели зовут только их.
import { request, requestRaw } from './client';
import type {
  Device,
  Episode,
  Genre,
  HistoryPage,
  Kind,
  Offer,
  Order,
  Page,
  Plan,
  Progress,
  Revenue,
  StartResult,
  Title,
  TitleFields,
  User,
  Wallet,
  WatchItem,
  WithTitle,
} from './types';

export interface TitleQuery {
  q?: string;
  kind?: Kind | '';
  genre?: string;
  access?: '' | 'subscription' | 'purchase';
  sort?: '' | 'new' | 'year' | 'name';
  cursor?: string;
  limit?: number;
}

export interface RegisterIn {
  email: string;
  password: string;
  display_name: string;
  birth_date: string;
}

export interface CheckoutIn {
  kind: 'subscription' | 'purchase';
  plan_id?: string;
  title_id?: string;
  card_number: string;
}

export interface ProgressIn {
  title_id: string;
  episode_id?: string;
  position_sec: number;
  duration_sec: number;
  client_ms: number;
}

export interface EpisodeIn {
  id?: string;
  season: number;
  number: number;
  name: string;
  duration_sec: number;
  media?: string;
}

const enc = encodeURIComponent;

export const api = {
  // ---------- identity ----------
  session: () => request<{ user: User | null }>('/auth/session').then((r) => r.user),
  login: (b: { email: string; password: string }) =>
    request<User>('/auth/login', { method: 'POST', body: b }),
  register: (b: RegisterIn) => request<User>('/auth/register', { method: 'POST', body: b }),
  logout: () => request<void>('/auth/logout', { method: 'POST' }),
  updateMe: (b: { display_name: string; birth_date: string; revision: number }) =>
    request<User>('/me', { method: 'PATCH', body: b }),
  changePassword: (b: { current_password: string; new_password: string }) =>
    request<void>('/me/password', { method: 'POST', body: b }),
  devices: () => request<{ items: Device[] }>('/me/devices'),
  revokeDevice: (id: string) => request<void>(`/me/devices/${enc(id)}`, { method: 'DELETE' }),

  // ---------- catalog ----------
  titles: (q: TitleQuery) => request<Page<Title>>('/titles', { query: { ...q } }),
  title: (id: string) => request<Title>(`/titles/${enc(id)}`),
  genres: () => request<{ items: Genre[] }>('/genres'),

  adminTitles: (q: { q?: string; status?: string; cursor?: string }) =>
    request<Page<Title>>('/admin/titles', { query: { ...q, limit: 50 } }),
  createTitle: (b: TitleFields) => request<Title>('/admin/titles', { method: 'POST', body: b }),
  editTitle: (id: string, b: TitleFields & { revision: number }) =>
    request<Title>(`/admin/titles/${enc(id)}`, { method: 'PUT', body: b }),
  deleteTitle: (id: string) => request<void>(`/admin/titles/${enc(id)}`, { method: 'DELETE' }),
  publishTitle: (id: string, revision: number) =>
    request<Title>(`/admin/titles/${enc(id)}/publish`, { method: 'POST', body: { revision } }),
  withdrawTitle: (id: string, revision: number, reason: string) =>
    request<Title>(`/admin/titles/${enc(id)}/withdraw`, { method: 'POST', body: { revision, reason } }),
  saveEpisode: (titleId: string, b: EpisodeIn) =>
    request<Episode>(`/admin/titles/${enc(titleId)}/episodes`, { method: 'PUT', body: b }),
  deleteEpisode: (titleId: string, id: string) =>
    request<void>(`/admin/titles/${enc(titleId)}/episodes/${enc(id)}`, { method: 'DELETE' }),

  // ---------- billing ----------
  plans: () => request<{ items: Plan[] }>('/billing/plans'),
  /** 201 — оплачено; 202 — исход выясняется (опрашиваем заказ); 402 — банк отказал (заказ в теле); 200 — повтор. */
  checkout: (b: CheckoutIn, key: string) =>
    requestRaw<Order>('/billing/checkout', { method: 'POST', body: b, headers: { 'Idempotency-Key': key } }),
  order: (id: string) => request<Order>(`/billing/orders/${enc(id)}`),
  wallet: () => request<Wallet>('/billing/wallet'),
  autoRenew: (enabled: boolean) =>
    request<void>('/billing/subscription/auto-renew', { method: 'POST', body: { enabled } }),
  offer: (titleId: string) => request<Offer>(`/billing/offers/${enc(titleId)}`),
  adminOrders: (status?: string) =>
    request<{ items: Order[] }>('/billing/admin/orders', { query: { status } }),
  refundOrder: (id: string, reason: string) =>
    request<Order>(`/billing/admin/orders/${enc(id)}/refund`, { method: 'POST', body: { reason } }),
  revenue: () => request<Revenue>('/billing/admin/revenue'),

  // ---------- playback ----------
  start: (b: { title_id: string; episode_id?: string }) =>
    request<StartResult>('/playback/start', { method: 'POST', body: b }),
  saveProgress: (b: ProgressIn) =>
    request<{ applied: boolean }>('/playback/progress', { method: 'PUT', body: b }),
  titleState: (id: string) =>
    request<{ progress: Progress | null; in_watchlist: boolean }>(`/playback/titles/${enc(id)}`),
  continueWatching: () => request<{ items: WithTitle<Progress>[] }>('/playback/continue'),
  history: (c: { before?: string; before_id?: string } = {}) =>
    request<HistoryPage>('/playback/history', { query: c }),
  watchlist: () => request<{ items: WithTitle<WatchItem>[] }>('/playback/watchlist'),
  setWatch: (id: string, on: boolean) =>
    request<void>(`/playback/watchlist/${enc(id)}`, { method: on ? 'PUT' : 'DELETE' }),
};
