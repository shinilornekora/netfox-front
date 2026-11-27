import { useEffect, useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { login as loginThunk } from '@/entities/session/model';
import { useAppDispatch, useAppSelector } from '@/shared/lib/redux';
import { Alert, Button, Card, TextField } from '@/shared/ui';
import * as m from './model';
import { RoutePath } from '@/shared/constants';

export default function LoginPage() {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const dispatch = useAppDispatch();
  const error = useAppSelector(m.selectAuthError);
  const pending = useAppSelector(m.selectLoginPending);
  useEffect(() => () => void dispatch(m.formReset()), [dispatch]);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    void dispatch(loginThunk({ email, password }));
  };

  return (
    <div className="page" style={{ maxWidth: 440, margin: '0 auto' }}>
      <Card>
        <form className="stack" onSubmit={submit} aria-label="Вход">
          <h1>Вход</h1>
          {error && <Alert>{error}</Alert>}
          <TextField
            label="Email"
            type="email"
            autoComplete="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
          <TextField
            label="Пароль"
            type="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          <Button type="submit" variant="primary" disabled={pending}>
            {pending ? 'Входим…' : 'Войти'}
          </Button>
          <p className="muted">
            Нет аккаунта? <Link to={RoutePath.register}>Зарегистрируйтесь</Link>
          </p>
        </form>
      </Card>
    </div>
  );
}
