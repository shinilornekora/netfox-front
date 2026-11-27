import { lazy } from 'react';

// Публичный API страницы: сам компонент (отдельный чанк) и то, что нужно app-слою.
export const TitleEditPage = lazy(
  () => import(/* webpackChunkName: "page-title-edit" */ './ui/TitleEditPage'),
);
export { titleEditSlice, titleReloaded } from './model';
