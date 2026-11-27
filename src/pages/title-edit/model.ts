// Редактор тайтла (админ). Каждое изменение несёт revision: если тайтл
// успели поправить в другой вкладке, сервер ответит 409 и ничего не затрёт.
import { createAsyncThunk, createSlice, isAnyOf } from '@reduxjs/toolkit';
import { api, type EpisodeIn } from '@/shared/api/endpoints';
import type { Title, TitleFields } from '@/shared/api/types';
import { createApiThunk, problemText } from '@/shared/lib/request';
import type { RootState } from '@/shared/lib/redux';

export interface TitleEditState {
  title: Title | null;
  loadError: string | null;
  /** Ошибка последнего действия (сохранить, опубликовать, серия…). */
  error: string | null;
  /** Ошибки полей формы из Problem.errors. */
  fields: Record<string, string>;
  notice: string | null;
}

const initialState: TitleEditState = { title: null, loadError: null, error: null, fields: {}, notice: null };

export const loadTitle = createApiThunk('titleEdit/load', (id: string) => api.title(id));
export const createTitle = createApiThunk('titleEdit/create', (b: TitleFields) => api.createTitle(b));
export const saveTitle = createApiThunk(
  'titleEdit/save',
  ({ id, ...b }: TitleFields & { id: string; revision: number }) => api.editTitle(id, b),
);
export const publishTitle = createApiThunk('titleEdit/publish', (t: Title) =>
  api.publishTitle(t.id, t.revision),
);
export const withdrawTitle = createApiThunk(
  'titleEdit/withdraw',
  ({ t, reason }: { t: Title; reason: string }) => api.withdrawTitle(t.id, t.revision, reason),
);
export const deleteTitle = createApiThunk('titleEdit/delete', (id: string) => api.deleteTitle(id));
// Серии меняют их список (и счётчик) — перечитываем тайтл целиком, чтобы взять новую ревизию.
export const saveEpisode = createApiThunk(
  'titleEdit/saveEpisode',
  async ({ titleId, ep }: { titleId: string; ep: EpisodeIn }, { dispatch }) => {
    const r = await api.saveEpisode(titleId, ep);
    void dispatch(loadTitle(titleId));
    return r;
  },
);
export const deleteEpisode = createApiThunk(
  'titleEdit/deleteEpisode',
  async ({ titleId, id }: { titleId: string; id: string }, { dispatch }) => {
    await api.deleteEpisode(titleId, id);
    void dispatch(loadTitle(titleId));
  },
);

/** Любое изменение тайтла или серий. */
const mutationPending = isAnyOf(
  createTitle.pending,
  saveTitle.pending,
  publishTitle.pending,
  withdrawTitle.pending,
  deleteTitle.pending,
  saveEpisode.pending,
  deleteEpisode.pending,
);
const mutationRejected = isAnyOf(
  createTitle.rejected,
  saveTitle.rejected,
  publishTitle.rejected,
  withdrawTitle.rejected,
  deleteTitle.rejected,
  saveEpisode.rejected,
  deleteEpisode.rejected,
);
/** Действия, у которых сервер может вернуть ошибки полей формы. */
const fieldsPending = isAnyOf(
  createTitle.pending,
  saveTitle.pending,
  publishTitle.pending,
  saveEpisode.pending,
);
const fieldsRejected = isAnyOf(
  createTitle.rejected,
  saveTitle.rejected,
  publishTitle.rejected,
  saveEpisode.rejected,
);

export const titleEditSlice = createSlice({
  name: 'titleEdit',
  initialState,
  reducers: {
    editClosed: () => initialState,
  },
  extraReducers: (b) =>
    b
      .addCase(loadTitle.pending, (s) => void (s.loadError = null))
      .addCase(loadTitle.rejected, (s, a) => void (s.loadError = problemText(a)))
      .addCase(saveTitle.fulfilled, (s) => void (s.notice = 'Сохранено'))
      .addCase(publishTitle.fulfilled, (s) => void (s.notice = 'Опубликовано — тайтл появился на витрине'))
      .addCase(withdrawTitle.fulfilled, (s) => void (s.notice = 'Снят с показа'))
      .addMatcher(
        isAnyOf(
          loadTitle.fulfilled,
          createTitle.fulfilled,
          saveTitle.fulfilled,
          publishTitle.fulfilled,
          withdrawTitle.fulfilled,
        ),
        (s, a) => void (s.title = a.payload),
      )
      .addMatcher(mutationPending, (s) => {
        s.error = null;
        s.notice = null;
      })
      .addMatcher(fieldsPending, (s) => void (s.fields = {}))
      .addMatcher(mutationRejected, (s, a) => void (s.error = problemText(a)))
      .addMatcher(fieldsRejected, (s, a) => void (s.fields = a.payload?.fields ?? {})),
});

export const { editClosed } = titleEditSlice.actions;

export const editOpened = createAsyncThunk('titleEdit/opened', async (id: string | null, { dispatch }) => {
  if (id !== null) await dispatch(loadTitle(id));
});

export const selectTitleEdit = (s: RootState) => s.titleEdit;

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
