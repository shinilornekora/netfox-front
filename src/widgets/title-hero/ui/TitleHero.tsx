import { Link, useLocation } from 'react-router-dom';
import type { Offer, Progress, Role, Title } from '@/shared/api/types';
import { Poster, actionsFor } from '@/entities/title';
import { CheckoutButton } from '@/features/checkout';
import { WatchlistToggle } from '@/features/watchlist-toggle';
import { RoutePath, pathTo } from '@/shared/constants';
import {
  ageLabel,
  formatClock,
  formatDate,
  formatDuration,
  formatMoney,
  kindLabel,
  plural,
  titleStatusLabel,
} from '@/shared/lib/format';
import { Alert, Badge, Card, ProgressBar } from '@/shared/ui';

/**
 * Шапка карточки тайтла: постер, описание, прогресс и блок «Доступ» —
 * войти, смотреть, купить (фича checkout), оформить подписку, «Смотреть позже» (фича watchlist-toggle).
 */
export function TitleHero({
  title: t,
  offer,
  progress,
  role,
  inWatchlist,
  watchError,
}: {
  title: Title;
  offer: Offer | null;
  progress: Progress | null;
  role: Role | null;
  inWatchlist: boolean;
  watchError: string | null;
}) {
  const location = useLocation();
  const actions = actionsFor(t, offer);
  const watchAction = actions.find((a) => a.type === 'watch');

  return (
    <Card className="title-hero">
      <Poster title={t} big />
      <div className="stack" style={{ flex: 1, minWidth: 0 }}>
        <div>
          <h1>{t.name}</h1>
          <p className="muted">
            {t.original_name && t.original_name !== t.name ? `${t.original_name} · ` : ''}
            {kindLabel[t.kind]} · {t.year} · {ageLabel(t.age_rating)}
            {t.kind === 'film'
              ? ` · ${formatDuration(t.duration_sec)}`
              : ` · ${t.episode_count} ${plural(t.episode_count, 'серия', 'серии', 'серий')}`}
          </p>
        </div>
        <div className="chips">
          {t.genres.map((g) => (
            <Badge key={g}>{g}</Badge>
          ))}
          {t.in_subscription && <Badge tone="ok">В подписке</Badge>}
          {t.status !== 'published' && <Badge tone="warn">{titleStatusLabel[t.status]}</Badge>}
        </div>
        <p>{t.description}</p>
        {t.withdraw_reason && <Alert tone="info">Снят с показа: {t.withdraw_reason}</Alert>}

        {role === 'viewer' && progress && !progress.finished && progress.duration_sec > 0 && (
          <div className="stack" style={{ gap: 6 }}>
            <span className="muted">
              Вы остановились на {formatClock(progress.position_sec)} из {formatClock(progress.duration_sec)}
            </span>
            <ProgressBar value={progress.position_sec / progress.duration_sec} label="Просмотрено" />
          </div>
        )}

        <section className="row" aria-label="Доступ">
          {role === null && (
            <Link to={RoutePath.login} state={{ from: location.pathname }} className="neu-btn primary">
              Войти, чтобы смотреть
            </Link>
          )}
          {role === 'admin' && (
            <Link to={pathTo.titleEdit(t.id)} className="neu-btn primary">
              Редактировать
            </Link>
          )}
          {role === 'viewer' &&
            actions.map((a) =>
              a.type === 'watch' ? (
                <Link key="watch" to={pathTo.watch(t.id)} className="neu-btn primary">
                  {progress && !progress.finished ? 'Продолжить' : 'Смотреть'}
                </Link>
              ) : a.type === 'buy' ? (
                <CheckoutButton
                  key="buy"
                  variant={actions.length === 1 ? 'primary' : 'default'}
                  target={{ kind: 'purchase', title_id: t.id, label: t.name, amount_kopecks: a.price }}
                >
                  Купить навсегда за {formatMoney(a.price)}
                </CheckoutButton>
              ) : a.type === 'subscribe' ? (
                <Link key="sub" to={RoutePath.plans} className="neu-btn primary">
                  Оформить подписку
                </Link>
              ) : (
                <span key="na" className="muted">
                  Сейчас недоступен для просмотра
                </span>
              ),
            )}
          {role === 'viewer' && <WatchlistToggle titleId={t.id} on={inWatchlist} />}
        </section>
        {watchAction?.type === 'watch' && (
          <p className="muted" style={{ fontSize: 14 }}>
            {watchAction.via === 'purchase'
              ? 'Фильм куплен — он ваш навсегда.'
              : `Доступно по подписке${offer?.valid_until ? ` до ${formatDate(offer.valid_until)}` : ''}.`}
          </p>
        )}
        {watchError && <Alert>{watchError}</Alert>}
      </div>
    </Card>
  );
}
