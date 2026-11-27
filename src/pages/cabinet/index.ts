import { lazy } from 'react';

// Публичный API страницы: сам компонент (отдельный чанк) и то, что нужно app-слою.
export const CabinetPage = lazy(() => import(/* webpackChunkName: "page-cabinet" */ './ui/CabinetPage'));
export { cabinetSlice, walletRefreshed as cabinetWalletRefreshed, devicesRequested } from './model';
