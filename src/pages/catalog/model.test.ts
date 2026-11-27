import { describe, expect, it } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/setup';
import { db } from '@/test/gateway';
import { title } from '@/test/fixtures';
import { makeStore } from '@/app/store';
import * as m from './model';

describe('витрина', () => {
  it('при открытии грузит опубликованное, новинки первыми, и жанры с тайтлами', async () => {
    const store = makeStore();
    await store.dispatch(m.catalogOpened());
    const names = m.selectItems(store.getState()).map((t) => t.name);
    expect(names[0]).toBe('Фантомас');
    expect(names).not.toContain('Алчность'); // черновик
    expect(m.selectGenres(store.getState()).every((g) => g.count > 0)).toBe(true);
  });

  it('передаёт фильтры в запрос без пустых значений', async () => {
    const urls: URL[] = [];
    server.use(
      http.get('*/api/titles', ({ request }) => {
        urls.push(new URL(request.url));
        return HttpResponse.json({ items: [] });
      }),
    );
    const store = makeStore();
    store.dispatch(m.filterChanged({ kind: 'film', genre: 'ужасы', sort: 'year' }));
    await store.dispatch(m.searchSubmitted());
    const p = urls.at(-1)!.searchParams;
    expect(p.get('kind')).toBe('film');
    expect(p.get('genre')).toBe('ужасы');
    expect(p.get('sort')).toBe('year');
    expect(p.get('limit')).toBe(String(m.PAGE_SIZE));
    expect(p.has('q')).toBe(false);
  });

  it('«Показать ещё» дописывает следующую страницу по курсору', async () => {
    for (let i = 0; i < 20; i++)
      db.titles.push(title({ id: `t-x${i}`, name: `Фильм ${i}`, published_at: '2026-01-01T00:00:00Z' }));
    const store = makeStore();
    await store.dispatch(m.catalogOpened());
    expect(m.selectItems(store.getState())).toHaveLength(m.PAGE_SIZE);
    expect(m.selectHasMore(store.getState())).toBe(true);
    await store.dispatch(m.moreRequested());
    expect(m.selectItems(store.getState())).toHaveLength(24);
    expect(m.selectHasMore(store.getState())).toBe(true);
    await store.dispatch(m.moreRequested());
    // 6 опубликованных из фикстур + 20 новых, без дублей
    expect(new Set(m.selectItems(store.getState()).map((t) => t.id)).size).toBe(26);
    expect(m.selectHasMore(store.getState())).toBe(false);
  });

  it('сброс фильтров возвращает начальные значения', async () => {
    const store = makeStore();
    store.dispatch(m.filterChanged({ q: 'луна' }));
    await store.dispatch(m.filtersReset());
    expect(m.selectFilters(store.getState())).toEqual(m.initialFilters);
  });
});
