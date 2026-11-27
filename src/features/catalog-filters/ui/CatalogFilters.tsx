import { useEffect, useRef, type FormEvent } from 'react';
import type { Genre } from '@/shared/api/types';
import { kindLabel } from '@/shared/lib/format';
import { useDebounce } from '@/shared/lib/hooks';
import { useAppDispatch, useAppSelector } from '@/shared/lib/redux';
import { Button, Card, Select, TextField } from '@/shared/ui';
import { filterChanged, initialFilters, selectFilters, type Filters } from '../model';

/**
 * Форма поиска по витрине. Что делать с фильтрами, решает страница:
 * onSearch — перезапросить ленту, onReset — сбросить фильтры и перезапросить.
 */
export function CatalogFilters({
  genres,
  onSearch,
  onReset,
}: {
  genres: Genre[];
  onSearch: () => void;
  onReset: () => void;
}) {
  const dispatch = useAppDispatch();
  const filters = useAppSelector(selectFilters);
  const change = (patch: Partial<Filters>) => dispatch(filterChanged(patch));
  // Строка, по которой последний раз искали: Enter, кнопка или автопоиск.
  const searchedQ = useRef(filters.q);
  const submit = () => {
    searchedQ.current = filters.q;
    onSearch();
  };
  const reset = () => {
    searchedQ.current = initialFilters.q;
    onReset();
  };

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    submit();
  };
  // Поиск по названию запускается сам, когда пользователь перестал печатать на 400 мс
  // (useDebounce). Enter и кнопка ищут сразу — тогда автопоиск по той же строке не повторяется.
  const debouncedQ = useDebounce(filters.q, 400);
  const search = useRef(onSearch);
  search.current = onSearch;
  useEffect(() => {
    if (debouncedQ === searchedQ.current) return;
    searchedQ.current = debouncedQ;
    search.current();
  }, [debouncedQ]);

  // Селекты применяются сразу.
  const pick = (patch: Partial<Filters>) => {
    change(patch);
    submit();
  };

  return (
    <Card>
      <form onSubmit={onSubmit} className="form-grid" aria-label="Поиск по каталогу">
        <TextField
          label="Название"
          type="search"
          value={filters.q}
          onChange={(e) => change({ q: e.target.value })}
          placeholder="Например, Метрополис"
        />
        <Select
          label="Тип"
          value={filters.kind}
          onChange={(e) => pick({ kind: e.target.value as Filters['kind'] })}
          options={[
            { value: '', label: 'Всё' },
            { value: 'film', label: kindLabel.film + 'ы' },
            { value: 'series', label: kindLabel.series + 'ы' },
          ]}
        />
        <Select
          label="Жанр"
          value={filters.genre}
          onChange={(e) => pick({ genre: e.target.value })}
          options={[{ value: '', label: 'Любой' }, ...genres.map((g) => ({ value: g.name, label: g.name }))]}
        />
        <Select
          label="Доступ"
          value={filters.access}
          onChange={(e) => pick({ access: e.target.value as Filters['access'] })}
          options={[
            { value: '', label: 'Любой' },
            { value: 'subscription', label: 'В подписке' },
            { value: 'purchase', label: 'Можно купить' },
          ]}
        />
        <Select
          label="Сортировка"
          value={filters.sort}
          onChange={(e) => pick({ sort: e.target.value as Filters['sort'] })}
          options={[
            { value: '', label: 'Сначала новинки' },
            { value: 'year', label: 'По году выхода' },
            { value: 'name', label: 'По названию' },
          ]}
        />
        <div className="row" style={{ alignSelf: 'end' }}>
          <Button type="submit" variant="primary">
            Найти
          </Button>
          <Button onClick={() => reset()}>Сбросить</Button>
        </div>
      </form>
    </Card>
  );
}
