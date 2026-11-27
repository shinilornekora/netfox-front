import { useEffect, useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { register as registerThunk } from '@/entities/session/model';
import { useAppDispatch, useAppSelector } from '@/shared/lib/redux';
import { Alert, Button, Card, TextField } from '@/shared/ui';
import * as m from './model';
import { RoutePath } from '@/shared/constants';

export default function RegisterPage() {
  const [form, setForm] = useState({ email: '', password: '', display_name: '', birth_date: '' });
  const dispatch = useAppDispatch();
  const error = useAppSelector(m.selectAuthError);
  const fields = useAppSelector(m.selectFieldErrors);
  const pending = useAppSelector(m.selectRegisterPending);
  useEffect(() => () => void dispatch(m.formReset()), [dispatch]);
  const set = (k: keyof typeof form) => (e: { target: { value: string } }) =>
    setForm({ ...form, [k]: e.target.value });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    void dispatch(registerThunk(form));
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
            minLength={2}
            maxLength={60}
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
            minLength={10}
            autoComplete="new-password"
            value={form.password}
            onChange={set('password')}
            error={fields.password}
          />
          <p className="muted" style={{ fontSize: 13, marginTop: -8 }}>
            Не короче 10 символов, буквы и цифры.
          </p>
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
            Уже есть аккаунт? <Link to={RoutePath.login}>Войдите</Link>
          </p>
        </form>
      </Card>
    </div>
  );
}
