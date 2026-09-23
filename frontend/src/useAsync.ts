import { useCallback, useEffect, useRef, useState } from 'react';

/** Загружает данные при изменении deps; reload() — перезапросить без мигания. */
export function useAsync<T>(load: () => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T | undefined>(undefined);
  const [error, setError] = useState<string | null>(null);
  const loadRef = useRef(load);
  loadRef.current = load;
  const seq = useRef(0);

  const reload = useCallback(async () => {
    const n = ++seq.current;
    try {
      const result = await loadRef.current();
      if (n === seq.current) {
        setData(result);
        setError(null);
      }
    } catch (e) {
      if (n === seq.current) setError(e instanceof Error ? e.message : 'Ошибка загрузки');
    }
  }, []);

  useEffect(() => {
    reload();
  }, deps);

  return { data, setData, error, reload };
}
