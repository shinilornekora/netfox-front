// Фича «Модерация тайтла» (админ): опубликовать, снять с показа, удалить черновик.
// Каждое действие несёт revision: если тайтл поправили в другой вкладке, сервер ответит 409.
import { api } from '@/shared/api/endpoints';
import type { Title } from '@/shared/api/types';
import { createApiThunk } from '@/shared/lib/request';

export const publishTitle = createApiThunk('moderation/publish', (t: Title) =>
  api.publishTitle(t.id, t.revision),
);
export const withdrawTitle = createApiThunk(
  'moderation/withdraw',
  ({ t, reason }: { t: Title; reason: string }) => api.withdrawTitle(t.id, t.revision, reason),
);
export const deleteTitle = createApiThunk('moderation/delete', (id: string) => api.deleteTitle(id));
