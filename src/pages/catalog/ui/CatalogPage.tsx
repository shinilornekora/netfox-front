import { selectRole } from '@/entities/session';
import { CatalogFilters } from '@/features/catalog-filters';
import { CatalogFeed } from '@/widgets/catalog-feed';
import { ContinueWatching } from '@/widgets/continue-watching';
import { usePageModel } from '@/shared/lib/hooks';
import { useAppDispatch, useAppSelector } from '@/shared/lib/redux';
import * as m from '../model';

/** Витрина: «Продолжить просмотр» (зрителю), фильтры и лента. */
export default function CatalogPage() {
  const dispatch = useAppDispatch();
  usePageModel(m.catalogOpened, m.catalogClosed);
  const view = useAppSelector(m.selectView);
  const genres = useAppSelector(m.selectGenres);
  const cont = useAppSelector(m.selectContinue);
  const role = useAppSelector(selectRole);

  return (
    <div className="page stack">
      <div>
        <h1>Кино, которое пережило век</h1>
        <p className="muted">Классика немого кино по подписке или навсегда — в один клик.</p>
      </div>

      {role === 'viewer' && <ContinueWatching items={cont} />}

      <CatalogFilters
        genres={genres}
        onSearch={() => void dispatch(m.searchSubmitted())}
        onReset={() => void dispatch(m.filtersReset())}
      />

      <CatalogFeed view={view} onMore={() => void dispatch(m.moreRequested())} />
    </div>
  );
}
