import { describe, expect, it } from 'vitest';
import { makeStore } from '@/app/store';
import { db, signIn } from '@/test/gateway';
import * as m from './model';

describe('плеер', () => {
  it('купленный фильм стартует с места остановки', async () => {
    signIn('u-anna');
    const store = makeStore();
    await store.dispatch(m.watchOpened({ id: 't-general' }));
    expect(store.getState().watch.start?.resume_at_sec).toBe(1800);
    expect(store.getState().watch.start?.via).toBe('purchase');
    expect(db.views[0]?.title_id).toBe('t-general');
  });

  it('без прав — 402 not_entitled', async () => {
    signIn('u-anna');
    const store = makeStore();
    await store.dispatch(m.watchOpened({ id: 't-metropolis' }));
    expect(store.getState().watch.startError).toMatchObject({ status: 402, code: 'not_entitled' });
  });

  it('18+ без даты рождения — 403', async () => {
    signIn('u-max');
    db.subscriptions.push({
      id: 's-x',
      user_id: 'u-max',
      plan_id: 'month',
      plan_name: 'Месяц',
      started_at: '2026-01-01T00:00:00Z',
      current_period_end: '2099-01-01T00:00:00Z',
      cancel_at_period_end: false,
    });
    const store = makeStore();
    await store.dispatch(m.watchOpened({ id: 't-haxan' }));
    expect(store.getState().watch.startError?.message).toBe(
      'Для фильмов 18+ укажите дату рождения в профиле',
    );
  });

  it('прогресс сохраняется и не дублируется на той же секунде', async () => {
    signIn('u-anna');
    const store = makeStore();
    await store.dispatch(m.watchOpened({ id: 't-general' }));
    await store.dispatch(m.positionReported({ position: 2400.4, duration: 4680 }));
    const p = db.progress.find((x) => x.title_id === 't-general')!;
    expect(p.position_sec).toBe(2400);
    const updated = p.updated_at;
    await store.dispatch(m.positionReported({ position: 2400.9, duration: 4680 }));
    expect(db.progress.find((x) => x.title_id === 't-general')!.updated_at).toBe(updated);
  });
});
