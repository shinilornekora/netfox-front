// Фича «Фильтры витрины»: что ищем (название, тип, жанр, доступ) и как сортируем.
// Сам запрос ленты делает страница каталога — она читает фильтры через selectFilters.
import { createSlice, type PayloadAction } from '@reduxjs/toolkit';
import type { Kind } from '@/shared/api/types';
import type { RootState } from '@/shared/lib/redux';

export interface Filters {
  q: string;
  kind: Kind | '';
  genre: string;
  access: '' | 'subscription' | 'purchase';
  sort: '' | 'new' | 'year' | 'name';
}

export const initialFilters: Filters = { q: '', kind: '', genre: '', access: '', sort: '' };

export const catalogFiltersSlice = createSlice({
  name: 'catalogFilters',
  initialState: initialFilters,
  reducers: {
    filterChanged: (s, a: PayloadAction<Partial<Filters>>) => ({ ...s, ...a.payload }),
    filtersCleared: () => initialFilters,
  },
});

export const { filterChanged, filtersCleared } = catalogFiltersSlice.actions;
export const selectFilters = (s: RootState) => s.catalogFilters;
