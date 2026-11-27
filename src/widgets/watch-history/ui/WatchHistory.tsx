import { Link } from 'react-router-dom';
import type { HistoryPage } from '@/shared/api/types';
import { pathTo } from '@/shared/constants';
import { formatDateTime, kindLabel } from '@/shared/lib/format';
import { Badge, Button, Card, Empty } from '@/shared/ui';

/** История просмотров с догрузкой «Показать ещё». */
export function WatchHistory({
  items,
  hasMore,
  onMore,
}: {
  items: HistoryPage['items'];
  hasMore: boolean;
  onMore: () => void;
}) {
  if (items.length === 0) return <Empty>Вы ещё ничего не смотрели</Empty>;
  return (
    <div className="stack">
      {items.map((h) => (
        <Card key={h.id} as="article" className="row-between">
          <div>
            <Link to={pathTo.title(h.title_id)}>
              <b>{h.title.name}</b>
            </Link>
            <div className="muted">
              {kindLabel[h.title.kind]} · {formatDateTime(h.started_at)} ·{' '}
              {h.via === 'purchase' ? 'покупка' : 'подписка'}
            </div>
          </div>
          {!h.title.available && <Badge tone="warn">Снят с показа</Badge>}
        </Card>
      ))}
      {hasMore && (
        <div className="row" style={{ justifyContent: 'center' }}>
          <Button onClick={() => onMore()}>Показать ещё</Button>
        </div>
      )}
    </div>
  );
}
