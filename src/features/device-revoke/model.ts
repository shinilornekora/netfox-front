// Фича «Завершить сеанс на устройстве». Своё устройство — это выход (sessionLost);
// чужое — список устройств перечитывает кабинет (app/listeners).
import { api } from '@/shared/api/endpoints';
import { createApiThunk } from '@/shared/lib/request';
import { sessionLost } from '@/entities/session';

export const deviceRevoked = createApiThunk(
  'devices/revoke',
  async (d: { id: string; current: boolean }, { dispatch }) => {
    await api.revokeDevice(d.id);
    if (d.current) dispatch(sessionLost());
    return d;
  },
);
