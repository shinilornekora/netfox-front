import { lazy } from 'react';

export const LoginPage = lazy(() => import(/* webpackChunkName: "page-login" */ './ui/LoginPage'));
