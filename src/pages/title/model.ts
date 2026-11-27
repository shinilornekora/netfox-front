// Карточка тайтла: сам тайтл для всех, а для зрителя — его права (offer)
// и состояние просмотра (прогресс, «Смотреть позже»).
import { createAsyncThunk, createSelector, createSlice } from '@reduxjs/toolkit';
import { api } from '@/shared/api/endpoints';
import type { Offer, Title } from '@/shared/api/types';
import { addRequestCases, createApiThunk, idle, problemText, type RequestState } from '@/shared/lib/request';
import type { RootState } from '@/shared/lib/redux';
import { selectRole } from '@/entities/session/model';

type TitleViewerState = Awaited<ReturnType<typeof api.titleState>>;

export interface TitlePageState {
  /** id открытой карточки; null — страница закрыта. */
  id: string | null;
  title: RequestState<Title>;
  offer: RequestState<Offer>;
  state: RequestState<TitleViewerState>;
  inWatchlist: boolean;
  watchError: string | null;
}

const initialState: TitlePageState = {
  id: null,
  title: idle(),
  offer: idle(),
  state: idle(),
  inWatchlist: false,
  watchError: null,
};

export const titleRequested = createApiThunk('title/title', (id: string) => api.title(id));
export const offerRequested = createApiThunk('title/offer', (id: string) => api.offer(id));
export const stateRequested = createApiThunk('title/state', (id: string) => api.titleState(id));
export const setWatch = createApiThunk('title/setWatch', ({ id, on }: { id: string; on: boolean }) =>
  api.setWatch(id, on),
);

export const titleSlice = createSlice({
  name: 'title',
  initialState,
  reducers: {
    gateOpened(s, a: { payload: string }) {
      s.id = a.payload;
    },
    titleClosed: () => initialState,
  },
  extraReducers: (b) => {
    b.addCase(stateRequested.fulfilled, (s, a) => void (s.inWatchlist = a.payload.in_watchlist))
      // «Смотреть позже»: переключается сразу, при ошибке откатывается.
      .addCase(setWatch.pending, (s, a) => {
        s.inWatchlist = a.meta.arg.on;
        s.watchError = null;
      })
      .addCase(setWatch.rejected, (s, a) => {
        s.inWatchlist = !a.meta.arg.on;
        s.watchError = problemText(a);
      });
    addRequestCases(b, titleRequested, (s) => s.title);
    addRequestCases(b, offerRequested, (s) => s.offer);
    addRequestCases(b, stateRequested, (s) => s.state);
  },
});

export const { titleClosed } = titleSlice.actions;
const { gateOpened } = titleSlice.actions;

/** Права и состояние просмотра — только зрителю. */
export const viewerDataRequested = createAsyncThunk(
  'title/viewerData',
  async (_: void, { dispatch, getState }) => {
    const s = getState() as RootState;
    const id = s.title.id;
    if (id === null || selectRole(s) !== 'viewer') return;
    await Promise.all([dispatch(offerRequested(id)), dispatch(stateRequested(id))]);
  },
);

export const titleOpened = createAsyncThunk('title/opened', async (id: string, { dispatch }) => {
  dispatch(gateOpened(id));
  await Promise.all([dispatch(titleRequested(id)), dispatch(viewerDataRequested())]);
});

export const watchToggled = createAsyncThunk(
  'title/watchToggled',
  async (_: void, { dispatch, getState }) => {
    const { id, inWatchlist } = (getState() as RootState).title;
    if (id !== null) await dispatch(setWatch({ id, on: !inWatchlist }));
  },
);

export type Action =
  | { type: 'watch'; via: 'subscription' | 'purchase' }
  | { type: 'buy'; price: number }
  | { type: 'subscribe' }
  | { type: 'unavailable' };

/** Что предложить зрителю на кнопках: смотреть, купить и/или оформить подписку. */
export function actionsFor(t: Title, o: Offer | null): Action[] {
  if (t.status !== 'published') return [{ type: 'unavailable' }];
  if (o?.can_watch) return [{ type: 'watch', via: o.via ?? 'subscription' }];
  const out: Action[] = [];
  if (t.in_subscription && !o?.subscription_active) out.push({ type: 'subscribe' });
  if (t.kind === 'film' && t.price_kopecks > 0 && !o?.owned)
    out.push({ type: 'buy', price: t.price_kopecks });
  if (out.length === 0) out.push({ type: 'unavailable' });
  return out;
}

export const selectInWatchlist = (s: RootState) => s.title.inWatchlist;
export const selectWatchError = (s: RootState) => s.title.watchError;
export const selectView = createSelector(
  (s: RootState) => s.title,
  (t) => ({
    title: t.title.data,
    error: t.title.error,
    pending: t.title.pending,
    offer: t.offer.data,
    progress: t.state.data?.progress ?? null,
  }),
);
