// Карточка тайтла: сам тайтл для всех, а для зрителя — его права (offer)
// и состояние просмотра (прогресс, «Смотреть позже»).
import { createAsyncThunk, createSelector, createSlice } from '@reduxjs/toolkit';
import { api } from '@/shared/api/endpoints';
import type { Offer, Title } from '@/shared/api/types';
import { addRequestCases, createApiThunk, idle, problemText, type RequestState } from '@/shared/lib/request';
import type { RootState } from '@/shared/lib/redux';
import { selectRole } from '@/entities/session';
import { setWatch } from '@/features/watchlist-toggle';

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
      // «Смотреть позже» (фича watchlist-toggle): переключается сразу, при ошибке откатывается.
      // Реагируем только на открытый тайтл.
      .addCase(setWatch.pending, (s, a) => {
        if (a.meta.arg.id !== s.id) return;
        s.inWatchlist = a.meta.arg.on;
        s.watchError = null;
      })
      .addCase(setWatch.rejected, (s, a) => {
        if (a.meta.arg.id !== s.id) return;
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

export const selectOpenTitleId = (s: RootState) => s.title.id;
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
