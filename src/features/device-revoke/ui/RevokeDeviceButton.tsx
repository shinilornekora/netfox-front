import { useAppDispatch } from '@/shared/lib/redux';
import { Button } from '@/shared/ui';
import { deviceRevoked } from '../model';

export function RevokeDeviceButton({ id, current }: { id: string; current: boolean }) {
  const dispatch = useAppDispatch();
  return (
    <Button small variant="danger" onClick={() => void dispatch(deviceRevoked({ id, current }))}>
      {current ? 'Выйти' : 'Завершить сеанс'}
    </Button>
  );
}
