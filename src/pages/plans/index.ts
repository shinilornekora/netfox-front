import { lazy } from 'react';

// Публичный API страницы: сам компонент (отдельный чанк) и то, что нужно app-слою.
export const PlansPage = lazy(() => import(/* webpackChunkName: "page-plans" */ './ui/PlansPage'));
export { plansSlice, walletRefreshed as plansWalletRefreshed } from './model';
