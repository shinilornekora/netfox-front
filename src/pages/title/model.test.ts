import { describe, expect, it } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/setup';
import { db, signIn } from '@/test/gateway';
import { makeStore } from '@/app/store';
import { setWatch } from '@/features/watchlist-toggle';
import * as m from './model';

describe('модель карточки', () => {
  it('зрителю грузит права и состояние просмотра', async () => {
    signIn('u-anna');
    const store = makeStore({ session: { status: 'authenticated', user: db.users[0]! } });
    await store.dispatch(m.titleOpened('t-general'));
    expect(store.getState().title.title.data?.name).toBe('Генерал');
    expect(store.getState().title.offer.data?.owned).toBe(true);
    expect(m.selectView(store.getState()).progress?.position_sec).toBe(1800);
  });

  it('гостю права не запрашивает', async () => {
    const store = makeStore();
    await store.dispatch(m.titleOpened('t-general'));
    expect(store.getState().title.offer.data).toBeNull();
  });

  it('«Смотреть позже» откатывается, если сервер не принял', async () => {
    signIn('u-anna');
    server.use(
      http.put('*/api/playback/watchlist/:id', () =>
        HttpResponse.json({ status: 503, code: 'unavailable', title: 'Сервис недоступен' }, { status: 503 }),
      ),
    );
    const store = makeStore({ session: { status: 'authenticated', user: db.users[0]! } });
    await store.dispatch(m.titleOpened('t-metropolis'));
    expect(m.selectInWatchlist(store.getState())).toBe(false);
    await store.dispatch(setWatch({ id: 't-metropolis', on: true }));
    expect(m.selectInWatchlist(store.getState())).toBe(false);
    expect(m.selectWatchError(store.getState())).toBe('Сервис недоступен');
  });
});
