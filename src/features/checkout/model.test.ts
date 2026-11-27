import { describe, expect, it } from 'vitest';
import { makeStore } from '@/app/store';
import { db, signIn } from '@/test/gateway';
import { cardNumbers } from '@/test/fixtures';
import * as m from './model';

const buyNosferatu = {
  kind: 'purchase' as const,
  title_id: 't-nosferatu',
  label: 'Носферату',
  amount_kopecks: 14900,
};

async function pay(card: string) {
  signIn('u-anna');
  const store = makeStore();
  store.dispatch(m.opened(buyNosferatu));
  store.dispatch(m.cardChanged(card));
  await store.dispatch(m.submitted());
  return store;
}

const checkout = (store: ReturnType<typeof makeStore>) => store.getState().checkout;

describe('оплата', () => {
  it('успешная карта: заказ оплачен, фильм выдан', async () => {
    const store = await pay(cardNumbers.ok);
    expect(checkout(store).phase).toBe('done');
    expect(db.purchases.some((p) => p.user_id === 'u-anna' && p.title_id === 't-nosferatu')).toBe(true);
  });

  it('402 — это результат: показываем причину отказа', async () => {
    const store = await pay(cardNumbers.insufficient);
    expect(checkout(store).phase).toBe('failed');
    expect(checkout(store).order?.failure).toBe('Недостаточно средств на карте');
  });

  it('после отказа повтор идёт с новым ключом — новый заказ', async () => {
    const store = await pay(cardNumbers.declined);
    const firstKey = checkout(store).key;
    store.dispatch(m.retried());
    expect(checkout(store).key).not.toBe(firstKey);
    store.dispatch(m.cardChanged(cardNumbers.ok));
    await store.dispatch(m.submitted());
    expect(checkout(store).phase).toBe('done');
    expect(db.orders.filter((o) => o.title_id === 't-nosferatu')).toHaveLength(2);
  });

  it('202 — опрашиваем заказ, пока recovery не доведёт его до конца', async () => {
    const store = await pay(cardNumbers.lost);
    expect(checkout(store).phase).toBe('done');
    expect(db.polls[checkout(store).order!.id]).toBe(2);
  });

  it('повтор с тем же ключом не создаёт второй заказ', async () => {
    const store = await pay(cardNumbers.ok);
    const before = db.orders.length;
    await m.checkoutRequest({
      key: checkout(store).key,
      body: { kind: 'purchase', title_id: 't-nosferatu', card_number: cardNumbers.ok },
    });
    expect(db.orders).toHaveLength(before);
  });

  it('конфликт (уже куплен) — понятная ошибка, окно остаётся на форме', async () => {
    signIn('u-anna');
    const store = makeStore();
    store.dispatch(
      m.opened({ kind: 'purchase', title_id: 't-general', label: 'Генерал', amount_kopecks: 9900 }),
    );
    store.dispatch(m.cardChanged(cardNumbers.ok));
    await store.dispatch(m.submitted());
    expect(checkout(store).phase).toBe('form');
    expect(checkout(store).error).toBe('Фильм уже куплен — он есть в разделе «Покупки»');
  });

  it('пока окно открыто, ключ не меняется — двойной клик не создаёт второй заказ', async () => {
    const store = await pay(cardNumbers.ok);
    const key = checkout(store).key;
    await store.dispatch(m.submitted()); // фаза уже «done» — повторной оплаты нет
    expect(checkout(store).key).toBe(key);
    expect(db.orders.filter((o) => o.title_id === 't-nosferatu')).toHaveLength(1);
  });
});
