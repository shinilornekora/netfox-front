// Личный кабинет зрителя: подписка и автопродление, покупки, заказы,
// история просмотров (keyset-пагинация по started_at + id) и устройства.
import { createAsyncThunk, createSlice, isAnyOf, type PayloadAction } from '@reduxjs/toolkit';
import { api } from '@/shared/api/endpoints';
import type { Device, HistoryPage, Wallet } from '@/shared/api/types';
import { addRequestCases, createApiThunk, idle, problemText, type RequestState } from '@/shared/lib/request';
import type { RootState } from '@/shared/lib/redux';
import { sessionLost } from '@/entities/session/model';

export type Tab = 'subscription' | 'purchases' | 'orders' | 'history' | 'devices';

export interface CabinetState {
  open: boolean;
  tab: Tab;
  wallet: RequestState<Wallet>;
  devices: RequestState<{ items: Device[] }>;
  history: HistoryPage['items'];
  historyNext: HistoryPage['next'];
  actionError: string | null;
  renewPending: boolean;
}

const initialState: CabinetState = {
  open: false,
  tab: 'subscription',
  wallet: idle(),
  devices: idle(),
  history: [],
  historyNext: {},
  actionError: null,
  renewPending: false,
};

export const walletRequested = createApiThunk('cabinet/wallet', () => api.wallet());
export const devicesRequested = createApiThunk('cabinet/devices', () => api.devices());
export const historyRequested = createApiThunk('cabinet/history', (c: HistoryPage['next']) => api.history(c));
export const autoRenewToggled = createApiThunk(
  'cabinet/autoRenew',
  async (enabled: boolean, { dispatch }) => {
    const r = await api.autoRenew(enabled);
    await dispatch(walletRefreshed());
    return r;
  },
);
export const deviceRevoked = createApiThunk(
  'cabinet/revoke',
  async (d: { id: string; current: boolean }, { dispatch }) => {
    await api.revokeDevice(d.id);
    // Завершили текущую сессию — это выход.
    if (d.current) dispatch(sessionLost());
    else await dispatch(devicesRequested());
    return d;
  },
);

export const cabinetSlice = createSlice({
  name: 'cabinet',
  initialState,
  reducers: {
    gateOpened: (s) => void (s.open = true),
    cabinetClosed(s) {
      s.open = false;
      s.history = [];
      s.historyNext = {};
    },
    tabChanged(s, a: PayloadAction<Tab>) {
      s.tab = a.payload;
      s.actionError = null;
    },
  },
  extraReducers: (b) => {
    b.addCase(historyRequested.fulfilled, (s, a) => {
      s.history = a.meta.arg.before ? [...s.history, ...a.payload.items] : a.payload.items;
      s.historyNext = a.payload.next;
    })
      .addCase(autoRenewToggled.pending, (s) => void (s.renewPending = true))
      .addMatcher(
        isAnyOf(autoRenewToggled.fulfilled, autoRenewToggled.rejected),
        (s) => void (s.renewPending = false),
      )
      .addMatcher(
        isAnyOf(autoRenewToggled.pending, deviceRevoked.pending),
        (s) => void (s.actionError = null),
      )
      .addMatcher(
        isAnyOf(autoRenewToggled.rejected, deviceRevoked.rejected),
        (s, a) => void (s.actionError = problemText(a)),
      );
    addRequestCases(b, walletRequested, (s) => s.wallet);
    addRequestCases(b, devicesRequested, (s) => s.devices);
  },
});

export const { tabChanged, cabinetClosed } = cabinetSlice.actions;
const { gateOpened } = cabinetSlice.actions;

/** Кошелёк перечитывается только пока кабинет открыт (после оплаты, смены автопродления). */
export const walletRefreshed = createAsyncThunk(
  'cabinet/walletRefreshed',
  async (_: void, { dispatch, getState }) => {
    if ((getState() as RootState).cabinet.open) await dispatch(walletRequested());
  },
);

export const cabinetOpened = createAsyncThunk('cabinet/opened', async (_: void, { dispatch }) => {
  dispatch(gateOpened());
  await Promise.all([
    dispatch(walletRequested()),
    dispatch(devicesRequested()),
    dispatch(historyRequested({})),
  ]);
});

export const historyMore = createAsyncThunk(
  'cabinet/historyMore',
  async (_: void, { dispatch, getState }) => {
    const next = (getState() as RootState).cabinet.historyNext;
    if (next.before) await dispatch(historyRequested(next));
  },
);

export const selectCabinet = (s: RootState) => s.cabinet;
