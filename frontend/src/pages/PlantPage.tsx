import { useState } from 'react';
import { Link, useParams } from 'react-router';
import { api } from '../api';
import { Sheet } from '../components/controls';
import { History } from '../components/History';
import { Icon, Thumb } from '../components/icons';
import { LightHint } from '../components/LightHint';
import { PlantActions } from '../components/PlantActions';
import { PlantStats } from '../components/PlantStats';
import { useToast } from '../components/toast';
import { WeekChart } from '../components/WeekChart';
import { ago, fmtDate, fmtHours, fmtNumber, plural, thumbClass } from '../format';
import type { PlantSummary, WeekStat } from '../types';
import { useAsync } from '../useAsync';

const WEEKS = 8;

/** Среднее за день по неделям, где есть данные о свете (город и лампа могли появиться недавно).
 *  Текущая неделя неполная — считаем только прошедшие дни. */
function avgLightPerDay(weeks: WeekStat[]): number {
  const todayIdx = (new Date().getDay() + 6) % 7;
  const withData = weeks.filter((w) => w.lamp_hours + w.sunshine_hours > 0);
  const days = withData.reduce((n, w) => n + (w.is_current ? todayIdx + 1 : 7), 0);
  return days ? withData.reduce((sum, w) => sum + w.lamp_hours + w.sunshine_hours, 0) / days : 0;
}

export function PlantPage() {
  const id = Number(useParams().id);
  const [version, setVersion] = useState(0);
  const [repotOpen, setRepotOpen] = useState(false);
  const { data, error } = useAsync(
    () => Promise.all([api.summary(id), api.weekly(id, WEEKS), api.fertilizers()]),
    [id, version],
  );
  const refresh = () => setVersion((v) => v + 1);

  if (error && !data) {
    return (
      <>
        <header className="page-head"><BackLink /></header>
        <p className="todo" data-status="late"><span className="todo__dot" />{error}</p>
      </>
    );
  }
  if (!data) return <header className="page-head"><BackLink /></header>;

  const [s, weeks, fertilizers] = data;
  const { plant } = s;
  const meta = [
    plant.species,
    plant.location,
    plant.pot_size_l != null ? `горшок ${fmtNumber(plant.pot_size_l)} л` : null,
  ].filter(Boolean).join(' · ');
  const totalW = weeks.reduce((n, w) => n + w.waterings, 0);
  const totalF = weeks.reduce((n, w) => n + w.feedings, 0);

  return (
    <>
      <header className="page-head">
        <BackLink />
        <div className="page-head__grow" />
        <Link className="icon-btn" to={`/settings?plant=${plant.id}`} aria-label="Настройки растения">
          <Icon name="settings" />
        </Link>
      </header>

      <div data-plant-id={plant.id}>
        <div className="hero">
          <Thumb className={thumbClass(plant)} large />
          <div>
            <h1 className="hero__name">{plant.name}</h1>
            <p className="hero__meta">
              {meta}{meta && '. '}
              {s.repot.last_at ? `Пересажен ${ago(s.repot.last_at)}. ` : ''}
              Проверка горшка: {s.repot.due_in_days <= 0 ? 'пора' : fmtDate(s.repot.next_check_date)}.
            </p>
          </div>
        </div>

        <PlantStats s={s} detailed />
        <div style={{ marginTop: 12 }}><LightHint light={s.light} /></div>
        <PlantActions s={s} fertilizers={fertilizers} onChanged={refresh} style={{ marginTop: 12 }} />
        <div className="btn-row" style={{ marginTop: 8 }}>
          <button className="btn btn--sm" type="button" onClick={() => setRepotOpen(true)}>
            <Icon name="repot" />Записать пересадку
          </button>
        </div>
        {plant.notes && <p className="notes">{plant.notes}</p>}
      </div>

      <div className="detail-layout">
        <section className="section">
          <h2 className="section__title">Активность за {WEEKS} недель</h2>
          <div className="panel chart">
            <div className="chart__summary">
              <div><b>{totalW}</b><span className="muted">{plural(totalW, ['полив', 'полива', 'поливов'])}</span></div>
              <div><b>{totalF}</b><span className="muted">{plural(totalF, ['подкормка', 'подкормки', 'подкормок'])}</span></div>
              <div><b>{fmtHours(avgLightPerDay(weeks))} ч</b><span className="muted">свет в день</span></div>
            </div>
            <WeekChart weeks={weeks} mode="events" />
            <div className="legend">
              <span style={{ '--c': 'var(--ev-water)' } as React.CSSProperties}>Полив</span>
              <span style={{ '--c': 'var(--ev-feed)' } as React.CSSProperties}>Подкормка</span>
            </div>
            <WeekChart weeks={weeks} mode="lamp" />
            <div className="legend">
              <span style={{ '--c': 'var(--ev-sun)' } as React.CSSProperties}>Солнце</span>
              <span style={{ '--c': 'var(--ev-lamp)' } as React.CSSProperties}>Лампа, часов за неделю</span>
            </div>
          </div>
        </section>

        <History plantId={plant.id} version={version} />
      </div>

      <Sheet open={repotOpen} onClose={() => setRepotOpen(false)}>
        <RepotForm s={s} onDone={() => { setRepotOpen(false); refresh(); }} onCancel={() => setRepotOpen(false)} />
      </Sheet>
    </>
  );
}

function BackLink() {
  return (
    <Link className="icon-btn" to="/" aria-label="Назад к растениям">
      <Icon name="back" />
    </Link>
  );
}

function RepotForm({ s, onDone, onCancel }: { s: PlantSummary; onDone: () => void; onCancel: () => void }) {
  const toast = useToast();
  const [size, setSize] = useState('');
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);

  return (
    <form
      style={{ display: 'grid', gap: 16 }}
      onSubmit={async (e) => {
        e.preventDefault();
        setBusy(true);
        try {
          const { id } = await api.repot(s.plant.id, {
            pot_size_after: size ? Number(size.replace(',', '.')) : null,
            note: note.trim() || null,
          });
          onDone();
          toast('Пересадка записана', async () => { await api.deleteRepotting(id); onDone(); });
        } catch {
          toast('Не удалось записать. Проверьте интернет и попробуйте ещё раз.');
        } finally {
          setBusy(false);
        }
      }}
    >
      <div>
        <h2 className="sheet__title">Пересадка: {s.plant.name}</h2>
        <p className="sheet__hint">
          {s.plant.pot_size_l != null ? `Сейчас горшок ${fmtNumber(s.plant.pot_size_l)} л` : 'Объём текущего горшка не указан'}
        </p>
      </div>
      <label>
        <span className="label">Новый горшок, л</span>
        <input className="input" inputMode="decimal" value={size} onChange={(e) => setSize(e.target.value)} placeholder="например, 5" />
      </label>
      <label>
        <span className="label">Заметка</span>
        <textarea className="textarea" value={note} onChange={(e) => setNote(e.target.value)} placeholder="Грунт, состояние корней…" />
      </label>
      <div className="btn-row">
        <button className="btn btn--primary" type="submit" disabled={busy} style={{ flex: 1 }}>Записать</button>
        <button className="btn" type="button" onClick={onCancel}>Отмена</button>
      </div>
    </form>
  );
}
