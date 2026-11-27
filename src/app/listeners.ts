// Связи между слайсами. В FSD фича не знает, какие страницы её используют,
// а страница не может импортировать другую страницу. Поэтому «после действия X
// перечитать данные Y» описано здесь, в app — единственном слое, который видит всех:
// listener middleware реагирует на экшены и изменения состояния и запускает thunk-и.
import { createListenerMiddleware } from '@reduxjs/toolkit';
import { selectRole } from '@/entities/session';
import { paid } from '@/features/checkout';
import { deviceRevoked } from '@/features/device-revoke';
import { deleteEpisode, saveEpisode } from '@/features/episode-editor';
import { refunded } from '@/features/order-refund';
import { autoRenewToggled } from '@/features/subscription-autorenew';
import { cabinetWalletRefreshed, devicesRequested } from '@/pages/cabinet';
import { continueRequested, selectCatalogOpen } from '@/pages/catalog';
import { plansWalletRefreshed } from '@/pages/plans';
import { salesRefreshed } from '@/pages/admin-sales';
import { offerRequested, selectOpenTitleId, viewerDataRequested } from '@/pages/title';
import { titleReloaded } from '@/pages/title-edit';
import type { AppDispatch, RootState } from './store';

export function createListeners() {
  const mw = createListenerMiddleware();
  const listen = mw.startListening.withTypes<RootState, AppDispatch>();

  // Оплата прошла — права и кошелёк изменились.
  listen({
    actionCreator: paid,
    effect: (_, api) => {
      const id = selectOpenTitleId(api.getState());
      void api.dispatch(plansWalletRefreshed());
      void api.dispatch(cabinetWalletRefreshed());
      if (id !== null) void api.dispatch(offerRequested(id));
    },
  });

  // Сменилась роль (вошли/вышли), пока страница открыта, — дозагружаем данные зрителя.
  listen({
    predicate: (_, cur, prev) => selectRole(cur) !== selectRole(prev),
    effect: (_, api) => {
      const s = api.getState();
      if (selectCatalogOpen(s) && selectRole(s) === 'viewer') void api.dispatch(continueRequested());
      void api.dispatch(plansWalletRefreshed());
      void api.dispatch(viewerDataRequested());
    },
  });

  // Автопродление переключили — в кабинете новая дата и надпись на кнопке.
  listen({
    actionCreator: autoRenewToggled.fulfilled,
    effect: (_, api) => void api.dispatch(cabinetWalletRefreshed()),
  });

  // Завершили сеанс на другом устройстве — перечитать список (своё устройство — это выход).
  listen({
    actionCreator: deviceRevoked.fulfilled,
    effect: (a, api) => {
      if (!a.payload.current) void api.dispatch(devicesRequested());
    },
  });

  // Серии изменились — у тайтла новый список и ревизия.
  for (const done of [saveEpisode.fulfilled, deleteEpisode.fulfilled]) {
    listen({
      actionCreator: done,
      effect: (a, api) => void api.dispatch(titleReloaded(a.meta.arg.titleId)),
    });
  }

  // Возврат денег — перечитать выручку и заказы.
  listen({
    actionCreator: refunded.fulfilled,
    effect: (_, api) => void api.dispatch(salesRefreshed()),
  });

  return mw;
}
