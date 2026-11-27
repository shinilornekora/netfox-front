// Фича «Смена пароля»: после неё остальные устройства выходят из аккаунта (это делает сервер).
import { api } from '@/shared/api/endpoints';
import { createApiThunk } from '@/shared/lib/request';

export const changePassword = createApiThunk(
  'profile/changePassword',
  (b: Parameters<typeof api.changePassword>[0]) => api.changePassword(b),
);
