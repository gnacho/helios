import { useEffect, useRef, useState } from 'react';
import type { InstallInfo } from '@/data/types';
import { apiFetch } from '@/data/api-client';

let cache: InstallInfo | null = null;
const listeners = new Set<() => void>();

function notify() {
  for (const l of listeners) l();
}

/** Invalida la caché de topología (tras PUT /api/config) y notifica a los
 *  hooks activos para que recarguen con la topología recién guardada. */
export function invalidateInstall(): void {
  cache = null;
  notify();
}

/** Topología resuelta de la instalación (GET /api/install). Cacheada a nivel de
 *  módulo: se resuelve al arrancar y cambia poco (solo si el admin la edita). */
export function useInstall(): InstallInfo | null {
  const [install, setInstall] = useState<InstallInfo | null>(cache);
  const loadedRef = useRef(cache !== null);

  useEffect(() => {
    const onInvalidate = () => setInstall(cache);
    listeners.add(onInvalidate);
    if (loadedRef.current) return;
    let alive = true;
    apiFetch<InstallInfo>('/api/install')
      .then((res) => {
        if (!alive) return;
        cache = res;
        setInstall(res);
      })
      .catch(() => {
        if (alive) setInstall(null);
      });
    loadedRef.current = true;
    return () => {
      alive = false;
      listeners.delete(onInvalidate);
    };
  }, []);

  return install;
}
