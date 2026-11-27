import { useEffect } from 'react';
import { TitleCard } from '@/entities/title/TitleCard';
import { useAppDispatch, useAppSelector } from '@/shared/lib/redux';
import { Alert, Button, Empty, Spinner } from '@/shared/ui';
import * as m from './model';

export default function WatchlistPage() {
  const dispatch = useAppDispatch();
  useEffect(() => void dispatch(m.watchlistOpened()), [dispatch]);
  const { items, pending, error } = useAppSelector(m.selectWatchlist);
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
                <Button small onClick={() => void dispatch(m.removed(i.title_id))}>
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
