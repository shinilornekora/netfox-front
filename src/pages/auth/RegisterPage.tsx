import { useEffect, useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { useUnit } from 'effector-react';
import { registerFx } from '@/entities/session/model';
import { Alert, Button, Card, TextField } from '@/shared/ui';
import * as m from './model';

export default function RegisterPage() {
  const [form, setForm] = useState({ email: '', password: '', display_name: '', birth_date: '' });
  const [error, fields, pending, register, reset] = useUnit([
    m.$authError,
    m.$fieldErrors,
    m.$registerPending,
    registerFx,
    m.formReset,
  ]);
  useEffect(() => () => reset(), [reset]);
  const set = (k: keyof typeof form) => (e: { target: { value: string } }) =>
    setForm({ ...form, [k]: e.target.value });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    register(form).catch(() => undefined);
  };

  return (
    <div className="page" style={{ maxWidth: 440, margin: '0 auto' }}>
      <Card>
        <form className="stack" onSubmit={submit} aria-label="Регистрация">
          <h1>Регистрация</h1>
          <p className="muted">Аккаунт зрителя: смотрите по подписке или покупайте фильмы навсегда.</p>
          {error && <Alert>{error}</Alert>}
          <TextField
            label="Как к вам обращаться"
            required
            autoComplete="nickname"
            value={form.display_name}
            onChange={set('display_name')}
            error={fields.display_name}
          />
          <TextField
            label="Email"
            type="email"
            required
            autoComplete="email"
            value={form.email}
            onChange={set('email')}
            error={fields.email}
          />
          <TextField
            label="Пароль"
            type="password"
            required
            minLength={8}
            autoComplete="new-password"
            value={form.password}
            onChange={set('password')}
            error={fields.password}
          />
          <TextField
            label="Дата рождения (для фильмов 18+)"
            type="date"
            value={form.birth_date}
            onChange={set('birth_date')}
            error={fields.birth_date}
          />
          <Button type="submit" variant="primary" disabled={pending}>
            {pending ? 'Создаём…' : 'Создать аккаунт'}
          </Button>
          <p className="muted">
            Уже есть аккаунт? <Link to="/login">Войдите</Link>
          </p>
        </form>
      </Card>
    </div>
  );
}
