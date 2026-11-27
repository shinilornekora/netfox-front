import { useNavigate } from 'react-router-dom';
import { logout } from '@/entities/session';
import { useAppDispatch } from '@/shared/lib/redux';
import { RoutePath } from '@/shared/constants';
import { Button } from '@/shared/ui';

/** «Выйти»: завершает сессию и возвращает на витрину. */
export function LogoutButton() {
  const dispatch = useAppDispatch();
  const navigate = useNavigate();
  return (
    <Button
      small
      onClick={() => {
        void dispatch(logout()).finally(() => navigate(RoutePath.catalog));
      }}
    >
      Выйти
    </Button>
  );
}
