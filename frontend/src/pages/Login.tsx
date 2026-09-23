import { useState } from 'react';
import { api } from '../api';

export function Login({ onLogin }: { onLogin: () => void }) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  return (
    <div className="login">
      <form
        className="login__card"
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          setError(null);
          try {
            await api.login(email.trim(), password);
            onLogin();
          } catch (err) {
            setError(err instanceof Error ? err.message : 'Не удалось войти');
          } finally {
            setBusy(false);
          }
        }}
      >
        <img className="logo" src="/icon.svg" alt="" />
        <h1>Мои растения</h1>
        <input
          className="input"
          name="email"
          type="email"
          inputMode="email"
          autoComplete="username"
          placeholder="Почта"
          aria-label="Почта"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          required
        />
        <input
          className="input"
          name="password"
          type="password"
          autoComplete="current-password"
          placeholder="Пароль"
          aria-label="Пароль"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          required
        />
        {error && <p className="form-error" role="alert">{error}</p>}
        <button className="btn-primary" type="submit" disabled={busy}>
          {busy ? 'Входим…' : 'Войти'}
        </button>
      </form>
    </div>
  );
}
