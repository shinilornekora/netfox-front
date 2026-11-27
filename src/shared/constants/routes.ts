// Все пути приложения в одном месте. Конфиг роутера (app/routes.tsx), меню,
// гарды и ссылки берут пути отсюда, поэтому строка '/login' нигде не повторяется.
import { generatePath } from 'react-router-dom';

export const RoutePath = {
  catalog: '/',
  title: '/title/:id',
  watch: '/watch/:id',
  plans: '/plans',
  login: '/login',
  register: '/register',
  forbidden: '/forbidden',
  cabinet: '/cabinet',
  profile: '/cabinet/profile',
  watchlist: '/watchlist',
  adminTitles: '/admin/titles',
  titleNew: '/admin/titles/new',
  titleEdit: '/admin/titles/:id',
  sales: '/admin/sales',
  notFound: '*',
} as const;

/** Параметры динамических маршрутов (для useParams<…>()). */
export type IdParams = { id: string };

/**
 * Ссылки на маршруты с параметрами. generatePath проверяет имена параметров
 * по типу шаблона: опечатка в `{ id }` — ошибка компиляции, а не битая ссылка.
 */
export const pathTo = {
  title: (id: string) => generatePath(RoutePath.title, { id }),
  watch: (id: string, ep?: string) =>
    generatePath(RoutePath.watch, { id }) + (ep ? `?ep=${encodeURIComponent(ep)}` : ''),
  titleEdit: (id: string) => generatePath(RoutePath.titleEdit, { id }),
};
