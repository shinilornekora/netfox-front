import { TitleCard } from '@/entities/title';
import { WatchlistRemoveButton } from '@/features/watchlist-toggle';
import { useAppSelector } from '@/shared/lib/redux';
import { Alert, Empty, Spinner } from '@/shared/ui';
import { usePageModel } from '@/shared/lib/hooks';
import * as m from '../model';

export default function WatchlistPage() {
  usePageModel(m.watchlistOpened);
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
              footer={<WatchlistRemoveButton titleId={i.title_id} />}
            />
          ))}
        </div>
      )}
    </div>
  );
}
