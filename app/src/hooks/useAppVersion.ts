import { useEffect, useState } from 'react';
import { apiFetch } from '@/data/api-client';
import pkg from '../../package.json';

// Versión de la app: /api/version en runtime (fuente del server, refleja lo
// desplegado) con fallback a la versión embebida en el bundle (package.json).
export function useAppVersion(): string {
  const [version, setVersion] = useState(pkg.version);

  useEffect(() => {
    let alive = true;
    apiFetch<{ version?: string }>('/api/version')
      .then((res) => {
        if (alive && res?.version) setVersion(res.version);
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, []);

  return version;
}
