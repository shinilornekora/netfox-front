// Связи между слайсами: listener middleware реагирует на экшены и изменения состояния
// и запускает thunk-и (оплата прошла → перечитать кошелёк и права, сменилась роль → данные зрителя).
import { createListenerMiddleware } from '@reduxjs/toolkit';
import { selectRole } from '@/entities/session/model';
import { paid } from '@/features/checkout/model';
import { walletRefreshed as cabinetWalletRefreshed } from '@/pages/cabinet/model';
import { continueRequested } from '@/pages/catalog/model';
import { walletRefreshed as plansWalletRefreshed } from '@/pages/plans/model';
import { offerRequested, viewerDataRequested } from '@/pages/title/model';
import type { AppDispatch, RootState } from './store';

export function createListeners() {
  const mw = createListenerMiddleware();
  const listen = mw.startListening.withTypes<RootState, AppDispatch>();

  // Оплата прошла — права и кошелёк изменились.
  listen({
    actionCreator: paid,
    effect: (_, api) => {
      const s = api.getState();
      void api.dispatch(plansWalletRefreshed());
      void api.dispatch(cabinetWalletRefreshed());
      if (s.title.id !== null) void api.dispatch(offerRequested(s.title.id));
    },
  });

  // Сменилась роль (вошли/вышли), пока страница открыта, — дозагружаем данные зрителя.
  listen({
    predicate: (_, cur, prev) => selectRole(cur) !== selectRole(prev),
    effect: (_, api) => {
      const s = api.getState();
      if (s.catalog.open && selectRole(s) === 'viewer') void api.dispatch(continueRequested());
      void api.dispatch(plansWalletRefreshed());
      void api.dispatch(viewerDataRequested());
    },
  });

  return mw;
}
