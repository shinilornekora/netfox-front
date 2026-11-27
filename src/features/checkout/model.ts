// Оплата — клиентская сторона саги billing.
//
//  - Idempotency-Key создаётся при открытии окна и живёт, пока окно открыто:
//    двойной клик или повтор после обрыва сети вернут тот же заказ (200), а не второй.
//  - 201 — оплачено; 402 — банк отказал (заказ в теле ответа, это результат, а не сбой);
//  - 202 — банк не ответил: заказ «в обработке», опрашиваем GET /orders/{id},
//    пока recovery на сервере не доведёт его до конца.
//  - После отказа ключ меняется: повтор с той же карты — это уже новый заказ.
import { attach, createEffect, createEvent, createStore, sample } from 'effector';
import { ApiError } from '@/shared/api/client';
import { api, type CheckoutIn } from '@/shared/api/endpoints';
import type { Order } from '@/shared/api/types';
import { errorMessage } from '@/shared/lib/format';
import { newKey } from '@/shared/lib/id';

export interface CheckoutTarget {
  kind: 'subscription' | 'purchase';
  plan_id?: string;
  title_id?: string;
  /** Что покупаем — для заголовка окна. */
  label: string;
  amount_kopecks: number;
}

export type Phase = 'form' | 'paying' | 'polling' | 'done' | 'failed';

/** Пауза между опросами заказа; в тестах — 0. */
export const pollDelay = { ms: 1500 };

export const opened = createEvent<CheckoutTarget>();
export const closed = createEvent();
export const cardChanged = createEvent<string>();
export const submitted = createEvent();
export const retried = createEvent();
/** Оплата прошла: модели страниц перезапрашивают права и кошелёк. */
export const paid = createEvent<Order>();

export const $target = createStore<CheckoutTarget | null>(null)
  .on(opened, (_, t) => t)
  .reset(closed);
export const $card = createStore('')
  .on(cardChanged, (_, c) => c)
  .reset(opened);
export const $key = createStore('').on(opened, () => newKey());
export const $order = createStore<Order | null>(null).reset(opened, retried);
export const $error = createStore<string | null>(null).reset(opened, submitted);
export const $phase = createStore<Phase>('form').reset(opened, retried);

export const checkoutFx = createEffect(
  async ({ body, key }: { body: CheckoutIn; key: string }): Promise<{ status: number; order: Order }> => {
    try {
      const r = await api.checkout(body, key);
      return { status: r.status, order: r.data };
    } catch (e) {
      if (e instanceof ApiError && e.status === 402 && e.body && typeof e.body === 'object' && 'id' in e.body)
        return { status: 402, order: e.body as Order };
      throw e;
    }
  },
);

export const pollFx = createEffect(async (id: string) => {
  await new Promise((r) => setTimeout(r, pollDelay.ms));
  return api.order(id);
});

const payFx = attach({
  source: { target: $target, card: $card, key: $key },
  mapParams: (_: void, { target, card, key }) => ({
    key,
    body: {
      kind: target!.kind,
      plan_id: target!.plan_id,
      title_id: target!.title_id,
      card_number: card.replace(/\D/g, ''),
    },
  }),
  effect: checkoutFx,
});

sample({
  clock: submitted,
  source: { target: $target, phase: $phase },
  filter: ({ target, phase }) => target !== null && phase === 'form',
  fn: () => undefined,
  target: payFx,
});

$phase
  .on(payFx, () => 'paying')
  .on(payFx.doneData, (_, r) =>
    r.order.status === 'completed' ? 'done' : r.order.status === 'charging' ? 'polling' : 'failed',
  )
  .on(payFx.fail, () => 'form')
  .on(pollFx.doneData, (p, o) => (o.status === 'completed' ? 'done' : o.status === 'charging' ? p : 'failed'))
  .on(pollFx.fail, () => 'polling');

$order.on(payFx.doneData, (_, r) => r.order).on(pollFx.doneData, (_, o) => o);
$error.on(payFx.failData, (_, e) => errorMessage(e));

// Отказ банка: следующий заход — новый заказ с новым ключом.
$key.on(retried, () => newKey());

// Пока заказ в обработке и окно открыто — опрашиваем.
sample({
  clock: [payFx.doneData.map((r) => r.order), pollFx.doneData],
  source: $target,
  filter: (t, o) => t !== null && o.status === 'charging',
  fn: (_, o) => o.id,
  target: pollFx,
});
sample({
  clock: pollFx.fail,
  source: { target: $target, order: $order },
  filter: ({ target, order }) => target !== null && order !== null,
  fn: ({ order }) => order!.id,
  target: pollFx,
});

sample({
  clock: [payFx.doneData.map((r) => r.order), pollFx.doneData],
  filter: (o) => o.status === 'completed',
  target: paid,
});
