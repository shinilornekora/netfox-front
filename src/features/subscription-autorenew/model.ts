// Фича «Автопродление подписки». Кошелёк после переключения перечитывает
// кабинет — связь описана в app/listeners.
import { api } from '@/shared/api/endpoints';
import { createApiThunk } from '@/shared/lib/request';

export const autoRenewToggled = createApiThunk('subscription/autoRenew', (enabled: boolean) =>
  api.autoRenew(enabled),
);
