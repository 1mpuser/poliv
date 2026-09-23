import type {
  AppSettings,
  EventType,
  FeedMethod,
  Fertilizer,
  FertilizerFields,
  HistoryEvent,
  LampSession,
  Plant,
  PlantFields,
  PlantSummary,
  WeekStat,
} from './types';

const TOKEN_KEY = 'token';

export const tokenStore = {
  get(): string | null {
    try { return localStorage.getItem(TOKEN_KEY); } catch { return null; }
  },
  set(token: string | null) {
    try {
      if (token) localStorage.setItem(TOKEN_KEY, token);
      else localStorage.removeItem(TOKEN_KEY);
    } catch { /* приватный режим — токен живёт до перезагрузки */ }
  },
};

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

/** Вызывается при 401 — приложение переводит на экран входа */
let onUnauthorized = () => {};
export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn;
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {};
  const token = tokenStore.get();
  if (token) headers.Authorization = `Bearer ${token}`;
  if (body !== undefined) headers['Content-Type'] = 'application/json';

  const res = await fetch(`/api${path}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (res.status === 401) {
    tokenStore.set(null);
    onUnauthorized();
  }
  if (!res.ok) {
    let message = 'Не удалось выполнить запрос';
    try {
      const data = await res.json();
      if (typeof data.detail === 'string') message = data.detail;
      else if (Array.isArray(data.detail)) message = 'Проверьте правильность заполнения полей';
    } catch { /* тело не JSON */ }
    throw new ApiError(res.status, message);
  }
  return res.status === 204 ? (undefined as T) : res.json();
}

const get = <T>(path: string) => request<T>('GET', path);
const post = <T>(path: string, body?: unknown) => request<T>('POST', path, body ?? {});
const patch = <T>(path: string, body: unknown) => request<T>('PATCH', path, body);
const del = (path: string) => request<void>('DELETE', path);

export const api = {
  async login(username: string, password: string) {
    const res = await fetch('/api/auth/token', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({ username, password }),
    });
    if (!res.ok) {
      throw new ApiError(res.status, res.status === 401 ? 'Неверный логин или пароль' : 'Сервер недоступен');
    }
    const { access_token } = await res.json();
    tokenStore.set(access_token);
  },

  // Растения
  summaries: () => get<PlantSummary[]>('/plants/summary'),
  summary: (id: number) => get<PlantSummary>(`/plants/${id}/summary`),
  plants: () => get<Plant[]>('/plants'),
  createPlant: (data: Partial<PlantFields>) => post<Plant>('/plants', data),
  updatePlant: (id: number, data: Partial<PlantFields>) => patch<Plant>(`/plants/${id}`, data),
  deletePlant: (id: number) => del(`/plants/${id}`),
  history(id: number, opts: { types?: EventType[]; from?: string; to?: string }) {
    const q = new URLSearchParams();
    opts.types?.forEach((t) => q.append('types', t));
    if (opts.from) q.set('date_from', opts.from);
    if (opts.to) q.set('date_to', opts.to);
    return get<HistoryEvent[]>(`/plants/${id}/history?${q}`);
  },
  weekly: (id: number, weeks = 8) => get<WeekStat[]>(`/plants/${id}/stats/weekly?weeks=${weeks}`),

  // Журналы
  water: (plantId: number) => post<{ id: number }>('/waterings', { plant_id: plantId }),
  deleteWatering: (id: number) => del(`/waterings/${id}`),
  feed: (plantId: number, fertilizerId: number, method: FeedMethod) =>
    post<{ id: number }>('/feedings', { plant_id: plantId, fertilizer_type_id: fertilizerId, method }),
  deleteFeeding: (id: number) => del(`/feedings/${id}`),
  repot: (plantId: number, data: { pot_size_after: number | null; note: string | null }) =>
    post<{ id: number }>('/repottings', { plant_id: plantId, ...data }),
  deleteRepotting: (id: number) => del(`/repottings/${id}`),

  // Лампа: plantId=null — общая лампа
  toggleLamp: (plantId: number | null) =>
    post<{ is_on: boolean; session: LampSession }>('/lamp-sessions/toggle', { plant_id: plantId }),
  sharedLampOn: async () => (await get<LampSession[]>('/lamp-sessions?shared=true&open=true')).length > 0,
  reopenLamp: (id: number) => patch<LampSession>(`/lamp-sessions/${id}`, { ended_at: null }),
  deleteLamp: (id: number) => del(`/lamp-sessions/${id}`),

  // Удобрения
  fertilizers: () => get<Fertilizer[]>('/fertilizers'),
  createFertilizer: (data: FertilizerFields) => post<Fertilizer>('/fertilizers', data),
  updateFertilizer: (id: number, data: Partial<FertilizerFields>) => patch<Fertilizer>(`/fertilizers/${id}`, data),
  deleteFertilizer: (id: number) => del(`/fertilizers/${id}`),

  // Глобальные настройки
  settings: () => get<AppSettings>('/settings'),
  updateSettings: (data: Partial<AppSettings>) => patch<AppSettings>('/settings', data),
};
