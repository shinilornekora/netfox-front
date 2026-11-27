import { useState } from 'react';
import type { Order } from '@/shared/api/types';
import { formatMoney } from '@/shared/lib/format';
import { useAppDispatch } from '@/shared/lib/redux';
import { Button, Modal, TextArea } from '@/shared/ui';
import { refunded } from '../model';

/** «Вернуть» у оплаченного заказа: окно с причиной и подтверждением. */
export function RefundButton({ order }: { order: Order }) {
  const dispatch = useAppDispatch();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState('');
  return (
    <>
      <Button small onClick={() => setOpen(true)}>
        Вернуть
      </Button>
      {open && (
        <Modal title="Возврат денег" onClose={() => setOpen(false)}>
          <form
            className="stack"
            onSubmit={(e) => {
              e.preventDefault();
              dispatch(refunded({ id: order.id, reason }))
                .unwrap()
                .then(() => {
                  setOpen(false);
                  setReason('');
                })
                .catch(() => setOpen(false));
            }}
          >
            <p>
              Вернуть {formatMoney(order.amount_kopecks)} за «
              {order.kind === 'subscription' ? 'подписку' : order.title_name}»? Доступ зрителя будет отозван.
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
              <Button onClick={() => setOpen(false)}>Отмена</Button>
            </div>
          </form>
        </Modal>
      )}
    </>
  );
}
