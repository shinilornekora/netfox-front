import { useEffect, useState } from 'react';
import type { Title } from '@/shared/api/types';
import { formatDuration } from '@/shared/lib/format';
import { useAppDispatch } from '@/shared/lib/redux';
import { Button, Card, TextField } from '@/shared/ui';
import { deleteEpisode, saveEpisode } from '../model';

/** Список серий с удалением и форма новой серии. fields — ошибки полей от сервера. */
export function EpisodeEditor({ title, fields }: { title: Title; fields: Record<string, string> }) {
  const dispatch = useAppDispatch();
  const saveEp = (p: Parameters<typeof saveEpisode>[0]) => dispatch(saveEpisode(p)).unwrap();
  const deleteEp = (p: Parameters<typeof deleteEpisode>[0]) => dispatch(deleteEpisode(p)).unwrap();
  const eps = title.episodes ?? [];
  const nextNo = (eps.at(-1)?.number ?? 0) + 1;
  const [ep, setEp] = useState({ season: 1, number: nextNo, name: '', minutes: 45 });
  useEffect(() => setEp((e) => ({ ...e, number: nextNo, name: '' })), [nextNo]);

  return (
    <Card as="section" className="stack">
      <h2>Серии ({eps.length})</h2>
      {eps.length > 0 && (
        <ul className="episodes" aria-label="Серии">
          {eps.map((e) => (
            <li key={e.id} className="neu-inset">
              <span>
                <b>
                  {e.season}×{String(e.number).padStart(2, '0')}
                </b>{' '}
                {e.name} <span className="muted">· {formatDuration(e.duration_sec)}</span>
              </span>
              <Button
                small
                variant="danger"
                aria-label={`Удалить серию ${e.name}`}
                onClick={() => deleteEp({ titleId: title.id, id: e.id }).catch(() => undefined)}
              >
                ✕
              </Button>
            </li>
          ))}
        </ul>
      )}
      <form
        className="form-grid"
        aria-label="Новая серия"
        onSubmit={(e) => {
          e.preventDefault();
          saveEp({
            titleId: title.id,
            ep: { season: ep.season, number: ep.number, name: ep.name, duration_sec: ep.minutes * 60 },
          }).catch(() => undefined);
        }}
      >
        <TextField
          label="Сезон"
          type="number"
          min={1}
          value={ep.season}
          onChange={(e) => setEp({ ...ep, season: Number(e.target.value) })}
        />
        <TextField
          label="Номер"
          type="number"
          min={1}
          value={ep.number}
          onChange={(e) => setEp({ ...ep, number: Number(e.target.value) })}
          error={fields.number}
        />
        <TextField
          label="Название серии"
          required
          value={ep.name}
          onChange={(e) => setEp({ ...ep, name: e.target.value })}
          error={fields.name}
        />
        <TextField
          label="Минут"
          type="number"
          min={1}
          value={ep.minutes}
          onChange={(e) => setEp({ ...ep, minutes: Number(e.target.value) })}
        />
        <div className="row" style={{ alignSelf: 'end' }}>
          <Button type="submit">Добавить серию</Button>
        </div>
      </form>
    </Card>
  );
}
