import { lazy } from 'react';

// Публичный API страницы: сам компонент (отдельный чанк) и то, что нужно app-слою.
export const CatalogPage = lazy(() => import(/* webpackChunkName: "page-catalog" */ './ui/CatalogPage'));
export { catalogSlice, continueRequested, selectCatalogOpen } from './model';
