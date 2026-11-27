import { lazy } from 'react';

// Публичный API страницы: сам компонент (отдельный чанк) и то, что нужно app-слою.
export const WatchPage = lazy(() => import(/* webpackChunkName: "page-watch" */ './ui/WatchPage'));
export { watchSlice } from './model';
