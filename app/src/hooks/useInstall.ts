import { useEffect, useRef, useState } from 'react';
import type { InstallInfo } from '@/data/types';
import { apiFetch } from '@/data/api-client';

let cache: InstallInfo | null = null;

/** Topología resuelta de la instalación (GET /api/install). Cacheada a nivel de
 *  módulo: se resuelve al arrancar y cambia poco (solo si el admin la edita). */
export function useInstall(): InstallInfo | null {
  const [install, setInstall] = useState<InstallInfo | null>(cache);
  const loadedRef = useRef(cache !== null);

  useEffect(() => {
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
    };
  }, []);

  return install;
}
