// Redux-стор приложения. Каждый слайс — модель своей страницы или фичи (FSD:
// pages/*/model.ts, features/*/model.ts, entities/*/model.ts), здесь они
// собираются в одно дерево. makeStore — фабрика: в тестах у каждого теста
// свой стор, как у отдельной вкладки браузера.
import { combineReducers, configureStore } from '@reduxjs/toolkit';
import { sessionSlice } from '@/entities/session/model';
import { checkoutSlice } from '@/features/checkout/model';
import { adminTitlesSlice } from '@/pages/admin-titles/model';
import { salesSlice } from '@/pages/admin-sales/model';
import { authSlice } from '@/pages/auth/model';
import { cabinetSlice } from '@/pages/cabinet/model';
import { catalogSlice } from '@/pages/catalog/model';
import { plansSlice } from '@/pages/plans/model';
import { titleSlice } from '@/pages/title/model';
import { titleEditSlice } from '@/pages/title-edit/model';
import { watchSlice } from '@/pages/watch/model';
import { watchlistSlice } from '@/pages/watchlist/model';
import { createListeners } from './listeners';

export const rootReducer = combineReducers({
  [sessionSlice.reducerPath]: sessionSlice.reducer,
  [authSlice.reducerPath]: authSlice.reducer,
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
