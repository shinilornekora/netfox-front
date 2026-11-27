// «Смотреть позже»: список и удаление из него.
import { createSlice } from '@reduxjs/toolkit';
import { api } from '@/shared/api/endpoints';
import type { WatchItem, WithTitle } from '@/shared/api/types';
import { createApiThunk, problemText } from '@/shared/lib/request';
import type { RootState } from '@/shared/lib/redux';

export interface WatchlistState {
  items: WithTitle<WatchItem>[];
  pending: boolean;
  error: string | null;
}

const initialState: WatchlistState = { items: [], pending: false, error: null };

export const watchlistOpened = createApiThunk('watchlist/list', () => api.watchlist());
export const removed = createApiThunk('watchlist/remove', async (id: string) => {
  await api.setWatch(id, false);
  return id;
});

export const watchlistSlice = createSlice({
  name: 'watchlist',
  initialState,
  reducers: {},
  extraReducers: (b) =>
    b
      .addCase(watchlistOpened.pending, (s) => {
        s.pending = true;
        s.error = null;
      })
      .addCase(watchlistOpened.fulfilled, (s, a) => {
        s.pending = false;
        s.items = a.payload.items;
      })
      .addCase(watchlistOpened.rejected, (s, a) => {
        s.pending = false;
        s.error = problemText(a);
      })
      .addCase(removed.fulfilled, (s, a) => {
        s.items = s.items.filter((i) => i.title_id !== a.payload);
      }),
});

export const selectWatchlist = (s: RootState) => s.watchlist;
