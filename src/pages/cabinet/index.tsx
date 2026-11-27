import { useGate, useUnit } from 'effector-react';
import { Link } from 'react-router-dom';
import { $user } from '@/entities/session/model';
import {
  formatDate,
  formatDateTime,
  formatMoney,
  kindLabel,
  orderStatusLabel,
  orderTone,
} from '@/shared/lib/format';
import { Alert, Badge, Button, Card, Empty, Spinner, Stat, Tabs } from '@/shared/ui';
import * as m from './model';

export default function CabinetPage() {
  useGate(m.CabinetGate);
  const [user, tab, setTab, wallet, walletError, devices, history, next, actionError] = useUnit([
    $user,
    m.$tab,
    m.tabChanged,
    m.walletQuery.$data,
    m.walletQuery.$error,
    m.devicesQuery.$data,
    m.$history,
    m.$historyNext,
    m.$actionError,
  ]);
  const [toggleRenew, revoke, more, renewPending] = useUnit([
    m.autoRenewToggled,
    m.deviceRevoked,
    m.historyMore,
    m.autoRenewFx.pending,
  ]);
  const sub = wallet?.subscription ?? null;

  return (
    <div className="page stack">
      <div className="row-between">
        <div>
          <h1>Личный кабинет</h1>
          <p className="muted">{user?.display_name}</p>
        </div>
        <Link to="/cabinet/profile" className="neu-btn small">
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
        sub ? (
          <Card className="stack" as="section">
            <h2>Подписка «{sub.plan_name}»</h2>
            <p>
              Действует до <b>{formatDate(sub.current_period_end)}</b>.{' '}
              {sub.cancel_at_period_end
                ? 'Продление отключено: после этой даты подписка закончится.'
                : 'Продлится автоматически.'}
            </p>
            <div className="row">
              <Button disabled={renewPending} onClick={() => toggleRenew(sub.cancel_at_period_end)}>
                {sub.cancel_at_period_end ? 'Включить автопродление' : 'Отключить автопродление'}
              </Button>
            </div>
          </Card>
        ) : (
          <Empty>
            Подписки нет. <Link to="/plans">Выбрать тариф</Link>
          </Empty>
        )
      ) : tab === 'purchases' ? (
        wallet.purchases.length === 0 ? (
          <Empty>Купленных фильмов пока нет</Empty>
        ) : (
          <div className="stack">
            {wallet.purchases.map((p) => (
              <Card key={p.title_id} as="article" className="row-between">
                <div>
                  <Link to={`/title/${p.title_id}`}>
                    <b>{p.title_name}</b>
                  </Link>
                  <div className="muted">
                    Куплен {formatDate(p.purchased_at)} за {formatMoney(p.amount_kopecks)}
                  </div>
                </div>
                <Link to={`/watch/${p.title_id}`} className="neu-btn small primary">
                  Смотреть
                </Link>
              </Card>
            ))}
          </div>
        )
      ) : tab === 'orders' ? (
        wallet.orders.length === 0 ? (
          <Empty>Заказов пока нет</Empty>
        ) : (
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
                {wallet.orders.map((o) => (
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
        )
      ) : tab === 'history' ? (
        history.length === 0 ? (
          <Empty>Вы ещё ничего не смотрели</Empty>
        ) : (
          <div className="stack">
            {history.map((h) => (
              <Card key={h.id} as="article" className="row-between">
                <div>
                  <Link to={`/title/${h.title_id}`}>
                    <b>{h.title.name}</b>
                  </Link>
                  <div className="muted">
                    {kindLabel[h.title.kind]} · {formatDateTime(h.started_at)} ·{' '}
                    {h.via === 'purchase' ? 'покупка' : 'подписка'}
                  </div>
                </div>
                {!h.title.available && <Badge tone="warn">Снят с показа</Badge>}
              </Card>
            ))}
            {next.before && (
              <div className="row" style={{ justifyContent: 'center' }}>
                <Button onClick={() => more()}>Показать ещё</Button>
              </div>
            )}
          </div>
        )
      ) : (
        <div className="stack">
          {(devices?.items ?? []).map((d) => (
            <Card key={d.id} as="article" className="row-between">
              <div>
                <b>{d.user_agent}</b> {d.current && <Badge tone="ok">Это устройство</Badge>}
                <div className="muted">
                  {d.ip} · вход {formatDateTime(d.created_at)} · активность {formatDateTime(d.last_seen_at)}
                </div>
              </div>
              <Button small variant="danger" onClick={() => revoke({ id: d.id, current: d.current })}>
                {d.current ? 'Выйти' : 'Завершить сеанс'}
              </Button>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
