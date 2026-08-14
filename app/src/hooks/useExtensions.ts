import { useEffect, useRef, useState } from 'react';
import type { ExtensionsConfig } from '@/data/types';
import { apiFetch } from '@/data/api-client';

let cache: ExtensionsConfig | null = null;
const listeners = new Set<() => void>();

function notify() {
  for (const l of listeners) l();
}

/** Invalida la caché de extensiones (tras PUT /api/extensions) y notifica a
 *  los hooks activos (la nav añade/quita el menú del cargador al vuelo). */
export function invalidateExtensions(): void {
  cache = null;
  notify();
}

/** Config de extensiones (GET /api/extensions). Cacheada a nivel de módulo:
 *  cambia poco (solo si el admin la edita en Ajustes). */
export function useExtensions(): ExtensionsConfig | null {
  const [ext, setExt] = useState<ExtensionsConfig | null>(cache);
  const loadedRef = useRef(cache !== null);

  useEffect(() => {
    const onInvalidate = () => setExt(cache);
    listeners.add(onInvalidate);
    if (loadedRef.current) return;
    let alive = true;
    apiFetch<ExtensionsConfig>('/api/extensions')
      .then((res) => {
        if (!alive) return;
        cache = res;
        setExt(res);
      })
      .catch(() => {
        if (alive) setExt(null);
      });
    loadedRef.current = true;
    return () => {
      alive = false;
      listeners.delete(onInvalidate);
    };
  }, []);

  return ext;
}
