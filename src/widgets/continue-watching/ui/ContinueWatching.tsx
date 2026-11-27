import type { Progress, WithTitle } from '@/shared/api/types';
import { TitleCard } from '@/entities/title';

/** «Продолжить просмотр»: недосмотренные тайтлы с полоской прогресса. */
export function ContinueWatching({ items }: { items: WithTitle<Progress>[] }) {
  if (items.length === 0) return null;
  return (
    <section className="stack" aria-label="Продолжить просмотр">
      <h2>Продолжить просмотр</h2>
      <div className="poster-grid">
        {items.map((p) => (
          <TitleCard
            key={p.title_id}
            title={p.title}
            progress={p.duration_sec ? p.position_sec / p.duration_sec : 0}
          />
        ))}
      </div>
    </section>
  );
}
