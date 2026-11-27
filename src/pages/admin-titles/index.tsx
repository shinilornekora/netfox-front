import { createEffect, createEvent, createStore, sample } from 'effector';
import { createGate, useGate, useUnit } from 'effector-react';
import type { FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { api } from '@/shared/api/endpoints';
import type { Title, TitleStatus } from '@/shared/api/types';
import { createRequest } from '@/shared/lib/request';
import { formatDateTime, formatMoney, kindLabel, titleStatusLabel } from '@/shared/lib/format';
import { Alert, Badge, Button, Card, Empty, Select, Spinner, TextField } from '@/shared/ui';

export const AdminTitlesGate = createGate<void>();
export const filterChanged = createEvent<{ q?: string; status?: TitleStatus | '' }>();
export const searched = createEvent();
export const $filter = createStore<{ q: string; status: TitleStatus | '' }>({ q: '', status: '' }).on(
  filterChanged,
  (f, p) => ({ ...f, ...p }),
);
export const listQuery = createRequest((f: { q: string; status: string }) => api.adminTitles(f));
sample({ clock: [AdminTitlesGate.open, searched], source: $filter, target: listQuery.start });

// Админский список отдаётся по 50, дальше — по next_cursor.
export const moreRequested = createEvent();
export const moreFx = createEffect((q: { q: string; status: string; cursor: string }) => api.adminTitles(q));
export const $items = createStore<Title[]>([])
  .on(listQuery.fx.doneData, (_, r) => r.items)
  .on(moreFx.doneData, (list, r) => [...list, ...r.items]);
export const $cursor = createStore('')
  .on(listQuery.fx.doneData, (_, r) => r.next_cursor || '')
  .on(moreFx.doneData, (_, r) => r.next_cursor || '');
sample({
  clock: moreRequested,
  source: { f: $filter, cursor: $cursor },
  filter: ({ cursor }) => cursor !== '',
  fn: ({ f, cursor }) => ({ ...f, cursor }),
  target: moreFx,
});

const tone = { draft: 'info', published: 'ok', withdrawn: 'warn' } as const;

export default function AdminTitlesPage() {
  useGate(AdminTitlesGate);
  const [filter, change, search, data, pending, error] = useUnit([
    $filter,
    filterChanged,
    searched,
    listQuery.$data,
    listQuery.$pending,
    listQuery.$error,
  ]);
  const [items, cursor, more] = useUnit([$items, $cursor, moreRequested]);
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    search();
  };

  return (
    <div className="page stack">
      <div className="row-between">
        <h1>Каталог</h1>
        <Link to="/admin/titles/new" className="neu-btn primary">
          + Новый тайтл
        </Link>
      </div>
      <Card>
        <form className="form-grid" onSubmit={onSubmit} aria-label="Фильтр тайтлов">
          <TextField label="Название" value={filter.q} onChange={(e) => change({ q: e.target.value })} />
          <Select
            label="Статус"
            value={filter.status}
            onChange={(e) => {
              change({ status: e.target.value as TitleStatus | '' });
              search();
            }}
            options={[
              { value: '', label: 'Все' },
              { value: 'draft', label: titleStatusLabel.draft },
              { value: 'published', label: titleStatusLabel.published },
              { value: 'withdrawn', label: titleStatusLabel.withdrawn },
            ]}
          />
          <div className="row" style={{ alignSelf: 'end' }}>
            <Button type="submit" variant="primary">
              Найти
            </Button>
          </div>
        </form>
      </Card>
      {error && <Alert>{error}</Alert>}
      {pending && !data ? (
        <Spinner />
      ) : items.length === 0 ? (
        <Empty>Тайтлов не найдено</Empty>
      ) : (
        <div className="table-wrap">
          <table className="table" aria-label="Тайтлы">
            <thead>
              <tr>
                <th>Название</th>
                <th>Тип</th>
                <th>Доступ</th>
                <th>Статус</th>
                <th>Изменён</th>
              </tr>
            </thead>
            <tbody>
              {items.map((t) => (
                <tr key={t.id}>
                  <td data-label="Название">
                    <Link to={`/admin/titles/${t.id}`}>{t.name}</Link> <span className="muted">{t.year}</span>
                  </td>
                  <td data-label="Тип">{kindLabel[t.kind]}</td>
                  <td data-label="Доступ">
                    {[t.in_subscription && 'подписка', t.price_kopecks > 0 && formatMoney(t.price_kopecks)]
                      .filter(Boolean)
                      .join(' · ') || '—'}
                  </td>
                  <td data-label="Статус">
                    <Badge tone={tone[t.status]}>{titleStatusLabel[t.status]}</Badge>
                  </td>
                  <td data-label="Изменён">{formatDateTime(t.updated_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {cursor && (
        <div className="row" style={{ justifyContent: 'center' }}>
          <Button onClick={() => more()}>Показать ещё</Button>
        </div>
      )}
    </div>
  );
}
