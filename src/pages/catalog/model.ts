// Витрина: фильтры, сортировка и keyset-пагинация («Показать ещё» по next_cursor).
import { createAsyncThunk, createSelector, createSlice, type PayloadAction } from '@reduxjs/toolkit';
import { api, type TitleQuery } from '@/shared/api/endpoints';
import type { Genre, Kind, Progress, Title, WithTitle } from '@/shared/api/types';
import { addRequestCases, createApiThunk, idle, problemText, type RequestState } from '@/shared/lib/request';
import type { RootState } from '@/shared/lib/redux';
import { selectRole } from '@/entities/session/model';

export interface Filters {
  q: string;
  kind: Kind | '';
  genre: string;
  access: '' | 'subscription' | 'purchase';
  sort: '' | 'new' | 'year' | 'name';
}

export const initialFilters: Filters = { q: '', kind: '', genre: '', access: '', sort: '' };
export const PAGE_SIZE = 12;

export interface CatalogState {
  /** Страница открыта (аналог gate): от этого зависят реакции на смену роли. */
  open: boolean;
  filters: Filters;
  items: Title[];
  cursor: string | null;
  pending: boolean;
  morePending: boolean;
  error: string | null;
  genres: RequestState<{ items: Genre[] }>;
  continue: RequestState<{ items: WithTitle<Progress>[] }>;
}

const initialState: CatalogState = {
  open: false,
  filters: initialFilters,
  items: [],
  cursor: null,
  pending: false,
  morePending: false,
  error: null,
  genres: idle(),
  continue: idle(),
};

const toQuery = (f: Filters): TitleQuery => ({ ...f, limit: PAGE_SIZE });
const catalog = (s: unknown) => (s as RootState).catalog;

/** Первая страница по текущим фильтрам. */
export const pageRequested = createApiThunk('catalog/page', (_: void, { getState }) =>
  api.titles(toQuery(catalog(getState()).filters)),
);
/** Следующие страницы: дописываются к ленте. */
export const nextPageRequested = createApiThunk('catalog/more', (_: void, { getState }) => {
  const { filters, cursor } = catalog(getState());
  return api.titles({ ...toQuery(filters), cursor: cursor ?? undefined });
});
export const genresRequested = createApiThunk('catalog/genres', () => api.genres());
export const continueRequested = createApiThunk('catalog/continue', () => api.continueWatching());

export const catalogSlice = createSlice({
  name: 'catalog',
  initialState,
  reducers: {
    gateOpened: (s) => void (s.open = true),
    gateClosed: (s) => void (s.open = false),
    filterChanged(s, a: PayloadAction<Partial<Filters>>) {
      s.filters = { ...s.filters, ...a.payload };
    },
    filtersCleared(s) {
      s.filters = initialFilters;
    },
  },
  extraReducers: (b) => {
    b.addCase(pageRequested.pending, (s) => {
      s.pending = true;
      s.error = null;
    })
      .addCase(pageRequested.fulfilled, (s, a) => {
        s.pending = false;
        s.items = a.payload.items;
        // Последняя страница: gateway отдаёт next_cursor пустой строкой.
        s.cursor = a.payload.next_cursor || null;
      })
      .addCase(pageRequested.rejected, (s, a) => {
        s.pending = false;
        s.error = problemText(a);
      })
      .addCase(nextPageRequested.pending, (s) => void (s.morePending = true))
      .addCase(nextPageRequested.rejected, (s) => void (s.morePending = false))
      .addCase(nextPageRequested.fulfilled, (s, a) => {
        s.morePending = false;
        // Тайтл мог сместиться между страницами (опубликовали новый) — без дублей.
        const seen = new Set(s.items.map((t) => t.id));
        s.items.push(...a.payload.items.filter((t) => !seen.has(t.id)));
        s.cursor = a.payload.next_cursor || null;
      });
    addRequestCases(b, genresRequested, (s) => s.genres);
    addRequestCases(b, continueRequested, (s) => s.continue);
  },
});

export const { filterChanged, gateClosed: catalogClosed } = catalogSlice.actions;
const { gateOpened, filtersCleared } = catalogSlice.actions;

/** Открытие витрины: первая страница, жанры и (для зрителя) «Продолжить просмотр». */
export const catalogOpened = createAsyncThunk('catalog/opened', async (_: void, { dispatch, getState }) => {
  dispatch(gateOpened());
  const jobs: Promise<unknown>[] = [dispatch(pageRequested()), dispatch(genresRequested())];
  if (selectRole(getState() as RootState) === 'viewer') jobs.push(dispatch(continueRequested()));
  await Promise.all(jobs);
});

export const searchSubmitted = () => pageRequested();

export const filtersReset = createAsyncThunk('catalog/reset', async (_: void, { dispatch }) => {
  dispatch(filtersCleared());
  await dispatch(pageRequested());
});

/** «Показать ещё» — только если есть курсор и предыдущая догрузка закончилась. */
export const moreRequested = createAsyncThunk('catalog/showMore', async (_: void, { dispatch, getState }) => {
  const { cursor, morePending } = catalog(getState());
  if (cursor !== null && !morePending) await dispatch(nextPageRequested());
});

export const selectFilters = (s: RootState) => s.catalog.filters;
export const selectItems = (s: RootState) => s.catalog.items;
export const selectHasMore = (s: RootState) => s.catalog.cursor !== null;
export const selectGenres = createSelector(
  (s: RootState) => s.catalog.genres.data,
  (d) => (d?.items ?? []).filter((g) => g.count > 0),
);
const noContinue: WithTitle<Progress>[] = [];
export const selectContinue = (s: RootState) => s.catalog.continue.data?.items ?? noContinue;
export const selectView = createSelector(
  (s: RootState) => s.catalog,
  (c) => ({
    items: c.items,
    pending: c.pending,
    morePending: c.morePending,
    error: c.error,
    hasMore: c.cursor !== null,
  }),
);
