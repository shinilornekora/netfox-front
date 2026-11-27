import { useState } from 'react';
import type { Order, OrderStatus } from '@/shared/api/types';
import { formatDate, formatDateTime, formatMoney, orderStatusLabel, orderTone } from '@/shared/lib/format';
import { useAppDispatch, useAppSelector } from '@/shared/lib/redux';
import { Alert, Badge, Button, Card, Empty, Modal, Select, Spinner, Stat, TextArea } from '@/shared/ui';
import { usePageModel } from '@/shared/lib/hooks';
import * as m from './model';

export default function SalesPage() {
  const dispatch = useAppDispatch();
  usePageModel(m.salesOpened);
  const sales = useAppSelector(m.selectSales);
  const { nextBefore, status, refundError, items: orders } = sales;
  const loaded = sales.orders.data;
  const plans = sales.plans.data;
  const rev = sales.revenue.data;
  const revError = sales.revenue.error;
  const more = () => void dispatch(m.moreRequested());
  const setStatus = (s: OrderStatus | '') => void dispatch(m.statusChanged(s));
  const planName = (id?: string) => plans?.items.find((p) => p.id === id)?.name ?? id;
  const [target, setTarget] = useState<Order | null>(null);
  const [reason, setReason] = useState('');
  const max = Math.max(1, ...(rev?.days ?? []).map((d) => d.kopecks));

  return (
    <div className="page stack">
      <h1>Продажи</h1>
      {revError && <Alert>{revError}</Alert>}
      {!rev ? (
        <Spinner />
      ) : (
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
      )}

      <Card as="section" className="stack">
        <div className="row-between">
          <h2 style={{ margin: 0 }}>Заказы</h2>
          <div style={{ minWidth: 200 }}>
            <Select
              label="Статус"
              value={status}
              onChange={(e) => setStatus(e.target.value as OrderStatus | '')}
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
        {refundError && <Alert>{refundError}</Alert>}
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
                    <td data-label="">
                      {o.status === 'completed' && (
                        <Button small onClick={() => setTarget(o)}>
                          Вернуть
                        </Button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {nextBefore && (
          <div className="row" style={{ justifyContent: 'center' }}>
            <Button onClick={() => more()}>Показать ещё</Button>
          </div>
        )}
      </Card>

      {target && (
        <Modal title="Возврат денег" onClose={() => setTarget(null)}>
          <form
            className="stack"
            onSubmit={(e) => {
              e.preventDefault();
              dispatch(m.refunded({ id: target.id, reason }))
                .unwrap()
                .then(() => {
                  setTarget(null);
                  setReason('');
                })
                .catch(() => setTarget(null));
            }}
          >
            <p>
              Вернуть {formatMoney(target.amount_kopecks)} за «
              {target.kind === 'subscription' ? 'подписку' : target.title_name}»? Доступ зрителя будет
              отозван.
            </p>
            <TextArea
              label="Причина возврата"
              required
              value={reason}
              onChange={(e) => setReason(e.target.value)}
            />
            <div className="row">
              <Button type="submit" variant="danger">
                Вернуть деньги
              </Button>
              <Button onClick={() => setTarget(null)}>Отмена</Button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  );
}
