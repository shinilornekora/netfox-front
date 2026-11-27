// Redux-стор приложения. Слайсы живут в своих слоях FSD — сущности (session),
// фичи (auth, catalogFilters, checkout) и страницы (модель каждой страницы) —
// и берутся только через публичный API слайса (его index.ts). Здесь они
// собираются в одно дерево. makeStore — фабрика: в тестах у каждого теста
// свой стор, как у отдельной вкладки браузера.
import { combineReducers, configureStore } from '@reduxjs/toolkit';
import { sessionSlice } from '@/entities/session';
import { authSlice } from '@/features/auth';
import { catalogFiltersSlice } from '@/features/catalog-filters';
import { checkoutSlice } from '@/features/checkout';
import { adminTitlesSlice } from '@/pages/admin-titles';
import { salesSlice } from '@/pages/admin-sales';
import { cabinetSlice } from '@/pages/cabinet';
import { catalogSlice } from '@/pages/catalog';
import { plansSlice } from '@/pages/plans';
import { titleSlice } from '@/pages/title';
import { titleEditSlice } from '@/pages/title-edit';
import { watchSlice } from '@/pages/watch';
import { watchlistSlice } from '@/pages/watchlist';
import { createListeners } from './listeners';

export const rootReducer = combineReducers({
  [sessionSlice.reducerPath]: sessionSlice.reducer,
  [authSlice.reducerPath]: authSlice.reducer,
  [catalogFiltersSlice.reducerPath]: catalogFiltersSlice.reducer,
  [checkoutSlice.reducerPath]: checkoutSlice.reducer,
  [catalogSlice.reducerPath]: catalogSlice.reducer,
  [titleSlice.reducerPath]: titleSlice.reducer,
  [watchSlice.reducerPath]: watchSlice.reducer,
  [plansSlice.reducerPath]: plansSlice.reducer,
  [cabinetSlice.reducerPath]: cabinetSlice.reducer,
  [watchlistSlice.reducerPath]: watchlistSlice.reducer,
  [adminTitlesSlice.reducerPath]: adminTitlesSlice.reducer,
  [titleEditSlice.reducerPath]: titleEditSlice.reducer,
  [salesSlice.reducerPath]: salesSlice.reducer,
});

export type RootState = ReturnType<typeof rootReducer>;

export function makeStore(preloadedState?: Partial<RootState>) {
  return configureStore({
    reducer: rootReducer,
    preloadedState,
    middleware: (getDefault) => getDefault().prepend(createListeners().middleware),
  });
}

export type AppStore = ReturnType<typeof makeStore>;
export type AppDispatch = AppStore['dispatch'];
