import { Link } from 'react-router-dom';
import type { Subscription } from '@/shared/api/types';
import { AutoRenewButton } from '@/features/subscription-autorenew';
import { RoutePath } from '@/shared/constants';
import { formatDate } from '@/shared/lib/format';
import { Card, Empty } from '@/shared/ui';

/** Текущая подписка и автопродление (фича subscription-autorenew). */
export function SubscriptionPanel({
  sub,
  renewPending,
}: {
  sub: Subscription | null;
  renewPending: boolean;
}) {
  if (!sub)
    return (
      <Empty>
        Подписки нет. <Link to={RoutePath.plans}>Выбрать тариф</Link>
      </Empty>
    );
  return (
    <Card className="stack" as="section">
      <h2>Подписка «{sub.plan_name}»</h2>
      <p>
        Действует до <b>{formatDate(sub.current_period_end)}</b>.{' '}
        {sub.cancel_at_period_end
          ? 'Продление отключено: после этой даты подписка закончится.'
          : 'Продлится автоматически.'}
      </p>
      <div className="row">
        <AutoRenewButton cancelAtPeriodEnd={sub.cancel_at_period_end} pending={renewPending} />
      </div>
    </Card>
  );
}
