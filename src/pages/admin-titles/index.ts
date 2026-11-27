import { lazy } from 'react';

// Публичный API страницы: сам компонент (отдельный чанк) и то, что нужно app-слою.
export const AdminTitlesPage = lazy(
  () => import(/* webpackChunkName: "page-admin-titles" */ './ui/AdminTitlesPage'),
);
export { adminTitlesSlice } from './model';
