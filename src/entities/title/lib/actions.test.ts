import { describe, expect, it } from 'vitest';
import { title } from '@/test/fixtures';
import { actionsFor } from './actions';

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
    expect(actionsFor(title(), offer()).map((a) => a.type)).toEqual(['subscribe', 'buy']);
  });

  it('с правами — только «Смотреть», с указанием, откуда доступ', () => {
    expect(actionsFor(title(), offer({ can_watch: true, via: 'purchase' }))).toEqual([
      { type: 'watch', via: 'purchase' },
    ]);
  });

  it('сериал не продаётся поштучно; фильм вне подписки — только покупка', () => {
    expect(actionsFor(title({ kind: 'series', price_kopecks: 0 }), offer()).map((a) => a.type)).toEqual([
      'subscribe',
    ]);
    expect(actionsFor(title({ in_subscription: false }), offer()).map((a) => a.type)).toEqual(['buy']);
  });

  it('снятый с показа недоступен', () => {
    expect(actionsFor(title({ status: 'withdrawn' }), null)).toEqual([{ type: 'unavailable' }]);
  });
});
