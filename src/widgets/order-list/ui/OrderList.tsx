import type { Order } from '@/shared/api/types';
import { formatDateTime, formatMoney, orderStatusLabel, orderTone } from '@/shared/lib/format';
import { Badge, Empty } from '@/shared/ui';

/** Заказы зрителя: дата, что куплено, сумма, карта и статус. */
export function OrderList({ items }: { items: Order[] }) {
  if (items.length === 0) return <Empty>Заказов пока нет</Empty>;
  return (
    <div className="table-wrap">
      <table className="table" aria-label="Заказы">
        <thead>
          <tr>
            <th>Дата</th>
            <th>Что</th>
            <th>Сумма</th>
            <th>Карта</th>
            <th>Статус</th>
          </tr>
        </thead>
        <tbody>
          {items.map((o) => (
            <tr key={o.id}>
              <td data-label="Дата">{formatDateTime(o.created_at)}</td>
              <td data-label="Что">{o.kind === 'subscription' ? 'Подписка' : o.title_name}</td>
              <td data-label="Сумма">{formatMoney(o.amount_kopecks)}</td>
              <td data-label="Карта">•••• {o.card_last4}</td>
              <td data-label="Статус">
                <Badge tone={orderTone[o.status]}>{orderStatusLabel[o.status]}</Badge>
                {o.failure && <div className="muted">{o.failure}</div>}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
