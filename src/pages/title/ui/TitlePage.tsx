import { useParams } from 'react-router-dom';
import { selectRole } from '@/entities/session';
import { actionsFor } from '@/entities/title';
import { EpisodeList } from '@/widgets/episode-list';
import { TitleHero } from '@/widgets/title-hero';
import type { IdParams } from '@/shared/constants';
import { usePageModel } from '@/shared/lib/hooks';
import { useAppSelector } from '@/shared/lib/redux';
import { Alert, Spinner } from '@/shared/ui';
import * as m from '../model';

/** Карточка тайтла: шапка с доступом и (у сериала) список серий. */
export default function TitlePage() {
  const { id = '' } = useParams<IdParams>();
  usePageModel(m.titleOpened, m.titleClosed, id);
  const view = useAppSelector(m.selectView);
  const role = useAppSelector(selectRole);
  const inWatchlist = useAppSelector(m.selectInWatchlist);
  const watchError = useAppSelector(m.selectWatchError);
  const { title: t, offer, progress } = view;

  if (view.error) return <Alert>{view.error}</Alert>;
  if (!t || t.id !== id) return <Spinner />;
  const canWatch = role === 'viewer' && actionsFor(t, offer).some((a) => a.type === 'watch');

  return (
    <div className="page stack">
      <TitleHero
        title={t}
        offer={offer}
        progress={progress}
        role={role}
        inWatchlist={inWatchlist}
        watchError={watchError}
      />
      {t.kind === 'series' && <EpisodeList title={t} canWatch={canWatch} />}
    </div>
  );
}
