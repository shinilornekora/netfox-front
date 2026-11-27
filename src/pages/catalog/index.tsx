import { useEffect, useRef, type FormEvent } from 'react';
import { TitleCard } from '@/entities/title/TitleCard';
import { selectRole } from '@/entities/session/model';
import { useAppDispatch, useAppSelector } from '@/shared/lib/redux';
import { kindLabel } from '@/shared/lib/format';
import { Alert, Button, Card, Empty, Select, Spinner, TextField } from '@/shared/ui';
import { useDebounce, usePageModel } from '@/shared/lib/hooks';
import * as m from './model';

export default function CatalogPage() {
  const dispatch = useAppDispatch();
  usePageModel(m.catalogOpened, m.catalogClosed);
  const filters = useAppSelector(m.selectFilters);
  const view = useAppSelector(m.selectView);
  const genres = useAppSelector(m.selectGenres);
  const cont = useAppSelector(m.selectContinue);
  const role = useAppSelector(selectRole);
  const change = (patch: Partial<m.Filters>) => dispatch(m.filterChanged(patch));
  // Строка, по которой последний раз искали: Enter, кнопка или автопоиск.
  const searchedQ = useRef(filters.q);
  const submit = () => {
    searchedQ.current = filters.q;
    void dispatch(m.searchSubmitted());
  };
  const reset = () => {
    searchedQ.current = m.initialFilters.q;
    void dispatch(m.filtersReset());
  };
  const more = () => void dispatch(m.moreRequested());

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    submit();
  };
  // Поиск по названию запускается сам, когда пользователь перестал печатать на 400 мс
  // (useDebounce). Enter и кнопка ищут сразу — тогда автопоиск по той же строке не повторяется.
  const debouncedQ = useDebounce(filters.q, 400);
  useEffect(() => {
    if (debouncedQ === searchedQ.current) return;
    searchedQ.current = debouncedQ;
    void dispatch(m.searchSubmitted());
  }, [debouncedQ, dispatch]);

  // Селекты применяются сразу.
  const pick = (patch: Partial<m.Filters>) => {
    change(patch);
    submit();
  };

  return (
    <div className="page stack">
      <div>
        <h1>Кино, которое пережило век</h1>
        <p className="muted">Классика немого кино по подписке или навсегда — в один клик.</p>
      </div>

      {role === 'viewer' && cont.length > 0 && (
        <section className="stack" aria-label="Продолжить просмотр">
          <h2>Продолжить просмотр</h2>
          <div className="poster-grid">
            {cont.map((p) => (
              <TitleCard
                key={p.title_id}
                title={p.title}
                progress={p.duration_sec ? p.position_sec / p.duration_sec : 0}
              />
            ))}
          </div>
        </section>
      )}

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
            onChange={(e) => pick({ kind: e.target.value as m.Filters['kind'] })}
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
            options={[
              { value: '', label: 'Любой' },
              ...genres.map((g) => ({ value: g.name, label: g.name })),
            ]}
          />
          <Select
            label="Доступ"
            value={filters.access}
            onChange={(e) => pick({ access: e.target.value as m.Filters['access'] })}
            options={[
              { value: '', label: 'Любой' },
              { value: 'subscription', label: 'В подписке' },
              { value: 'purchase', label: 'Можно купить' },
            ]}
          />
          <Select
            label="Сортировка"
            value={filters.sort}
            onChange={(e) => pick({ sort: e.target.value as m.Filters['sort'] })}
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

      {view.error && <Alert>{view.error}</Alert>}
      {view.pending && view.items.length === 0 ? (
        <Spinner />
      ) : view.items.length === 0 ? (
        <Empty>Ничего не нашлось — попробуйте другой жанр или название</Empty>
      ) : (
        <section aria-label="Каталог" className="stack">
          <div className="poster-grid">
            {view.items.map((t) => (
              <TitleCard key={t.id} title={t} />
            ))}
          </div>
          {view.hasMore && (
            <div className="row" style={{ justifyContent: 'center' }}>
              <Button onClick={() => more()} disabled={view.morePending}>
                {view.morePending ? 'Загружаем…' : 'Показать ещё'}
              </Button>
            </div>
          )}
        </section>
      )}
    </div>
  );
}
