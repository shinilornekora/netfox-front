import { describe, expect, it } from 'vitest';
import { allSettled, fork } from 'effector';
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
  const scope = fork();
  await allSettled(m.opened, { scope, params: buyNosferatu });
  await allSettled(m.cardChanged, { scope, params: card });
  await allSettled(m.submitted, { scope });
  return scope;
}

describe('оплата', () => {
  it('успешная карта: заказ оплачен, фильм выдан', async () => {
    const scope = await pay(cardNumbers.ok);
    expect(scope.getState(m.$phase)).toBe('done');
    expect(db.purchases.some((p) => p.user_id === 'u-anna' && p.title_id === 't-nosferatu')).toBe(true);
  });

  it('402 — это результат: показываем причину отказа', async () => {
    const scope = await pay(cardNumbers.insufficient);
    expect(scope.getState(m.$phase)).toBe('failed');
    expect(scope.getState(m.$order)?.failure).toBe('Недостаточно средств на карте');
  });

  it('после отказа повтор идёт с новым ключом — новый заказ', async () => {
    const scope = await pay(cardNumbers.declined);
    const firstKey = scope.getState(m.$key);
    await allSettled(m.retried, { scope });
    expect(scope.getState(m.$key)).not.toBe(firstKey);
    await allSettled(m.cardChanged, { scope, params: cardNumbers.ok });
    await allSettled(m.submitted, { scope });
    expect(scope.getState(m.$phase)).toBe('done');
    expect(db.orders.filter((o) => o.title_id === 't-nosferatu')).toHaveLength(2);
  });

  it('202 — опрашиваем заказ, пока recovery не доведёт его до конца', async () => {
    const scope = await pay(cardNumbers.lost);
    expect(scope.getState(m.$phase)).toBe('done');
    expect(db.polls[scope.getState(m.$order)!.id]).toBe(2);
  });

  it('повтор с тем же ключом не создаёт второй заказ', async () => {
    const scope = await pay(cardNumbers.ok);
    const before = db.orders.length;
    await allSettled(m.checkoutFx, {
      scope,
      params: {
        key: scope.getState(m.$key),
        body: { kind: 'purchase', title_id: 't-nosferatu', card_number: cardNumbers.ok },
      },
    });
    expect(db.orders).toHaveLength(before);
  });

  it('конфликт (уже куплен) — понятная ошибка, окно остаётся на форме', async () => {
    signIn('u-anna');
    const scope = fork();
    await allSettled(m.opened, {
      scope,
      params: { kind: 'purchase', title_id: 't-general', label: 'Генерал', amount_kopecks: 9900 },
    });
    await allSettled(m.cardChanged, { scope, params: cardNumbers.ok });
    await allSettled(m.submitted, { scope });
    expect(scope.getState(m.$phase)).toBe('form');
    expect(scope.getState(m.$error)).toBe('Фильм уже куплен — он есть в разделе «Покупки»');
  });
});
