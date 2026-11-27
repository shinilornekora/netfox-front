// Сессия зрителя на Redux Toolkit. Сама сессия — HttpOnly-cookie plenka_sid,
// JS её не видит; здесь только статус и профиль. При старте приложения
// GET /auth/session отвечает 200 и для гостя ({user: null}) — гость не ошибка.
import { createAsyncThunk, createSlice, type PayloadAction } from '@reduxjs/toolkit';
import { api, type RegisterIn } from '@/shared/api/endpoints';
import type { Role, User } from '@/shared/api/types';
import { createApiThunk } from '@/shared/lib/request';
import type { RootState } from '@/shared/lib/redux';

export type AuthStatus = 'unknown' | 'authenticated' | 'anonymous';

export interface SessionState {
  status: AuthStatus;
  user: User | null;
}

const initialState: SessionState = { status: 'unknown', user: null };

/** Старт приложения: восстановить сессию по cookie. */
export const restoreSession = createApiThunk('session/restore', () => api.session());
export const login = createApiThunk('session/login', (b: { email: string; password: string }) =>
  api.login(b),
);
export const register = createApiThunk('session/register', (b: RegisterIn) => api.register(b));
export const logout = createAsyncThunk('session/logout', async () => {
  try {
    await api.logout();
  } catch {
    /* локально выходим в любом случае */
  }
});

const received = (s: SessionState, user: User) => {
  s.status = 'authenticated';
  s.user = user;
};
const lost = (s: SessionState) => {
  s.status = 'anonymous';
  s.user = null;
};

export const sessionSlice = createSlice({
  name: 'session',
  initialState,
  reducers: {
    /** 401 на запросе, отзыв текущего устройства и т. п. */
    sessionLost: lost,
    userUpdated(s, a: PayloadAction<User>) {
      s.user = a.payload;
    },
  },
  extraReducers: (b) =>
    b
      .addCase(restoreSession.fulfilled, (s, a) => (a.payload ? received(s, a.payload) : lost(s)))
      .addCase(restoreSession.rejected, lost)
      .addCase(login.fulfilled, (s, a) => received(s, a.payload))
      .addCase(register.fulfilled, (s, a) => received(s, a.payload))
      .addCase(logout.fulfilled, lost),
});

export const { sessionLost, userUpdated } = sessionSlice.actions;

export const selectStatus = (s: RootState) => s.session.status;
export const selectUser = (s: RootState) => s.session.user;
export const selectRole = (s: RootState): Role | null => s.session.user?.role ?? null;
export const selectIsAuthenticated = (s: RootState) => s.session.status === 'authenticated';

export function hasRole(role: Role | null, allowed: readonly Role[]): boolean {
  return role !== null && allowed.includes(role);
}
