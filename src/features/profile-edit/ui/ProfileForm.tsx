import { useEffect, useState, type FormEvent } from 'react';
import { selectUser } from '@/entities/session';
import { useAppDispatch, useAppSelector } from '@/shared/lib/redux';
import { Alert, Badge, Button, Card, TextField } from '@/shared/ui';
import { roleLabel } from '@/shared/lib/format';
import type { Problem } from '@/shared/lib/request';
import { updateMe } from '../model';

export function ProfileForm() {
  const user = useAppSelector(selectUser);
  const dispatch = useAppDispatch();
  const [pending, setPending] = useState(false);
  const [form, setForm] = useState({ display_name: '', birth_date: '' });
  const [msg, setMsg] = useState<{ tone: 'success' | 'error'; text: string } | null>(null);
  useEffect(() => {
    if (user) setForm({ display_name: user.display_name, birth_date: user.birth_date ?? '' });
  }, [user]);
  if (!user) return null;

  const submit = (e: FormEvent) => {
    e.preventDefault();
    setPending(true);
    dispatch(updateMe({ ...form, revision: user.revision }))
      .unwrap()
      .then(() => setMsg({ tone: 'success', text: 'Профиль сохранён' }))
      .catch((err: Problem) => setMsg({ tone: 'error', text: err.message }))
      .finally(() => setPending(false));
  };

  return (
    <Card>
      <form className="stack" onSubmit={submit} aria-label="Профиль">
        <div className="row-between">
          <h1 style={{ margin: 0 }}>Профиль</h1>
          <Badge>{roleLabel[user.role]}</Badge>
        </div>
        <p className="muted">{user.email}</p>
        {msg && <Alert tone={msg.tone}>{msg.text}</Alert>}
        <TextField
          label="Как к вам обращаться"
          value={form.display_name}
          onChange={(e) => setForm({ ...form, display_name: e.target.value })}
        />
        <TextField
          label="Дата рождения"
          type="date"
          value={form.birth_date}
          onChange={(e) => setForm({ ...form, birth_date: e.target.value })}
        />
        <p className="muted" style={{ fontSize: 13 }}>
          Дата рождения нужна для фильмов 18+ и возрастных ограничений.
        </p>
        <Button type="submit" variant="primary" disabled={pending}>
          Сохранить
        </Button>
      </form>
    </Card>
  );
}
