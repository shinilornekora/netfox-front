import { lazy } from 'react';

// Публичный API страницы: сам компонент (отдельный чанк) и то, что нужно app-слою.
export const WatchlistPage = lazy(
  () => import(/* webpackChunkName: "page-watchlist" */ './ui/WatchlistPage'),
);
export { watchlistSlice } from './model';
