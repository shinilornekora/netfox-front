// Фича «Серии сериала» (админ): добавить и удалить серию.
// Серии меняют список и ревизию тайтла — редактор перечитывает тайтл (app/listeners).
import { api, type EpisodeIn } from '@/shared/api/endpoints';
import { createApiThunk } from '@/shared/lib/request';

export const saveEpisode = createApiThunk(
  'episodes/save',
  ({ titleId, ep }: { titleId: string; ep: EpisodeIn }) => api.saveEpisode(titleId, ep),
);
export const deleteEpisode = createApiThunk(
  'episodes/delete',
  async ({ titleId, id }: { titleId: string; id: string }) => {
    await api.deleteEpisode(titleId, id);
  },
);
