import { render } from '@testing-library/react';
import { fork, type Scope } from 'effector';
import { Provider } from 'effector-react';
import { createMemoryRouter, RouterProvider } from 'react-router-dom';
import { routes } from '@/app/routes';
import { signIn } from './gateway';
import { users } from './fixtures';

/**
 * Рендерит всё приложение в изолированном scope Effector:
 * у каждого теста своё состояние сторов, как у отдельной вкладки браузера.
 */
export function renderApp(path = '/', opts: { as?: keyof typeof users } = {}) {
  // «Браузер с живой сессией»: cookie plenka_sid есть, SPA восстановит её через /auth/session.
  if (opts.as) signIn(users[opts.as]!.id);
  const scope: Scope = fork();
  const router = createMemoryRouter(routes, { initialEntries: [path] });
  const utils = render(
    <Provider value={scope}>
      <RouterProvider router={router} />
    </Provider>,
  );
  return { scope, router, ...utils };
}
