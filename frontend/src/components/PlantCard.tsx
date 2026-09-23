import { Link } from 'react-router';
import { fmtDate, thumbClass } from '../format';
import type { Fertilizer, PlantSummary } from '../types';
import { Icon, Thumb } from './icons';
import { LightHint } from './LightHint';
import { PlantActions } from './PlantActions';
import { PlantStats } from './PlantStats';

export function PlantCard({
  s,
  fertilizers,
  onChanged,
}: {
  s: PlantSummary;
  fertilizers: Fertilizer[];
  onChanged: () => void;
}) {
  const { plant, repot } = s;
  const place = [plant.species, plant.location].filter(Boolean).join(' · ');

  return (
    <article className="plant-card" data-plant-id={plant.id}>
      <Link className="plant-card__head" to={`/plants/${plant.id}`}>
        <Thumb className={thumbClass(plant)} />
        <span>
          <h2 className="plant-card__name">{plant.name}</h2>
          {place && <span className="plant-card__place">{place}</span>}
        </span>
        <Icon name="chev" className="i chev" />
      </Link>

      <PlantStats s={s} />

      <LightHint light={s.light} />

      <p className="plant-card__foot" data-status={repot.status}>
        <Icon name="repot" />
        <span>
          {repot.last_at ? `Пересадка ${fmtDate(repot.last_at)}` : 'Пересадок не было'}
          {' · проверка '}
          <b>{repot.due_in_days <= 0 ? 'пора' : fmtDate(repot.next_check_date)}</b>
        </span>
      </p>

      <PlantActions s={s} fertilizers={fertilizers} onChanged={onChanged} />
    </article>
  );
}
