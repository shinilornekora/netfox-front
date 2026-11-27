import { useAppDispatch } from '@/shared/lib/redux';
import { Button } from '@/shared/ui';
import { autoRenewToggled } from '../model';

/** Включить или отключить автопродление. cancelAtPeriodEnd — продление сейчас отключено. */
export function AutoRenewButton({
  cancelAtPeriodEnd,
  pending,
}: {
  cancelAtPeriodEnd: boolean;
  pending: boolean;
}) {
  const dispatch = useAppDispatch();
  return (
    <Button disabled={pending} onClick={() => void dispatch(autoRenewToggled(cancelAtPeriodEnd))}>
      {cancelAtPeriodEnd ? 'Включить автопродление' : 'Отключить автопродление'}
    </Button>
  );
}
