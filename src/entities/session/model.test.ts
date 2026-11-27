import { describe, expect, it } from 'vitest';
import { makeStore } from '@/app/store';
import { db, signIn } from '@/test/gateway';
import { login, logout, register, restoreSession, selectRole, selectStatus, selectUser } from './model';

describe('модель сессии', () => {
  it('гость — не ошибка: /auth/session отвечает {user: null}', async () => {
    const store = makeStore();
    await store.dispatch(restoreSession());
    expect(selectStatus(store.getState())).toBe('anonymous');
    expect(selectUser(store.getState())).toBeNull();
  });

  it('восстанавливает сессию по cookie', async () => {
    signIn('u-admin');
    const store = makeStore();
    await store.dispatch(restoreSession());
    expect(selectStatus(store.getState())).toBe('authenticated');
    expect(selectRole(store.getState())).toBe('admin');
  });

  it('логин сохраняет пользователя', async () => {
    const store = makeStore();
    await store.dispatch(login({ email: 'anna@example.com', password: 'password123' }));
    expect(selectUser(store.getState())?.display_name).toBe('Анна Зрителева');
    expect(db.sessionDevice).not.toBeNull();
  });

  it('неверный пароль оставляет гостем', async () => {
    const store = makeStore({ session: { status: 'anonymous', user: null } });
    const res = await store.dispatch(login({ email: 'anna@example.com', password: 'nope' }));
    expect(login.rejected.match(res)).toBe(true);
    expect(selectStatus(store.getState())).toBe('anonymous');
  });

  it('регистрация создаёт зрителя', async () => {
    const store = makeStore();
    await store.dispatch(
      register({ email: 'new@example.com', password: 'password123', display_name: 'Новый', birth_date: '' }),
    );
    expect(selectRole(store.getState())).toBe('viewer');
  });

  it('выход очищает сессию', async () => {
    const store = makeStore();
    await store.dispatch(login({ email: 'admin@plenka.local', password: 'admin12345' }));
    await store.dispatch(logout());
    expect(selectStatus(store.getState())).toBe('anonymous');
    expect(selectUser(store.getState())).toBeNull();
    expect(db.sessionDevice).toBeNull();
  });
});
