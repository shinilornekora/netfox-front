// Фича «Возврат денег» (админ). После возврата продажи перечитывают выручку и заказы (app/listeners).
import { api } from '@/shared/api/endpoints';
import { createApiThunk } from '@/shared/lib/request';

export const refunded = createApiThunk('orders/refund', ({ id, reason }: { id: string; reason: string }) =>
  api.refundOrder(id, reason),
);
