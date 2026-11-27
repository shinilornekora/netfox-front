import { Suspense, useContext, useEffect, useState } from 'react';
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { logout, selectRole, selectUser } from '@/entities/session/model';
import { useAppDispatch, useAppSelector } from '@/shared/lib/redux';
import { CheckoutModal } from '@/features/checkout/CheckoutModal';
import { roleLabel } from '@/shared/lib/format';
import { Button, Spinner } from '@/shared/ui';
import { SidebarSlot } from './sidebarSlot';
import { RoutePath } from '@/shared/constants';

function Logo() {
  return (
    <>
      <span aria-hidden>🦊</span> Net<b>fox</b>
    </>
  );
}

/**
 * Каркас: боковое меню слева (на десктопе всегда открыто), на телефоне и планшете —
 * верхняя полоска с кнопкой, по которой меню выезжает поверх контента.
 */
export function Layout() {
  const user = useAppSelector(selectUser);
  const role = useAppSelector(selectRole);
  const dispatch = useAppDispatch();
  const navigate = useNavigate();
  const location = useLocation();
  const extra = useContext(SidebarSlot);
  const [open, setOpen] = useState(false);

  // Меню закрывается при переходе на другую страницу и по Esc.
  useEffect(() => setOpen(false), [location.pathname]);
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false);
    document.addEventListener('keydown', onKey);
    document.body.classList.add('no-scroll');
    return () => {
      document.removeEventListener('keydown', onKey);
      document.body.classList.remove('no-scroll');
    };
  }, [open]);

  return (
    <div className="shell">
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
          onClick={() => setOpen(true)}
        >
          <span aria-hidden>☰</span>
        </button>
      </header>

      {open && <div className="sidebar-backdrop" onClick={() => setOpen(false)} aria-hidden />}

      <aside id="sidebar" className={`sidebar${open ? ' open' : ''}`}>
        <div className="sidebar-head">
          <NavLink to={RoutePath.catalog} className="logo" aria-label="Netfox — на главную">
            <Logo />
          </NavLink>
          <button
            type="button"
            className="neu-btn small sidebar-close"
            aria-label="Закрыть меню"
            onClick={() => setOpen(false)}
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
              <Button
                small
                onClick={() => {
                  void dispatch(logout()).finally(() => navigate(RoutePath.catalog));
                }}
              >
                Выйти
              </Button>
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

      <main className="main">
        <Suspense fallback={<Spinner />}>
          <Outlet />
        </Suspense>
      </main>
      <CheckoutModal />
    </div>
  );
}
