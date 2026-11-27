import { api } from '@/shared/api/endpoints';
import { createApiThunk } from '@/shared/lib/request';
import { userUpdated } from '@/entities/session/model';

/** Сохранить профиль; новый профиль (с новой ревизией) сразу попадает в сессию. */
export const updateMe = createApiThunk(
  'profile/updateMe',
  async (b: Parameters<typeof api.updateMe>[0], { dispatch }) => {
    const u = await api.updateMe(b);
    dispatch(userUpdated(u));
    return u;
  },
);
export const changePassword = createApiThunk(
  'profile/changePassword',
  (b: Parameters<typeof api.changePassword>[0]) => api.changePassword(b),
);
