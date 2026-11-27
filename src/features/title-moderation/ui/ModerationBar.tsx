import { useState } from 'react';
import { Link } from 'react-router-dom';
import type { Title } from '@/shared/api/types';
import { titleStatusLabel } from '@/shared/lib/format';
import { useAppDispatch } from '@/shared/lib/redux';
import { pathTo } from '@/shared/constants';
import { Badge, Button, Card, Modal, TextArea } from '@/shared/ui';
import { deleteTitle, publishTitle, withdrawTitle } from '../model';

/** Статус тайтла и действия модерации. onDeleted — куда уйти после удаления черновика. */
export function ModerationBar({ title, onDeleted }: { title: Title; onDeleted: () => void }) {
  const dispatch = useAppDispatch();
  // .unwrap(): промис отклоняется при ошибке API — для then/catch в обработчиках.
  const publish = () => dispatch(publishTitle(title)).unwrap();
  const withdraw = (reason: string) => dispatch(withdrawTitle({ t: title, reason })).unwrap();
  const remove = () => dispatch(deleteTitle(title.id)).unwrap();
  const [withdrawing, setWithdrawing] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);

  return (
    <>
      <Card as="section" className="row-between">
        <div className="row">
          <Badge tone={title.status === 'published' ? 'ok' : title.status === 'draft' ? 'info' : 'warn'}>
            {titleStatusLabel[title.status]}
          </Badge>
          <span className="muted">ревизия {title.revision}</span>
          {title.withdraw_reason && <span className="muted">Причина: {title.withdraw_reason}</span>}
        </div>
        <div className="row">
          {title.status !== 'published' && (
            <Button variant="primary" onClick={() => publish().catch(() => undefined)}>
              Опубликовать
            </Button>
          )}
          {title.status === 'published' && (
            <Button onClick={() => setWithdrawing(true)}>Снять с показа</Button>
          )}
          {title.status === 'published' && (
            <Link to={pathTo.title(title.id)} className="neu-btn">
              На витрине
            </Link>
          )}
          {title.status === 'draft' && (
            <Button variant="danger" onClick={() => setConfirmDelete(true)}>
              Удалить
            </Button>
          )}
        </div>
      </Card>

      {withdrawing && (
        <WithdrawDialog
          onClose={() => setWithdrawing(false)}
          onSubmit={(reason) =>
            withdraw(reason)
              .then(() => setWithdrawing(false))
              .catch(() => undefined)
          }
        />
      )}
      {confirmDelete && (
        <Modal title="Удалить черновик?" onClose={() => setConfirmDelete(false)}>
          <p>«{title.name}» будет удалён без возможности восстановления.</p>
          <div className="row">
            <Button
              variant="danger"
              onClick={() =>
                remove()
                  .then(() => onDeleted())
                  .catch(() => setConfirmDelete(false))
              }
            >
              Удалить
            </Button>
            <Button onClick={() => setConfirmDelete(false)}>Отмена</Button>
          </div>
        </Modal>
      )}
    </>
  );
}

function WithdrawDialog({ onClose, onSubmit }: { onClose: () => void; onSubmit: (r: string) => void }) {
  const [reason, setReason] = useState('');
  return (
    <Modal title="Снять с показа" onClose={onClose}>
      <form
        className="stack"
        onSubmit={(e) => {
          e.preventDefault();
          onSubmit(reason);
        }}
      >
        <p className="muted">
          Купившие сохранят фильм в «Покупках», но смотреть его не смогут, пока он снят.
        </p>
        <TextArea label="Причина" required value={reason} onChange={(e) => setReason(e.target.value)} />
        <div className="row">
          <Button type="submit" variant="danger">
            Снять с показа
          </Button>
          <Button onClick={onClose}>Отмена</Button>
        </div>
      </form>
    </Modal>
  );
}
