// Демо-режим (GitHub Pages и `npm run dev:mock`): бэкенд эмулируется прямо в браузере
// через MSW — те же обработчики, что и в интеграционных тестах. Состояние «базы»
// сохраняется в localStorage, поэтому покупки, прогресс и вход переживают перезагрузку.
import { createDb, db, handlers, replaceDb, type Db } from '@/test/gateway';
import { pollDelay } from '@/features/checkout/model';

const STORAGE_KEY = 'nf_demo_db_v1';

export const demoAccounts = [
  { role: 'Зритель', email: 'anna@example.com', password: 'password123' },
  { role: 'Зритель без даты рождения', email: 'max@example.com', password: 'password123' },
  { role: 'Администратор', email: 'admin@plenka.local', password: 'admin12345' },
] as const;

/** Короткий ролик CC0 вместо фильма: в демо нет сервера с видео. */
const DEMO_STREAM = 'https://interactive-examples.mdn.mozilla.net/media/cc0-videos/flower.mp4';

export function demoDb(): Db {
  const d = createDb();
  d.demoStream = DEMO_STREAM;
  return d;
}

function load(): Db | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? (JSON.parse(raw) as Db) : null;
  } catch {
    return null;
  }
}

function save() {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(db));
  } catch {
    /* приватный режим: демо просто не переживёт перезагрузку */
  }
}

/** Сбросить демо-данные к исходным. */
export function resetDemo() {
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    /* ignore */
  }
  replaceDb(demoDb());
}

export async function startDemo() {
  replaceDb(load() ?? demoDb());
  pollDelay.ms = 1200;
  const { setupWorker } = await import('msw/browser');
  const worker = setupWorker(...handlers);
  worker.events.on('response:mocked', () => save());
  await worker.start({
    onUnhandledRequest: 'bypass',
    quiet: true,
    serviceWorker: {
      url: `${import.meta.env.BASE_URL}mockServiceWorker.js`,
      options: { scope: import.meta.env.BASE_URL },
    },
  });
}
