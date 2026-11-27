// Маршруты приложения: объект-конфиг для createBrowserRouter (data router).
// В тестах тот же массив отдаётся в createMemoryRouter.
// Сами пути (RoutePath) лежат в shared/constants — ими пользуются и меню, и ссылки.
import { lazy, useEffect } from 'react';
import type { RouteObject } from 'react-router-dom';
import { restoreSession, sessionLost } from '@/entities/session/model';
import { useAppDispatch } from '@/shared/lib/redux';
import { onUnauthorized } from '@/shared/api/client';
import { RoutePath } from '@/shared/constants';
import { ErrorPage, ForbiddenPage, NotFoundPage } from '@/pages/not-found';
import { GuestOnly, RequireAuth, RequireRole } from './guards';
import { Layout } from './Layout';

// Каждая страница — отдельный чанк: админку зритель не скачивает.
const CatalogPage = lazy(() => import(/* webpackChunkName: "page-catalog" */ '@/pages/catalog'));
const TitlePage = lazy(() => import(/* webpackChunkName: "page-title" */ '@/pages/title'));
const WatchPage = lazy(() => import(/* webpackChunkName: "page-watch" */ '@/pages/watch'));
const PlansPage = lazy(() => import(/* webpackChunkName: "page-plans" */ '@/pages/plans'));
const LoginPage = lazy(() => import(/* webpackChunkName: "page-auth-login" */ '@/pages/auth/LoginPage'));
const RegisterPage = lazy(
  () => import(/* webpackChunkName: "page-auth-register" */ '@/pages/auth/RegisterPage'),
);
const CabinetPage = lazy(() => import(/* webpackChunkName: "page-cabinet" */ '@/pages/cabinet'));
const ProfilePage = lazy(() => import(/* webpackChunkName: "page-profile" */ '@/pages/profile'));
const WatchlistPage = lazy(() => import(/* webpackChunkName: "page-watchlist" */ '@/pages/watchlist'));
const AdminTitlesPage = lazy(
  () => import(/* webpackChunkName: "page-admin-titles" */ '@/pages/admin-titles'),
);
const TitleEditPage = lazy(() => import(/* webpackChunkName: "page-title-edit" */ '@/pages/title-edit'));
const SalesPage = lazy(() => import(/* webpackChunkName: "page-admin-sales" */ '@/pages/admin-sales'));

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
