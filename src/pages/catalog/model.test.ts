import { describe, expect, it } from 'vitest';
import { allSettled, fork } from 'effector';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/setup';
import { db } from '@/test/gateway';
import { title } from '@/test/fixtures';
import { openGate } from '@/test/gate';
import * as m from './model';

describe('витрина', () => {
  it('при открытии грузит опубликованное, новинки первыми, и жанры с тайтлами', async () => {
    const scope = fork();
    await openGate(m.CatalogGate, scope);
    const names = scope.getState(m.$items).map((t) => t.name);
    expect(names[0]).toBe('Фантомас');
    expect(names).not.toContain('Алчность'); // черновик
    expect(scope.getState(m.$genres).every((g) => g.count > 0)).toBe(true);
  });

  it('передаёт фильтры в запрос без пустых значений', async () => {
    const urls: URL[] = [];
    server.use(
      http.get('*/api/titles', ({ request }) => {
        urls.push(new URL(request.url));
        return HttpResponse.json({ items: [] });
      }),
    );
    const scope = fork();
    await allSettled(m.filterChanged, { scope, params: { kind: 'film', genre: 'ужасы', sort: 'year' } });
    await allSettled(m.searchSubmitted, { scope });
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
    const scope = fork();
    await openGate(m.CatalogGate, scope);
    expect(scope.getState(m.$items)).toHaveLength(m.PAGE_SIZE);
    expect(scope.getState(m.$hasMore)).toBe(true);
    await allSettled(m.moreRequested, { scope });
    expect(scope.getState(m.$items)).toHaveLength(24);
    expect(scope.getState(m.$hasMore)).toBe(true);
    await allSettled(m.moreRequested, { scope });
    // 6 опубликованных из фикстур + 20 новых, без дублей
    expect(new Set(scope.getState(m.$items).map((t) => t.id)).size).toBe(26);
    expect(scope.getState(m.$hasMore)).toBe(false);
  });

  it('сброс фильтров возвращает начальные значения', async () => {
    const scope = fork();
    await allSettled(m.filterChanged, { scope, params: { q: 'луна' } });
    await allSettled(m.filtersReset, { scope });
    expect(scope.getState(m.$filters)).toEqual(m.initialFilters);
  });
});
