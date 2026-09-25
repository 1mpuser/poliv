import type {
  AdminUser,
  AppSettings,
  Daylight,
  EventType,
  FeedMethod,
  Fertilizer,
  FertilizerFields,
  HistoryEvent,
  Lamp,
  LampFields,
  LampSession,
  LampToggle,
  Me,
  Plant,
  Place,
  PlantFields,
  PlantSummary,
  ScheduleInterval,
  WeekStat,
  YandexDevice,
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
  async login(email: string, password: string) {
    const res = await fetch('/api/auth/token', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({ username: email, password }),
    });
    if (!res.ok) {
      throw new ApiError(res.status, res.status === 401 ? 'Неверная почта или пароль' : 'Сервер недоступен');
    }
    const { access_token } = await res.json();
    tokenStore.set(access_token);
  },
  me: () => get<Me>('/auth/me'),
  /** Остальные устройства разлогиниваются, это получает новый токен */
  async changePassword(current: string, next: string) {
    const { access_token } = await post<{ access_token: string }>('/auth/password', {
      current_password: current,
      new_password: next,
    });
    tokenStore.set(access_token);
  },

  // Админка
  adminUsers: () => get<AdminUser[]>('/admin/users'),
  adminCreateUser: (email: string, password: string) => post<AdminUser>('/admin/users', { email, password }),
  adminSetPassword: (id: number, password: string) => post<void>(`/admin/users/${id}/password`, { password }),
  adminBlock: (id: number) => post<void>(`/admin/users/${id}/block`),
  adminUnblock: (id: number) => post<void>(`/admin/users/${id}/unblock`),
  adminDelete: (id: number) => del(`/admin/users/${id}`),

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

  // Лампы
  lamps: () => get<Lamp[]>('/lamps'),
  createLamp: (data: LampFields & { plant_ids: number[] }) => post<Lamp>('/lamps', data),
  updateLamp: (id: number, data: Partial<LampFields> & { plant_ids?: number[] }) => patch<Lamp>(`/lamps/${id}`, data),
  /** Лампа уходит в архив: растения без лампы, история часов сохраняется */
  archiveLamp: (id: number) => del(`/lamps/${id}`),
  setLampSchedule: (lampId: number, intervals: ScheduleInterval[]) =>
    request<ScheduleInterval[]>('PUT', `/lamps/${lampId}/schedule`, { intervals }),
  setPlantLamp: (plantId: number, lampId: number | null) =>
    request<{ lamp_id: number | null }>('PUT', `/plants/${plantId}/lamp`, { lamp_id: lampId }),
  // Кнопка гасит то, что горит (вручную, по расписанию, досветка), иначе включает вручную
  toggleLamp: (plantId: number) => post<LampToggle>('/lamp-sessions/toggle', { plant_id: plantId }),
  toggleLampById: (lampId: number) => post<LampToggle>(`/lamps/${lampId}/toggle`),
  /** Отмена выключения: вернуть прежний конец (null — снова горит вручную) */
  restoreLampEnd: (id: number, endedAt: string | null) => patch<LampSession>(`/lamp-sessions/${id}`, { ended_at: endedAt }),
  deleteLampSession: (id: number) => del(`/lamp-sessions/${id}`),

  // Умный дом Яндекса
  yandexDevices: () => get<YandexDevice[]>('/yandex/devices'),
  /** null — удалить токен */
  setYandexToken: (token: string | null) => request<AppSettings>('PUT', '/settings/yandex-token', { token }),

  // Свет
  geocode: (q: string) => get<Place[]>(`/light/geocode?q=${encodeURIComponent(q)}`),
  lightToday: () => get<Daylight | null>('/light/today'),

  // Удобрения
  fertilizers: () => get<Fertilizer[]>('/fertilizers'),
  createFertilizer: (data: FertilizerFields) => post<Fertilizer>('/fertilizers', data),
  updateFertilizer: (id: number, data: Partial<FertilizerFields>) => patch<Fertilizer>(`/fertilizers/${id}`, data),
  deleteFertilizer: (id: number) => del(`/fertilizers/${id}`),

  // Глобальные настройки
  settings: () => get<AppSettings>('/settings'),
  updateSettings: (data: Partial<AppSettings>) => patch<AppSettings>('/settings', data),
};
