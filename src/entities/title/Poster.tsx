import type { Title, TitleRef } from '@/shared/api/types';
import { ageLabel } from '@/shared/lib/format';

type PosterSource = Pick<Title | TitleRef, 'name' | 'poster_hue' | 'poster_url' | 'age_rating'>;

/**
 * Постер тайтла. Если картинки нет — градиент по poster_hue,
 * как в Плёнке: у каждого тайтла свой узнаваемый цвет.
 */
export function Poster({ title, big = false }: { title: PosterSource; big?: boolean }) {
  const h = title.poster_hue;
  const bg = title.poster_url
    ? `url(${title.poster_url}) center / cover`
    : `linear-gradient(160deg, hsl(${h} 70% 55%), hsl(${(h + 40) % 360} 60% 28%))`;
  return (
    <div className={`poster${big ? ' big' : ''}`} style={{ background: bg }} aria-hidden>
      <span className="age">{ageLabel(title.age_rating)}</span>
      {!title.poster_url && <span>{title.name}</span>}
    </div>
  );
}
