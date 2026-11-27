import type { ReactNode } from 'react';
import { NavLink } from 'react-router-dom';
import { selectRole, selectUser } from '@/entities/session';
import { LogoutButton } from '@/features/auth';
import { RoutePath } from '@/shared/constants';
import { roleLabel } from '@/shared/lib/format';
import { useAppSelector } from '@/shared/lib/redux';

function Logo() {
  return (
    <>
      <span aria-hidden>🦊</span> Net<b>fox</b>
    </>
  );
}

/** Верхняя полоска на телефоне и планшете: логотип и кнопка меню. */
export function Topbar({ open, onOpen }: { open: boolean; onOpen: () => void }) {
  return (
    <header className="topbar">
      <NavLink to={RoutePath.catalog} className="logo" aria-hidden tabIndex={-1}>
        <Logo />
      </NavLink>
      <button
        type="button"
        className="neu-btn small burger"
        aria-label="Открыть меню"
        aria-expanded={open}
        aria-controls="sidebar"
        onClick={onOpen}
      >
        <span aria-hidden>☰</span>
      </button>
    </header>
  );
}

/**
 * Боковое меню: навигация по роли, текущий пользователь и выход.
 * extra — дополнительный блок внизу (в демо-режиме — демо-входы).
 */
export function Sidebar({ open, onClose, extra }: { open: boolean; onClose: () => void; extra?: ReactNode }) {
  const user = useAppSelector(selectUser);
  const role = useAppSelector(selectRole);
  return (
    <>
      {open && <div className="sidebar-backdrop" onClick={onClose} aria-hidden />}

      <aside id="sidebar" className={`sidebar${open ? ' open' : ''}`}>
        <div className="sidebar-head">
          <NavLink to={RoutePath.catalog} className="logo" aria-label="Netfox — на главную">
            <Logo />
          </NavLink>
          <button
            type="button"
            className="neu-btn small sidebar-close"
            aria-label="Закрыть меню"
            onClick={onClose}
          >
            ✕
          </button>
        </div>

        <nav className="nav" aria-label="Основная навигация">
          <NavLink to={RoutePath.catalog} end>
            Каталог
          </NavLink>
          <NavLink to={RoutePath.plans}>Подписка</NavLink>
          {role === 'viewer' && (
            <>
              <NavLink to={RoutePath.watchlist}>Смотреть позже</NavLink>
              <NavLink to={RoutePath.cabinet} end>
                Кабинет
              </NavLink>
            </>
          )}
          {role === 'admin' && (
            <>
              <NavLink to={RoutePath.adminTitles}>Управление каталогом</NavLink>
              <NavLink to={RoutePath.sales}>Продажи</NavLink>
            </>
          )}
        </nav>

        <div className="spacer" />

        {user ? (
          <div className="sidebar-user neu-inset">
            <NavLink to={RoutePath.profile} className="user-name" title={roleLabel[user.role]}>
              {user.display_name || user.email}
            </NavLink>
            <div className="muted user-role">{roleLabel[user.role]}</div>
            <div className="row" style={{ gap: 8 }}>
              <LogoutButton />
            </div>
          </div>
        ) : (
          <div className="row" style={{ gap: 8 }}>
            <NavLink to={RoutePath.login} className="neu-btn small">
              Войти
            </NavLink>
            <NavLink to={RoutePath.register} className="neu-btn small primary">
              Регистрация
            </NavLink>
          </div>
        )}

        {extra}
      </aside>
    </>
  );
}
