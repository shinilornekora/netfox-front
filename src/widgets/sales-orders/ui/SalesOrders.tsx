import type { Order, OrderStatus } from '@/shared/api/types';
import { RefundButton } from '@/features/order-refund';
import { formatDateTime, formatMoney, orderStatusLabel, orderTone } from '@/shared/lib/format';
import { Alert, Badge, Button, Card, Empty, Select, Spinner } from '@/shared/ui';

/** Заказы в админке: фильтр по статусу, таблица с возвратом (фича order-refund) и догрузка. */
export function SalesOrders({
  orders,
  loaded,
  status,
  onStatus,
  planName,
  error,
  hasMore,
  onMore,
}: {
  orders: Order[];
  loaded: boolean;
  status: OrderStatus | '';
  onStatus: (s: OrderStatus | '') => void;
  planName: (id?: string) => string | undefined;
  error: string | null;
  hasMore: boolean;
  onMore: () => void;
}) {
  return (
    <Card as="section" className="stack">
      <div className="row-between">
        <h2 style={{ margin: 0 }}>Заказы</h2>
        <div style={{ minWidth: 200 }}>
          <Select
            label="Статус"
            value={status}
            onChange={(e) => onStatus(e.target.value as OrderStatus | '')}
            options={[
              { value: '', label: 'Все' },
              ...(Object.keys(orderStatusLabel) as OrderStatus[]).map((s) => ({
                value: s,
                label: orderStatusLabel[s],
              })),
            ]}
          />
        </div>
      </div>
      {error && <Alert>{error}</Alert>}
      {!loaded ? (
        <Spinner />
      ) : orders.length === 0 ? (
        <Empty>Заказов нет</Empty>
      ) : (
        <div className="table-wrap">
          <table className="table" aria-label="Заказы">
            <thead>
              <tr>
                <th>Дата</th>
                <th>Что</th>
                <th>Сумма</th>
                <th>Статус</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {orders.map((o) => (
                <tr key={o.id}>
                  <td data-label="Дата">{formatDateTime(o.created_at)}</td>
                  <td data-label="Что">
                    {o.kind === 'subscription' ? `Подписка «${planName(o.plan_id)}»` : o.title_name}
                  </td>
                  <td data-label="Сумма">{formatMoney(o.amount_kopecks)}</td>
                  <td data-label="Статус">
                    <Badge tone={orderTone[o.status]}>{orderStatusLabel[o.status]}</Badge>
                  </td>
                  <td data-label="">{o.status === 'completed' && <RefundButton order={o} />}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {hasMore && (
        <div className="row" style={{ justifyContent: 'center' }}>
          <Button onClick={() => onMore()}>Показать ещё</Button>
        </div>
      )}
    </Card>
  );
}
