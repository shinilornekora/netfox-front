import { useUnit } from 'effector-react';
import type { FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { cards } from '@/shared/lib/testCards';
import { formatCard, formatMoney } from '@/shared/lib/format';
import { Alert, Button, Modal, Spinner, TextField } from '@/shared/ui';
import * as m from './model';

/** Окно оплаты: подписка или покупка фильма. Рисуется один раз в Layout. */
export function CheckoutModal() {
  const [target, card, phase, order, error] = useUnit([m.$target, m.$card, m.$phase, m.$order, m.$error]);
  const [close, change, submit, retry] = useUnit([m.closed, m.cardChanged, m.submitted, m.retried]);
  if (!target) return null;

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    submit();
  };

  if (phase === 'done') {
    return (
      <Modal title="Оплата прошла" onClose={close}>
        <Alert tone="success">
          {target.kind === 'subscription'
            ? 'Подписка оформлена — смотрите всё, что в неё входит.'
            : 'Фильм ваш навсегда — он появился в разделе «Покупки».'}
        </Alert>
        <p className="muted">Карта •••• {order?.card_last4}</p>
        <div className="row">
          <Link to="/cabinet" className="neu-btn" onClick={() => close()}>
            В кабинет
          </Link>
          <Button variant="primary" onClick={() => close()}>
            Готово
          </Button>
        </div>
      </Modal>
    );
  }

  return (
    <Modal title={`Оплата: ${target.label}`} onClose={close}>
      <p className="price">{formatMoney(target.amount_kopecks)}</p>
      {phase === 'polling' ? (
        <div className="stack" role="status">
          <Spinner label="Ждём ответа банка" />
          <p>
            Банк ещё не подтвердил платёж. Заказ в обработке — не закрывайте окно, это займёт несколько
            секунд.
          </p>
        </div>
      ) : phase === 'failed' ? (
        <div className="stack">
          <Alert>Оплата не прошла: {order?.failure ?? 'банк отклонил операцию'}</Alert>
          <div className="row">
            <Button variant="primary" onClick={() => retry()}>
              Попробовать другую карту
            </Button>
            <Button onClick={() => close()}>Закрыть</Button>
          </div>
        </div>
      ) : (
        <form className="stack" onSubmit={onSubmit} aria-label="Данные карты">
          <TextField
            label="Номер карты"
            inputMode="numeric"
            autoComplete="cc-number"
            value={formatCard(card)}
            onChange={(e) => change(e.target.value)}
            placeholder="0000 0000 0000 0000"
            required
          />
          <div className="stack" style={{ gap: 6 }}>
            <span className="muted" style={{ fontSize: 13 }}>
              Тестовые карты платёжного симулятора (деньги не списываются):
            </span>
            <div className="cards-hint">
              {cards.map((c) => (
                <Button key={c.number} small onClick={() => change(c.number)} title={c.number}>
                  {c.label}
                </Button>
              ))}
            </div>
          </div>
          {error && <Alert>{error}</Alert>}
          <div className="row">
            <Button type="submit" variant="primary" disabled={phase === 'paying'}>
              {phase === 'paying' ? 'Оплачиваем…' : `Оплатить ${formatMoney(target.amount_kopecks)}`}
            </Button>
            <Button onClick={() => close()}>Отмена</Button>
          </div>
        </form>
      )}
    </Modal>
  );
}
