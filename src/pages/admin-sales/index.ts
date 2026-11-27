import { lazy } from 'react';

// Публичный API страницы: сам компонент (отдельный чанк) и то, что нужно app-слою.
export const SalesPage = lazy(() => import(/* webpackChunkName: "page-admin-sales" */ './ui/SalesPage'));
export { salesSlice, salesRefreshed } from './model';
