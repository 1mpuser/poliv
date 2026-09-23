import { StrictMode, useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter, Navigate, Route, Routes } from 'react-router';
import { setUnauthorizedHandler, tokenStore } from './api';
import { IconSprite } from './components/icons';
import { Layout } from './components/Layout';
import { ToastProvider } from './components/toast';
import { Dashboard } from './pages/Dashboard';
import { Login } from './pages/Login';
import { NewPlant } from './pages/NewPlant';
import { PlantPage } from './pages/PlantPage';
import { SettingsPage } from './pages/SettingsPage';
import './styles.css';

function App() {
  const [authed, setAuthed] = useState(() => tokenStore.get() !== null);
  useEffect(() => setUnauthorizedHandler(() => setAuthed(false)), []);

  if (!authed) return <Login onLogin={() => setAuthed(true)} />;

  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<Dashboard />} />
        <Route path="plants/new" element={<NewPlant />} />
        <Route path="plants/:id" element={<PlantPage />} />
        <Route path="settings" element={<SettingsPage onLogout={() => { tokenStore.set(null); setAuthed(false); }} />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
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
