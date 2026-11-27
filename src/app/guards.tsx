import type { ReactNode } from 'react';
import { Navigate, Outlet, useLocation } from 'react-router-dom';
import { hasRole, selectRole, selectStatus } from '@/entities/session/model';
import { useAppSelector } from '@/shared/lib/redux';
import type { Role } from '@/shared/api/types';
import { Spinner } from '@/shared/ui';
import { RoutePath } from '@/shared/constants';

/** Пока сессия восстанавливается — показываем загрузку, а не мигаем редиректом. */
export function RequireAuth() {
  const status = useAppSelector(selectStatus);
  const location = useLocation();
  if (status === 'unknown') return <Spinner />;
  if (status === 'anonymous')
    return <Navigate to={RoutePath.login} replace state={{ from: location.pathname }} />;
  return <Outlet />;
}

export function RequireRole({ roles }: { roles: readonly Role[] }) {
  const role = useAppSelector(selectRole);
  if (!hasRole(role, roles)) return <Navigate to={RoutePath.forbidden} replace />;
  return <Outlet />;
}

export function GuestOnly({ children }: { children: ReactNode }) {
  const status = useAppSelector(selectStatus);
  const location = useLocation();
  const from = (location.state as { from?: string } | null)?.from ?? RoutePath.catalog;
  if (status === 'authenticated') return <Navigate to={from} replace />;
  return <>{children}</>;
}
