# Netfox — онлайн-кинотеатр

**Демо:** https://shinilornekora.github.io/netfox-front/ (бэкенд эмулируется в браузере, входы: `anna@example.com` / `password123`, `max@example.com` / `password123`, `admin@plenka.local` / `admin12345`)

Онлайн-кинотеатр с двумя ролями: **зритель** и **администратор**. В репозитории всё приложение: SPA (корень) и бэкенд — микросервисы на Go в `backend/` (gateway, identity, catalog, billing, playback; перенесены из [Плёнки](https://github.com/shinilornekora/plenka)). Фронт ходит только в `gateway` по `/api`.

## Запуск одной командой

Нужен только [Docker](https://www.docker.com/products/docker-desktop/) (Docker Desktop на Windows и macOS). Из корня репозитория:

```bash
docker compose up -d --build
```

Через 3–6 минут при первом запуске (потом — секунды) откройте **http://localhost:8080**. Команда собирает и поднимает PostgreSQL, Kafka, пять сервисов бэкенда, фронтенд (nginx) и сама наполняет каталог: 16 фильмов и сериалов немого кино.

На Windows то же самое — двойной клик по **`start.bat`**: он проверит Docker, поднимет всё, дождётся каталога и откроет сайт. На Linux и macOS — `./start.sh`. Остановить: `stop.bat` или `docker compose down`; стереть данные и начать заново: `reset.bat` или `docker compose down -v`.

| Кто           | Вход                                                                                  |
| ------------- | ------------------------------------------------------------------------------------- |
| Администратор | `admin@plenka.local` / `admin12345`                                                   |
| Зритель       | зарегистрируйтесь на сайте (пароль от 10 символов, буквы и цифры)                     |
| Оплата        | `4242 4242 4242 4242` — успешно; остальные тестовые карты есть кнопками в окне оплаты |

Вместо фильмов плеер показывает 60-секундный ролик `deploy/media/sample.mp4`. Сами фильмы (около 12 ГБ, archive.org) скачивает `scripts/download_films.bat` — каждый становится доступен сразу после загрузки.

**Стек:** TypeScript · React 18 · Redux Toolkit (+ react-redux) · React Router (data router) · webpack (основной сборщик) и Vite (второй конфиг) · Vitest · Testing Library · MSW. Стиль — неоморфизм на чистом CSS без UI-библиотек, тёмная тема по системной настройке, боковое меню с адаптивом. Стек, стиль и критерии приёмки взяты из фронтенда турфирмы [TourFlow](https://github.com/shinilornekora/tourflow-front).

## Возможности по ролям

| Роль          | Что доступно                                                                                                                                                                                                                                                                                                                                                            |
| ------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Гость         | Каталог с поиском, фильтрами (тип, жанр, доступ) и сортировкой, «Показать ещё» по курсору; карточка фильма или сериала со списком серий; тарифы подписки; вход и регистрация                                                                                                                                                                                            |
| Зритель       | Покупка фильма навсегда и оформление подписки (тестовые карты, ключ идемпотентности, отказ банка, «потерянный ответ» с опросом заказа); плеер с продолжением с места остановки и пингами прогресса; «Продолжить просмотр»; «Смотреть позже»; кабинет: подписка и автопродление, покупки, заказы, история, устройства; профиль с датой рождения (для 18+) и смена пароля |
| Администратор | Каталог тайтлов с фильтром по статусу; создание фильма или сериала, редактирование с контролем ревизии, серии (добавить, удалить), публикация, снятие с показа с причиной, удаление черновика; продажи: выручка, график по дням, заказы с фильтром, возврат с причиной                                                                                                  |

## Разработка фронтенда

```bash
npm install
npm start              # webpack-dev-server на :3000, HMR + React Fast Refresh, /api → gateway (VITE_GATEWAY_URL, по умолчанию :8080)
npm run start:mock     # то же без бэкенда: фейковый gateway на MSW в браузере
npm run build          # продакшен-сборка webpack в dist/
npm run build:analyze  # то же + webpack-bundle-analyzer
npm run dev            # Vite dev-сервер (второй сборщик), dev:mock — без бэкенда
npm run build:vite     # продакшен-сборка Vite
npm run build:pages    # демо-сборка для GitHub Pages
```

Бэкенд для разработки — тот же `docker compose up -d --build`: nginx с `/api` слушает `:8080`, это адрес по умолчанию для прокси dev-сервера. Если gateway запущен отдельно (`HTTP_ADDR=:8090`), укажите `VITE_GATEWAY_URL=http://localhost:8090`.

**Проверки:** `npm run typecheck` (tsc), `npm run lint` (ESLint), `npm run format:check` (Prettier), `npm run check` — всё сразу. Перед коммитом husky запускает lint-staged (eslint --fix + prettier на изменённых файлах) и проверку типов.

**Оптимизация прод-сборки webpack:** `mode: production` (минификация terser и tree shaking), `contenthash` в именах файлов, `splitChunks` с vendor-чанками React и Redux, `runtimeChunk`, ленивые страницы через `React.lazy` с именованными чанками, извлечение и минификация CSS, `hidden-source-map`. Демо-код (MSW) вырезается из обычной сборки через `DefinePlugin`.

## Тесты

```bash
npm test                    # все тесты (72)
npm run test:unit           # юнит: слайсы и thunk-и Redux, HTTP-клиент, утилиты, UI-кит
npm run test:integration    # интеграционные: всё приложение против фейкового gateway
npm run coverage
```

- **Юнит-тесты** (`*.test.ts(x)` рядом с кодом) проверяют слайсы Redux на отдельном сторе `makeStore()` для каждого теста: диспатчат thunk, дожидаются его (`await store.dispatch(...)`) и смотрят состояние через селекторы — сессию по cookie, витрину с фильтрами и курсорной пагинацией без дублей, кнопки доступа на карточке, оптимистичное «Смотреть позже» с откатом, плеер (402, 403 для 18+, дедупликация пингов), оплату (201, 402 как результат, 202 с опросом, новый ключ после отказа, повтор с тем же ключом без второго заказа), HTTP-клиент (CSRF-заголовок, разбор problem+json, сеть, 401).
- **Интеграционные тесты** (`src/test/integration/*.int.test.tsx`) рендерят приложение целиком (роутер + Redux + UI) и проходят сценарии гостя, зрителя и администратора. Сеть подменяет MSW: `src/test/gateway.ts` — in-memory gateway с теми же контрактами, что у сервисов Плёнки, включая правила валидации (пароль от 10 символов с буквами и цифрами, готовность к публикации, цены) и тексты ошибок.

## Соответствие требованиям курсовой

| Требование                                                        | Где                                                                                                                                                                                        |
| ----------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Роли и разный интерфейс                                           | Меню и кнопки зависят от роли (`widgets/sidebar`, `widgets/title-hero`), гарды маршрутов `RequireAuth`, `RequireRole` (`app/guards.tsx`)                                                   |
| Правила предметной области                                        | Смотреть можно только с правами (подписка или покупка), сериал не продаётся поштучно, 18+ требует дату рождения, админ не покупает и не смотрит, удалить можно только черновик             |
| Создание, получение, изменение, удаление                          | Тайтлы (создать, изменить, опубликовать, снять, удалить черновик), серии (добавить, удалить), заказы (создать оплатой, вернуть), «Смотреть позже», профиль, устройства (завершить сеанс)   |
| Фильтры, сортировка, пагинация                                    | Каталог: поиск, тип, жанр, доступ, сортировка, «Показать ещё» по `next_cursor`; история просмотров — keyset по `before`; фильтры тайтлов и заказов в админке                               |
| Страницы                                                          | Регистрация, вход, 404, «нет доступа» и страница ошибки роутера, каталог, карточка, плеер, тарифы, личный кабинет, профиль, «Смотреть позже», админка каталога, редактор, продажи          |
| Маршрутизация                                                     | `createBrowserRouter` + `RouterProvider`, `errorElement`, вложенные маршруты с `Outlet`, `RoutePath`, ленивые страницы (`app/routes.tsx`)                                                  |
| React 18, TypeScript, webpack dev/prod, ESLint + Prettier + husky | см. `package.json`, `webpack/`, `eslint.config.mjs`, `.husky/`                                                                                                                             |
| Хуки                                                              | `useState`, `useEffect` с очисткой, `useContext`, `useCallback`, `useRef`, `useId`; свои хуки `useDebounce` (автопоиск в каталоге), `usePageModel`, `useProgressPing` в `shared/lib/hooks` |
| Архитектура                                                       | Feature-Sliced Design: слои app/pages/widgets/features/entities/shared, публичный API слайсов, правила импорта в ESLint                                                                    |
| Тесты                                                             | 52 юнит и 27 интеграционных, CI в `.github/workflows/ci.yml`; ручной прогон всех сценариев против настоящего бэкенда Плёнки                                                                |

## Структура репозитория

```
docker-compose.yml   всё приложение одной командой
Dockerfile           образ фронтенда: сборка webpack → nginx (nginx.conf проксирует /api в gateway)
start.bat, start.sh  запуск в один клик; stop.bat, reset.bat
src/                 SPA (Feature-Sliced, см. ниже)
backend/             Go: go.work, contracts, kit, services/{gateway,identity,catalog,billing,playback}, tools
backend/Dockerfile   один образ на все сервисы, аргумент SERVICE выбирает какой
deploy/              init.sql (схемы и роли PostgreSQL), media/sample.mp4
scripts/             seed.py (наполнение каталога), e2e.py, chaos.py, download_films.bat
docs/backend/        архитектура, решения и замеры бэкенда (ARCHITECTURE.md, TRADEOFFS.md, evidence)
```

Пути в `docs/backend` написаны для исходного репозитория Плёнки: `services/…`, `kit/…` теперь лежат в `backend/`, а `frontend/` заменён этим SPA.

### Фронтенд (Feature-Sliced Design)

Слои сверху вниз: `app → pages → widgets → features → entities → shared`. Слой импортирует только слои ниже себя, слайсы одного слоя друг друга не видят, а в чужой слайс входят только через его публичный API — `index.ts` (`@/features/checkout`, но не `@/features/checkout/model`). Внутри слайса — сегменты `ui/`, `model.ts`, `lib/`. Правила проверяет ESLint (`@typescript-eslint/no-restricted-imports` в `eslint.config.mjs`), так что нарушение не пройдёт ни pre-commit, ни CI.

```
src/
  app/        маршруты (createBrowserRouter), гарды ролей, layout, стили-неоморфизм;
              store.ts — Redux-стор, listeners.ts — связи между слайсами
  pages/      страницы: index.ts (React.lazy — отдельный чанк) + ui/ + model.ts (слайс страницы)
              catalog, title, watch, plans, login, register, cabinet, profile, watchlist,
              admin-titles, title-edit, admin-sales, not-found
  widgets/    самостоятельные блоки страниц из фич и сущностей:
              sidebar (меню и верхняя полоска), catalog-feed, continue-watching,
              title-hero, episode-list, subscription-panel, purchase-list, order-list,
              watch-history, device-list, revenue-overview, sales-orders
  features/   действия пользователя:
              auth (вход, регистрация, выход), catalog-filters (фильтры и автопоиск),
              checkout (окно оплаты, сага billing), watchlist-toggle («Смотреть позже»),
              subscription-autorenew, device-revoke, profile-edit, password-change,
              title-moderation (опубликовать, снять, удалить), episode-editor, order-refund
  entities/   session (кто вошёл, login/register/logout), title (постер, карточка, actionsFor)
  shared/     api (клиент + эндпоинты + типы), constants (RoutePath, pathTo), lib (в т. ч. свои хуки lib/hooks), ui-кит
  test/       MSW-gateway, фикстуры, интеграционные тесты
  demo/       демо-режим для GitHub Pages
```

Как слои договариваются, не нарушая правил:

- **Фича не знает, где её используют.** Кнопка «Смотреть позже» диспатчит `setWatch`; карточка тайтла и страница списка сами реагируют на него в своих `extraReducers` (страница может импортировать фичу, наоборот — нет).
- **«После X перечитать Y»** — в `app/listeners.ts`, единственном слое, который видит всех: после оплаты — кошелёк и права, после автопродления — кошелёк кабинета, после завершения чужого сеанса — список устройств, после изменения серий — тайтл в редакторе, после возврата — выручка и заказы.
- **Страница — композиция.** `pages/*/ui` собирают виджеты и фичи и передают им данные своей модели; бизнес-логика — в `model.ts` и фичах.

## Бэкенд: проверки

```bash
# один раз: роль без SUPERUSER (суперпользователь обходит RLS, тесты такую роль не примут)
docker compose exec postgres psql -U plenka -c "CREATE ROLE plenka_test LOGIN PASSWORD 'plenka_test'" \
  -c "CREATE DATABASE netfox_test OWNER plenka_test"
cd backend
export TEST_PG_DSN="postgres://plenka_test:plenka_test@localhost:5433/netfox_test?sslmode=disable"
for m in kit tools services/*; do (cd $m && go test -count=1 -p 1 ./...); done
python3 ../scripts/e2e.py http://localhost:8080      # сквозной сценарий через gateway
```

CI (`.github/workflows/ci.yml`) проверяет фронтенд (типы, линт, формат, тесты, обе сборки), бэкенд (`go vet` и тесты на PostgreSQL) и сборку всех Docker-образов.

## Состояние: Redux Toolkit

Всё состояние приложения — один Redux-стор (`src/app/store.ts`), собранный `configureStore` из слайсов. Слайс живёт в `model.ts` своего слайса FSD и попадает в стор через публичный API:

| Слайс                                             | Файл                                                  | Что хранит                                                                      |
| ------------------------------------------------- | ----------------------------------------------------- | ------------------------------------------------------------------------------- |
| `session`                                         | `entities/session/model.ts`                           | статус входа и профиль; thunk-и `restoreSession`, `login`, `register`, `logout` |
| `auth`                                            | `features/auth/model.ts`                              | ошибки и «идёт запрос» форм входа и регистрации                                 |
| `catalogFilters`                                  | `features/catalog-filters/model.ts`                   | фильтры витрины: название, тип, жанр, доступ, сортировка                        |
| `checkout`                                        | `features/checkout/model.ts`                          | окно оплаты: цель, карта, ключ идемпотентности, заказ, фаза                     |
| `catalog`                                         | `pages/catalog/model.ts`                              | лента, курсор, жанры, «Продолжить просмотр»                                     |
| `title`, `watch`, `plans`, `cabinet`, `watchlist` | `pages/*/model.ts`                                    | карточка, плеер, тарифы, кабинет, «Смотреть позже»                              |
| `adminTitles`, `titleEdit`, `sales`               | `pages/admin-*/model.ts`, `pages/title-edit/model.ts` | админка                                                                         |

Как это устроено:

- **Чтение** — селекторы (`selectRole`, `selectView`…) через типизированный `useAppSelector`; составные значения — `createSelector`, чтобы компонент не перерисовывался зря.
- **Синхронные изменения** — экшены из `createSlice` (`filterChanged`, `cardChanged`, `tabChanged`). Редьюсеры чистые: ключ идемпотентности генерируется в `prepare`, а не в редьюсере.
- **Запросы** — `createAsyncThunk` через обёртку `createApiThunk` (`shared/lib/request.ts`): ошибка API превращается в сериализуемый `Problem` (`message`, `code`, `status`, `fields`) в `action.payload`. Типовое «данные / ошибка / загрузка» добавляет `addRequestCases`.
- **Открытие страницы** — компонент в `useEffect` диспатчит `…Opened` (thunk, который запускает нужные запросы) и при уходе `…Closed`.
- **Действия-фичи без своего слайса** (`setWatch`, `autoRenewToggled`, `deviceRevoked`, `publishTitle`, `saveEpisode`, `refunded`…) — thunk-и в `features/*/model.ts`; их ошибки и «идёт запрос» показывают модели страниц через `extraReducers`.
- **Связи между слайсами** — `listenerMiddleware` (`app/listeners.ts`): после `checkout/paid` перечитываются кошелёк и права, при смене роли — данные зрителя на открытой странице, после действий-фич — данные страниц, которые от них зависят.
- **Тесты** — `makeStore()` создаёт новый стор на каждый тест, как отдельную вкладку браузера.

## Сессия и безопасность

Сессия — HttpOnly-cookie `plenka_sid`, которую ставит identity: JS её не видит, поэтому в приложении нет токена, а состояние входа — ответ `GET /api/auth/session` (гостю он отвечает `{user: null}`, это не ошибка). Каждый запрос несёт заголовок `X-Plenka-Client`: без него gateway отклоняет изменяющие запросы (защита от CSRF). Если обычный запрос получил 401 (сеанс завершили с другого устройства или сменили пароль), клиент сообщает об этом, `Root` диспатчит `sessionLost`, и интерфейс переходит в гостевой режим.

## Оплата

Клиентская сторона оркестрируемой саги billing (`features/checkout/model.ts`, thunk `submitted` и опрос `pollOrder`):

- `Idempotency-Key` создаётся при открытии окна: двойной клик или повтор после обрыва вернут тот же заказ (200), а не второй;
- 201 — оплачено, права выданы; 402 — банк отказал, заказ с причиной приходит в теле, это результат, а не сбой; 409 — понятная ошибка (уже куплен, подписка уже есть, предыдущая оплата в обработке);
- 202 — банк не ответил: заказ «в обработке», окно опрашивает `GET /api/billing/orders/{id}`, пока recovery на сервере не доведёт его до конца;
- после отказа ключ меняется: повтор — это новый заказ.

Тестовые карты платёжного симулятора есть в окне кнопками: `4242 4242 4242 4242` (успешно), `4000 0000 0000 0002` (банк отклонил), `4000 0000 0000 9995` (мало средств), `4000 0000 0000 0259` (ответ потерялся).

## Демо на GitHub Pages

На каждый пуш в `main` workflow `.github/workflows/pages.yml` собирает демо (`VITE_MOCK=1`) и кладёт его в ветку `gh-pages`. В демо работает тот же фейковый gateway, что и в интеграционных тестах; данные лежат в `localStorage` браузера и сбрасываются кнопкой «Сбросить». Вместо фильмов плеер показывает короткий ролик CC0. Глубокие ссылки работают через `404.html` (копия `index.html`).
