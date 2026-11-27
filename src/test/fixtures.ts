// Тестовые данные: классика немого кино, как в сиде Плёнки (scripts/seed.py).
import type { Episode, Plan, Title, User } from '@/shared/api/types';

const now = '2026-11-01T10:00:00Z';

export const users: Record<string, User & { password: string }> = {
  viewer: {
    id: 'u-anna',
    email: 'anna@example.com',
    display_name: 'Анна Зрителева',
    birth_date: '1995-05-12',
    role: 'viewer',
    created_at: now,
    revision: 1,
    password: 'password123',
  },
  // Без даты рождения: фильмы 18+ ему закрыты, пока не укажет её в профиле.
  newbie: {
    id: 'u-max',
    email: 'max@example.com',
    display_name: 'Максим Новичков',
    birth_date: null,
    role: 'viewer',
    created_at: now,
    revision: 1,
    password: 'password123',
  },
  admin: {
    id: 'u-admin',
    email: 'admin@plenka.local',
    display_name: 'Администратор',
    birth_date: '1990-01-01',
    role: 'admin',
    created_at: now,
    revision: 1,
    password: 'admin12345',
  },
};

export function title(over: Partial<Title> = {}): Title {
  return {
    id: 't-metropolis',
    kind: 'film',
    name: 'Метрополис',
    original_name: 'Metropolis',
    year: 1927,
    description: 'Город будущего, где элита живёт в небоскрёбах, а рабочие — в подземельях у машин.',
    age_rating: 12,
    genres: ['фантастика', 'драма'],
    duration_sec: 9180,
    in_subscription: true,
    price_kopecks: 19900,
    poster_hue: 210,
    status: 'published',
    published_at: '2026-10-01T10:00:00Z',
    updated_at: now,
    revision: 2,
    episode_count: 0,
    ...over,
  };
}

export function episode(over: Partial<Episode> = {}): Episode {
  return { id: 'e-1', season: 1, number: 1, name: 'Тень гильотины', duration_sec: 3240, ...over };
}

export function catalog(): Title[] {
  return [
    title(),
    title({
      id: 't-moon',
      name: 'Путешествие на Луну',
      original_name: 'Le Voyage dans la Lune',
      year: 1902,
      description: 'Учёные стреляют снарядом из пушки и попадают Луне прямо в глаз.',
      age_rating: 0,
      genres: ['фантастика', 'приключения'],
      duration_sec: 780,
      price_kopecks: 0,
      poster_hue: 45,
      published_at: '2026-10-05T10:00:00Z',
    }),
    title({
      id: 't-nosferatu',
      name: 'Носферату. Симфония ужаса',
      original_name: 'Nosferatu, eine Symphonie des Grauens',
      year: 1922,
      description: 'Агент по недвижимости едет в Трансильванию к графу Орлоку.',
      age_rating: 16,
      genres: ['ужасы'],
      duration_sec: 5640,
      in_subscription: false,
      price_kopecks: 14900,
      poster_hue: 280,
      published_at: '2026-10-10T10:00:00Z',
    }),
    title({
      id: 't-general',
      name: 'Генерал',
      original_name: 'The General',
      year: 1926,
      description: 'Машинист в одиночку отбивает свой паровоз у армии северян.',
      age_rating: 6,
      genres: ['комедия', 'приключения'],
      duration_sec: 4680,
      price_kopecks: 9900,
      poster_hue: 20,
      published_at: '2026-10-12T10:00:00Z',
    }),
    title({
      id: 't-haxan',
      name: 'Ведьмы',
      original_name: 'Häxan',
      year: 1922,
      description: 'Документально-игровое исследование колдовства от Средневековья до XX века.',
      age_rating: 18,
      genres: ['ужасы', 'документальный'],
      duration_sec: 6300,
      in_subscription: true,
      price_kopecks: 0,
      poster_hue: 0,
      published_at: '2026-10-15T10:00:00Z',
    }),
    title({
      id: 't-fantomas',
      kind: 'series',
      name: 'Фантомас',
      original_name: 'Fantômas',
      year: 1913,
      description: 'Неуловимый гений преступного мира против инспектора Жюва.',
      age_rating: 12,
      genres: ['детектив', 'приключения'],
      duration_sec: 0,
      in_subscription: true,
      price_kopecks: 0,
      poster_hue: 150,
      published_at: '2026-10-20T10:00:00Z',
      episode_count: 3,
      episodes: [
        episode(),
        episode({ id: 'e-2', number: 2, name: 'Жюв против Фантомаса', duration_sec: 3720 }),
        episode({ id: 'e-3', number: 3, name: 'Мертвец-убийца', duration_sec: 5400 }),
      ],
    }),
    title({
      id: 't-greed',
      name: 'Алчность',
      original_name: 'Greed',
      year: 1924,
      description: 'Дантист, его жена и лотерейный билет на пять тысяч долларов.',
      age_rating: 16,
      genres: ['драма'],
      duration_sec: 8400,
      in_subscription: false,
      price_kopecks: 0,
      poster_hue: 60,
      status: 'draft',
      published_at: null,
      revision: 1,
    }),
  ];
}

export const plans: Plan[] = [
  { id: 'month', name: 'Месяц', period_days: 30, price_kopecks: 29900 },
  { id: 'year', name: 'Год', period_days: 365, price_kopecks: 299000 },
];

/** Тестовые карты платёжного симулятора Плёнки (services/billing/internal/adapter/paysim). */
export const cardNumbers = {
  ok: '4242424242424242',
  declined: '4000000000000002',
  insufficient: '4000000000009995',
  lost: '4000000000000259',
} as const;
