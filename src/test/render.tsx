import { render } from '@testing-library/react';
import { Provider } from 'react-redux';
import { createMemoryRouter, RouterProvider } from 'react-router-dom';
import { routes } from '@/app/routes';
import { makeStore } from '@/app/store';
import { signIn } from './gateway';
import { users } from './fixtures';

/**
 * Рендерит всё приложение со своим Redux-стором:
 * у каждого теста своё состояние, как у отдельной вкладки браузера.
 */
export function renderApp(path = '/', opts: { as?: keyof typeof users } = {}) {
  // «Браузер с живой сессией»: cookie plenka_sid есть, SPA восстановит её через /auth/session.
  if (opts.as) signIn(users[opts.as]!.id);
  const store = makeStore();
  const router = createMemoryRouter(routes, { initialEntries: [path] });
  const utils = render(
    <Provider store={store}>
      <RouterProvider router={router} />
    </Provider>,
  );
  return { store, router, ...utils };
}
