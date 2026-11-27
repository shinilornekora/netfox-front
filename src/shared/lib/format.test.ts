import { describe, expect, it } from 'vitest';
import { ApiError } from '@/shared/api/client';
import {
  ageOn,
  errorFields,
  errorMessage,
  formatCard,
  formatClock,
  formatDate,
  formatDuration,
  formatMoney,
  plural,
} from './format';

describe('форматирование', () => {
  it('деньги из копеек в рубли', () => {
    expect(formatMoney(39900).replace(/\s/g, ' ')).toBe('399 ₽');
    expect(formatMoney(null)).toBe('—');
  });

  it('дата без сдвига часового пояса', () => {
    expect(formatDate('2026-12-10T23:30:00Z')).toBe('10 декабря 2026 г.');
  });

  it('длительность фильма и позиция плеера', () => {
    expect(formatDuration(5400)).toBe('1 ч 30 мин');
    expect(formatDuration(2700)).toBe('45 мин');
    expect(formatDuration(7200)).toBe('2 ч');
    expect(formatClock(65)).toBe('1:05');
    expect(formatClock(3725)).toBe('1:02:05');
  });

  it('склонение', () => {
    expect(plural(1, 'фильм', 'фильма', 'фильмов')).toBe('фильм');
    expect(plural(3, 'фильм', 'фильма', 'фильмов')).toBe('фильма');
    expect(plural(11, 'фильм', 'фильма', 'фильмов')).toBe('фильмов');
    expect(plural(22, 'фильм', 'фильма', 'фильмов')).toBe('фильма');
  });

  it('возраст считается по дню рождения', () => {
    const today = new Date(2026, 10, 27);
    expect(ageOn('2008-11-27', today)).toBe(18);
    expect(ageOn('2008-11-28', today)).toBe(17);
  });

  it('номер карты: только цифры группами по 4', () => {
    expect(formatCard('4242-4242 4242x4242')).toBe('4242 4242 4242 4242');
  });

  it('ошибки API', () => {
    const e = new ApiError(422, 'invalid', 'Проверьте данные формы', { email: 'Неверный email' });
    expect(errorMessage(e)).toBe('Проверьте данные формы');
    expect(errorFields(e)).toEqual({ email: 'Неверный email' });
    expect(errorFields(new Error('x'))).toEqual({});
  });
});
