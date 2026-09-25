import { useState, type CSSProperties } from 'react';
import { api } from '../api';
import type { FeedMethod, Fertilizer, PlantSummary } from '../types';
import { Segmented, Sheet } from './controls';
import { Icon } from './icons';
import { useToast } from './toast';

interface Props {
  s: PlantSummary;
  fertilizers: Fertilizer[];
  /** Перезапросить сводку после действия или отмены */
  onChanged: () => void;
  style?: CSSProperties;
}

const NETWORK_ERROR = 'Не удалось записать. Проверьте интернет и нажмите ещё раз.';

/** «Полить / Подкормить / Лампа» + тост с отменой (поведение из макета) */
export function PlantActions({ s, fertilizers, onChanged, style }: Props) {
  const toast = useToast();
  const [busy, setBusy] = useState<string | null>(null);
  const [done, setDone] = useState<string | null>(null);
  const [feedOpen, setFeedOpen] = useState(false);
  const plantId = s.plant.id;

  async function run(key: string, action: () => Promise<{ message: string; undo: () => Promise<void> }>) {
    setBusy(key);
    setDone(key);
    window.setTimeout(() => setDone((d) => (d === key ? null : d)), 1200);
    navigator.vibrate?.(15);
    try {
      const { message, undo } = await action();
      onChanged();
      toast(message, async () => {
        try { await undo(); } catch { toast('Не удалось отменить'); }
        onChanged();
      });
    } catch {
      toast(NETWORK_ERROR);
    } finally {
      setBusy(null);
    }
  }

  const water = () =>
    run('water', async () => {
      const { id } = await api.water(plantId);
      return { message: 'Полив записан', undo: () => api.deleteWatering(id) };
    });

  const lampBrief = s.light.lamp;

  const lamp = () =>
    run('lamp', async () => {
      const { is_on, session, previous_ended_at, plug_error } = await api.toggleLamp(plantId);
      const base = `${lampBrief?.name ?? 'Лампа'}: ${is_on ? 'включена' : 'выключена'}`;
      const message = plug_error ? `${base}, но розетка не ответила: ${plug_error}` : base;
      return is_on
        ? { message, undo: () => api.deleteLampSession(session.id) }
        : { message, undo: async () => { await api.restoreLampEnd(session.id, previous_ended_at); } };
    });

  const feed = (fertilizerId: number, method: FeedMethod) => {
    setFeedOpen(false);
    run('feed', async () => {
      const { id } = await api.feed(plantId, fertilizerId, method);
      return { message: 'Подкормка записана', undo: () => api.deleteFeeding(id) };
    });
  };

  const cls = (key: string) => `action${done === key ? ' is-done' : ''}`;

  return (
    <>
      <div className="actions" style={style}>
        <button className={cls('water')} type="button" data-primary disabled={busy === 'water'} onClick={water}>
          <Icon name="water" />Полить
        </button>
        <button className={cls('feed')} type="button" disabled={busy === 'feed'} onClick={() => setFeedOpen(true)}>
          <Icon name="feed" />Подкормить
        </button>
        <button
          className={cls('lamp')}
          type="button"
          aria-pressed={s.lamp.is_on}
          disabled={busy === 'lamp' || !lampBrief}
          title={lampBrief ? undefined : 'Привяжите лампу в настройках'}
          onClick={lamp}
        >
          <Icon name="lamp" />
          <span>{s.lamp.is_on ? 'Лампа горит' : 'Лампа'}</span>
        </button>
      </div>
      <Sheet open={feedOpen} onClose={() => setFeedOpen(false)}>
        <FeedForm s={s} fertilizers={fertilizers} onSubmit={feed} onCancel={() => setFeedOpen(false)} />
      </Sheet>
    </>
  );
}

function FeedForm({
  s,
  fertilizers,
  onSubmit,
  onCancel,
}: {
  s: PlantSummary;
  fertilizers: Fertilizer[];
  onSubmit: (fertilizerId: number, method: FeedMethod) => void;
  onCancel: () => void;
}) {
  // Предзаполнено предложенным следующим типом
  const [fertId, setFertId] = useState<number | undefined>(s.feed.next?.id ?? fertilizers[0]?.id);
  const [method, setMethod] = useState<FeedMethod>('root');
  const fert = fertilizers.find((f) => f.id === fertId);
  const dose = fert ? (method === 'root' ? fert.root_dose_ml_per_l : fert.foliar_dose_ml_per_l) : null;

  if (fertilizers.length === 0) {
    return (
      <>
        <h2 className="sheet__title">Подкормка</h2>
        <p className="sheet__hint">Сначала добавьте удобрение в настройках.</p>
        <button className="btn" type="button" onClick={onCancel}>Закрыть</button>
      </>
    );
  }

  return (
    <form
      style={{ display: 'grid', gap: 16 }}
      onSubmit={(e) => { e.preventDefault(); if (fertId) onSubmit(fertId, method); }}
    >
      <div>
        <h2 className="sheet__title">Подкормка: {s.plant.name}</h2>
        {s.feed.next && <p className="sheet__hint">По очереди следующее: {s.feed.next.name}</p>}
      </div>
      <label>
        <span className="label">Удобрение</span>
        <select className="select" value={fertId} onChange={(e) => setFertId(Number(e.target.value))}>
          {fertilizers.map((f) => (
            <option key={f.id} value={f.id}>
              {f.name}{f.npk ? ` (NPK ${f.npk})` : ''}{f.id === s.feed.next?.id ? ' — по очереди' : ''}
            </option>
          ))}
        </select>
      </label>
      <div>
        <span className="label">Способ</span>
        <Segmented
          label="Способ подкормки"
          value={method}
          onChange={setMethod}
          options={[{ value: 'root', label: 'Под корень' }, { value: 'foliar', label: 'По листу' }]}
        />
      </div>
      <p className="sheet__hint">
        {dose != null ? `Доза: ${dose} мл на 1 л воды` : 'Доза не указана — заполните её в настройках удобрения'}
      </p>
      <div className="btn-row">
        <button className="btn btn--primary" type="submit" style={{ flex: 1 }}>Записать подкормку</button>
        <button className="btn" type="button" onClick={onCancel}>Отмена</button>
      </div>
    </form>
  );
}
