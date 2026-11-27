// Карточка тайтла: сам тайтл для всех, а для зрителя — его права (offer)
// и состояние просмотра (прогресс, «Смотреть позже»).
import { combine, createEffect, createEvent, createStore, sample } from 'effector';
import { createGate } from 'effector-react';
import { api } from '@/shared/api/endpoints';
import type { Offer, Title } from '@/shared/api/types';
import { createRequest } from '@/shared/lib/request';
import { errorMessage } from '@/shared/lib/format';
import { $role } from '@/entities/session/model';
import { paid } from '@/features/checkout/model';

export const TitleGate = createGate<{ id: string }>();

export const titleQuery = createRequest((id: string) => api.title(id));
export const offerQuery = createRequest((id: string) => api.offer(id));
export const stateQuery = createRequest((id: string) => api.titleState(id));

export const watchToggled = createEvent();
export const setWatchFx = createEffect(({ id, on }: { id: string; on: boolean }) => api.setWatch(id, on));

const $id = TitleGate.state.map((s) => s.id ?? null);

sample({ clock: TitleGate.open, fn: (p) => p.id, target: titleQuery.start });
sample({
  clock: [TitleGate.open, $role.updates],
  source: { id: $id, role: $role },
  filter: ({ id, role }) => id !== null && role === 'viewer',
  fn: ({ id }) => id!,
  target: [offerQuery.start, stateQuery.start],
});
sample({ clock: TitleGate.close, target: [titleQuery.reset, offerQuery.reset, stateQuery.reset] });

// После оплаты права изменились.
sample({ clock: paid, source: $id, filter: Boolean, target: offerQuery.start });

/** «Смотреть позже»: переключается сразу, при ошибке откатывается. */
export const $inWatchlist = createStore(false)
  .on(stateQuery.fx.doneData, (_, s) => s.in_watchlist)
  .on(setWatchFx, (_, p) => p.on)
  .on(setWatchFx.fail, (_, { params }) => !params.on)
  .reset(TitleGate.close);
export const $watchError = createStore<string | null>(null)
  .on(setWatchFx.failData, (_, e) => errorMessage(e))
  .reset(setWatchFx);

sample({
  clock: watchToggled,
  source: { id: $id, on: $inWatchlist },
  filter: ({ id }) => id !== null,
  fn: ({ id, on }) => ({ id: id!, on: !on }),
  target: setWatchFx,
});

export type Action =
  | { type: 'watch'; via: 'subscription' | 'purchase' }
  | { type: 'buy'; price: number }
  | { type: 'subscribe' }
  | { type: 'unavailable' };

/** Что предложить зрителю на кнопках: смотреть, купить и/или оформить подписку. */
export function actionsFor(t: Title, o: Offer | null): Action[] {
  if (t.status !== 'published') return [{ type: 'unavailable' }];
  if (o?.can_watch) return [{ type: 'watch', via: o.via ?? 'subscription' }];
  const out: Action[] = [];
  if (t.in_subscription && !o?.subscription_active) out.push({ type: 'subscribe' });
  if (t.kind === 'film' && t.price_kopecks > 0 && !o?.owned)
    out.push({ type: 'buy', price: t.price_kopecks });
  if (out.length === 0) out.push({ type: 'unavailable' });
  return out;
}

export const $view = combine({
  title: titleQuery.$data,
  error: titleQuery.$error,
  pending: titleQuery.$pending,
  offer: offerQuery.$data,
  progress: stateQuery.$data.map((s) => s?.progress ?? null),
});
