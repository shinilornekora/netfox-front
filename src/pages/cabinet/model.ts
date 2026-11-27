// Личный кабинет зрителя: подписка и автопродление, покупки, заказы,
// история просмотров (keyset-пагинация по started_at + id) и устройства.
import { createEffect, createEvent, createStore, sample } from 'effector';
import { createGate } from 'effector-react';
import { api } from '@/shared/api/endpoints';
import type { HistoryPage } from '@/shared/api/types';
import { createRequest } from '@/shared/lib/request';
import { errorMessage } from '@/shared/lib/format';
import { paid } from '@/features/checkout/model';
import { sessionLost } from '@/entities/session/model';

export type Tab = 'subscription' | 'purchases' | 'orders' | 'history' | 'devices';

export const CabinetGate = createGate<void>();
export const tabChanged = createEvent<Tab>();
export const historyMore = createEvent();
export const autoRenewToggled = createEvent<boolean>();
export const deviceRevoked = createEvent<{ id: string; current: boolean }>();

export const $tab = createStore<Tab>('subscription').on(tabChanged, (_, t) => t);

export const walletQuery = createRequest((_: void) => api.wallet());
export const devicesQuery = createRequest((_: void) => api.devices());
export const historyFx = createEffect((c: HistoryPage['next']) => api.history(c));

export const autoRenewFx = createEffect(api.autoRenew);
export const revokeFx = createEffect(async (d: { id: string; current: boolean }) => {
  await api.revokeDevice(d.id);
  return d;
});

export const $history = createStore<HistoryPage['items']>([])
  .on(historyFx.done, (list, { params, result }) =>
    params.before ? [...list, ...result.items] : result.items,
  )
  .reset(CabinetGate.close);
export const $historyNext = createStore<HistoryPage['next']>({})
  .on(historyFx.doneData, (_, r) => r.next)
  .reset(CabinetGate.close);
export const $actionError = createStore<string | null>(null)
  .on([autoRenewFx.failData, revokeFx.failData], (_, e) => errorMessage(e))
  .reset([autoRenewFx, revokeFx, tabChanged]);

sample({
  clock: [CabinetGate.open, paid, autoRenewFx.done],
  source: CabinetGate.status,
  filter: Boolean,
  fn: () => undefined,
  target: walletQuery.start,
});
sample({ clock: CabinetGate.open, fn: () => undefined, target: devicesQuery.start });
sample({ clock: CabinetGate.open, fn: () => ({}), target: historyFx });
sample({
  clock: historyMore,
  source: $historyNext,
  filter: (n) => Boolean(n.before),
  target: historyFx,
});

sample({ clock: autoRenewToggled, target: autoRenewFx });
sample({ clock: deviceRevoked, target: revokeFx });
sample({
  clock: revokeFx.doneData,
  filter: (d) => !d.current,
  fn: () => undefined,
  target: devicesQuery.start,
});
// Завершили текущую сессию — это выход.
sample({ clock: revokeFx.doneData, filter: (d) => d.current, target: sessionLost });
