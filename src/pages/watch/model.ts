// Плеер: старт просмотра (проверка прав и возраста на сервере, токен потока)
// и пинги прогресса. Прогресс монотонен по client_ms: устаревший пинг
// с другого устройства сервер не применит.
import { createAsyncThunk, createSlice } from '@reduxjs/toolkit';
import { api, type ProgressIn } from '@/shared/api/endpoints';
import type { StartResult } from '@/shared/api/types';
import { createApiThunk, problemText } from '@/shared/lib/request';
import type { RootState } from '@/shared/lib/redux';

export interface StartError {
  code: string;
  status: number;
  message: string;
}

export interface WatchState {
  /** Что открыто: тайтл и, для сериала, серия. */
  gate: { id: string; ep?: string } | null;
  start: StartResult | null;
  startError: StartError | null;
  pending: boolean;
  /** Последняя сохранённая позиция — чтобы не слать одинаковые пинги. */
  saved: number;
}

const initialState: WatchState = { gate: null, start: null, startError: null, pending: false, saved: -1 };

export const startRequested = createApiThunk('watch/start', (p: { id: string; ep?: string }) =>
  api.start({ title_id: p.id, episode_id: p.ep }),
);
export const progressSaved = createApiThunk('watch/save', (p: ProgressIn) => api.saveProgress(p));

export const watchSlice = createSlice({
  name: 'watch',
  initialState,
  reducers: {
    gateOpened(s, a: { payload: { id: string; ep?: string } }) {
      s.gate = a.payload;
    },
    watchClosed: () => initialState,
  },
  extraReducers: (b) =>
    b
      .addCase(startRequested.pending, (s) => {
        s.pending = true;
        s.start = null;
        s.startError = null;
      })
      .addCase(startRequested.fulfilled, (s, a) => {
        s.pending = false;
        s.start = a.payload;
        s.saved = -1;
      })
      .addCase(startRequested.rejected, (s, a) => {
        s.pending = false;
        s.startError = {
          code: a.payload?.code ?? 'unknown',
          status: a.payload?.status ?? 0,
          message: problemText(a),
        };
      })
      .addCase(progressSaved.fulfilled, (s, a) => void (s.saved = a.meta.arg.position_sec)),
});

export const { watchClosed } = watchSlice.actions;
const { gateOpened } = watchSlice.actions;

export const watchOpened = createAsyncThunk(
  'watch/opened',
  async (p: { id: string; ep?: string }, { dispatch }) => {
    dispatch(gateOpened(p));
    await dispatch(startRequested(p));
  },
);

/** Позиция плеера: компонент шлёт её раз в 10 с, на паузе и в конце. */
export const positionReported = createAsyncThunk(
  'watch/position',
  async (p: { position: number; duration: number }, { dispatch, getState }) => {
    const { gate, start, saved } = (getState() as RootState).watch;
    if (!gate || start === null || Math.floor(p.position) === saved || p.duration <= 0) return;
    await dispatch(
      progressSaved({
        title_id: gate.id,
        episode_id: start.episode?.id,
        position_sec: Math.floor(p.position),
        duration_sec: Math.floor(p.duration),
        client_ms: Date.now(),
      }),
    );
  },
);

export const selectWatch = (s: RootState) => s.watch;
