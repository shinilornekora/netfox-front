import { Link } from 'react-router-dom';
import { selectUser } from '@/entities/session';
import { DeviceList } from '@/widgets/device-list';
import { OrderList } from '@/widgets/order-list';
import { PurchaseList } from '@/widgets/purchase-list';
import { SubscriptionPanel } from '@/widgets/subscription-panel';
import { WatchHistory } from '@/widgets/watch-history';
import { RoutePath } from '@/shared/constants';
import { formatDate } from '@/shared/lib/format';
import { usePageModel } from '@/shared/lib/hooks';
import { useAppDispatch, useAppSelector } from '@/shared/lib/redux';
import { Alert, Spinner, Stat, Tabs } from '@/shared/ui';
import * as m from '../model';

/** Кабинет зрителя: сводка и вкладки, каждая вкладка — свой виджет. */
export default function CabinetPage() {
  const dispatch = useAppDispatch();
  usePageModel(m.cabinetOpened, m.cabinetClosed);
  const user = useAppSelector(selectUser);
  const c = useAppSelector(m.selectCabinet);
  const { tab, history, historyNext: next, actionError, renewPending } = c;
  const wallet = c.wallet.data;
  const walletError = c.wallet.error;
  const setTab = (t: m.Tab) => dispatch(m.tabChanged(t));
  const sub = wallet?.subscription ?? null;

  return (
    <div className="page stack">
      <div className="row-between">
        <div>
          <h1>Личный кабинет</h1>
          <p className="muted">{user?.display_name}</p>
        </div>
        <Link to={RoutePath.profile} className="neu-btn small">
          Профиль и пароль
        </Link>
      </div>

      {walletError && <Alert>{walletError}</Alert>}
      {wallet && (
        <div className="stats">
          <Stat label="Подписка" value={sub ? `до ${formatDate(sub.current_period_end)}` : 'нет'} />
          <Stat label="Куплено фильмов" value={wallet.purchases.length} />
          <Stat label="Заказов" value={wallet.orders.length} />
        </div>
      )}

      <Tabs
        value={tab}
        onChange={setTab}
        items={[
          { value: 'subscription', label: 'Подписка' },
          { value: 'purchases', label: `Покупки (${wallet?.purchases.length ?? 0})` },
          { value: 'orders', label: 'Заказы' },
          { value: 'history', label: 'История' },
          { value: 'devices', label: 'Устройства' },
        ]}
      />
      {actionError && <Alert>{actionError}</Alert>}

      {!wallet ? (
        <Spinner />
      ) : tab === 'subscription' ? (
        <SubscriptionPanel sub={sub} renewPending={renewPending} />
      ) : tab === 'purchases' ? (
        <PurchaseList items={wallet.purchases} />
      ) : tab === 'orders' ? (
        <OrderList items={wallet.orders} />
      ) : tab === 'history' ? (
        <WatchHistory
          items={history}
          hasMore={Boolean(next.before)}
          onMore={() => void dispatch(m.historyMore())}
        />
      ) : (
        <DeviceList items={c.devices.data?.items ?? []} />
      )}
    </div>
  );
}
