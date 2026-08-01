// usePush — Web Push: detección de soporte, suscripción y baja.
// Reglas (skill web-push-alerts): el prompt nativo SOLO tras gesto del usuario;
// iOS necesita la PWA instalada; sin secure context (LAN HTTP) el push está
// dormido y se activará solo cuando la app se sirva por HTTPS.
// Helios no tiene modo demo en server: no hay estado 'demo'.
import { useCallback, useEffect, useState } from 'react';
import { apiFetch, apiPost } from '@/data/api-client';

export type SoportePush =
  | 'ok' // se puede suscribir aquí y ahora
  | 'requiere-https' // LAN HTTP: dormido hasta servir por HTTPS
  | 'ios-necesita-instalacion' // iOS sin PWA en Home Screen
  | 'no-configurado' // el servidor no tiene claves VAPID
  | 'no-soportado';

export interface EstadoPush {
  permiso: NotificationPermission | 'desconocido';
  suscrito: boolean;
  cargando: boolean;
  error: string | null;
}

function esStandalone(): boolean {
  return (
    window.matchMedia('(display-mode: standalone)').matches ||
    (navigator as { standalone?: boolean }).standalone === true
  );
}

function esIOS(): boolean {
  return (
    /iPad|iPhone|iPod/.test(navigator.userAgent) ||
    (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)
  );
}

function soportaPush(): boolean {
  return 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window;
}

function urlBase64ToUint8Array(base64: string): Uint8Array {
  const padding = '='.repeat((4 - (base64.length % 4)) % 4);
  const b64 = (base64 + padding).replace(/-/g, '+').replace(/_/g, '/');
  const raw = atob(b64);
  const salida = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) salida[i] = raw.charCodeAt(i);
  return salida;
}

export function usePush(): {
  soporte: SoportePush;
  estado: EstadoPush;
  activar: () => Promise<boolean>;
  desactivar: () => Promise<void>;
} {
  const [soporte, setSoporte] = useState<SoportePush>('no-soportado');
  const [estado, setEstado] = useState<EstadoPush>({
    permiso: 'desconocido',
    suscrito: false,
    cargando: true,
    error: null,
  });

  useEffect(() => {
    let cancelado = false;
    (async () => {
      // Sin contexto seguro (http://192.168.x.x): no hay SW ni PushManager
      // funcional. NO es un bug: se activará al servir la app por HTTPS.
      if (!window.isSecureContext) {
        if (!cancelado) {
          setSoporte('requiere-https');
          setEstado((e) => ({ ...e, cargando: false }));
        }
        return;
      }
      if (!soportaPush()) {
        if (!cancelado) {
          setSoporte('no-soportado');
          setEstado((e) => ({ ...e, cargando: false }));
        }
        return;
      }
      if (esIOS() && !esStandalone()) {
        if (!cancelado) {
          setSoporte('ios-necesita-instalacion');
          setEstado((e) => ({ ...e, permiso: Notification.permission, cargando: false }));
        }
        return;
      }
      try {
        await apiFetch('/api/push/vapid-public-key');
        const reg = await navigator.serviceWorker.ready;
        const sub = await reg.pushManager.getSubscription();
        // Re-sincronización: si el navegador tiene suscripción, asegurar que el
        // servidor la conoce (cubre pushsubscriptionchange perdidos).
        if (sub) {
          apiPost('/api/push/subscribe', sub).catch(() => {});
        }
        if (!cancelado) {
          setSoporte('ok');
          setEstado({ permiso: Notification.permission, suscrito: sub !== null, cargando: false, error: null });
        }
      } catch {
        // 503 del servidor: sin claves VAPID configuradas
        if (!cancelado) {
          setSoporte('no-configurado');
          setEstado((e) => ({ ...e, cargando: false }));
        }
      }
    })();
    return () => {
      cancelado = true;
    };
  }, []);

  // LLAMAR SOLO DESDE UN GESTO DEL USUARIO (onClick).
  const activar = useCallback(async (): Promise<boolean> => {
    if (soporte !== 'ok') return false;
    setEstado((e) => ({ ...e, cargando: true, error: null }));
    try {
      const permiso = await Notification.requestPermission();
      if (permiso !== 'granted') {
        setEstado({ permiso, suscrito: false, cargando: false, error: null });
        return false;
      }
      const { publicKey } = await apiFetch<{ publicKey: string }>('/api/push/vapid-public-key');
      const reg = await navigator.serviceWorker.ready;
      const sub = await reg.pushManager.subscribe({
        userVisibleOnly: true, // contrato: todo push visible
        applicationServerKey: urlBase64ToUint8Array(publicKey) as BufferSource,
      });
      await apiPost('/api/push/subscribe', sub);
      setEstado({ permiso, suscrito: true, cargando: false, error: null });
      return true;
    } catch (err) {
      setEstado((e) => ({
        ...e,
        cargando: false,
        error: err instanceof Error ? err.message : 'error',
      }));
      return false;
    }
  }, [soporte]);

  const desactivar = useCallback(async (): Promise<void> => {
    setEstado((e) => ({ ...e, cargando: true, error: null }));
    try {
      const reg = await navigator.serviceWorker.ready;
      const sub = await reg.pushManager.getSubscription();
      if (sub) {
        await apiFetch('/api/push/unsubscribe', {
          method: 'DELETE',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ endpoint: sub.endpoint }),
        }).catch(() => {});
        await sub.unsubscribe();
      }
      setEstado((e) => ({ ...e, suscrito: false, cargando: false }));
    } catch (err) {
      setEstado((e) => ({
        ...e,
        cargando: false,
        error: err instanceof Error ? err.message : 'error',
      }));
    }
  }, []);

  return { soporte, estado, activar, desactivar };
}
