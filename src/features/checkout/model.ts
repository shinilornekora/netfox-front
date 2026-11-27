// Оплата — клиентская сторона саги billing.
//
//  - Idempotency-Key создаётся при открытии окна и живёт, пока окно открыто:
//    двойной клик или повтор после обрыва сети вернут тот же заказ (200), а не второй.
//  - 201 — оплачено; 402 — банк отказал (заказ в теле ответа, это результат, а не сбой);
//  - 202 — банк не ответил: заказ «в обработке», опрашиваем GET /orders/{id},
//    пока recovery на сервере не доведёт его до конца.
//  - После отказа ключ меняется: повтор с той же карты — это уже новый заказ.
import { createAction, createAsyncThunk, createSlice, type PayloadAction } from '@reduxjs/toolkit';
import { ApiError } from '@/shared/api/client';
import { api, type CheckoutIn } from '@/shared/api/endpoints';
import type { Order } from '@/shared/api/types';
import { errorMessage } from '@/shared/lib/format';
import { newKey } from '@/shared/lib/id';
import type { RootState } from '@/shared/lib/redux';

export interface CheckoutTarget {
  kind: 'subscription' | 'purchase';
  plan_id?: string;
  title_id?: string;
  /** Что покупаем — для заголовка окна. */
  label: string;
  amount_kopecks: number;
}

export type Phase = 'form' | 'paying' | 'polling' | 'done' | 'failed';

export interface CheckoutState {
  target: CheckoutTarget | null;
  card: string;
  /** Idempotency-Key текущего заказа. */
  key: string;
  order: Order | null;
  error: string | null;
  phase: Phase;
}

/** Пауза между опросами заказа; в тестах — 0. */
export const pollDelay = { ms: 1500 };

const initialState: CheckoutState = {
  target: null,
  card: '',
  key: '',
  order: null,
  error: null,
  phase: 'form',
};

/** Оплата прошла: модели страниц перезапрашивают права и кошелёк (см. app/listeners). */
export const paid = createAction<Order>('checkout/paid');

const phaseAfter = (o: Order, current: Phase): Phase =>
  o.status === 'completed' ? 'done' : o.status === 'charging' ? current : 'failed';

export const checkoutSlice = createSlice({
  name: 'checkout',
  initialState,
  reducers: {
    opened: {
      // Ключ генерируется в prepare: редьюсер остаётся чистой функцией.
      reducer(s, a: PayloadAction<{ target: CheckoutTarget; key: string }>) {
        s.target = a.payload.target;
        s.key = a.payload.key;
        s.card = '';
        s.order = null;
        s.error = null;
        s.phase = 'form';
      },
      prepare: (target: CheckoutTarget) => ({ payload: { target, key: newKey() } }),
    },
    closed(s) {
      s.target = null;
    },
    cardChanged(s, a: PayloadAction<string>) {
      s.card = a.payload;
    },
    /** Отказ банка: следующий заход — новый заказ с новым ключом. */
    retried: {
      reducer(s, a: PayloadAction<string>) {
        s.key = a.payload;
        s.order = null;
        s.phase = 'form';
      },
      prepare: () => ({ payload: newKey() }),
    },
    errorCleared(s) {
      s.error = null;
    },
    payStarted(s) {
      s.phase = 'paying';
    },
    payAnswered(s, a: PayloadAction<Order>) {
      s.order = a.payload;
      s.phase = phaseAfter(a.payload, 'polling');
    },
    payFailed(s, a: PayloadAction<string>) {
      s.phase = 'form';
      s.error = a.payload;
    },
    polled(s, a: PayloadAction<Order>) {
      s.order = a.payload;
      s.phase = phaseAfter(a.payload, s.phase);
    },
    pollFailed(s) {
      s.phase = 'polling';
    },
  },
});

export const { opened, closed, cardChanged, retried } = checkoutSlice.actions;
const { errorCleared, payStarted, payAnswered, payFailed, polled, pollFailed } = checkoutSlice.actions;

/** POST /billing/checkout. 402 — не сбой: в теле ответа заказ с причиной отказа. */
export async function checkoutRequest({
  body,
  key,
}: {
  body: CheckoutIn;
  key: string;
}): Promise<{ status: number; order: Order }> {
  try {
    const r = await api.checkout(body, key);
    return { status: r.status, order: r.data };
  } catch (e) {
    if (e instanceof ApiError && e.status === 402 && e.body && typeof e.body === 'object' && 'id' in e.body)
      return { status: 402, order: e.body as Order };
    throw e;
  }
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

/** Пока заказ в обработке и окно открыто — опрашиваем его. */
const pollOrder = createAsyncThunk('checkout/poll', async (id: string, { getState, dispatch }) => {
  while ((getState() as RootState).checkout.target !== null) {
    await sleep(pollDelay.ms);
    try {
      const o = await api.order(id);
      dispatch(polled(o));
      if (o.status === 'completed') dispatch(paid(o));
      if (o.status !== 'charging') return;
    } catch {
      // Сеть моргнула — заказ никуда не делся, спрашиваем ещё раз.
      dispatch(pollFailed());
    }
  }
});

/** «Оплатить»: заказ с ключом идемпотентности, затем — опрос, если банк не ответил сразу. */
export const submitted = createAsyncThunk('checkout/submit', async (_: void, { getState, dispatch }) => {
  dispatch(errorCleared());
  const { target, card, key, phase } = (getState() as RootState).checkout;
  if (!target || phase !== 'form') return;
  dispatch(payStarted());
  let order: Order;
  try {
    ({ order } = await checkoutRequest({
      key,
      body: {
        kind: target.kind,
        plan_id: target.plan_id,
        title_id: target.title_id,
        card_number: card.replace(/\D/g, ''),
      },
    }));
  } catch (e) {
    dispatch(payFailed(errorMessage(e)));
    return;
  }
  dispatch(payAnswered(order));
  if (order.status === 'completed') dispatch(paid(order));
  else if (order.status === 'charging') await dispatch(pollOrder(order.id));
});

export const selectCheckout = (s: RootState) => s.checkout;
