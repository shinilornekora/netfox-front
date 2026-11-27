// Плеер: старт просмотра (проверка прав и возраста на сервере, токен потока)
// и пинги прогресса. Прогресс монотонен по client_ms: устаревший пинг
// с другого устройства сервер не применит.
import { createEffect, createEvent, createStore, sample } from 'effector';
import { createGate } from 'effector-react';
import { ApiError } from '@/shared/api/client';
import { api } from '@/shared/api/endpoints';
import type { StartResult } from '@/shared/api/types';
import { errorMessage } from '@/shared/lib/format';

export const WatchGate = createGate<{ id: string; ep?: string }>();

export interface StartError {
  code: string;
  status: number;
  message: string;
}

export const startFx = createEffect((p: { id: string; ep?: string }) =>
  api.start({ title_id: p.id, episode_id: p.ep }),
);
export const saveFx = createEffect(api.saveProgress);

/** Позиция плеера: компонент шлёт её раз в 10 с, на паузе и в конце. */
export const positionReported = createEvent<{ position: number; duration: number }>();

export const $start = createStore<StartResult | null>(null)
  .on(startFx.doneData, (_, r) => r)
  .reset(WatchGate.close, startFx);
export const $startError = createStore<StartError | null>(null)
  .on(startFx.failData, (_, e) => ({
    code: e instanceof ApiError ? e.code : 'unknown',
    status: e instanceof ApiError ? e.status : 0,
    message: errorMessage(e),
  }))
  .reset(WatchGate.close, startFx);
/** Последняя сохранённая позиция — чтобы не слать одинаковые пинги. */
export const $saved = createStore<number>(-1)
  .on(saveFx.done, (_, { params }) => params.position_sec)
  .reset(WatchGate.close, startFx.done);

sample({ clock: WatchGate.open, target: startFx });

sample({
  clock: positionReported,
  source: { gate: WatchGate.state, start: $start, saved: $saved },
  filter: ({ start, saved }, p) => start !== null && Math.floor(p.position) !== saved && p.duration > 0,
  fn: ({ gate, start }, p) => ({
    title_id: gate.id,
    episode_id: start?.episode?.id,
    position_sec: Math.floor(p.position),
    duration_sec: Math.floor(p.duration),
    client_ms: Date.now(),
  }),
  target: saveFx,
});
