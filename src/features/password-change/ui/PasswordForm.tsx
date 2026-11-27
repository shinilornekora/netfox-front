import { useState, type FormEvent } from 'react';
import { useAppDispatch } from '@/shared/lib/redux';
import { Alert, Button, Card, TextField } from '@/shared/ui';
import type { Problem } from '@/shared/lib/request';
import { changePassword } from '../model';

export function PasswordForm() {
  const dispatch = useAppDispatch();
  const [pending, setPending] = useState(false);
  const [form, setForm] = useState({ current_password: '', new_password: '' });
  const [msg, setMsg] = useState<{ tone: 'success' | 'error'; text: string } | null>(null);
  const [fields, setFields] = useState<Record<string, string>>({});

  const submit = (e: FormEvent) => {
    e.preventDefault();
    setPending(true);
    dispatch(changePassword(form))
      .unwrap()
      .then(() => {
        setForm({ current_password: '', new_password: '' });
        setFields({});
        setMsg({ tone: 'success', text: 'Пароль изменён. Другие устройства вышли из аккаунта' });
      })
      .catch((err: Problem) => {
        setFields(err.fields);
        setMsg({ tone: 'error', text: err.message });
      })
      .finally(() => setPending(false));
  };

  return (
    <Card>
      <form className="stack" onSubmit={submit} aria-label="Смена пароля">
        <h2 style={{ margin: 0 }}>Смена пароля</h2>
        {msg && <Alert tone={msg.tone}>{msg.text}</Alert>}
        <TextField
          label="Текущий пароль"
          type="password"
          autoComplete="current-password"
          required
          value={form.current_password}
          onChange={(e) => setForm({ ...form, current_password: e.target.value })}
          error={fields.current_password}
        />
        <TextField
          label="Новый пароль"
          type="password"
          autoComplete="new-password"
          required
          minLength={10}
          value={form.new_password}
          onChange={(e) => setForm({ ...form, new_password: e.target.value })}
          error={fields.new_password}
        />
        <Button type="submit" disabled={pending}>
          Сменить пароль
        </Button>
      </form>
    </Card>
  );
}
