// Состояние форм входа и регистрации: общая ошибка, ошибки полей и «идёт запрос».
import { createSlice, isAnyOf } from '@reduxjs/toolkit';
import { login, register } from '@/entities/session/model';
import { problemText } from '@/shared/lib/request';
import type { RootState } from '@/shared/lib/redux';

export interface AuthFormState {
  error: string | null;
  fields: Record<string, string>;
  loginPending: boolean;
  registerPending: boolean;
}

const initialState: AuthFormState = { error: null, fields: {}, loginPending: false, registerPending: false };

export const authSlice = createSlice({
  name: 'auth',
  initialState,
  reducers: {
    formReset: (s) => ({ ...initialState, loginPending: s.loginPending, registerPending: s.registerPending }),
  },
  extraReducers: (b) =>
    b
      .addCase(login.pending, (s) => void (s.loginPending = true))
      .addCase(register.pending, (s) => void (s.registerPending = true))
      .addMatcher(isAnyOf(login.fulfilled, login.rejected), (s) => void (s.loginPending = false))
      .addMatcher(isAnyOf(register.fulfilled, register.rejected), (s) => void (s.registerPending = false))
      .addMatcher(isAnyOf(login.pending, register.pending), (s) => {
        s.error = null;
        s.fields = {};
      })
      .addMatcher(isAnyOf(login.rejected, register.rejected), (s, a) => {
        s.error = problemText(a);
        s.fields = a.payload?.fields ?? {};
      }),
});

export const { formReset } = authSlice.actions;

export const selectAuthError = (s: RootState) => s.auth.error;
export const selectFieldErrors = (s: RootState) => s.auth.fields;
export const selectLoginPending = (s: RootState) => s.auth.loginPending;
export const selectRegisterPending = (s: RootState) => s.auth.registerPending;
