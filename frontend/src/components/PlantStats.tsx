import { DAYS, fmtHours, plural } from '../format';
import type { PlantSummary, Status } from '../types';
import { Icon, type IconName } from './icons';

interface TileProps {
  icon: IconName;
  kind: 'water' | 'feed' | 'lamp';
  status: Status;
  value: string;
  unit: string;
  label: string;
  flag: string;
}

function Tile({ icon, kind, status, value, unit, label, flag }: TileProps) {
  return (
    <div className="stat" data-stat={kind} data-status={status}>
      <Icon name={icon} className="i stat__icon" />
      <span className="stat__value">
        <span>{value}</span>
        {unit && <small>{unit}</small>}
      </span>
      <span className="stat__label" title={label}>{label}</span>
      <span className="stat__flag">{flag}</span>
    </div>
  );
}

const FLAG: Record<Status, string> = { ok: '', soon: 'скоро', late: 'пора', off: '' };

/** Три плитки статуса. detailed — подписи с нормами для экрана растения. */
export function PlantStats({ s, detailed }: { s: PlantSummary; detailed?: boolean }) {
  const { water, feed, light } = s;

  // Счётчик сбрасывается и поливом, и проверкой грунта — показываем дни с последнего касания
  const touchDays = [water.days_since, water.days_since_check]
    .filter((x): x is number => x != null)
    .reduce<number | undefined>((min, v) => (min === undefined ? v : Math.min(min, v)), undefined);
  const soilDue = water.status === 'late' || water.status === 'soon';
  const soilLabel = soilDue
    ? 'Проверить грунт…'
    : touchDays === undefined
      ? 'проверено недавно'
      : `проверено ${touchDays} ${plural(touchDays, DAYS)} назад`;

  let feedValue = '—';
  let feedUnit = '';
  let feedLabel: string;
  if (!feed.enabled) {
    feedLabel = 'подкормка выкл.';
  } else if (!feed.next) {
    feedLabel = 'нет удобрений';
  } else if (feed.status === 'off') {
    feedLabel = `${feed.next.name}: не в сезон`;
  } else {
    feedValue = String(Math.max(0, feed.due_in_days ?? 0));
    feedUnit = 'д';
    feedLabel = detailed ? `до ${feed.next.name}, раз в ${feed.interval_days} д` : `до ${feed.next.name}`;
  }

  return (
    <div className={`stats${detailed ? ' stats--lg' : ''}`}>
      <Tile
        icon="water"
        kind="water"
        status={water.status}
        value={touchDays === undefined ? '—' : String(touchDays)}
        unit={touchDays === undefined ? '' : 'д'}
        label={
          detailed
            ? `${soilLabel}, норма ${water.interval_days} д`
            : soilLabel
        }
        flag={FLAG[water.status]}
      />
      <Tile icon="feed" kind="feed" status={feed.status} value={feedValue} unit={feedUnit} label={feedLabel} flag={FLAG[feed.status]} />
      <Tile
        icon="lamp"
        kind="lamp"
        status={light.status}
        value={fmtHours(light.total_hours)}
        unit="ч"
        label={
          detailed && light.natural_hours !== null
            ? `солнце ${fmtHours(light.natural_hours)} + лампа ${fmtHours(light.lamp_hours)} из ${fmtHours(light.target_hours)}`
            : `свет из ${fmtHours(light.target_hours)}`
        }
        flag={light.status === 'ok' ? '' : 'мало'}
      />
    </div>
  );
}
