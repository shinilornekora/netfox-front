import { useCallback, useEffect, useRef } from 'react';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { formatClock } from '@/shared/lib/format';
import { useAppDispatch, useAppSelector } from '@/shared/lib/redux';
import { Alert, Card, Spinner } from '@/shared/ui';
import * as m from './model';

const PING_MS = 10_000;

export default function WatchPage() {
  const { id = '' } = useParams();
  const [params] = useSearchParams();
  const ep = params.get('ep') ?? undefined;
  const dispatch = useAppDispatch();
  useEffect(() => {
    void dispatch(m.watchOpened({ id, ep }));
    return () => void dispatch(m.watchClosed());
  }, [dispatch, id, ep]);
  const { start, startError: error, pending } = useAppSelector(m.selectWatch);
  const report = useCallback(
    (p: { position: number; duration: number }) => void dispatch(m.positionReported(p)),
    [dispatch],
  );
  const video = useRef<HTMLVideoElement>(null);

  // Пинг прогресса, пока видео играет; плюс на паузе, в конце и при уходе со страницы.
  useEffect(() => {
    const v = video.current;
    if (!start || !v) return;
    const send = () => report({ position: v.currentTime, duration: v.duration || start.duration_sec });
    const timer = setInterval(() => !v.paused && send(), PING_MS);
    v.addEventListener('pause', send);
    v.addEventListener('ended', send);
    return () => {
      clearInterval(timer);
      v.removeEventListener('pause', send);
      v.removeEventListener('ended', send);
      send();
    };
  }, [start, report]);

  if (error) {
    return (
      <div className="page" style={{ maxWidth: 560, margin: '0 auto' }}>
        <Card className="stack">
          <h1>Не получается включить</h1>
          <Alert>{error.message}</Alert>
          {error.code === 'not_entitled' && (
            <div className="row">
              <Link to="/plans" className="neu-btn primary">
                Выбрать подписку
              </Link>
              <Link to={`/title/${id}`} className="neu-btn">
                К фильму
              </Link>
            </div>
          )}
          {error.status === 403 && (
            <Link to="/cabinet/profile" className="neu-btn">
              Указать дату рождения
            </Link>
          )}
          {error.code !== 'not_entitled' && error.status !== 403 && <Link to={`/title/${id}`}>К тайтлу</Link>}
        </Card>
      </div>
    );
  }
  if (pending || !start) return <Spinner label="Готовим видео" />;

  return (
    <div className="page stack">
      <div className="row-between">
        <h1>{start.episode ? `Серия ${start.episode.number}. ${start.episode.name}` : 'Просмотр'}</h1>
        <Link to={`/title/${id}`} className="neu-btn small">
          ← К тайтлу
        </Link>
      </div>
      {start.demo && <Alert tone="info">Демо-ролик: полная версия фильма ещё загружается на сервер.</Alert>}
      {start.resume_at_sec > 0 && <p className="muted">Продолжаем с {formatClock(start.resume_at_sec)}</p>}
      <video
        ref={video}
        className="player"
        src={start.stream_url}
        controls
        autoPlay
        aria-label="Плеер"
        onLoadedMetadata={(e) => {
          if (start.resume_at_sec > 0) e.currentTarget.currentTime = start.resume_at_sec;
        }}
      />
      <p className="muted" style={{ fontSize: 13 }}>
        Доступ: {start.via === 'purchase' ? 'покупка' : 'подписка'}. Ссылка на поток действует 4 часа.
      </p>
    </div>
  );
}
