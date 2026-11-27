import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import type { Title, TitleRef } from '@/shared/api/types';
import { formatMoney, kindLabel } from '@/shared/lib/format';
import { Badge, ProgressBar } from '@/shared/ui';
import { Poster } from './Poster';

/** Карточка тайтла в сетке витрины. Вся карточка — ссылка на страницу тайтла. */
export function TitleCard({
  title,
  progress,
  footer,
}: {
  title: Title | TitleRef;
  /** Доля просмотренного 0..1 — для «Продолжить просмотр». */
  progress?: number;
  footer?: ReactNode;
}) {
  const full = 'in_subscription' in title ? title : null;
  return (
    <article className="neu-card title-card" aria-label={title.name}>
      <Link to={`/title/${title.id}`} className="stack" style={{ gap: 8, color: 'inherit' }}>
        <Poster title={title} />
        <h3>{title.name}</h3>
      </Link>
      <div className="muted" style={{ fontSize: 13 }}>
        {kindLabel[title.kind]} · {title.year}
      </div>
      {full && (
        <div className="chips">
          {full.in_subscription && <Badge tone="ok">В подписке</Badge>}
          {full.kind === 'film' && full.price_kopecks > 0 && (
            <Badge tone="info">{formatMoney(full.price_kopecks)}</Badge>
          )}
        </div>
      )}
      {'available' in title && !title.available && <Badge tone="warn">Снят с показа</Badge>}
      {progress !== undefined && <ProgressBar value={progress} label={`Просмотрено: ${title.name}`} />}
      {footer}
    </article>
  );
}
