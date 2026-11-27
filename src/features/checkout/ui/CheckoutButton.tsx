import type { ReactNode } from 'react';
import { useAppDispatch } from '@/shared/lib/redux';
import { Button } from '@/shared/ui';
import { opened, type CheckoutTarget } from '../model';

/** Кнопка «Оформить» / «Купить»: открывает окно оплаты для выбранной цели. */
export function CheckoutButton({
  target,
  children,
  variant = 'primary',
  disabled,
  'aria-label': ariaLabel,
}: {
  target: CheckoutTarget;
  children: ReactNode;
  variant?: 'primary' | 'default';
  disabled?: boolean;
  'aria-label'?: string;
}) {
  const dispatch = useAppDispatch();
  return (
    <Button
      variant={variant}
      disabled={disabled}
      aria-label={ariaLabel}
      onClick={() => dispatch(opened(target))}
    >
      {children}
    </Button>
  );
}
