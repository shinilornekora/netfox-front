import { Suspense, useContext, useEffect, useState } from 'react';
import { Outlet, useLocation } from 'react-router-dom';
import { CheckoutModal } from '@/features/checkout';
import { Sidebar, Topbar } from '@/widgets/sidebar';
import { Spinner } from '@/shared/ui';
import { SidebarSlot } from './sidebarSlot';

/**
 * Каркас: боковое меню слева (на десктопе всегда открыто), на телефоне и планшете —
 * верхняя полоска с кнопкой, по которой меню выезжает поверх контента.
 * Окно оплаты рисуется один раз здесь: открыть его может любая страница.
 */
export function Layout() {
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
      <Topbar open={open} onOpen={() => setOpen(true)} />
      <Sidebar open={open} onClose={() => setOpen(false)} extra={extra} />
      <main className="main">
        <Suspense fallback={<Spinner />}>
          <Outlet />
        </Suspense>
      </main>
      <CheckoutModal />
    </div>
  );
}
