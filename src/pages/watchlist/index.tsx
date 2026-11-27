import { createEffect, createStore, sample } from 'effector';
import { createGate, useGate, useUnit } from 'effector-react';
import { TitleCard } from '@/entities/title/TitleCard';
import { api } from '@/shared/api/endpoints';
import type { WatchItem, WithTitle } from '@/shared/api/types';
import { createRequest } from '@/shared/lib/request';
import { Alert, Button, Empty, Spinner } from '@/shared/ui';

export const WatchlistGate = createGate<void>();
export const listQuery = createRequest((_: void) => api.watchlist());
export const removeFx = createEffect(async (id: string) => {
  await api.setWatch(id, false);
  return id;
});
export const $items = createStore<WithTitle<WatchItem>[]>([])
  .on(listQuery.fx.doneData, (_, r) => r.items)
  .on(removeFx.doneData, (list, id) => list.filter((i) => i.title_id !== id));
sample({ clock: WatchlistGate.open, fn: () => undefined, target: listQuery.start });

export default function WatchlistPage() {
  useGate(WatchlistGate);
  const [items, pending, error, remove] = useUnit([$items, listQuery.$pending, listQuery.$error, removeFx]);
  return (
    <div className="page stack">
      <h1>Смотреть позже</h1>
      {error && <Alert>{error}</Alert>}
      {pending && items.length === 0 ? (
        <Spinner />
      ) : items.length === 0 ? (
        <Empty>Список пуст — добавляйте фильмы звёздочкой на странице фильма</Empty>
      ) : (
        <div className="poster-grid">
          {items.map((i) => (
            <TitleCard
              key={i.title_id}
              title={i.title}
              footer={
                <Button small onClick={() => remove(i.title_id).catch(() => undefined)}>
                  Убрать
                </Button>
              }
            />
          ))}
        </div>
      )}
    </div>
  );
}
