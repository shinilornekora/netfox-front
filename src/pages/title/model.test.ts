import { describe, expect, it } from 'vitest';
import { allSettled, fork } from 'effector';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/setup';
import { db, signIn } from '@/test/gateway';
import { title } from '@/test/fixtures';
import { $user } from '@/entities/session/model';
import * as m from './model';

const offer = (over = {}) => ({
  known: true,
  owned: false,
  subscription_active: false,
  in_subscription: true,
  price_kopecks: 19900,
  can_watch: false,
  ...over,
});

describe('действия на карточке тайтла', () => {
  it('без прав: и подписка, и покупка фильма', () => {
    expect(m.actionsFor(title(), offer()).map((a) => a.type)).toEqual(['subscribe', 'buy']);
  });

  it('с правами — только «Смотреть», с указанием, откуда доступ', () => {
    expect(m.actionsFor(title(), offer({ can_watch: true, via: 'purchase' }))).toEqual([
      { type: 'watch', via: 'purchase' },
    ]);
  });

  it('сериал не продаётся поштучно; фильм вне подписки — только покупка', () => {
    expect(m.actionsFor(title({ kind: 'series', price_kopecks: 0 }), offer()).map((a) => a.type)).toEqual([
      'subscribe',
    ]);
    expect(m.actionsFor(title({ in_subscription: false }), offer()).map((a) => a.type)).toEqual(['buy']);
  });

  it('снятый с показа недоступен', () => {
    expect(m.actionsFor(title({ status: 'withdrawn' }), null)).toEqual([{ type: 'unavailable' }]);
  });
});

describe('модель карточки', () => {
  it('зрителю грузит права и состояние просмотра', async () => {
    signIn('u-anna');
    const scope = fork({ values: [[$user, db.users[0]!]] });
    await allSettled(m.TitleGate.open, { scope, params: { id: 't-general' } });
    expect(scope.getState(m.titleQuery.$data)?.name).toBe('Генерал');
    expect(scope.getState(m.offerQuery.$data)?.owned).toBe(true);
    expect(scope.getState(m.$view).progress?.position_sec).toBe(1800);
  });

  it('гостю права не запрашивает', async () => {
    const scope = fork();
    await allSettled(m.TitleGate.open, { scope, params: { id: 't-general' } });
    expect(scope.getState(m.offerQuery.$data)).toBeNull();
  });

  it('«Смотреть позже» откатывается, если сервер не принял', async () => {
    signIn('u-anna');
    server.use(
      http.put('*/api/playback/watchlist/:id', () =>
        HttpResponse.json({ status: 503, code: 'unavailable', title: 'Сервис недоступен' }, { status: 503 }),
      ),
    );
    const scope = fork({ values: [[$user, db.users[0]!]] });
    await allSettled(m.TitleGate.open, { scope, params: { id: 't-metropolis' } });
    expect(scope.getState(m.$inWatchlist)).toBe(false);
    await allSettled(m.watchToggled, { scope });
    expect(scope.getState(m.$inWatchlist)).toBe(false);
    expect(scope.getState(m.$watchError)).toBe('Сервис недоступен');
  });
});
