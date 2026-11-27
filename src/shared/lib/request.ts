// Общие кирпичики для Redux: thunk поверх запроса к API и типовое
// состояние «данные / ошибка / загрузка», чтобы слайсы страниц не дублировали
// одинаковые редьюсеры.
import { createAsyncThunk, type ActionReducerMapBuilder, type AsyncThunk } from '@reduxjs/toolkit';
import { ApiError } from '@/shared/api/client';
import { errorFields, errorMessage } from './format';

/** Ошибка запроса в сериализуемом виде: в Redux-состоянии нельзя хранить экземпляры классов. */
export interface Problem {
  message: string;
  code: string;
  status: number;
  fields: Record<string, string>;
}

export function toProblem(e: unknown): Problem {
  return {
    message: errorMessage(e),
    code: e instanceof ApiError ? e.code : 'unknown',
    status: e instanceof ApiError ? e.status : 0,
    fields: errorFields(e),
  };
}

type ThunkApi = { rejectValue: Problem };
type ThunkTools = { getState: () => unknown; dispatch: (a: unknown) => unknown };

/**
 * createAsyncThunk, у которого любая ошибка API превращается в Problem
 * (action.payload у rejected). Компоненты получают её же из `.unwrap()`.
 */
export function createApiThunk<R, A = void>(
  type: string,
  handler: (arg: A, tools: ThunkTools) => Promise<R>,
): AsyncThunk<R, A, ThunkApi> {
  return createAsyncThunk<R, A, ThunkApi>(type, async (arg, t) => {
    try {
      return await handler(arg, { getState: t.getState, dispatch: t.dispatch as ThunkTools['dispatch'] });
    } catch (e) {
      return t.rejectWithValue(toProblem(e));
    }
  });
}

/** Сообщение об ошибке из rejected-экшена createApiThunk. */
export const problemText = (a: { payload?: Problem; error: { message?: string } }) =>
  a.payload?.message ?? a.error.message ?? 'Неизвестная ошибка';

export interface RequestState<D> {
  data: D | null;
  error: string | null;
  pending: boolean;
}

export const idle = <D>(): RequestState<D> => ({ data: null, error: null, pending: false });

/**
 * Типовые редьюсеры запроса: pending — загрузка и сброс ошибки,
 * fulfilled — данные, rejected — текст ошибки. Это матчеры, поэтому в builder
 * их вызывают последними (после addCase) — и рядом можно добавить свои addCase на тот же thunk.
 */
export function addRequestCases<S, D, A>(
  builder: ActionReducerMapBuilder<S>,
  thunk: AsyncThunk<D, A, ThunkApi>,
  pick: (s: S) => RequestState<D>,
) {
  builder
    .addMatcher(thunk.pending.match, (s) => {
      const r = pick(s as S);
      r.pending = true;
      r.error = null;
    })
    .addMatcher(thunk.fulfilled.match, (s, a) => {
      const r = pick(s as S);
      r.pending = false;
      (r as RequestState<unknown>).data = a.payload;
    })
    .addMatcher(thunk.rejected.match, (s, a) => {
      const r = pick(s as S);
      r.pending = false;
      r.error = problemText(a);
    });
}
