// Маршруты приложения: объект-конфиг для createBrowserRouter (data router).
// В тестах тот же массив отдаётся в createMemoryRouter.
// Сами пути (RoutePath) лежат в shared/constants — ими пользуются и меню, и ссылки.
import { useEffect } from 'react';
import type { RouteObject } from 'react-router-dom';
import { restoreSession, sessionLost } from '@/entities/session';
import { useAppDispatch } from '@/shared/lib/redux';
import { onUnauthorized } from '@/shared/api/client';
import { RoutePath } from '@/shared/constants';
// Каждая страница — отдельный чанк (React.lazy в index.ts страницы): админку зритель не скачивает.
import { AdminTitlesPage } from '@/pages/admin-titles';
import { CabinetPage } from '@/pages/cabinet';
import { CatalogPage } from '@/pages/catalog';
import { LoginPage } from '@/pages/login';
import { ErrorPage, ForbiddenPage, NotFoundPage } from '@/pages/not-found';
import { PlansPage } from '@/pages/plans';
import { ProfilePage } from '@/pages/profile';
import { RegisterPage } from '@/pages/register';
import { SalesPage } from '@/pages/admin-sales';
import { TitlePage } from '@/pages/title';
import { TitleEditPage } from '@/pages/title-edit';
import { WatchPage } from '@/pages/watch';
import { WatchlistPage } from '@/pages/watchlist';
import { GuestOnly, RequireAuth, RequireRole } from './guards';
import { Layout } from './Layout';

/** Корневой элемент: восстанавливает сессию и рисует общий layout. */
export function Root() {
  const dispatch = useAppDispatch();
  useEffect(() => {
    void dispatch(restoreSession());
    // Сессию отозвали (другое устройство, смена пароля) — стор узнаёт по 401.
    return onUnauthorized(() => dispatch(sessionLost()));
  }, [dispatch]);
  return <Layout />;
}

const guest = (el: JSX.Element) => <GuestOnly>{el}</GuestOnly>;

export const routes: RouteObject[] = [
  {
    path: RoutePath.catalog,
    element: <Root />,
    errorElement: <ErrorPage />,
    children: [
      { index: true, element: <CatalogPage /> },
      { path: RoutePath.title, element: <TitlePage /> },
      { path: RoutePath.plans, element: <PlansPage /> },
      { path: RoutePath.login, element: guest(<LoginPage />) },
      { path: RoutePath.register, element: guest(<RegisterPage />) },
      { path: RoutePath.forbidden, element: <ForbiddenPage /> },
      {
        element: <RequireAuth />,
        children: [
          { path: RoutePath.profile, element: <ProfilePage /> },
          {
            element: <RequireRole roles={['viewer']} />,
            children: [
              { path: RoutePath.watch, element: <WatchPage /> },
              { path: RoutePath.cabinet, element: <CabinetPage /> },
              { path: RoutePath.watchlist, element: <WatchlistPage /> },
            ],
          },
          {
            element: <RequireRole roles={['admin']} />,
            children: [
              { path: RoutePath.adminTitles, element: <AdminTitlesPage /> },
              { path: RoutePath.titleNew, element: <TitleEditPage /> },
              { path: RoutePath.titleEdit, element: <TitleEditPage /> },
              { path: RoutePath.sales, element: <SalesPage /> },
            ],
          },
        ],
      },
      { path: RoutePath.notFound, element: <NotFoundPage /> },
    ],
  },
];
