// Сессия зрителя на Effector. Сама сессия — HttpOnly-cookie plenka_sid,
// JS её не видит; здесь только статус и профиль. При старте приложения
// GET /auth/session отвечает 200 и для гостя ({user: null}) — гость не ошибка.
import { combine, createEffect, createEvent, createStore, sample } from 'effector';
import { api } from '@/shared/api/endpoints';
import type { Role, User } from '@/shared/api/types';

export type AuthStatus = 'unknown' | 'authenticated' | 'anonymous';

export const appStarted = createEvent();
export const sessionReceived = createEvent<User>();
export const sessionLost = createEvent();
export const userUpdated = createEvent<User>();

export const restoreFx = createEffect(api.session);
export const loginFx = createEffect(api.login);
export const registerFx = createEffect(api.register);
export const logoutFx = createEffect(async () => {
  try {
    await api.logout();
  } catch {
    /* локально выходим в любом случае */
  }
});

export const $status = createStore<AuthStatus>('unknown');
export const $user = createStore<User | null>(null);
export const $role = $user.map((u): Role | null => u?.role ?? null);
export const $isAuthenticated = $status.map((s) => s === 'authenticated');
export const $session = combine({ status: $status, user: $user });

sample({ clock: appStarted, target: restoreFx });
sample({ clock: restoreFx.doneData, filter: Boolean, target: sessionReceived });
sample({ clock: restoreFx.doneData, filter: (u) => !u, target: sessionLost });
sample({ clock: restoreFx.fail, target: sessionLost });

sample({ clock: [loginFx.doneData, registerFx.doneData], target: sessionReceived });
sample({ clock: logoutFx.finally, target: sessionLost });

$status.on(sessionReceived, () => 'authenticated').on(sessionLost, () => 'anonymous');
$user
  .on(sessionReceived, (_, u) => u)
  .on(userUpdated, (_, u) => u)
  .reset(sessionLost);

export function hasRole(role: Role | null, allowed: readonly Role[]): boolean {
  return role !== null && allowed.includes(role);
}
