import { useEffect, type RefObject } from 'react';

export interface PlaybackPoint {
  position: number;
  duration: number;
}

/**
 * Сообщает прогресс просмотра: раз в `everyMs`, пока видео играет, а также
 * на паузе, в конце и при уходе со страницы. Подписки на события <video>
 * и таймер снимаются в функции очистки эффекта. Новый `session` (другая серия)
 * перезапускает отслеживание.
 */
export function useProgressPing(
  video: RefObject<HTMLVideoElement>,
  session: { duration_sec: number } | null,
  report: (p: PlaybackPoint) => void,
  everyMs = 10_000,
): void {
  useEffect(() => {
    const v = video.current;
    if (!session || !v) return;
    const send = () => report({ position: v.currentTime, duration: v.duration || session.duration_sec });
    const timer = setInterval(() => !v.paused && send(), everyMs);
    v.addEventListener('pause', send);
    v.addEventListener('ended', send);
    return () => {
      clearInterval(timer);
      v.removeEventListener('pause', send);
      v.removeEventListener('ended', send);
      send();
    };
  }, [video, session, report, everyMs]);
}
