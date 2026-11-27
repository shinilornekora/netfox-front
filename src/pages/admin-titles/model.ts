// Админский список тайтлов: фильтр по названию и статусу, по 50 строк, дальше — по next_cursor.
import { createAsyncThunk, createSlice, type PayloadAction } from '@reduxjs/toolkit';
import { api } from '@/shared/api/endpoints';
import type { Page, Title, TitleStatus } from '@/shared/api/types';
import { addRequestCases, createApiThunk, idle, type RequestState } from '@/shared/lib/request';
import type { RootState } from '@/shared/lib/redux';

export interface AdminFilter {
  q: string;
  status: TitleStatus | '';
}

export interface AdminTitlesState {
  filter: AdminFilter;
  list: RequestState<Page<Title>>;
  items: Title[];
  cursor: string;
}

const initialState: AdminTitlesState = { filter: { q: '', status: '' }, list: idle(), items: [], cursor: '' };

export const searched = createApiThunk('adminTitles/list', (_: void, { getState }) =>
  api.adminTitles((getState() as RootState).adminTitles.filter),
);
const nextPage = createApiThunk('adminTitles/more', (q: AdminFilter & { cursor: string }) =>
  api.adminTitles(q),
);

export const adminTitlesSlice = createSlice({
  name: 'adminTitles',
  initialState,
  reducers: {
    filterChanged(s, a: PayloadAction<Partial<AdminFilter>>) {
      s.filter = { ...s.filter, ...a.payload };
    },
  },
  extraReducers: (b) => {
    b.addCase(searched.fulfilled, (s, a) => {
      s.items = a.payload.items;
      s.cursor = a.payload.next_cursor || '';
    }).addCase(nextPage.fulfilled, (s, a) => {
      s.items.push(...a.payload.items);
      s.cursor = a.payload.next_cursor || '';
    });
    addRequestCases(b, searched, (s) => s.list);
  },
});

export const { filterChanged } = adminTitlesSlice.actions;
export const adminTitlesOpened = searched;

export const moreRequested = createAsyncThunk(
  'adminTitles/moreRequested',
  async (_: void, { dispatch, getState }) => {
    const { filter, cursor } = (getState() as RootState).adminTitles;
    if (cursor !== '') await dispatch(nextPage({ ...filter, cursor }));
  },
);

export const selectAdminTitles = (s: RootState) => s.adminTitles;
