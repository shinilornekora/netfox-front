import { Link } from 'react-router-dom';
import type { Title } from '@/shared/api/types';
import { pathTo } from '@/shared/constants';
import { formatDuration } from '@/shared/lib/format';
import { Card } from '@/shared/ui';

/** Серии сериала; canWatch — показывать кнопку «▶» у каждой серии. */
export function EpisodeList({ title: t, canWatch }: { title: Title; canWatch: boolean }) {
  const eps = t.episodes ?? [];
  return (
    <Card as="section">
      <h2>Серии</h2>
      {eps.length === 0 ? (
        <p className="muted">Серий пока нет.</p>
      ) : (
        <ul className="episodes" aria-label="Серии">
          {eps.map((e) => (
            <li key={e.id} className="neu-inset">
              <span>
                <b>
                  {e.season}×{String(e.number).padStart(2, '0')}
                </b>{' '}
                {e.name} <span className="muted">· {formatDuration(e.duration_sec)}</span>
              </span>
              {canWatch && (
                <Link
                  to={pathTo.watch(t.id, e.id)}
                  className="neu-btn small"
                  aria-label={`Смотреть: ${e.name}`}
                >
                  ▶
                </Link>
              )}
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}
