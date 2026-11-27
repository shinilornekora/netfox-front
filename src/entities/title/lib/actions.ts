import type { Offer, Title } from '@/shared/api/types';

export type TitleAction =
  | { type: 'watch'; via: 'subscription' | 'purchase' }
  | { type: 'buy'; price: number }
  | { type: 'subscribe' }
  | { type: 'unavailable' };

/** Что предложить зрителю на кнопках: смотреть, купить и/или оформить подписку. */
export function actionsFor(t: Title, o: Offer | null): TitleAction[] {
  if (t.status !== 'published') return [{ type: 'unavailable' }];
  if (o?.can_watch) return [{ type: 'watch', via: o.via ?? 'subscription' }];
  const out: TitleAction[] = [];
  if (t.in_subscription && !o?.subscription_active) out.push({ type: 'subscribe' });
  if (t.kind === 'film' && t.price_kopecks > 0 && !o?.owned)
    out.push({ type: 'buy', price: t.price_kopecks });
  if (out.length === 0) out.push({ type: 'unavailable' });
  return out;
}
