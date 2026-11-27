import { lazy } from 'react';

// Публичный API страницы: сам компонент (отдельный чанк) и то, что нужно app-слою.
export const ProfilePage = lazy(() => import(/* webpackChunkName: "page-profile" */ './ui/ProfilePage'));
