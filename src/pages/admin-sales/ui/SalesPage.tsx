import { RevenueOverview } from '@/widgets/revenue-overview';
import { SalesOrders } from '@/widgets/sales-orders';
import { usePageModel } from '@/shared/lib/hooks';
import { useAppDispatch, useAppSelector } from '@/shared/lib/redux';
import { Alert, Spinner } from '@/shared/ui';
import * as m from '../model';

/** Продажи (админ): выручка и заказы с возвратами. */
export default function SalesPage() {
  const dispatch = useAppDispatch();
  usePageModel(m.salesOpened);
  const sales = useAppSelector(m.selectSales);
  const plans = sales.plans.data;
  const rev = sales.revenue.data;
  const planName = (id?: string) => plans?.items.find((p) => p.id === id)?.name ?? id;

  return (
    <div className="page stack">
      <h1>Продажи</h1>
      {sales.revenue.error && <Alert>{sales.revenue.error}</Alert>}
      {!rev ? <Spinner /> : <RevenueOverview rev={rev} />}
      <SalesOrders
        orders={sales.items}
        loaded={Boolean(sales.orders.data)}
        status={sales.status}
        onStatus={(s) => void dispatch(m.statusChanged(s))}
        planName={planName}
        error={sales.refundError}
        hasMore={sales.nextBefore !== ''}
        onMore={() => void dispatch(m.moreRequested())}
      />
    </div>
  );
}
