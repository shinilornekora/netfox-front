import { useGate, useUnit } from 'effector-react';
import { useEffect, useState, type FormEvent } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import type { Kind, TitleFields } from '@/shared/api/types';
import { AGE_RATINGS, GENRES } from '@/shared/lib/genres';
import { formatDuration, titleStatusLabel } from '@/shared/lib/format';
import { Poster } from '@/entities/title/Poster';
import { Alert, Badge, Button, Card, Modal, Select, Spinner, TextArea, TextField } from '@/shared/ui';
import * as m from './model';

export default function TitleEditPage() {
  const { id = null } = useParams();
  useGate(m.EditGate, { id });
  const navigate = useNavigate();
  const [title, loadError, error, fields, notice] = useUnit([
    m.$title,
    m.$loadError,
    m.$error,
    m.$fields,
    m.$notice,
  ]);
  const [create, save, publish, withdraw, remove] = useUnit([
    m.createFx,
    m.saveFx,
    m.publishFx,
    m.withdrawFx,
    m.deleteFx,
  ]);
  const [form, setForm] = useState<TitleFields>(m.emptyFields);
  const [withdrawing, setWithdrawing] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);

  useEffect(() => {
    if (title && title.id === id) setForm(m.fieldsOf(title));
    if (!id) setForm(m.emptyFields);
  }, [title, id]);

  if (loadError) return <Alert>{loadError}</Alert>;
  if (id && (!title || title.id !== id)) return <Spinner />;

  const set = <K extends keyof TitleFields>(k: K, v: TitleFields[K]) => setForm((f) => ({ ...f, [k]: v }));
  const toggleGenre = (g: string) =>
    set('genres', form.genres.includes(g) ? form.genres.filter((x) => x !== g) : [...form.genres, g]);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const body: TitleFields = { ...form, price_kopecks: form.kind === 'series' ? 0 : form.price_kopecks };
    if (!title || !id) {
      create(body)
        .then((t) => navigate(`/admin/titles/${t.id}`, { replace: true }))
        .catch(() => undefined);
    } else {
      save({ ...body, id: title.id, revision: title.revision }).catch(() => undefined);
    }
  };

  return (
    <div className="page stack">
      <div className="row-between">
        <h1>{id ? `Редактирование: ${title?.name}` : 'Новый тайтл'}</h1>
        <Link to="/admin/titles" className="neu-btn small">
          ← К каталогу
        </Link>
      </div>
      {notice && <Alert tone="success">{notice}</Alert>}
      {error && <Alert>{error}</Alert>}

      {title && id && (
        <Card as="section" className="row-between">
          <div className="row">
            <Badge tone={title.status === 'published' ? 'ok' : title.status === 'draft' ? 'info' : 'warn'}>
              {titleStatusLabel[title.status]}
            </Badge>
            <span className="muted">ревизия {title.revision}</span>
            {title.withdraw_reason && <span className="muted">Причина: {title.withdraw_reason}</span>}
          </div>
          <div className="row">
            {title.status !== 'published' && (
              <Button variant="primary" onClick={() => publish(title).catch(() => undefined)}>
                Опубликовать
              </Button>
            )}
            {title.status === 'published' && (
              <Button onClick={() => setWithdrawing(true)}>Снять с показа</Button>
            )}
            {title.status === 'published' && (
              <Link to={`/title/${title.id}`} className="neu-btn">
                На витрине
              </Link>
            )}
            {title.status === 'draft' && (
              <Button variant="danger" onClick={() => setConfirmDelete(true)}>
                Удалить
              </Button>
            )}
          </div>
        </Card>
      )}

      <Card>
        <form className="stack" onSubmit={submit} aria-label="Карточка тайтла">
          <div className="title-hero">
            <Poster title={{ ...form, poster_url: title?.poster_url }} big />
            <div className="form-grid" style={{ flex: 1 }}>
              <Select
                label="Тип"
                value={form.kind}
                disabled={Boolean(id)}
                onChange={(e) => set('kind', e.target.value as Kind)}
                options={[
                  { value: 'film', label: 'Фильм' },
                  { value: 'series', label: 'Сериал' },
                ]}
              />
              <TextField
                label="Название"
                required
                value={form.name}
                onChange={(e) => set('name', e.target.value)}
                error={fields.name}
              />
              <TextField
                label="Оригинальное название"
                value={form.original_name}
                onChange={(e) => set('original_name', e.target.value)}
              />
              <TextField
                label="Год"
                type="number"
                value={form.year}
                onChange={(e) => set('year', Number(e.target.value))}
                error={fields.year}
              />
              <Select
                label="Возраст"
                value={String(form.age_rating)}
                onChange={(e) => set('age_rating', Number(e.target.value))}
                options={AGE_RATINGS.map((a) => ({ value: String(a), label: `${a}+` }))}
              />
              {form.kind === 'film' && (
                <TextField
                  label="Длительность, мин"
                  type="number"
                  min={1}
                  value={Math.round(form.duration_sec / 60)}
                  onChange={(e) => set('duration_sec', Number(e.target.value) * 60)}
                  error={fields.duration_sec}
                />
              )}
              {form.kind === 'film' && (
                <TextField
                  label="Цена покупки, ₽ (0 — не продаётся)"
                  type="number"
                  min={0}
                  value={form.price_kopecks / 100}
                  onChange={(e) => set('price_kopecks', Math.round(Number(e.target.value) * 100))}
                  error={fields.price_kopecks}
                />
              )}
              <TextField
                label="Цвет постера (0–359)"
                type="number"
                min={0}
                max={359}
                value={form.poster_hue}
                onChange={(e) => set('poster_hue', Number(e.target.value))}
              />
              <label className="row" style={{ alignSelf: 'end' }}>
                <input
                  type="checkbox"
                  checked={form.in_subscription}
                  onChange={(e) => set('in_subscription', e.target.checked)}
                />
                Входит в подписку
              </label>
            </div>
          </div>
          <TextArea
            label="Описание"
            rows={4}
            value={form.description}
            onChange={(e) => set('description', e.target.value)}
          />
          <fieldset className="neu-inset" style={{ border: 0 }}>
            <legend className="muted" style={{ fontWeight: 700 }}>
              Жанры
            </legend>
            <div className="chips">
              {GENRES.map((g) => (
                <Button key={g} small aria-pressed={form.genres.includes(g)} onClick={() => toggleGenre(g)}>
                  {g}
                </Button>
              ))}
            </div>
            {fields.genres && <em className="error">{fields.genres}</em>}
          </fieldset>
          <div className="row">
            <Button type="submit" variant="primary">
              {id ? 'Сохранить' : 'Создать черновик'}
            </Button>
          </div>
        </form>
      </Card>

      {title && id && title.kind === 'series' && <Episodes />}

      {withdrawing && title && (
        <WithdrawDialog
          onClose={() => setWithdrawing(false)}
          onSubmit={(reason) =>
            withdraw({ t: title, reason })
              .then(() => setWithdrawing(false))
              .catch(() => undefined)
          }
        />
      )}
      {confirmDelete && title && (
        <Modal title="Удалить черновик?" onClose={() => setConfirmDelete(false)}>
          <p>«{title.name}» будет удалён без возможности восстановления.</p>
          <div className="row">
            <Button
              variant="danger"
              onClick={() =>
                remove(title.id)
                  .then(() => navigate('/admin/titles'))
                  .catch(() => setConfirmDelete(false))
              }
            >
              Удалить
            </Button>
            <Button onClick={() => setConfirmDelete(false)}>Отмена</Button>
          </div>
        </Modal>
      )}
    </div>
  );
}

function WithdrawDialog({ onClose, onSubmit }: { onClose: () => void; onSubmit: (r: string) => void }) {
  const [reason, setReason] = useState('');
  return (
    <Modal title="Снять с показа" onClose={onClose}>
      <form
        className="stack"
        onSubmit={(e) => {
          e.preventDefault();
          onSubmit(reason);
        }}
      >
        <p className="muted">
          Купившие сохранят фильм в «Покупках», но смотреть его не смогут, пока он снят.
        </p>
        <TextArea label="Причина" required value={reason} onChange={(e) => setReason(e.target.value)} />
        <div className="row">
          <Button type="submit" variant="danger">
            Снять с показа
          </Button>
          <Button onClick={onClose}>Отмена</Button>
        </div>
      </form>
    </Modal>
  );
}

function Episodes() {
  const [title, fields, saveEp, deleteEp] = useUnit([
    m.$title,
    m.$fields,
    m.saveEpisodeFx,
    m.deleteEpisodeFx,
  ]);
  const eps = title?.episodes ?? [];
  const nextNo = (eps.at(-1)?.number ?? 0) + 1;
  const [ep, setEp] = useState({ season: 1, number: nextNo, name: '', minutes: 45 });
  useEffect(() => setEp((e) => ({ ...e, number: nextNo, name: '' })), [nextNo]);
  if (!title) return null;

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
