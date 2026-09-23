import { StrictMode, useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter, Navigate, Route, Routes } from 'react-router';
import { api, setUnauthorizedHandler, tokenStore } from './api';
import { IconSprite } from './components/icons';
import { Layout } from './components/Layout';
import { ToastProvider } from './components/toast';
import { MeContext } from './me';
import { AdminPage } from './pages/AdminPage';
import { Dashboard } from './pages/Dashboard';
import { Login } from './pages/Login';
import { NewPlant } from './pages/NewPlant';
import { PlantPage } from './pages/PlantPage';
import { SettingsPage } from './pages/SettingsPage';
import type { Me } from './types';
import './styles.css';

function App() {
  const [authed, setAuthed] = useState(() => tokenStore.get() !== null);
  const [me, setMe] = useState<Me | null>(null);
  useEffect(() => setUnauthorizedHandler(() => { setAuthed(false); setMe(null); }), []);
  useEffect(() => {
    if (authed) api.me().then(setMe).catch(() => { /* 401 обработает onUnauthorized */ });
  }, [authed]);

  if (!authed) return <Login onLogin={() => setAuthed(true)} />;
  if (!me) return null;

  const logout = () => { tokenStore.set(null); setMe(null); setAuthed(false); };

  return (
    <MeContext.Provider value={me}>
      <Routes>
        <Route element={<Layout />}>
          <Route index element={<Dashboard />} />
          <Route path="plants/new" element={<NewPlant />} />
          <Route path="plants/:id" element={<PlantPage />} />
          <Route path="settings" element={<SettingsPage onLogout={logout} />} />
          {me.is_admin && <Route path="admin" element={<AdminPage />} />}
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
    </MeContext.Provider>
  );
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <ToastProvider>
        <IconSprite />
        <App />
      </ToastProvider>
    </BrowserRouter>
  </StrictMode>,
);
