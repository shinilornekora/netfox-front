// Типы контракта API Плёнки (plenka/services/*/internal/adapter/rest).
// Все ответы gateway — JSON; ошибки — RFC 7807 (application/problem+json).

export type Role = 'viewer' | 'admin';
export type Kind = 'film' | 'series';
export type TitleStatus = 'draft' | 'published' | 'withdrawn';

export interface User {
  id: string;
  email: string;
  display_name: string;
  birth_date: string | null;
  role: Role;
  created_at: string;
  revision: number;
}

export interface Episode {
  id: string;
  season: number;
  number: number;
  name: string;
  duration_sec: number;
  media?: string;
}

export interface Title {
  id: string;
  kind: Kind;
  name: string;
  original_name: string;
  year: number;
  description: string;
  age_rating: number;
  genres: string[];
  duration_sec: number;
  in_subscription: boolean;
  price_kopecks: number;
  poster_hue: number;
  poster_url?: string;
  media?: string;
  status: TitleStatus;
  withdraw_reason?: string;
  published_at: string | null;
  updated_at: string;
  revision: number;
  episode_count: number;
  episodes?: Episode[];
}

/** Короткая ссылка на тайтл из реплики playback. */
export interface TitleRef {
  id: string;
  name: string;
  kind: Kind;
  year: number;
  age_rating: number;
  poster_hue: number;
  poster_url?: string;
  available: boolean;
}

/** Keyset-пагинация каталога: следующая страница — по next_cursor. */
export interface Page<T> {
  items: T[];
  next_cursor?: string;
}

export interface Genre {
  name: string;
  count: number;
}

export interface Device {
  id: string;
  user_agent: string;
  ip: string;
  created_at: string;
  last_seen_at: string;
  current: boolean;
}

export interface Progress {
  title_id: string;
  episode_id?: string;
  position_sec: number;
  duration_sec: number;
  finished: boolean;
  updated_at: string;
}

export type WithTitle<T> = T & { title: TitleRef };

export interface HistoryItem {
  id: string;
  title_id: string;
  episode_id?: string;
  via: string;
  started_at: string;
}

export interface HistoryPage {
  items: WithTitle<HistoryItem>[];
  next: { before?: string; before_id?: string };
}

export interface WatchItem {
  title_id: string;
  added_at: string;
}

export interface StartResult {
  stream_url: string;
  resume_at_sec: number;
  duration_sec: number;
  via: 'subscription' | 'purchase';
  episode?: Episode;
  expires_at: string;
  demo: boolean;
}

export interface Plan {
  id: string;
  name: string;
  period_days: number;
  price_kopecks: number;
}

export type OrderStatus = 'charging' | 'failed' | 'completed' | 'refunding' | 'refunded';

export interface Order {
  id: string;
  kind: 'subscription' | 'purchase';
  plan_id?: string;
  title_id?: string;
  title_name?: string;
  amount_kopecks: number;
  status: OrderStatus;
  failure?: string;
  card_last4: string;
  created_at: string;
  updated_at: string;
  user_id?: string;
}

export interface Subscription {
  id: string;
  plan_id: string;
  plan_name: string;
  started_at: string;
  current_period_end: string;
  cancel_at_period_end: boolean;
}

export interface Purchase {
  title_id: string;
  title_name: string;
  purchased_at: string;
  amount_kopecks: number;
}

export interface Wallet {
  subscription: Subscription | null;
  purchases: Purchase[];
  orders: Order[];
}

/** Что зритель может сделать с тайтлом: смотреть, купить или оформить подписку. */
export interface Offer {
  known: boolean;
  owned: boolean;
  subscription_active: boolean;
  in_subscription: boolean;
  price_kopecks: number;
  can_watch: boolean;
  via?: 'subscription' | 'purchase';
  valid_until?: string;
}

export interface Revenue {
  total_kopecks: number;
  completed_orders: number;
  refunded_orders: number;
  failed_orders: number;
  active_subscriptions: number;
  days: { day: string; kopecks: number; orders: number }[];
}

export interface TitleFields {
  kind: Kind;
  name: string;
  original_name: string;
  year: number;
  description: string;
  age_rating: number;
  genres: string[];
  duration_sec: number;
  in_subscription: boolean;
  price_kopecks: number;
  poster_hue: number;
  media: string;
}

/** Тело ошибки RFC 7807. */
export interface Problem {
  status: number;
  code: string;
  title: string;
  detail?: string;
  errors?: Record<string, string>;
  trace_id?: string;
}
