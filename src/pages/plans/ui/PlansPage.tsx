import { Link } from 'react-router-dom';
import { selectRole } from '@/entities/session';
import { useAppSelector } from '@/shared/lib/redux';
import { CheckoutButton } from '@/features/checkout';
import { formatDate, formatMoney, plural } from '@/shared/lib/format';
import { Alert, Card, Spinner } from '@/shared/ui';
import { usePageModel } from '@/shared/lib/hooks';
import * as m from '../model';
import { RoutePath } from '@/shared/constants';

export default function PlansPage() {
  usePageModel(m.plansOpened, m.plansClosed);
  const plans = useAppSelector(m.selectPlans);
  const error = useAppSelector(m.selectPlansError);
  const sub = useAppSelector(m.selectSubscription);
  const role = useAppSelector(selectRole);

  return (
    <div className="page stack">
      <div>
        <h1>Подписка</h1>
        <p className="muted">
          Все фильмы и сериалы с отметкой «В подписке». Отменить продление можно в кабинете.
        </p>
      </div>
      {error && <Alert>{error}</Alert>}
      {sub && (
        <Alert tone="success">
          У вас подписка «{sub.plan_name}» до {formatDate(sub.current_period_end)}
          {sub.cancel_at_period_end ? ' — продление отключено' : ''}.
        </Alert>
      )}
      {!plans ? (
        <Spinner />
      ) : (
        <div className="plans">
          {plans.items.map((p) => {
            const perMonth = Math.round((p.price_kopecks / p.period_days) * 30);
            return (
              <Card key={p.id} as="article" className="stack">
                <h2>{p.name}</h2>
                <div className="price">{formatMoney(p.price_kopecks)}</div>
                <p className="muted">
                  {p.period_days} {plural(p.period_days, 'день', 'дня', 'дней')}
                  {p.period_days > 31 ? ` · ≈ ${formatMoney(perMonth)} в месяц` : ''}
                </p>
                {role === null && (
                  <Link to={RoutePath.login} state={{ from: RoutePath.plans }} className="neu-btn primary">
                    Войти и оформить
                  </Link>
                )}
                {role === 'viewer' && (
                  <CheckoutButton
                    disabled={Boolean(sub)}
                    aria-label={`Оформить: ${p.name}`}
                    target={{
                      kind: 'subscription',
                      plan_id: p.id,
                      label: `подписка «${p.name}»`,
                      amount_kopecks: p.price_kopecks,
                    }}
                  >
                    {sub ? 'Уже оформлена' : 'Оформить'}
                  </CheckoutButton>
                )}
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
}
