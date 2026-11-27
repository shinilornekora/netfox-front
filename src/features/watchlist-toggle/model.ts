// Фича «Смотреть позже»: добавить тайтл в список или убрать из него.
// Карточка тайтла и страница списка сами решают, как показать результат (extraReducers).
import { api } from '@/shared/api/endpoints';
import { createApiThunk } from '@/shared/lib/request';

export const setWatch = createApiThunk('watchlist/set', ({ id, on }: { id: string; on: boolean }) =>
  api.setWatch(id, on),
);
