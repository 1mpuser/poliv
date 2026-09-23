import { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router';
import { api } from '../api';
import { Field, Segmented, Stepper } from '../components/controls';
import { AccountSection } from '../components/AccountSection';
import { FertilizerList } from '../components/FertilizerList';
import { PlantForm } from '../components/PlantForm';
import { useToast } from '../components/toast';
import { DAYS } from '../format';
import type { AppSettings, Plant, PlantFields } from '../types';
import { useAsync } from '../useAsync';

type Theme = 'system' | 'light' | 'dark';

function readTheme(): Theme {
  try { return (localStorage.getItem('theme') as Theme) || 'system'; } catch { return 'system'; }
}
function applyTheme(t: Theme) {
  if (t === 'system') delete document.documentElement.dataset.theme;
  else document.documentElement.dataset.theme = t;
  try { localStorage.setItem('theme', t); } catch { /* тема применится только до перезагрузки */ }
}

const fields = ({ id: _id, added_at: _a, ...rest }: Plant): PlantFields => rest;
const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

export function SettingsPage({ onLogout }: { onLogout: () => void }) {
  const toast = useToast();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const { data, reload } = useAsync(() => Promise.all([api.plants(), api.settings()]), []);

  // Черновики: сохраняются одной кнопкой «Сохранить», как в макете
  const [drafts, setDrafts] = useState<Record<number, PlantFields>>({});
  const [appDraft, setAppDraft] = useState<AppSettings | null>(null);
  const [theme, setTheme] = useState<Theme>(readTheme);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!data) return;
    const [plants, app] = data;
    setDrafts(Object.fromEntries(plants.map((p) => [p.id, fields(p)])));
    setAppDraft(app);
  }, [data]);

  if (!data || !appDraft) {
    return <header className="page-head"><div className="page-head__grow"><h1 className="page-head__title">Настройки</h1></div></header>;
  }

  const [plants, app] = data;
  const fromUrl = Number(params.get('plant'));
  const selectedId = plants.some((p) => p.id === fromUrl) ? fromUrl : plants[0]?.id;
  const selected = plants.find((p) => p.id === selectedId);
  const draft = selectedId != null ? drafts[selectedId] : undefined;

  const dirtyPlants = plants.filter((p) => drafts[p.id] && !same(drafts[p.id], fields(p)));
  const appDirty = !same(appDraft, app);

  async function save() {
    const invalid = dirtyPlants.find((p) => !drafts[p.id].name.trim());
    if (invalid) {
      toast('У растения должно быть название');
      return;
    }
    setBusy(true);
    try {
      await Promise.all([
        ...dirtyPlants.map((p) => api.updatePlant(p.id, drafts[p.id])),
        appDirty ? api.updateSettings(appDraft!) : null,
      ]);
      await reload();
      toast('Настройки сохранены');
    } catch (e) {
      toast(e instanceof Error ? e.message : 'Не удалось сохранить. Проверьте интернет и попробуйте ещё раз.');
    } finally {
      setBusy(false);
    }
  }

  async function removePlant(p: Plant) {
    if (!confirm(`Удалить «${p.name}» вместе со всей историей? Это нельзя отменить.`)) return;
    try {
      await api.deletePlant(p.id);
      setParams({}, { replace: true });
      await reload();
      toast(`${p.name} удалён(а)`);
    } catch {
      toast('Не удалось удалить');
    }
  }

  return (
    <>
      <header className="page-head">
        <div className="page-head__grow">
          <h1 className="page-head__title">Настройки</h1>
          <p className="page-head__sub">Сохраняются для всех устройств</p>
        </div>
      </header>

      <form noValidate onSubmit={(e) => { e.preventDefault(); save(); }}>
        <div className="settings-layout">
          <section className="section" style={{ marginTop: 0 }}>
            <h2 className="section__title">Растение</h2>
            {plants.length === 0 ? (
              <p className="empty">Растений пока нет</p>
            ) : (
              <>
                {plants.length <= 4 ? (
                  <div style={{ marginBottom: 12 }}>
                    <Segmented
                      label="Растение"
                      value={selectedId!}
                      onChange={(id) => setParams({ plant: String(id) }, { replace: true })}
                      options={plants.map((p) => ({ value: p.id, label: drafts[p.id]?.name || p.name }))}
                    />
                  </div>
                ) : (
                  <select
                    className="select"
                    style={{ marginBottom: 12 }}
                    aria-label="Растение"
                    value={selectedId}
                    onChange={(e) => setParams({ plant: e.target.value }, { replace: true })}
                  >
                    {plants.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
                  </select>
                )}
                {draft && (
                  <PlantForm
                    key={selectedId}
                    value={draft}
                    onChange={(v) => setDrafts((d) => ({ ...d, [selectedId!]: v }))}
                  />
                )}
                {selected && (
                  <div className="btn-row" style={{ marginTop: 12 }}>
                    <button className="btn btn--sm" type="button" onClick={() => navigate(`/plants/${selected.id}`)}>
                      История растения
                    </button>
                    <button className="btn btn--sm btn--danger" type="button" onClick={() => removePlant(selected)}>
                      Удалить растение
                    </button>
                  </div>
                )}
              </>
            )}
          </section>

          <section className="section">
            <h2 className="section__title">Сезон</h2>
            <Segmented
              label="Сезон"
              value={appDraft.current_season}
              onChange={(v) => setAppDraft({ ...appDraft, current_season: v })}
              options={[{ value: 'active', label: 'Активный рост' }, { value: 'dormant', label: 'Покой' }]}
            />
            <p className="field__hint" style={{ marginTop: 8 }}>
              Определяет, какой интервал подкормки брать у удобрения.
            </p>

            <h2 className="section__title" style={{ marginTop: 28 }}>Напоминания</h2>
            <div className="form-group">
              <Field label="Предупреждать заранее" hint="За столько дней статус станет жёлтым">
                {(id) => (
                  <Stepper
                    labelledBy={id}
                    min={0}
                    max={5}
                    units={DAYS}
                    value={appDraft.notify_days_ahead}
                    onChange={(v) => setAppDraft({ ...appDraft, notify_days_ahead: v })}
                  />
                )}
              </Field>
            </div>

            <h2 className="section__title" style={{ marginTop: 28 }}>Удобрения</h2>
            <FertilizerList />

            <h2 className="section__title" style={{ marginTop: 28 }}>Оформление</h2>
            {/* Тема применяется сразу и хранится локально на устройстве */}
            <Segmented
              label="Тема"
              value={theme}
              onChange={(t) => { setTheme(t); applyTheme(t); }}
              options={[
                { value: 'system', label: 'Как в системе' },
                { value: 'light', label: 'Светлая' },
                { value: 'dark', label: 'Тёмная' },
              ]}
            />

            <h2 className="section__title" style={{ marginTop: 28 }}>Аккаунт</h2>
            <AccountSection onLogout={onLogout} />
          </section>
        </div>

        <div className="save-bar">
          <button className="btn-primary" type="submit" disabled={busy || (dirtyPlants.length === 0 && !appDirty)}>
            {dirtyPlants.length === 0 && !appDirty ? 'Изменений нет' : 'Сохранить'}
          </button>
        </div>
      </form>
    </>
  );
}
