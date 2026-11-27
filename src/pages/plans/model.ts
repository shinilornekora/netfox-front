import { sample } from 'effector';
import { createGate } from 'effector-react';
import { api } from '@/shared/api/endpoints';
import { createRequest } from '@/shared/lib/request';
import { $role } from '@/entities/session/model';
import { paid } from '@/features/checkout/model';

export const PlansGate = createGate<void>();
export const plansQuery = createRequest((_: void) => api.plans());
export const walletQuery = createRequest((_: void) => api.wallet());

sample({ clock: PlansGate.open, fn: () => undefined, target: plansQuery.start });
sample({
  clock: [PlansGate.open, $role.updates, paid],
  source: { role: $role, open: PlansGate.status },
  filter: ({ role, open }) => open && role === 'viewer',
  fn: () => undefined,
  target: walletQuery.start,
});

export const $subscription = walletQuery.$data.map((w) => w?.subscription ?? null);
