import { lazy } from 'react';

// Публичный API страницы: сам компонент (отдельный чанк) и то, что нужно app-слою.
export const TitlePage = lazy(() => import(/* webpackChunkName: "page-title" */ './ui/TitlePage'));
export { titleSlice, offerRequested, viewerDataRequested, selectOpenTitleId } from './model';
