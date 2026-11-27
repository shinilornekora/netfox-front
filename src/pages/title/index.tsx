import { Link, useLocation, useParams } from 'react-router-dom';
import { selectRole } from '@/entities/session/model';
import { useAppDispatch, useAppSelector } from '@/shared/lib/redux';
import { Poster } from '@/entities/title/Poster';
import { opened as checkoutOpened } from '@/features/checkout/model';
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
import { Alert, Badge, Button, Card, ProgressBar, Spinner } from '@/shared/ui';
import { usePageModel } from '@/shared/lib/hooks';
import * as m from './model';
import { RoutePath, pathTo, type IdParams } from '@/shared/constants';

export default function TitlePage() {
  const { id = '' } = useParams<IdParams>();
  const dispatch = useAppDispatch();
  usePageModel(m.titleOpened, m.titleClosed, id);
  const location = useLocation();
  const view = useAppSelector(m.selectView);
  const role = useAppSelector(selectRole);
  const inWatchlist = useAppSelector(m.selectInWatchlist);
  const watchError = useAppSelector(m.selectWatchError);
  const toggle = () => void dispatch(m.watchToggled());
  const buy = (t: Parameters<typeof checkoutOpened>[0]) => dispatch(checkoutOpened(t));
  const { title: t, offer, progress } = view;

  if (view.error) return <Alert>{view.error}</Alert>;
  if (!t || t.id !== id) return <Spinner />;

  const actions = m.actionsFor(t, offer);
  const watchAction = actions.find((a) => a.type === 'watch');

  return (
    <div className="page stack">
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
                Вы остановились на {formatClock(progress.position_sec)} из{' '}
                {formatClock(progress.duration_sec)}
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
                  <Button
                    key="buy"
                    variant={actions.length === 1 ? 'primary' : 'default'}
                    onClick={() =>
                      buy({
                        kind: 'purchase',
                        title_id: t.id,
                        label: t.name,
                        amount_kopecks: a.price,
                      })
                    }
                  >
                    Купить навсегда за {formatMoney(a.price)}
                  </Button>
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
            {role === 'viewer' && (
              <Button aria-pressed={inWatchlist} onClick={() => toggle()}>
                {inWatchlist ? '★ В списке' : '☆ Смотреть позже'}
              </Button>
            )}
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

      {t.kind === 'series' && (
        <Card as="section">
          <h2>Серии</h2>
          {(t.episodes ?? []).length === 0 ? (
            <p className="muted">Серий пока нет.</p>
          ) : (
            <ul className="episodes" aria-label="Серии">
              {(t.episodes ?? []).map((e) => (
                <li key={e.id} className="neu-inset">
                  <span>
                    <b>
                      {e.season}×{String(e.number).padStart(2, '0')}
                    </b>{' '}
                    {e.name} <span className="muted">· {formatDuration(e.duration_sec)}</span>
                  </span>
                  {role === 'viewer' && watchAction && (
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
      )}
    </div>
  );
}
