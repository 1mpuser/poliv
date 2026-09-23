import { useState } from 'react';
import { api } from '../api';
import { useToast } from '../components/toast';
import { fmtDate } from '../format';
import { useMe } from '../me';
import { generatePassword } from '../password';
import type { AdminUser } from '../types';
import { useAsync } from '../useAsync';

interface Issued {
  title: string;
  email: string;
  password: string;
}

/** Выданные почта и пароль — показываются один раз, до закрытия */
function IssuedCredentials({ issued, onClose }: { issued: Issued; onClose: () => void }) {
  const toast = useToast();
  const text = `Поливалка: ${location.origin}\nПочта: ${issued.email}\nПароль: ${issued.password}`;
  return (
    <div className="creds" role="status">
      <b>{issued.title}</b>
      <span>Почта: <code>{issued.email}</code></span>
      <span>Пароль: <code>{issued.password}</code></span>
      <span className="creds__hint">Пароль больше не покажется — скопируйте и передайте его сейчас.</span>
      <div className="btn-row">
        <button
          className="btn btn--sm btn--primary"
          type="button"
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(text);
              toast('Скопировано');
            } catch {
              toast('Не удалось скопировать — выделите текст вручную');
            }
          }}
        >
          Скопировать
        </button>
        <button className="btn btn--sm" type="button" onClick={onClose}>Готово</button>
      </div>
    </div>
  );
}

export function AdminPage() {
  const me = useMe();
  const toast = useToast();
  const { data: users, reload } = useAsync(() => api.adminUsers(), []);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState(generatePassword);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [issued, setIssued] = useState<Issued | null>(null);

  async function create() {
    setError(null);
    if (!email.includes('@')) return setError('Укажите почту');
    if (password.length < 8) return setError('Пароль — не короче 8 символов');
    setBusy(true);
    try {
      const user = await api.adminCreateUser(email.trim(), password);
      setIssued({ title: 'Учётка создана', email: user.email, password });
      setEmail('');
      setPassword(generatePassword());
      reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не удалось создать учётку');
    } finally {
      setBusy(false);
    }
  }

  async function act(action: () => Promise<unknown>, done: string) {
    try {
      await action();
      toast(done);
      reload();
    } catch (e) {
      toast(e instanceof Error ? e.message : 'Не удалось выполнить');
    }
  }

  async function resetPassword(u: AdminUser) {
    if (!confirm(`Выдать новый пароль для ${u.email}? Старый перестанет работать на всех устройствах.`)) return;
    const next = generatePassword();
    try {
      await api.adminSetPassword(u.id, next);
      setIssued({ title: 'Новый пароль', email: u.email, password: next });
    } catch (e) {
      toast(e instanceof Error ? e.message : 'Не удалось сменить пароль');
    }
  }

  function remove(u: AdminUser) {
    const typed = prompt(`Учётка удалится вместе со всеми растениями и историей.\nДля подтверждения введите почту: ${u.email}`);
    if (typed === null) return;
    if (typed.trim().toLowerCase() !== u.email) return toast('Почта не совпала — удаление отменено');
    act(() => api.adminDelete(u.id), `${u.email} удалена`);
  }

  return (
    <>
      <header className="page-head">
        <div className="page-head__grow">
          <h1 className="page-head__title">Админка</h1>
          <p className="page-head__sub">Учётки выдаются только здесь, у каждой свои растения</p>
        </div>
      </header>

      {issued && <IssuedCredentials issued={issued} onClose={() => setIssued(null)} />}

      <div className="settings-layout">
        <section className="section" style={{ marginTop: 0 }}>
          <h2 className="section__title">Новая учётка</h2>
          <form
            className="form-group"
            noValidate
            onSubmit={(e) => { e.preventDefault(); create(); }}
          >
            <div className="field field--stack">
              <label className="field__label" htmlFor="new-email">Почта — это логин</label>
              <input id="new-email" className="input" type="email" inputMode="email" autoComplete="off"
                value={email} onChange={(e) => setEmail(e.target.value)} placeholder="friend@example.com" />
            </div>
            <div className="field field--stack">
              <label className="field__label" htmlFor="new-password">Пароль</label>
              <div className="input-row">
                <input id="new-password" className="input" autoComplete="off" spellCheck={false}
                  value={password} onChange={(e) => setPassword(e.target.value)} />
                <button className="btn btn--sm" type="button" onClick={() => setPassword(generatePassword())}>
                  Сгенерировать
                </button>
              </div>
            </div>
            {error && <p className="form-error" role="alert">{error}</p>}
            <div className="field">
              <button className="btn btn--primary" type="submit" disabled={busy} style={{ width: '100%' }}>
                Создать учётку
              </button>
            </div>
          </form>
        </section>

        <section className="section">
          <h2 className="section__title">Учётки{users ? ` · ${users.length}` : ''}</h2>
          <div className="form-group">
            {users?.map((u) => {
              const self = u.id === me.id;
              return (
                <div className="user-row" key={u.id}>
                  <div className="user-row__text">
                    <div className="user-row__email">{u.email}</div>
                    <div className="user-row__meta">
                      <span>с {fmtDate(u.created_at)}</span>
                      {u.is_admin && <span className="badge">админ</span>}
                      {self && <span className="badge">это вы</span>}
                      {u.blocked_at && <span className="badge badge--late">заблокирована</span>}
                    </div>
                  </div>
                  <div className="btn-row">
                    {self ? (
                      <span className="user-row__meta">Свой пароль — в «Настройках»</span>
                    ) : (
                      <button className="btn btn--sm" type="button" onClick={() => resetPassword(u)}>Новый пароль</button>
                    )}
                    {!self && (u.blocked_at ? (
                      <button className="btn btn--sm" type="button" onClick={() => act(() => api.adminUnblock(u.id), 'Учётка разблокирована')}>
                        Разблокировать
                      </button>
                    ) : (
                      <button className="btn btn--sm" type="button" onClick={() => act(() => api.adminBlock(u.id), 'Учётка заблокирована')}>
                        Заблокировать
                      </button>
                    ))}
                    {!self && (
                      <button className="btn btn--sm btn--danger" type="button" onClick={() => remove(u)}>Удалить</button>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        </section>
      </div>
    </>
  );
}
