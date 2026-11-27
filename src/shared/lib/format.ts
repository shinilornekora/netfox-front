import type { Kind, OrderStatus, Role, TitleStatus } from '@/shared/api/types';
import { ApiError } from '@/shared/api/client';

const rub = new Intl.NumberFormat('ru-RU', { style: 'currency', currency: 'RUB', maximumFractionDigits: 0 });

/** Деньги в Плёнке — целые копейки. */
export function formatMoney(kopecks?: number | null): string {
  if (kopecks === undefined || kopecks === null) return '—';
  return rub.format(kopecks / 100);
}

const dateFmt = new Intl.DateTimeFormat('ru-RU', { day: 'numeric', month: 'long', year: 'numeric' });
const dateTimeFmt = new Intl.DateTimeFormat('ru-RU', { dateStyle: 'medium', timeStyle: 'short' });

/** «2026-12-10…» → «10 декабря 2026 г.» без сдвига часового пояса. */
export function formatDate(iso: string): string {
  const [y, m, d] = iso.slice(0, 10).split('-').map(Number);
  return dateFmt.format(new Date(y ?? 1970, (m ?? 1) - 1, d ?? 1));
}

export function formatDateTime(iso: string): string {
  return dateTimeFmt.format(new Date(iso));
}

/** 5400 → «1 ч 30 мин», 2700 → «45 мин». */
export function formatDuration(sec: number): string {
  const total = Math.max(0, Math.round(sec / 60));
  const h = Math.floor(total / 60);
  const m = total % 60;
  if (h === 0) return `${m} мин`;
  return m === 0 ? `${h} ч` : `${h} ч ${m} мин`;
}

/** Позиция плеера: 3725 → «1:02:05», 65 → «1:05». */
export function formatClock(sec: number): string {
  const s = Math.max(0, Math.floor(sec));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const r = String(s % 60).padStart(2, '0');
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${r}` : `${m}:${r}`;
}

export function plural(n: number, one: string, few: string, many: string): string {
  const m10 = n % 10;
  const m100 = n % 100;
  if (m10 === 1 && m100 !== 11) return one;
  if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return few;
  return many;
}

export const roleLabel: Record<Role, string> = {
  viewer: 'Зритель',
  admin: 'Администратор',
};

export const kindLabel: Record<Kind, string> = {
  film: 'Фильм',
  series: 'Сериал',
};

export const titleStatusLabel: Record<TitleStatus, string> = {
  draft: 'Черновик',
  published: 'Опубликован',
  withdrawn: 'Снят с показа',
};

export const orderStatusLabel: Record<OrderStatus, string> = {
  charging: 'В обработке',
  completed: 'Оплачен',
  failed: 'Отклонён',
  refunding: 'Возврат…',
  refunded: 'Возвращён',
};

export const orderTone: Record<OrderStatus, 'ok' | 'bad' | 'info' | 'warn'> = {
  charging: 'warn',
  completed: 'ok',
  failed: 'bad',
  refunding: 'warn',
  refunded: 'info',
};

export function ageLabel(age: number): string {
  return `${age}+`;
}

/** Полных лет на дату today (по умолчанию — сегодня). */
export function ageOn(birth: string, today: Date = new Date()): number {
  const [y, m, d] = birth.slice(0, 10).split('-').map(Number);
  let age = today.getFullYear() - (y ?? 0);
  const before =
    today.getMonth() + 1 < (m ?? 1) || (today.getMonth() + 1 === m && today.getDate() < (d ?? 1));
  if (before) age -= 1;
  return age;
}

/** Только цифры номера карты, группами по 4: «4242 4242 4242 4242». */
export function formatCard(raw: string): string {
  return raw
    .replace(/\D/g, '')
    .slice(0, 19)
    .replace(/(\d{4})(?=\d)/g, '$1 ');
}

export function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  if (e instanceof Error) return e.message;
  return 'Неизвестная ошибка';
}

export function errorFields(e: unknown): Record<string, string> {
  return e instanceof ApiError ? e.fields : {};
}
