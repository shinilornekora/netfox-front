// Редактор тайтла (админ): загрузка, создание и сохранение карточки.
// Публикация/снятие/удаление (фича title-moderation) и серии (фича episode-editor)
// меняют тот же тайтл — их результат и ошибки редактор показывает через extraReducers.
// Каждое изменение несёт revision: если тайтл успели поправить в другой вкладке,
// сервер ответит 409 и ничего не затрёт.
import { createAsyncThunk, createSlice, isAnyOf } from '@reduxjs/toolkit';
import { api } from '@/shared/api/endpoints';
import { deleteTitle, publishTitle, withdrawTitle } from '@/features/title-moderation';
import { deleteEpisode, saveEpisode } from '@/features/episode-editor';
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

/** Перечитать открытый тайтл (после изменения серий — у него новая ревизия). */
export const titleReloaded = createAsyncThunk(
  'titleEdit/reloaded',
  async (id: string, { dispatch, getState }) => {
    if ((getState() as RootState).titleEdit.title?.id === id) await dispatch(loadTitle(id));
  },
);

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
