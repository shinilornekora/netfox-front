// Фича «Редактирование профиля»: новый профиль (с новой ревизией) сразу попадает в сессию.
import { api } from '@/shared/api/endpoints';
import { createApiThunk } from '@/shared/lib/request';
import { userUpdated } from '@/entities/session';

export const updateMe = createApiThunk(
  'profile/updateMe',
  async (b: Parameters<typeof api.updateMe>[0], { dispatch }) => {
    const u = await api.updateMe(b);
    dispatch(userUpdated(u));
    return u;
  },
);
