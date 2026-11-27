// Редактор тайтла (админ). Каждое изменение несёт revision: если тайтл
// успели поправить в другой вкладке, сервер ответит 409 и ничего не затрёт.
import { createEffect, createEvent, createStore, sample } from 'effector';
import { createGate } from 'effector-react';
import { api, type EpisodeIn } from '@/shared/api/endpoints';
import type { Title, TitleFields } from '@/shared/api/types';
import { errorFields, errorMessage } from '@/shared/lib/format';

export const EditGate = createGate<{ id: string | null }>();

export const loadFx = createEffect(api.title);
export const createFx = createEffect(api.createTitle);
export const saveFx = createEffect(({ id, ...b }: TitleFields & { id: string; revision: number }) =>
  api.editTitle(id, b),
);
export const publishFx = createEffect((t: Title) => api.publishTitle(t.id, t.revision));
export const withdrawFx = createEffect(({ t, reason }: { t: Title; reason: string }) =>
  api.withdrawTitle(t.id, t.revision, reason),
);
export const deleteFx = createEffect((id: string) => api.deleteTitle(id));
export const saveEpisodeFx = createEffect(({ titleId, ep }: { titleId: string; ep: EpisodeIn }) =>
  api.saveEpisode(titleId, ep),
);
export const deleteEpisodeFx = createEffect(({ titleId, id }: { titleId: string; id: string }) =>
  api.deleteEpisode(titleId, id),
);

export const messageShown = createEvent<string>();

const mutations = [createFx, saveFx, publishFx, withdrawFx, deleteFx, saveEpisodeFx, deleteEpisodeFx];

export const $title = createStore<Title | null>(null)
  .on(
    [loadFx.doneData, createFx.doneData, saveFx.doneData, publishFx.doneData, withdrawFx.doneData],
    (_, t) => t,
  )
  .reset(EditGate.close);
export const $loadError = createStore<string | null>(null)
  .on(loadFx.failData, (_, e) => errorMessage(e))
  .reset(EditGate.close, loadFx);
export const $error = createStore<string | null>(null)
  .on(
    mutations.map((fx) => fx.failData),
    (_, e) => errorMessage(e),
  )
  .reset(EditGate.close, ...mutations);
export const $fields = createStore<Record<string, string>>({})
  .on([createFx.failData, saveFx.failData, publishFx.failData, saveEpisodeFx.failData], (_, e) =>
    errorFields(e),
  )
  .reset(EditGate.close, createFx, saveFx, publishFx, saveEpisodeFx);
export const $notice = createStore<string | null>(null)
  .on(messageShown, (_, m) => m)
  .on(saveFx.done, () => 'Сохранено')
  .on(publishFx.done, () => 'Опубликовано — тайтл появился на витрине')
  .on(withdrawFx.done, () => 'Снят с показа')
  .reset(EditGate.close, createFx, deleteFx, saveEpisodeFx, deleteEpisodeFx, saveFx, publishFx, withdrawFx);

sample({ clock: EditGate.open, filter: (p) => p.id !== null, fn: (p) => p.id!, target: loadFx });
// Серии меняют их список (и счётчик) — перечитываем тайтл целиком, чтобы взять новую ревизию.
sample({
  clock: [saveEpisodeFx.done, deleteEpisodeFx.done],
  fn: ({ params }) => params.titleId,
  target: loadFx,
});

export const emptyFields: TitleFields = {
  kind: 'film',
  name: '',
  original_name: '',
  year: new Date().getFullYear(),
  description: '',
  age_rating: 0,
  genres: [],
  duration_sec: 5400,
  in_subscription: true,
  price_kopecks: 0,
  poster_hue: 200,
  media: '',
};

export function fieldsOf(t: Title): TitleFields {
  return {
    kind: t.kind,
    name: t.name,
    original_name: t.original_name,
    year: t.year,
    description: t.description,
    age_rating: t.age_rating,
    genres: t.genres,
    duration_sec: t.duration_sec,
    in_subscription: t.in_subscription,
    price_kopecks: t.price_kopecks,
    poster_hue: t.poster_hue,
    media: t.media ?? '',
  };
}
