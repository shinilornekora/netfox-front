import { useDebugValue, useEffect, useRef } from 'react';
import type { ThunkAction, UnknownAction } from '@reduxjs/toolkit';
import { useAppDispatch, type RootState } from '@/shared/lib/redux';

/** То, что можно передать в dispatch: обычный экшен или thunk (createAsyncThunk). */
type Dispatchable = UnknownAction | ThunkAction<unknown, RootState, unknown, UnknownAction>;

/**
 * Связывает страницу с её Redux-моделью: при открытии (и при смене аргумента,
 * например id из URL) диспатчит `opened(arg)`, при уходе со страницы — `closed()`.
 *
 * Аргумент сравнивается по содержимому (JSON), а не по ссылке: объект
 * `{ id, ep }`, собранный заново на каждом рендере, не перезапускает эффект.
 * Последнее значение лежит в useRef — эффекту не нужна сама ссылка на объект.
 */
export function usePageModel<A = void>(
  opened: (arg: A) => Dispatchable,
  closed?: () => Dispatchable,
  arg?: A,
): void {
  const dispatch = useAppDispatch();
  const key = JSON.stringify(arg ?? null);
  const argRef = useRef(arg as A);
  argRef.current = arg as A;
  useEffect(() => {
    void dispatch(opened(argRef.current));
    return () => {
      if (closed) void dispatch(closed());
    };
  }, [dispatch, opened, closed, key]);
  useDebugValue(key);
}
