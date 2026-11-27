import { describe, expect, it } from 'vitest';
import { configureStore, createSlice } from '@reduxjs/toolkit';
import { ApiError } from '@/shared/api/client';
import { addRequestCases, createApiThunk, idle, type RequestState } from './request';

describe('createApiThunk + addRequestCases', () => {
  it('хранит данные, ошибку и очищает ошибку при новом запуске', async () => {
    let ok = false;
    const double = createApiThunk('t/double', async (n: number) => {
      if (!ok) throw new ApiError(500, 'x', 'Упало', { name: 'плохо' });
      return n * 2;
    });
    const slice = createSlice({
      name: 't',
      initialState: { r: idle<number>() } as { r: RequestState<number> },
      reducers: {},
      extraReducers: (b) => addRequestCases(b, double, (s) => s.r),
    });
    const store = configureStore({ reducer: slice.reducer });

    const failed = await store.dispatch(double(1));
    expect(store.getState().r).toEqual({ data: null, error: 'Упало', pending: false });
    // Ошибка API приходит в payload сериализуемой: код, статус и ошибки полей.
    expect(failed.payload).toEqual({ message: 'Упало', code: 'x', status: 500, fields: { name: 'плохо' } });

    ok = true;
    const p = store.dispatch(double(21));
    expect(store.getState().r.pending).toBe(true);
    expect(store.getState().r.error).toBeNull();
    await p;
    expect(store.getState().r).toEqual({ data: 42, error: null, pending: false });
  });
});
