// Продажи (админ): выручка, заказы с фильтром по статусу и возвраты.
import { createAsyncThunk, createSlice } from '@reduxjs/toolkit';
import { api } from '@/shared/api/endpoints';
import type { Order, OrderStatus, Plan, Revenue } from '@/shared/api/types';
import { addRequestCases, createApiThunk, idle, problemText, type RequestState } from '@/shared/lib/request';
import type { RootState } from '@/shared/lib/redux';
import { refunded } from '@/features/order-refund';

type OrdersPage = Awaited<ReturnType<typeof api.adminOrders>>;

export interface SalesState {
  status: OrderStatus | '';
  plans: RequestState<{ items: Plan[] }>;
  revenue: RequestState<Revenue>;
  orders: RequestState<OrdersPage>;
  items: Order[];
  nextBefore: string;
  refundError: string | null;
}

const initialState: SalesState = {
  status: '',
  plans: idle(),
  revenue: idle(),
  orders: idle(),
  items: [],
  nextBefore: '',
  refundError: null,
};

export const plansRequested = createApiThunk('sales/plans', () => api.plans());
export const revenueRequested = createApiThunk('sales/revenue', () => api.revenue());
export const ordersRequested = createApiThunk('sales/orders', (_: void, { getState }) =>
  api.adminOrders({ status: (getState() as RootState).sales.status }),
);
const nextPage = createApiThunk('sales/more', (q: { status: string; before: string }) => api.adminOrders(q));

export const salesSlice = createSlice({
  name: 'sales',
  initialState,
  reducers: {
    statusSet(s, a: { payload: OrderStatus | '' }) {
      s.status = a.payload;
    },
  },
  extraReducers: (b) => {
    b.addCase(ordersRequested.fulfilled, (s, a) => {
      s.items = a.payload.items;
      s.nextBefore = a.payload.next_before;
    })
      .addCase(nextPage.fulfilled, (s, a) => {
        s.items.push(...a.payload.items);
        s.nextBefore = a.payload.next_before;
      })
      .addCase(refunded.pending, (s) => void (s.refundError = null))
      .addCase(refunded.rejected, (s, a) => void (s.refundError = problemText(a)));
    addRequestCases(b, plansRequested, (s) => s.plans);
    addRequestCases(b, revenueRequested, (s) => s.revenue);
    addRequestCases(b, ordersRequested, (s) => s.orders);
  },
});

const { statusSet } = salesSlice.actions;

export const salesOpened = createAsyncThunk('sales/opened', async (_: void, { dispatch }) => {
  await Promise.all([dispatch(revenueRequested()), dispatch(plansRequested()), dispatch(ordersRequested())]);
});

export const statusChanged = createAsyncThunk(
  'sales/statusChanged',
  async (status: OrderStatus | '', { dispatch }) => {
    dispatch(statusSet(status));
    await dispatch(ordersRequested());
  },
);

/** После возврата (фича order-refund) — перечитать выручку и заказы. */
export const salesRefreshed = createAsyncThunk('sales/refreshed', async (_: void, { dispatch }) => {
  await Promise.all([dispatch(revenueRequested()), dispatch(ordersRequested())]);
});

export const moreRequested = createAsyncThunk(
  'sales/moreRequested',
  async (_: void, { dispatch, getState }) => {
    const { status, nextBefore } = (getState() as RootState).sales;
    if (nextBefore !== '') await dispatch(nextPage({ status, before: nextBefore }));
  },
);

export const selectSales = (s: RootState) => s.sales;
