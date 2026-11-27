import { describe, expect, it } from 'vitest';
import { allSettled, fork } from 'effector';
import { db, signIn } from '@/test/gateway';
import { $role, $status, $user, appStarted, loginFx, logoutFx, registerFx } from './model';

describe('модель сессии', () => {
  it('гость — не ошибка: /auth/session отвечает {user: null}', async () => {
    const scope = fork();
    await allSettled(appStarted, { scope });
    expect(scope.getState($status)).toBe('anonymous');
    expect(scope.getState($user)).toBeNull();
  });

  it('восстанавливает сессию по cookie', async () => {
    signIn('u-admin');
    const scope = fork();
    await allSettled(appStarted, { scope });
    expect(scope.getState($status)).toBe('authenticated');
    expect(scope.getState($role)).toBe('admin');
  });

  it('логин сохраняет пользователя', async () => {
    const scope = fork();
    await allSettled(loginFx, { scope, params: { email: 'anna@example.com', password: 'password1' } });
    expect(scope.getState($user)?.display_name).toBe('Анна Зрителева');
    expect(db.sessionDevice).not.toBeNull();
  });

  it('неверный пароль оставляет гостем', async () => {
    const scope = fork({ values: [[$status, 'anonymous']] });
    const res = await allSettled(loginFx, { scope, params: { email: 'anna@example.com', password: 'nope' } });
    expect(res.status).toBe('fail');
    expect(scope.getState($status)).toBe('anonymous');
  });

  it('регистрация создаёт зрителя', async () => {
    const scope = fork();
    await allSettled(registerFx, {
      scope,
      params: { email: 'new@example.com', password: 'password1', display_name: 'Новый', birth_date: '' },
    });
    expect(scope.getState($role)).toBe('viewer');
  });

  it('выход очищает сессию', async () => {
    const scope = fork();
    await allSettled(loginFx, { scope, params: { email: 'admin@plenka.local', password: 'admin12345' } });
    await allSettled(logoutFx, { scope });
    expect(scope.getState($status)).toBe('anonymous');
    expect(scope.getState($user)).toBeNull();
    expect(db.sessionDevice).toBeNull();
  });
});
