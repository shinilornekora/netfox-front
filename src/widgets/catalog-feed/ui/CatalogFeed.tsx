import type { Title } from '@/shared/api/types';
import { TitleCard } from '@/entities/title';
import { Alert, Button, Empty, Spinner } from '@/shared/ui';

export interface CatalogFeedView {
  items: Title[];
  pending: boolean;
  morePending: boolean;
  error: string | null;
  hasMore: boolean;
}

/** Лента витрины: сетка карточек и «Показать ещё». */
export function CatalogFeed({ view, onMore }: { view: CatalogFeedView; onMore: () => void }) {
  return (
    <>
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
              <Button onClick={() => onMore()} disabled={view.morePending}>
                {view.morePending ? 'Загружаем…' : 'Показать ещё'}
              </Button>
            </div>
          )}
        </section>
      )}
    </>
  );
}
