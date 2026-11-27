import type { Revenue } from '@/shared/api/types';
import { formatDate, formatMoney } from '@/shared/lib/format';
import { Card, Stat } from '@/shared/ui';

/** Выручка: сводные цифры и столбики по дням. */
export function RevenueOverview({ rev }: { rev: Revenue }) {
  const max = Math.max(1, ...rev.days.map((d) => d.kopecks));
  return (
    <>
      <div className="stats">
        <Stat label="Выручка" value={formatMoney(rev.total_kopecks)} />
        <Stat label="Оплачено заказов" value={rev.completed_orders} />
        <Stat label="Возвратов" value={rev.refunded_orders} />
        <Stat label="Отказов банка" value={rev.failed_orders} />
        <Stat label="Активных подписок" value={rev.active_subscriptions} />
      </div>
      {rev.days.length > 0 && (
        <Card as="section">
          <h2>Выручка по дням</h2>
          <div className="bars neu-inset" role="img" aria-label="Выручка по дням">
            {rev.days.map((d) => (
              <div
                key={d.day}
                style={{ height: `${Math.max(4, (d.kopecks / max) * 100)}%` }}
                title={`${formatDate(d.day)}: ${formatMoney(d.kopecks)}, заказов ${d.orders}`}
              />
            ))}
          </div>
        </Card>
      )}
    </>
  );
}
