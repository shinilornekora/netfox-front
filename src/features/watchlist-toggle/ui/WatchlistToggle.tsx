import { useAppDispatch } from '@/shared/lib/redux';
import { Button } from '@/shared/ui';
import { setWatch } from '../model';

/** Звёздочка на карточке тайтла: переключает «Смотреть позже». */
export function WatchlistToggle({ titleId, on }: { titleId: string; on: boolean }) {
  const dispatch = useAppDispatch();
  return (
    <Button aria-pressed={on} onClick={() => void dispatch(setWatch({ id: titleId, on: !on }))}>
      {on ? '★ В списке' : '☆ Смотреть позже'}
    </Button>
  );
}

/** «Убрать» в списке «Смотреть позже». */
export function WatchlistRemoveButton({ titleId }: { titleId: string }) {
  const dispatch = useAppDispatch();
  return (
    <Button small onClick={() => void dispatch(setWatch({ id: titleId, on: false }))}>
      Убрать
    </Button>
  );
}
