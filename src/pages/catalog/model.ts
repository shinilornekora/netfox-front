// Витрина: фильтры, сортировка и keyset-пагинация («Показать ещё» по next_cursor).
import { combine, createEvent, createStore, sample } from 'effector';
import { createGate } from 'effector-react';
import { api, type TitleQuery } from '@/shared/api/endpoints';
import type { Kind, Title } from '@/shared/api/types';
import { createRequest } from '@/shared/lib/request';
import { $role } from '@/entities/session/model';

export interface Filters {
  q: string;
  kind: Kind | '';
  genre: string;
  access: '' | 'subscription' | 'purchase';
  sort: '' | 'new' | 'year' | 'name';
}

export const initialFilters: Filters = { q: '', kind: '', genre: '', access: '', sort: '' };
export const PAGE_SIZE = 12;

export const CatalogGate = createGate<void>();

export const filterChanged = createEvent<Partial<Filters>>();
export const searchSubmitted = createEvent();
export const filtersReset = createEvent();
export const moreRequested = createEvent();

export const $filters = createStore<Filters>(initialFilters)
  .on(filterChanged, (f, patch) => ({ ...f, ...patch }))
  .reset(filtersReset);

/** Первая страница по текущим фильтрам. */
export const pageQuery = createRequest((q: TitleQuery) => api.titles(q));
/** Следующие страницы: дописываются к ленте. */
export const moreQuery = createRequest((q: TitleQuery) => api.titles(q));
export const genresQuery = createRequest((_: void) => api.genres());
export const continueQuery = createRequest((_: void) => api.continueWatching());

export const $items = createStore<Title[]>([])
  .on(pageQuery.fx.doneData, (_, p) => p.items)
  .on(moreQuery.fx.doneData, (list, p) => {
    // Тайтл мог сместиться между страницами (опубликовали новый) — без дублей.
    const seen = new Set(list.map((t) => t.id));
    return [...list, ...p.items.filter((t) => !seen.has(t.id))];
  });
export const $cursor = createStore<string | null>(null)
  .on(pageQuery.fx.doneData, (_, p) => p.next_cursor ?? null)
  .on(moreQuery.fx.doneData, (_, p) => p.next_cursor ?? null);
export const $hasMore = $cursor.map(Boolean);
export const $genres = genresQuery.$data.map((d) => (d?.items ?? []).filter((g) => g.count > 0));
export const $continue = continueQuery.$data.map((d) => d?.items ?? []);

const toQuery = (f: Filters): TitleQuery => ({ ...f, limit: PAGE_SIZE });

sample({
  clock: [CatalogGate.open, searchSubmitted, filtersReset],
  source: $filters,
  fn: toQuery,
  target: pageQuery.start,
});

sample({
  clock: moreRequested,
  source: { f: $filters, cursor: $cursor, busy: moreQuery.$pending },
  filter: ({ cursor, busy }) => cursor !== null && !busy,
  fn: ({ f, cursor }): TitleQuery => ({ ...toQuery(f), cursor: cursor ?? undefined }),
  target: moreQuery.start,
});

sample({ clock: CatalogGate.open, fn: () => undefined, target: genresQuery.start });

// «Продолжить просмотр» — только у зрителя.
sample({
  clock: [CatalogGate.open, $role.updates],
  source: { role: $role, open: CatalogGate.status },
  filter: ({ role, open }) => open && role === 'viewer',
  fn: () => undefined,
  target: continueQuery.start,
});

export const $view = combine({
  items: $items,
  pending: pageQuery.$pending,
  morePending: moreQuery.$pending,
  error: pageQuery.$error,
  hasMore: $hasMore,
});
