import { Link } from 'react-router-dom';
import type { Purchase } from '@/shared/api/types';
import { pathTo } from '@/shared/constants';
import { formatDate, formatMoney } from '@/shared/lib/format';
import { Card, Empty } from '@/shared/ui';

/** Купленные навсегда фильмы. */
export function PurchaseList({ items }: { items: Purchase[] }) {
  if (items.length === 0) return <Empty>Купленных фильмов пока нет</Empty>;
  return (
    <div className="stack">
      {items.map((p) => (
        <Card key={p.title_id} as="article" className="row-between">
          <div>
            <Link to={pathTo.title(p.title_id)}>
              <b>{p.title_name}</b>
            </Link>
            <div className="muted">
              Куплен {formatDate(p.purchased_at)} за {formatMoney(p.amount_kopecks)}
            </div>
          </div>
          <Link to={pathTo.watch(p.title_id)} className="neu-btn small primary">
            Смотреть
          </Link>
        </Card>
      ))}
    </div>
  );
}
