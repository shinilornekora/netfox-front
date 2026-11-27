// Тарифы подписки и (для зрителя) текущая подписка из кошелька.
import { createAsyncThunk, createSlice } from '@reduxjs/toolkit';
import { api } from '@/shared/api/endpoints';
import type { Plan, Wallet } from '@/shared/api/types';
import { addRequestCases, createApiThunk, idle, type RequestState } from '@/shared/lib/request';
import type { RootState } from '@/shared/lib/redux';
import { selectRole } from '@/entities/session';

export interface PlansState {
  open: boolean;
  plans: RequestState<{ items: Plan[] }>;
  wallet: RequestState<Wallet>;
}

const initialState: PlansState = { open: false, plans: idle(), wallet: idle() };

export const plansRequested = createApiThunk('plans/plans', () => api.plans());
export const walletRequested = createApiThunk('plans/wallet', () => api.wallet());

export const plansSlice = createSlice({
  name: 'plans',
  initialState,
  reducers: {
    gateOpened: (s) => void (s.open = true),
    plansClosed: (s) => void (s.open = false),
  },
  extraReducers: (b) => {
    addRequestCases(b, plansRequested, (s) => s.plans);
    addRequestCases(b, walletRequested, (s) => s.wallet);
  },
});

export const { plansClosed } = plansSlice.actions;
const { gateOpened } = plansSlice.actions;

/** Кошелёк — только у зрителя и только пока страница открыта. */
export const walletRefreshed = createAsyncThunk(
  'plans/walletRefreshed',
  async (_: void, { dispatch, getState }) => {
    const s = getState() as RootState;
    if (s.plans.open && selectRole(s) === 'viewer') await dispatch(walletRequested());
  },
);

export const plansOpened = createAsyncThunk('plans/opened', async (_: void, { dispatch }) => {
  dispatch(gateOpened());
  await Promise.all([dispatch(plansRequested()), dispatch(walletRefreshed())]);
});

export const selectPlans = (s: RootState) => s.plans.plans.data;
export const selectPlansError = (s: RootState) => s.plans.plans.error;
export const selectSubscription = (s: RootState) => s.plans.wallet.data?.subscription ?? null;
